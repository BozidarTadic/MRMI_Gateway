package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/inbox"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/registry"
	"MRMI_Gateway/internal/version"
)

// RuntimePeers is a thread-safe list of dynamically registered peers.
type RuntimePeers struct {
	mu    sync.RWMutex
	peers []config.PeerConfig
}

func NewRuntimePeers() *RuntimePeers { return &RuntimePeers{} }

func (rp *RuntimePeers) Add(p config.PeerConfig) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.peers = append(rp.peers, p)
}

func (rp *RuntimePeers) All() []config.PeerConfig {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	out := make([]config.PeerConfig, len(rp.peers))
	copy(out, rp.peers)
	return out
}

// ServerDeps holds optional dependencies for the HTTP server.
// All fields may be nil; endpoints that require a nil dep return 503.
type ServerDeps struct {
	Engine         *policy.Engine
	Audit          *audit.Log
	PrivKey        ed25519.PrivateKey
	Peers          *peercache.Cache
	Gateway        *core.Gateway
	DLQ            *delivery.DLQ
	CRL            *crl.Store
	Inbox          *inbox.Inbox
	Registry       *registry.Registry
	RuntimePeers   *RuntimePeers
	OnConfigReload func() error // called by POST /api/v1/config/reload
}

type HTTPServer struct {
	*http.Server
	startTime time.Time
}

// auditSignPayload is the canonical form of the audit response that is signed.
// Fields are in alphabetical JSON key order so the payload is deterministic.
type auditSignPayload struct {
	ADRVersion    string `json:"adr_version"`
	AppVersion    string `json:"app_version"`
	ApplicableLaw string `json:"applicable_law"`
	NodeID        string `json:"node_id"`
	RootHash      string `json:"root_hash"`
	Timestamp     int64  `json:"timestamp"`
	Version       int    `json:"version"`
}

// auditResponse is the full response returned to callers, extending the
// sign payload with the signature field.
type auditResponse struct {
	ADRVersion    string `json:"adr_version"`
	AppVersion    string `json:"app_version"`
	ApplicableLaw string `json:"applicable_law"`
	NodeID        string `json:"node_id"`
	RootHash      string `json:"root_hash"`
	Timestamp     int64  `json:"timestamp"`
	Version       int    `json:"version"`
	Signature     string `json:"signature"`
}

