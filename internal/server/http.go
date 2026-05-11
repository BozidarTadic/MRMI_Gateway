package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/version"
)

type HTTPServer struct {
	*http.Server
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

// NewHTTPServer builds the HTTP server. privKey may be nil (unsigned dev-mode
// responses). peerCache may be nil (empty /peers/audit response).
func NewHTTPServer(cfg config.Config, engine *policy.Engine, auditLog *audit.Log, privKey ed25519.PrivateKey, peerCache *peercache.Cache) *HTTPServer {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if engine == nil {
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
		if auditLog != nil {
			rootHash = auditLog.RootHash()
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
		if privKey != nil {
			raw, _ := json.Marshal(payload)
			sig = "ed25519:" + base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, raw))
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
		if peerCache == nil {
			_, _ = w.Write([]byte("{}"))
			return
		}
		_ = json.NewEncoder(w).Encode(peerCache.All())
	})

	return &HTTPServer{
		Server: &http.Server{
			Addr:              cfg.Network.HTTPListenAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}
