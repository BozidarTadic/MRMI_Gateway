// Package acceptance contains end-to-end tests that start a real gateway node
// in-process and exercise all management REST API endpoints.
// Run with: go test ./test/acceptance/ -v -timeout 60s
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/inbox"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// startNode starts a full gateway node and returns the HTTP base URL.
func startNode(t *testing.T) (string, *core.Gateway, *delivery.DLQ, *crl.Store, *audit.Log, *inbox.Inbox) {
	t.Helper()

	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "acceptance-node"
	cfg.Node.Region = "RS"
	cfg.Policy.Audit.HTTPSWellKnown = true
	cfg.Policy.Outbound.AllowTo = []string{"RU", "BY", "KZ"}

	auditLog := audit.New()
	crlStore := crl.New()
	engine, err := policy.NewEngine(cfg, auditLog, crlStore)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}

	dlq := delivery.NewDLQ()
	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)

	msgInbox := inbox.New()
	gw.SetOnAllow(func(env core.Envelope) {
		msgInbox.Publish(inbox.Event{
			IdempotencyKey:  env.IdempotencyKey,
			SenderRegion:    env.SenderRegion,
			RecipientRegion: env.RecipientRegion,
			TrustTier:       env.TrustTier,
			Payload:         env.Payload,
			Timestamp:       env.Timestamp,
		})
	})

	// Start gRPC on a free port (needed for server wiring).
	grpcAdapter := grpctransport.NewAdapter(gw)
	grpcSrv, err := grpctransport.NewServer(":0", grpcAdapter, nil)
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	go func() { _ = grpcSrv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = grpcSrv.Shutdown(ctx)
	})

	// Find a free port for HTTP.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	httpAddr := l.Addr().String()
	_ = l.Close()
	cfg.Network.HTTPListenAddr = httpAddr

	httpSrv := server.NewHTTPServer(cfg, server.ServerDeps{
		Engine:  engine,
		Audit:   auditLog,
		Peers:   nil,
		Gateway: gw,
		DLQ:     dlq,
		CRL:     crlStore,
		Inbox:   msgInbox,
	})
	go func() { _ = httpSrv.ListenAndServe() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	})

	time.Sleep(50 * time.Millisecond)

	return "http://" + httpAddr, gw, dlq, crlStore, auditLog, msgInbox
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func decodeJSON(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// TestHealthz confirms the legacy health endpoint is still served.
func TestHealthz(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)
	resp := get(t, base+"/healthz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestStatus verifies GET /api/v1/status returns the expected fields.
func TestStatus(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)
	resp := get(t, base+"/api/v1/status")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var m map[string]any
	decodeJSON(t, resp.Body, &m)

	if m["node_id"] != "acceptance-node" {
		t.Fatalf("unexpected node_id: %v", m["node_id"])
	}
	if _, ok := m["uptime_seconds"]; !ok {
		t.Fatal("uptime_seconds missing from status")
	}
}

// TestSendEnvelope_Allow sends a valid envelope and expects an ALLOW decision.
func TestSendEnvelope_Allow(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)

	body := map[string]any{
		"idempotency_key":  "acc-test-001",
		"sender_region":    "RS",
		"recipient_region": "RU",
		"trust_tier":       1,
		"payload":          []byte("hello"),
	}
	resp := postJSON(t, base+"/api/v1/envelopes", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}
	var m map[string]any
	decodeJSON(t, resp.Body, &m)
	if m["decision"] != "ALLOW" {
		t.Fatalf("expected ALLOW, got %v", m["decision"])
	}
}

// TestSendEnvelope_Deny sends an envelope to a blocked region and expects DENY.
func TestSendEnvelope_Deny(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)

	body := map[string]any{
		"idempotency_key":  "acc-test-002",
		"sender_region":    "RS",
		"recipient_region": "US",
	}
	resp := postJSON(t, base+"/api/v1/envelopes", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var m map[string]any
	decodeJSON(t, resp.Body, &m)
	if m["decision"] != "DENY" {
		t.Fatalf("expected DENY for blocked region, got %v", m["decision"])
	}
}

// TestSendEnvelope_MissingKey returns 400 when idempotency_key is absent.
func TestSendEnvelope_MissingKey(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)

	resp := postJSON(t, base+"/api/v1/envelopes", map[string]any{"sender_region": "RS"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestAuditLatest returns entries after sending an envelope.
func TestAuditLatest(t *testing.T) {
	base, gw, _, _, _, _ := startNode(t)

	// Send an envelope to populate the audit log.
	_, _ = gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "audit-seed",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})

	resp := get(t, base+"/api/v1/audit/latest")
	defer resp.Body.Close()

	var entries []map[string]any
	decodeJSON(t, resp.Body, &entries)
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
}

