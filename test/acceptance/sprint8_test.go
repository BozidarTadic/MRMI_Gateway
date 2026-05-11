package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/inbox"
	"MRMI_Gateway/internal/peerdiscovery"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	"MRMI_Gateway/internal/token"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// startNodeV3 starts a gateway node with v0.3 deps (RuntimeApps, JWT, RuntimePeers).
func startNodeV3(t *testing.T, cfg config.Config) (string, *server.RuntimeApps) {
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
		msgInbox.Publish(inbox.Event{IdempotencyKey: env.IdempotencyKey})
	})

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
	httpAddr := l.Addr().String()
	_ = l.Close()
	cfg.Network.HTTPListenAddr = httpAddr

	runtimeApps := server.NewRuntimeApps()
	httpSrv := server.NewHTTPServer(cfg, server.ServerDeps{
		Engine:       engine,
		Audit:        auditLog,
		Gateway:      gw,
		DLQ:          dlq,
		CRL:          crlStore,
		Inbox:        msgInbox,
		RuntimePeers: server.NewRuntimePeers(),
		RuntimeApps:  runtimeApps,
		OnConfigSave: func(newCfg config.Config) error { return engine.Reload(newCfg) },
	})
	go func() { _ = httpSrv.ListenAndServe() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	})

	time.Sleep(50 * time.Millisecond)
	return "http://" + httpAddr, runtimeApps
}

func makeJWT(t *testing.T, secret, scope string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"scope": scope,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return tok
}

func authReq(t *testing.T, method, url string, body any, key string) *http.Response {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		if strings.HasPrefix(key, "ey") {
			req.Header.Set("Authorization", "Bearer "+key)
		} else {
			req.Header.Set("X-MRMI-Key", key)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// ── App management ───────────────────────────────────────────────────────────

func TestApps_RegisterAndList(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "apps-test"
	cfg.API.APIKey = "test-key"
	base, _ := startNodeV3(t, cfg)

	// Register an app
	resp := authReq(t, http.MethodPost, base+"/api/v1/apps/register",
		map[string]any{"app_id": "my-app", "webhook_url": "https://app.example.com/hook"},
		"test-key")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var reg map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		t.Fatal(err)
	}
	if reg["app_id"] != "my-app" {
		t.Fatalf("unexpected app_id %q", reg["app_id"])
	}
	if reg["api_key"] == "" {
		t.Fatal("expected non-empty api_key in response")
	}

	// List apps — should contain "my-app"
	listResp := authReq(t, http.MethodGet, base+"/api/v1/apps", nil, "test-key")
	defer listResp.Body.Close()
	var apps []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&apps); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range apps {
		if a["app_id"] == "my-app" {
			found = true
		}
	}
	if !found {
		t.Fatalf("registered app not found in list: %v", apps)
	}
}

func TestApps_DeleteRemovesFromList(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "apps-del-test"
	cfg.API.APIKey = "test-key"
	base, _ := startNodeV3(t, cfg)

	authReq(t, http.MethodPost, base+"/api/v1/apps/register",
		map[string]any{"app_id": "delete-me"}, "test-key").Body.Close()

	del := authReq(t, http.MethodDelete, base+"/api/v1/apps/delete-me", nil, "test-key")
	defer del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 on delete, got %d", del.StatusCode)
	}

	listResp := authReq(t, http.MethodGet, base+"/api/v1/apps", nil, "test-key")
	defer listResp.Body.Close()
	var apps []map[string]any
	json.NewDecoder(listResp.Body).Decode(&apps)
	for _, a := range apps {
		if a["app_id"] == "delete-me" {
			t.Fatal("deleted app still appears in list")
		}
	}
}

func TestApps_RegisterRequiresOperatorScope(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "apps-auth-test"
	cfg.API.JWTSecret = "test-jwt-secret"
	base, _ := startNodeV3(t, cfg)

	// read-scope JWT → 401
	readTok := makeJWT(t, "test-jwt-secret", "read")
	resp := authReq(t, http.MethodPost, base+"/api/v1/apps/register",
		map[string]any{"app_id": "unauthorized-app"}, readTok)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with read scope, got %d", resp.StatusCode)
	}

	// operator-scope JWT → 201
	opTok := makeJWT(t, "test-jwt-secret", "operator")
	resp2 := authReq(t, http.MethodPost, base+"/api/v1/apps/register",
		map[string]any{"app_id": "authorized-app"}, opTok)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 with operator scope, got %d", resp2.StatusCode)
	}
}

// ── Dashboard UI ─────────────────────────────────────────────────────────────

func TestDashboard_UIServed(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "ui-test"
	base, _ := startNodeV3(t, cfg)

	resp, err := http.Get(base + "/ui/")
	if err != nil {
		t.Fatalf("GET /ui/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html, got %q", ct)
	}
}

func TestDashboard_AppJSServed(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "ui-js-test"
	base, _ := startNodeV3(t, cfg)

	resp, err := http.Get(base + "/ui/app.js")
	if err != nil {
		t.Fatalf("GET /ui/app.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDashboard_UIRedirect(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "ui-redirect-test"
	base, _ := startNodeV3(t, cfg)

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(base + "/ui")
	if err != nil {
		t.Fatalf("GET /ui: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect, got %d", resp.StatusCode)
	}
}

// ── JWT auth ─────────────────────────────────────────────────────────────────

func TestJWT_ReadScopeAllowsGetApps(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "jwt-read-test"
	cfg.API.JWTSecret = "s3cr3t"
	base, _ := startNodeV3(t, cfg)

	readTok := makeJWT(t, "s3cr3t", "read")
	resp := authReq(t, http.MethodGet, base+"/api/v1/apps", nil, readTok)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with read JWT, got %d", resp.StatusCode)
	}
}

func TestJWT_ExpiredTokenDenied(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "jwt-exp-test"
	cfg.API.JWTSecret = "s3cr3t"
	base, _ := startNodeV3(t, cfg)

	expiredTok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"scope": "operator",
		"exp":   time.Now().Add(-time.Hour).Unix(),
	}).SignedString([]byte("s3cr3t"))

	resp := authReq(t, http.MethodGet, base+"/api/v1/apps", nil, expiredTok)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with expired JWT, got %d", resp.StatusCode)
	}
}

