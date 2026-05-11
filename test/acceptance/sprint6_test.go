package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	"MRMI_Gateway/internal/registry"
	"MRMI_Gateway/internal/server"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
	"MRMI_Gateway/internal/webhook"
)

// startNodeWithCfg is like startNode but accepts a pre-built config for Sprint 6 tests.
func startNodeWithCfg(t *testing.T, cfg config.Config) (string, *core.Gateway) {
	t.Helper()

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
		})
	})

	reg := registry.New(cfg)
	runtimePeers := server.NewRuntimePeers()

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

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()
	httpAddr := "127.0.0.1:" + port
	cfg.Network.HTTPListenAddr = httpAddr

	httpSrv := server.NewHTTPServer(cfg, server.ServerDeps{
		Engine:       engine,
		Audit:        auditLog,
		Gateway:      gw,
		DLQ:          dlq,
		CRL:          crlStore,
		Inbox:        msgInbox,
		Registry:     reg,
		RuntimePeers: runtimePeers,
		OnConfigReload: func() error {
			newCfg := cfg
			newCfg.Policy.Outbound.AllowTo = append(newCfg.Policy.Outbound.AllowTo, "US")
			return engine.Reload(newCfg)
		},
	})
	go func() { _ = httpSrv.ListenAndServe() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	})

	time.Sleep(50 * time.Millisecond)
	return "http://" + httpAddr, gw
}

// TestAPIKey_Unauthorized rejects requests with wrong key.
func TestAPIKey_Unauthorized(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "auth-test-node"
	cfg.API.APIKey = "mrmi_op_secret"

	base, _ := startNodeWithCfg(t, cfg)

	// No key → 401
	resp, _ := http.Post(base+"/api/v1/peers/register", "application/json",
		jsonBody(t, map[string]any{"addr": "localhost:9999"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", resp.StatusCode)
	}

	// Wrong key → 401
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/peers/register",
		jsonBody(t, map[string]any{"addr": "localhost:9999"}))
	req.Header.Set("X-MRMI-Key", "wrong-key")
	req.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", resp2.StatusCode)
	}
}

// TestAPIKey_Authorized allows requests with correct key.
func TestAPIKey_Authorized(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "auth-test-node"
	cfg.API.APIKey = "mrmi_op_secret"

	base, _ := startNodeWithCfg(t, cfg)

	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/peers/register",
		jsonBody(t, map[string]any{"addr": "localhost:9999", "region": "RU", "node_scope": "regional"}))
	req.Header.Set("X-MRMI-Key", "mrmi_op_secret")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with correct key, got %d", resp.StatusCode)
	}
}

// TestPeersRegister_AppearsInList verifies registered peers show up in GET /api/v1/peers.
func TestPeersRegister_AppearsInList(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "peer-reg-node"
	base, _ := startNodeWithCfg(t, cfg)

	postJSON(t, base+"/api/v1/peers/register", map[string]any{
		"addr": "ru-peer.example.com:7777", "region": "RU", "node_scope": "regional",
	})

	resp := get(t, base+"/api/v1/peers")
	defer resp.Body.Close()
	var peers []map[string]any
	decodeJSON(t, resp.Body, &peers)

	found := false
	for _, p := range peers {
		if p["addr"] == "ru-peer.example.com:7777" {
			found = true
		}
	}
	if !found {
		t.Fatal("registered peer not found in GET /api/v1/peers")
	}
}

// TestConfigReload applies new policy via POST /api/v1/config/reload.
func TestConfigReload_AppliesNewPolicy(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "reload-node"
	base, gw := startNodeWithCfg(t, cfg)

	// Before reload: US is denied.
	resp1, _ := gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "pre-reload-us",
			SenderRegion:    "RS",
			RecipientRegion: "US",
		},
	})
	if resp1.Decision != "DENY" {
		t.Fatalf("expected DENY before reload, got %q", resp1.Decision)
	}

	// Trigger reload.
	resp := postJSON(t, base+"/api/v1/config/reload", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// After reload: US is now allowed (our stub adds it).
	time.Sleep(50 * time.Millisecond)
	resp2, _ := gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "post-reload-us",
			SenderRegion:    "RS",
			RecipientRegion: "US",
		},
	})
	if resp2.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW after reload, got %q", resp2.Decision)
	}
}