// TestDLQ_ListAndRemove verifies DLQ list + remove endpoints.
func TestDLQ_ListAndRemove(t *testing.T) {
	base, _, dlq, _, _, _ := startNode(t)

	dlq.Append(delivery.DLQEntry{
		PeerAddr: "localhost:7777",
		Attempts: 3,
		Envelope: core.Envelope{
			IdempotencyKey:  "dlq-env-01",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})

	resp := get(t, base+"/api/v1/dlq")
	defer resp.Body.Close()

	var entries []map[string]any
	decodeJSON(t, resp.Body, &entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(entries))
	}
	if entries[0]["envelope_id"] != "dlq-env-01" {
		t.Fatalf("unexpected envelope_id: %v", entries[0]["envelope_id"])
	}

	// Remove entry.
	req, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/dlq/0", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE dlq: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	if dlq.Size() != 0 {
		t.Fatal("DLQ should be empty after remove")
	}
}

// TestDLQ_Replay re-submits a DLQ entry through the gateway.
func TestDLQ_Replay(t *testing.T) {
	base, _, dlq, _, auditLog, _ := startNode(t)

	dlq.Append(delivery.DLQEntry{
		PeerAddr: "localhost:7777",
		Attempts: 3,
		Envelope: core.Envelope{
			IdempotencyKey:  "dlq-replay-01",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})

	countBefore := len(auditLog.Entries())

	resp := postJSON(t, base+"/api/v1/dlq/0/replay", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}
	var m map[string]any
	decodeJSON(t, resp.Body, &m)
	if m["decision"] != "ALLOW" {
		t.Fatalf("expected ALLOW on replay, got %v", m["decision"])
	}

	if len(auditLog.Entries()) <= countBefore {
		t.Fatal("audit log should have grown after replay")
	}
	if dlq.Size() != 0 {
		t.Fatal("DLQ should be empty after successful replay")
	}
}

// TestCRL_SubmitAndList verifies CRL revocation submission + listing.
func TestCRL_SubmitAndList(t *testing.T) {
	base, _, _, crlStore, _, _ := startNode(t)

	// Submit first signature.
	body1 := map[string]any{
		"node_id":       "bad-node-01",
		"reason":        "compromised key",
		"signature_b64": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	resp1 := postJSON(t, base+"/api/v1/crl", body1)
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp1.Body)
		t.Fatalf("expected 200, got %d: %s", resp1.StatusCode, bodyBytes)
	}
	var r1 map[string]any
	decodeJSON(t, resp1.Body, &r1)
	if r1["is_effective"] == true {
		t.Fatal("should not be effective with only 1 signature")
	}

	// Listing shows the entry.
	resp2 := get(t, base+"/api/v1/crl")
	defer resp2.Body.Close()
	var entries []map[string]any
	decodeJSON(t, resp2.Body, &entries)
	if len(entries) == 0 {
		t.Fatal("expected CRL entry after submission")
	}
	if entries[0]["node_id"] != "bad-node-01" {
		t.Fatalf("unexpected node_id: %v", entries[0]["node_id"])
	}

	// Submit second signature to reach quorum.
	body2 := map[string]any{
		"node_id":       "bad-node-01",
		"reason":        "compromised key",
		"signature_b64": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	}
	resp3 := postJSON(t, base+"/api/v1/crl", body2)
	defer resp3.Body.Close()
	var r3 map[string]any
	decodeJSON(t, resp3.Body, &r3)
	if r3["is_effective"] != true {
		t.Fatal("expected is_effective=true after 2 signatures")
	}

	if !crlStore.IsRevoked("bad-node-01") {
		t.Fatal("node should be revoked in crl store")
	}
}

// TestSSEStream verifies that allowed envelopes arrive on the SSE stream.
func TestSSEStream(t *testing.T) {
	base, gw, _, _, _, _ := startNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Give the SSE connection a moment to register.
	time.Sleep(20 * time.Millisecond)

	// Send an envelope that should appear as an SSE event.
	_, _ = gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "sse-test-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})

	// Read one SSE event line.
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	line := string(buf[:n])

	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected SSE data line, got: %q", line)
	}

	var ev map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
		// Try trimming the double newline suffix.
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if err2 := json.Unmarshal([]byte(trimmed), &ev); err2 != nil {
			t.Fatalf("parse SSE event JSON: %v (raw: %q)", err, line)
		}
	}
	if ev["idempotency_key"] != "sse-test-001" {
		t.Fatalf("unexpected SSE event: %v", ev)
	}
}

// TestWellKnownAudit verifies the signed audit endpoint still works.
func TestWellKnownAudit(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)
	resp := get(t, base+"/.well-known/mrmi-audit")
	defer resp.Body.Close()

	var m map[string]any
	decodeJSON(t, resp.Body, &m)
	if m["node_id"] != "acceptance-node" {
		t.Fatalf("unexpected node_id: %v", m["node_id"])
	}
}

// TestDLQ_ReplayOutOfRange returns 404 for a non-existent DLQ index.
func TestDLQ_ReplayOutOfRange(t *testing.T) {
	base, _, _, _, _, _ := startNode(t)
	resp := postJSON(t, fmt.Sprintf("%s/api/v1/dlq/999/replay", base), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