// ── PUT /api/v1/config ───────────────────────────────────────────────────────

func TestConfigPut_RequiresOperator(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "config-put-test"
	cfg.API.JWTSecret = "s3cr3t"
	base, _ := startNodeV3(t, cfg)

	readTok := makeJWT(t, "s3cr3t", "read")
	resp := authReq(t, http.MethodPut, base+"/api/v1/config",
		config.DefaultBalancedConfig(), readTok)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with read scope on PUT /config, got %d", resp.StatusCode)
	}
}

func TestConfigPut_OperatorReloadsPolicy(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "config-reload-test"
	cfg.API.APIKey = "op-key"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}
	base, runtimeApps := startNodeV3(t, cfg)
	_ = runtimeApps

	newCfg := config.DefaultBalancedConfig()
	newCfg.Node.NodeID = "config-reload-test"
	newCfg.Policy.Outbound.AllowTo = []string{"RU", "BY"}

	resp := authReq(t, http.MethodPut, base+"/api/v1/config", newCfg, "op-key")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

// ── Peer gossip exchange ─────────────────────────────────────────────────────

func TestPeerGossip_ExchangePeers(t *testing.T) {
	reg1 := peerdiscovery.New()
	reg2 := peerdiscovery.New()

	reg1.Announce(peerdiscovery.PeerInfo{NodeID: "n1", Addr: ":7001", NodeScope: "regional", Region: "RS"})
	reg2.Announce(peerdiscovery.PeerInfo{NodeID: "n2", Addr: ":7002", NodeScope: "regional", Region: "RU"})

	cfg1 := config.DefaultBalancedConfig()
	cfg1.Node.NodeID = "gossip-n1"

	cfg2 := config.DefaultBalancedConfig()
	cfg2.Node.NodeID = "gossip-n2"

	gw1 := makeMinGateway(t, cfg1)
	gw2 := makeMinGateway(t, cfg2)

	deps1 := grpctransport.DiscoveryDeps{PeerRegistry: reg1, NodeCfg: cfg1, TokenStore: token.New()}
	deps2 := grpctransport.DiscoveryDeps{PeerRegistry: reg2, NodeCfg: cfg2, TokenStore: token.New()}

	adapter1 := grpctransport.NewAdapterWithDiscovery(gw1, nil, nil, deps1)
	adapter2 := grpctransport.NewAdapterWithDiscovery(gw2, nil, nil, deps2)

	srv1, _ := grpctransport.NewServer(":0", adapter1, nil)
	srv2, _ := grpctransport.NewServer(":0", adapter2, nil)

	go srv1.Serve()
	go srv2.Serve()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv1.Shutdown(ctx)
		srv2.Shutdown(ctx)
	})

	time.Sleep(30 * time.Millisecond)

	// Dial node2 from node1 and exchange peers
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, srv2.Addr(), nil)
	if err != nil {
		t.Fatalf("dial node2: %v", err)
	}
	defer client.Close()

	n1peers := reg1.Known()
	entries := make([]grpctransport.PeerEntry, 0, len(n1peers))
	for _, p := range n1peers {
		entries = append(entries, grpctransport.PeerEntry{
			NodeID: p.NodeID, Addr: p.Addr, NodeScope: p.NodeScope, Region: p.Region,
		})
	}

	resp, err := client.ExchangePeers(context.Background(), &grpctransport.PeerListRequest{
		SenderNodeID: cfg1.Node.NodeID,
		KnownPeers:   entries,
	})
	if err != nil {
		t.Fatalf("ExchangePeers: %v", err)
	}

	// Node2 should return its known peer (n2) and we should have received it
	found := false
	for _, p := range resp.Peers {
		if p.NodeID == "n2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected n2 in gossip response, got: %v", resp.Peers)
	}

	// After the exchange, node2's registry should now know about n1
	time.Sleep(10 * time.Millisecond)
	known2 := reg2.Known()
	foundN1 := false
	for _, p := range known2 {
		if p.NodeID == "n1" {
			foundN1 = true
		}
	}
	if !foundN1 {
		t.Fatalf("expected n1 in node2 registry after gossip, got: %v", known2)
	}
}

func makeMinGateway(t *testing.T, cfg config.Config) *core.Gateway {
	t.Helper()
	auditLog := audit.New()
	crlStore := crl.New()
	engine, err := policy.NewEngine(cfg, auditLog, crlStore)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	return core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
}

// TestApps_RegisterDuplicate overwrites an existing registration.
func TestApps_RegisterDuplicate(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "apps-dup-test"
	cfg.API.APIKey = "test-key"
	base, _ := startNodeV3(t, cfg)

	for range 2 {
		resp := authReq(t, http.MethodPost, base+"/api/v1/apps/register",
			map[string]any{"app_id": "dup-app", "webhook_url": fmt.Sprintf("https://example.com/%d", time.Now().UnixNano())},
			"test-key")
		resp.Body.Close()
	}

	listResp := authReq(t, http.MethodGet, base+"/api/v1/apps", nil, "test-key")
	defer listResp.Body.Close()
	var apps []map[string]any
	json.NewDecoder(listResp.Body).Decode(&apps)

	count := 0
	for _, a := range apps {
		if a["app_id"] == "dup-app" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 entry for dup-app, got %d", count)
	}
}
