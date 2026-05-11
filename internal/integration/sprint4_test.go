package integration

// Sprint 4 integration tests.
// Covers: signed /.well-known/mrmi-audit, root hash gossip via ShareRootHash,
// policy hot-reload within 5 seconds, and the /peers/audit HTTP endpoint.

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/hotreload"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// startHTTPNode starts a gateway with an HTTP server and returns its HTTP address.
func startHTTPNode(t *testing.T, cfg config.Config, privKey ed25519.PrivateKey, peerCache *peercache.Cache) (node, string) {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("startHTTPNode %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	var adapter grpctransport.GatewayService
	if peerCache != nil {
		adapter = grpctransport.NewAdapterFull(gw, nil, peerCache)
	} else {
		adapter = grpctransport.NewAdapter(gw)
	}
	srv, err := grpctransport.NewServer(":0", adapter, nil)
	if err != nil {
		t.Fatalf("startHTTPNode %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, _ := net.SplitHostPort(srv.Addr())
	dialAddr := fmt.Sprintf("localhost:%s", port)
	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, nil)
	if err != nil {
		t.Fatalf("startHTTPNode %s: dial: %v", cfg.Node.NodeID, err)
	}
	t.Cleanup(func() { _ = client.Close() })

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
		PrivKey: privKey,
		Peers:   peerCache,
	})
	go func() { _ = httpSrv.ListenAndServe() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	})

	// Wait briefly for the HTTP server to start.
	time.Sleep(50 * time.Millisecond)

	return node{client: client, auditLog: auditLog, addr: dialAddr}, "http://" + httpAddr
}

// TestHTTPS_WellKnownSigned verifies that the audit endpoint returns a valid
// Ed25519 signature when https_well_known = true and a private key is provided.
func TestHTTPS_WellKnownSigned(t *testing.T) {
	priv, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	cfg := rsConfig()
	cfg.Policy.Audit.HTTPSWellKnown = true
	_, httpBase := startHTTPNode(t, cfg, priv, nil)

	resp, err := http.Get(httpBase + "/.well-known/mrmi-audit")
	if err != nil {
		t.Fatalf("GET well-known: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	var wk struct {
		ADRVersion    string `json:"adr_version"`
		AppVersion    string `json:"app_version"`
		ApplicableLaw string `json:"applicable_law"`
		NodeID        string `json:"node_id"`
		RootHash      string `json:"root_hash"`
		Timestamp     int64  `json:"timestamp"`
		Version       int    `json:"version"`
		Signature     string `json:"signature"`
	}
	if err := json.Unmarshal(body, &wk); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if wk.NodeID != "rs-node-01" {
		t.Fatalf("unexpected node_id %q", wk.NodeID)
	}
	if !strings.HasPrefix(wk.Signature, "ed25519:") {
		t.Fatalf("expected ed25519: prefix in signature, got %q", wk.Signature)
	}

	// Verify the signature.
	canonical, _ := json.Marshal(struct {
		ADRVersion    string `json:"adr_version"`
		AppVersion    string `json:"app_version"`
		ApplicableLaw string `json:"applicable_law"`
		NodeID        string `json:"node_id"`
		RootHash      string `json:"root_hash"`
		Timestamp     int64  `json:"timestamp"`
		Version       int    `json:"version"`
	}{
		ADRVersion:    wk.ADRVersion,
		AppVersion:    wk.AppVersion,
		ApplicableLaw: wk.ApplicableLaw,
		NodeID:        wk.NodeID,
		RootHash:      wk.RootHash,
		Timestamp:     wk.Timestamp,
		Version:       wk.Version,
	})

	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(wk.Signature, "ed25519:"))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !ed25519.Verify(pub, canonical, sigBytes) {
		t.Fatal("signature verification failed")
	}
}

// TestHTTPS_WellKnownDisabled verifies that the endpoint returns 404 when
// https_well_known = false.
func TestHTTPS_WellKnownDisabled(t *testing.T) {
	cfg := rsConfig()
	cfg.Policy.Audit.HTTPSWellKnown = false
	_, httpBase := startHTTPNode(t, cfg, nil, nil)

	resp, err := http.Get(httpBase + "/.well-known/mrmi-audit")
	if err != nil {
		t.Fatalf("GET well-known: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when https_well_known=false, got %d", resp.StatusCode)
	}
}