// NewHTTPServer builds the HTTP server.
// authMiddleware wraps a handler with X-MRMI-Key authentication.
// When cfg.API.APIKey is empty the request passes through unauthenticated.
func authMiddleware(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if apiKey != "" && r.Header.Get("X-MRMI-Key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func NewHTTPServer(cfg config.Config, deps ServerDeps) *HTTPServer {
	mux := http.NewServeMux()
	startTime := time.Now()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return authMiddleware(cfg.API.APIKey, h)
	}

	// ── legacy / well-known ──────────────────────────────────────────────────

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if deps.Engine == nil {
			http.Error(w, "policy engine not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	mux.HandleFunc("/.well-known/mrmi-audit", func(w http.ResponseWriter, _ *http.Request) {
		if !cfg.Policy.Audit.HTTPSWellKnown {
			http.NotFound(w, nil)
			return
		}

		rootHash := "sha256:BOOTSTRAP_PLACEHOLDER"
		if deps.Audit != nil {
			rootHash = deps.Audit.RootHash()
		}
		ts := time.Now().Unix()

		payload := auditSignPayload{
			ADRVersion:    version.ADR,
			AppVersion:    version.App,
			ApplicableLaw: cfg.Node.ApplicableLaw,
			NodeID:        cfg.Node.NodeID,
			RootHash:      rootHash,
			Timestamp:     ts,
			Version:       1,
		}

		sig := ""
		if deps.PrivKey != nil {
			raw, _ := json.Marshal(payload)
			sig = "ed25519:" + base64.StdEncoding.EncodeToString(ed25519.Sign(deps.PrivKey, raw))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auditResponse{
			ADRVersion:    payload.ADRVersion,
			AppVersion:    payload.AppVersion,
			ApplicableLaw: payload.ApplicableLaw,
			NodeID:        payload.NodeID,
			RootHash:      payload.RootHash,
			Timestamp:     payload.Timestamp,
			Version:       payload.Version,
			Signature:     sig,
		})
	})

	mux.HandleFunc("/peers/audit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if deps.Peers == nil {
			_, _ = w.Write([]byte("{}"))
			return
		}
		_ = json.NewEncoder(w).Encode(deps.Peers.All())
	})

	// ── management API ───────────────────────────────────────────────────────

	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_id":        cfg.Node.NodeID,
			"region":         cfg.Node.Region,
			"node_scope":     cfg.Node.NodeScope,
			"profile":        cfg.Profile.Name,
			"applicable_law": cfg.Node.ApplicableLaw,
			"app_version":    version.App,
			"adr_version":    version.ADR,
			"uptime_seconds": int64(time.Since(startTime).Seconds()),
		})
	})

	mux.HandleFunc("GET /api/v1/audit/latest", func(w http.ResponseWriter, r *http.Request) {
		if deps.Audit == nil {
			http.Error(w, "audit log not available", http.StatusServiceUnavailable)
			return
		}
		n := 20
		if q := r.URL.Query().Get("n"); q != "" {
			if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 200 {
				n = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deps.Audit.Recent(n))
	})

	mux.HandleFunc("POST /api/v1/envelopes", func(w http.ResponseWriter, r *http.Request) {
		if deps.Gateway == nil {
			http.Error(w, "gateway not available", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			IdempotencyKey  string `json:"idempotency_key"`
			SenderRegion    string `json:"sender_region"`
			RecipientRegion string `json:"recipient_region"`
			TrustTier       uint32 `json:"trust_tier"`
			Payload         []byte `json:"payload"`
			SenderIdentity  []byte `json:"sender_identity,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.IdempotencyKey == "" {
			http.Error(w, "idempotency_key is required", http.StatusBadRequest)
			return
		}
		resp, err := deps.Gateway.SendEnvelope(r.Context(), core.SendRequest{
			Envelope: core.Envelope{
				IdempotencyKey:  req.IdempotencyKey,
				SenderRegion:    req.SenderRegion,
				RecipientRegion: req.RecipientRegion,
				TrustTier:       req.TrustTier,
				Payload:         req.Payload,
				SenderIdentity:  req.SenderIdentity,
				Timestamp:       time.Now().UnixMilli(),
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"decision":            string(resp.Decision),
			"reason":              resp.Reason,
			"profile":             resp.Profile,
			"node_id":             resp.NodeID,
			"audit_root_hash":     resp.AuditRootHash,
			"peer_audit_root_hash": resp.PeerAuditRootHash,
		})
	})

	// DLQ endpoints

	mux.HandleFunc("GET /api/v1/dlq", func(w http.ResponseWriter, _ *http.Request) {
		if deps.DLQ == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		entries := deps.DLQ.Entries()
		type dlqDTO struct {
			Index           int    `json:"index"`
			PeerAddr        string `json:"peer_addr"`
			Attempts        int    `json:"attempts"`
			LastError       string `json:"last_error,omitempty"`
			FirstSeenUnix   int64  `json:"first_seen_unix"`
			LastAttemptUnix int64  `json:"last_attempt_unix"`
			EnvelopeID      string `json:"envelope_id"`
			SenderRegion    string `json:"sender_region"`
			RecipientRegion string `json:"recipient_region"`
		}
		out := make([]dlqDTO, len(entries))
		for i, e := range entries {
			lastErr := ""
			if e.LastErr != nil {
				lastErr = e.LastErr.Error()
			}
			out[i] = dlqDTO{
				Index:           i,
				PeerAddr:        e.PeerAddr,
				Attempts:        e.Attempts,
				LastError:       lastErr,
				FirstSeenUnix:   e.FirstSeenUnix,
				LastAttemptUnix: e.LastAttemptUnix,
				EnvelopeID:      e.Envelope.IdempotencyKey,
				SenderRegion:    e.Envelope.SenderRegion,
				RecipientRegion: e.Envelope.RecipientRegion,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("DELETE /api/v1/dlq/{index}", func(w http.ResponseWriter, r *http.Request) {
		if deps.DLQ == nil {
			http.Error(w, "dlq not available", http.StatusServiceUnavailable)
			return
		}
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || idx < 0 {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}
		deps.DLQ.Remove(idx)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/dlq/{index}/replay", func(w http.ResponseWriter, r *http.Request) {
		if deps.DLQ == nil || deps.Gateway == nil {
			http.Error(w, "dlq or gateway not available", http.StatusServiceUnavailable)
			return
		}
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || idx < 0 {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}
		entries := deps.DLQ.Entries()
		if idx >= len(entries) {
			http.Error(w, "index out of range", http.StatusNotFound)
			return
		}
		entry := entries[idx]
		resp, err := deps.Gateway.SendEnvelope(r.Context(), core.SendRequest{Envelope: entry.Envelope})
		if err != nil {
			http.Error(w, fmt.Sprintf("replay failed: %v", err), http.StatusInternalServerError)
			return
		}
		deps.DLQ.Remove(idx)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"decision": string(resp.Decision),
			"reason":   resp.Reason,
		})
	})

	// CRL endpoints

	mux.HandleFunc("GET /api/v1/crl", func(w http.ResponseWriter, _ *http.Request) {
		if deps.CRL == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		entries := deps.CRL.Entries()
		type crlDTO struct {
			NodeID      string `json:"node_id"`
			Reason      string `json:"reason"`
			SigCount    int    `json:"sig_count"`
			IsEffective bool   `json:"is_effective"`
			RevokedAt   int64  `json:"revoked_at_unix"`
		}
		out := make([]crlDTO, len(entries))
		for i, e := range entries {
			out[i] = crlDTO{
				NodeID:      e.NodeID,
				Reason:      e.Reason,
				SigCount:    len(e.Signatures),
				IsEffective: len(e.Signatures) >= 2,
				RevokedAt:   e.RevokedAt.Unix(),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("POST /api/v1/crl", func(w http.ResponseWriter, r *http.Request) {
		if deps.CRL == nil {
			http.Error(w, "crl store not available", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			NodeID      string `json:"node_id"`
			Reason      string `json:"reason"`
			SignatureB64 string `json:"signature_b64"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.NodeID == "" || req.SignatureB64 == "" {
			http.Error(w, "node_id and signature_b64 are required", http.StatusBadRequest)
			return
		}
		sig, err := base64.StdEncoding.DecodeString(req.SignatureB64)
		if err != nil {
			http.Error(w, "invalid signature_b64 encoding", http.StatusBadRequest)
			return
		}
		deps.CRL.Revoke(req.NodeID, req.Reason, sig)
		effective := deps.CRL.IsRevoked(req.NodeID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_id":      req.NodeID,
			"is_effective": effective,
		})
	})

	// SSE stream endpoint

	mux.HandleFunc("GET /api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		if deps.Inbox == nil {
			http.Error(w, "inbox not available", http.StatusServiceUnavailable)
			return
		}
		flusher, canFlush := w.(http.Flusher)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		if canFlush {
			flusher.Flush()
		}

		id, ch := deps.Inbox.Subscribe()
		defer deps.Inbox.Unsubscribe(id)

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, open := <-ch:
				if !open {
					return
				}
				data, _ := json.Marshal(ev)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				if canFlush {
					flusher.Flush()
				}
			}
		}
	})

	// ── management write endpoints (auth required) ───────────────────────────

	mux.HandleFunc("POST /api/v1/peers/register", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NodeID    string `json:"node_id"`
			Addr      string `json:"addr"`
			NodeScope string `json:"node_scope"`
			Region    string `json:"region"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Addr == "" {
			http.Error(w, "addr is required", http.StatusBadRequest)
			return
		}
		if deps.RuntimePeers != nil {
			deps.RuntimePeers.Add(config.PeerConfig{
				Region:    req.Region,
				Addr:      req.Addr,
				NodeScope: req.NodeScope,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"registered": true, "addr": req.Addr})
	}))

	mux.HandleFunc("GET /api/v1/peers", auth(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type peerDTO struct {
			NodeID    string `json:"node_id,omitempty"`
			Addr      string `json:"addr"`
			NodeScope string `json:"node_scope,omitempty"`
			Region    string `json:"region,omitempty"`
			Source    string `json:"source"` // "config" | "runtime"
		}
		var out []peerDTO
		for k, p := range cfg.Network.Peers {
			out = append(out, peerDTO{NodeID: k, Addr: p.Addr, NodeScope: p.NodeScope, Region: p.Region, Source: "config"})
		}
		if deps.RuntimePeers != nil {
			for _, p := range deps.RuntimePeers.All() {
				out = append(out, peerDTO{Addr: p.Addr, NodeScope: p.NodeScope, Region: p.Region, Source: "runtime"})
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))

	mux.HandleFunc("POST /api/v1/dlq/{index}/discard", auth(func(w http.ResponseWriter, r *http.Request) {
		if deps.DLQ == nil {
			http.Error(w, "dlq not available", http.StatusServiceUnavailable)
			return
		}
		idx, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || idx < 0 {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}
		deps.DLQ.Remove(idx)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/v1/config/reload", auth(func(w http.ResponseWriter, _ *http.Request) {
		if deps.OnConfigReload == nil {
			http.Error(w, "config reload not available", http.StatusServiceUnavailable)
			return
		}
		if err := deps.OnConfigReload(); err != nil {
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"reloaded": true})
	}))

	mux.HandleFunc("POST /api/v1/revoke/{node_id}", auth(func(w http.ResponseWriter, r *http.Request) {
		if deps.CRL == nil {
			http.Error(w, "crl store not available", http.StatusServiceUnavailable)
			return
		}
		nodeID := r.PathValue("node_id")
		var req struct {
			Reason       string `json:"reason"`
			SignatureB64 string `json:"signature_b64"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SignatureB64 == "" {
			http.Error(w, "reason and signature_b64 required", http.StatusBadRequest)
			return
		}
		sig, err := base64.StdEncoding.DecodeString(req.SignatureB64)
		if err != nil {
			http.Error(w, "invalid signature_b64", http.StatusBadRequest)
			return
		}
		deps.CRL.Revoke(nodeID, req.Reason, sig)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_id":      nodeID,
			"is_effective": deps.CRL.IsRevoked(nodeID),
		})
	}))

	// ── discovery endpoints ───────────────────────────────────────────────────

	mux.HandleFunc("GET /api/v1/discover", func(w http.ResponseWriter, r *http.Request) {
		if deps.Registry == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		q := r.URL.Query().Get("q")
		queryType := r.URL.Query().Get("type")
		results := deps.Registry.Discover(q, queryType)
		if results == nil {
			results = []registry.DiscoveryResult{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	})

	mux.HandleFunc("POST /api/v1/connect", func(w http.ResponseWriter, r *http.Request) {
		if deps.Registry == nil {
			http.Error(w, "registry not available", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			OpaqueToken     string `json:"opaque_token"`
			RequesterID     string `json:"requester_id"`
			RequesterRegion string `json:"requester_region"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OpaqueToken == "" {
			http.Error(w, "opaque_token required", http.StatusBadRequest)
			return
		}
		result := deps.Registry.Connect(req.OpaqueToken, req.RequesterID, req.RequesterRegion)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	return &HTTPServer{
		Server: &http.Server{
			Addr:              cfg.Network.HTTPListenAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		startTime: startTime,
	}
}