// TestDLQ_Discard removes an entry via POST /api/v1/dlq/{id}/discard.
func TestDLQ_Discard(t *testing.T) {
	base, _, dlq, _, _, _ := startNode(t)

	dlq.Append(delivery.DLQEntry{
		PeerAddr: "discard-test:7777",
		Envelope: core.Envelope{IdempotencyKey: "discard-01"},
	})

	resp := postJSON(t, base+"/api/v1/dlq/0/discard", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if dlq.Size() != 0 {
		t.Fatal("DLQ should be empty after discard")
	}
}

// TestRevoke_ViaAPIEndpoint adds a revocation via POST /api/v1/revoke/{node_id}.
func TestRevoke_ViaAPIEndpoint(t *testing.T) {
	base, _, _, crlStore, _, _ := startNode(t)

	body := map[string]any{
		"reason":       "test revoke",
		"signature_b64": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	resp := postJSON(t, base+"/api/v1/revoke/bad-node-revoke", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var m map[string]any
	decodeJSON(t, resp.Body, &m)
	if m["node_id"] != "bad-node-revoke" {
		t.Fatalf("unexpected node_id: %v", m["node_id"])
	}
	// Needs 2 sigs to be effective.
	if m["is_effective"] == true {
		t.Fatal("should not be effective with 1 sig")
	}
	_ = crlStore
}

// TestDiscovery_RoundTrip registers users via config and exercises discover + connect.
func TestDiscovery_RoundTrip(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "discovery-node"
	cfg.Apps = map[string]config.AppConfig{
		"rs-app": {
			AutoAccept: "auto_all",
			Users: map[string]config.UserConfig{
				"user-marko": {DisplayHint: "Marko Petrović", Region: "RS"},
			},
		},
	}
	base, _ := startNodeWithCfg(t, cfg)

	// Discover.
	resp := get(t, base+"/api/v1/discover?q=marko&type=display_hint")
	defer resp.Body.Close()

	var results []map[string]any
	decodeJSON(t, resp.Body, &results)
	if len(results) == 0 {
		t.Fatal("expected discovery results for 'marko'")
	}
	token, _ := results[0]["opaque_token"].(string)
	if token == "" {
		t.Fatal("opaque_token must not be empty")
	}

	// Connect.
	connResp := postJSON(t, base+"/api/v1/connect", map[string]any{
		"opaque_token":     token,
		"requester_id":     "ru-user-01",
		"requester_region": "RU",
	})
	defer connResp.Body.Close()
	if connResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(connResp.Body)
		t.Fatalf("expected 200, got %d: %s", connResp.StatusCode, body)
	}
	var conn map[string]any
	decodeJSON(t, connResp.Body, &conn)
	if conn["status"] != "ACCEPTED" {
		t.Fatalf("expected ACCEPTED (auto_all), got %q", conn["status"])
	}
}

// TestWebhook_DeliveredOnAllow verifies webhook fires on ALLOW.
func TestWebhook_DeliveredOnAllow(t *testing.T) {
	var received []byte
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		received = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "webhook-test-node"
	cfg.Apps = map[string]config.AppConfig{
		"hook-app": {
			WebhookURL:    webhookSrv.URL,
			WebhookSecret: "test-secret",
			WebhookTimeout: 5,
		},
	}
	_, gw := startNodeWithCfg(t, cfg)
	gw.SetNotifier(webhook.New(cfg))

	_, _ = gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "hook-test-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(received) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if len(received) == 0 {
		t.Fatal("webhook not received within 2s")
	}
	var payload map[string]any
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("parse webhook body: %v", err)
	}
	if payload["idempotency_key"] != "hook-test-001" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}