// TestRootHashGossip_StoredInPeerCache verifies that ShareRootHash stores the
// received hash in the peer cache and it appears at /peers/audit.
func TestRootHashGossip_StoredInPeerCache(t *testing.T) {
	cache := peercache.New()
	cfg := ruConfig()
	cfg.Policy.Audit.HTTPSWellKnown = true
	ruNode, httpBase := startHTTPNode(t, cfg, nil, cache)

	// Simulate RS gossipping its root hash to RU.
	_, err := ruNode.client.ShareRootHash(context.Background(), &grpctransport.RootHashMessage{
		NodeID:    "rs-node-01",
		RootHash:  "sha256:abcdef1234",
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("ShareRootHash: %v", err)
	}

	// Check /peers/audit.
	resp, err := http.Get(httpBase + "/peers/audit")
	if err != nil {
		t.Fatalf("GET /peers/audit: %v", err)
	}
	defer resp.Body.Close()

	var peers map[string]struct {
		RootHash  string `json:"root_hash"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		t.Fatalf("parse /peers/audit: %v", err)
	}
	entry, ok := peers["rs-node-01"]
	if !ok {
		t.Fatal("rs-node-01 not found in /peers/audit")
	}
	if entry.RootHash != "sha256:abcdef1234" {
		t.Fatalf("unexpected root hash %q", entry.RootHash)
	}
}

// TestHotReload_PolicyUpdatedWithin5s writes a new TOML config to a temp file
// and verifies the policy engine picks up the new allow-list within 5 seconds.
func TestHotReload_PolicyUpdatedWithin5s(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/node.toml"

	// Initial config: RS→US is denied.
	write := func(allowUS bool) {
		allowTo := `["RU", "BY", "KZ", "AM"]`
		if allowUS {
			allowTo = `["RU", "BY", "KZ", "AM", "US"]`
		}
		toml := fmt.Sprintf(`
[node]
node_id        = "rs-node-01"
node_scope     = "regional"
region         = "RS"
operator_id    = "ops"
policy_version = "v2"
applicable_law = "RS-GDPR"
signed_by      = "ed25519:REPLACE_ME"

[policy.outbound]
allow_to = %s

[network]
http_listen_addr = ":8080"
grpc_port        = 7777
`, allowTo)
		if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(false)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	watcher := hotreload.New()
	go watcher.Watch(ctx, path, func(newCfg config.Config) {
		_ = engine.Reload(newCfg)
	})

	// Confirm RS→US is denied initially.
	if r := engine.Evaluate(policy.Request{SenderRegion: "RS", RecipientRegion: "US"}); r.Decision != policy.DecisionDeny {
		t.Fatalf("expected DENY before reload, got %q", r.Decision)
	}

	// Give watcher time to record initial mtime.
	time.Sleep(700 * time.Millisecond)

	// Write config that allows US.
	write(true)

	// Wait up to 4 seconds for the engine to pick up the new config.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		r := engine.Evaluate(policy.Request{SenderRegion: "RS", RecipientRegion: "US"})
		if r.Decision == policy.DecisionAllow {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("policy did not reload within 4 seconds of config file change")
}

// TestRootHashGossip_ShareRootHashRoundTrip verifies that a ShareRootHash
// call is properly received, acknowledged, and stored in the peer cache.
func TestRootHashGossip_ShareRootHashRoundTrip(t *testing.T) {
	cache := peercache.New()

	// Start a RU node with a peer cache.
	ruCfg := ruConfig()
	ruAuditLog := audit.New()
	ruEngine, _ := policy.NewEngine(ruCfg, ruAuditLog, nil)
	ruGW := core.NewGateway(ruCfg, ruEngine, ruAuditLog, dedup.New(ruCfg.Profile.DedupTTL), nil)
	ruAdapter := grpctransport.NewAdapterFull(ruGW, nil, cache)
	ruSrv, err := grpctransport.NewServer(":0", ruAdapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ruSrv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ruSrv.Shutdown(ctx)
	})

	_, port, _ := net.SplitHostPort(ruSrv.Addr())
	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ruClient, err := grpctransport.Dial(dialCtx, fmt.Sprintf("localhost:%s", port), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ruClient.Close() })

	ack, err := ruClient.ShareRootHash(context.Background(), &grpctransport.RootHashMessage{
		NodeID:    "rs-node-01",
		RootHash:  "sha256:deadbeef",
		Timestamp: 12345,
	})
	if err != nil {
		t.Fatalf("ShareRootHash: %v", err)
	}
	if !ack.Accepted {
		t.Fatal("expected Accepted=true in RootHashAck")
	}

	all := cache.All()
	if all["rs-node-01"].RootHash != "sha256:deadbeef" {
		t.Fatalf("expected sha256:deadbeef in cache, got %q", all["rs-node-01"].RootHash)
	}
}

// loadPubKeyFromCert is a helper used in the signing test.
func loadPubKeyFromCert(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	pubBytes, _ := x509.MarshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
}
