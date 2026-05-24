package acceptance

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	storebb "MRMI_Gateway/internal/store/bbolt"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// launchNode starts an in-process gateway node using the provided config and
// audit log. It returns the HTTP base URL and an idempotent shutdown function.
// t.Cleanup is also registered as a fallback so the node always stops at
// the end of the test even if the caller does not invoke shutdown explicitly.
func launchNode(t *testing.T, cfg config.Config, auditLog *audit.Log) (string, func()) {
	t.Helper()

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

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	httpAddr := l.Addr().String()
	_ = l.Close()
	cfg.Network.HTTPListenAddr = httpAddr

	httpSrv := server.NewHTTPServer(cfg, server.Deps{
		Engine:       engine,
		Audit:        auditLog,
		Gateway:      gw,
		DLQ:          dlq,
		CRL:          crlStore,
		Inbox:        msgInbox,
		RuntimePeers: server.NewRuntimePeers(),
		RuntimeApps:  server.NewRuntimeApps(),
	})
	go func() { _ = httpSrv.ListenAndServe() }()
	time.Sleep(50 * time.Millisecond)

	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = httpSrv.Shutdown(ctx)
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
			defer cancel2()
			_ = grpcSrv.Shutdown(ctx2)
		})
	}
	t.Cleanup(shutdown)
	return "http://" + httpAddr, shutdown
}

// moduleRoot returns the Go module root by locating go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

// TestAuditPersistence_SurvivesRestart starts a bbolt-backed node, sends an
// envelope, shuts the node down, reopens the same store, and asserts that
// /api/v1/audit/latest returns the entry written during the first run.
func TestAuditPersistence_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "audit-persist-node"
	cfg.Node.Region = "RS"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	// --- First run ---
	store1, err := storebb.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store1.Close() }) // fallback; explicit close below
	auditLog1 := audit.New()
	auditLog1.SetStore(store1)
	base1, shutdown1 := launchNode(t, cfg, auditLog1)

	resp := postJSON(t, base1+"/api/v1/envelopes", map[string]any{
		"idempotency_key":  "persist-001",
		"sender_region":    "RS",
		"recipient_region": "RU",
		"trust_tier":       1,
	})
	resp.Body.Close()

	// Explicitly stop the node and close the store before reopening.
	// bbolt is a single-writer database; it must be closed before a second Open.
	shutdown1()
	_ = store1.Close()

	// --- Second run (simulated restart) ---
	store2, err := storebb.Open(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })

	auditLog2 := audit.New() // empty in-memory chain; store carries history
	auditLog2.SetStore(store2)
	base2, _ := launchNode(t, cfg, auditLog2)

	auditResp := get(t, base2+"/api/v1/audit/latest")
	defer auditResp.Body.Close()
	var entries []map[string]any
	decodeJSON(t, auditResp.Body, &entries)

	if len(entries) == 0 {
		t.Fatal("expected audit entries to survive node restart via bbolt backend")
	}
	found := false
	for _, e := range entries {
		if e["sender_region"] == "RS" && e["recipient_region"] == "RU" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("RS→RU audit entry not found after restart; got: %v", entries)
	}
}

// TestCLINodeStatus_PrintsNodeID starts a node and runs `mrmi node status`,
// asserting that the configured node_id appears in the output.
func TestCLINodeStatus_PrintsNodeID(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "cli-status-node"
	cfg.Node.Region = "RS"

	base, _ := launchNode(t, cfg, audit.New())

	root := moduleRoot(t)
	cmd := exec.Command("go", "run", "./cmd/mrmi", "node", "status", "--url", base)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mrmi node status: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "cli-status-node") {
		t.Errorf("expected node_id %q in output\ngot:\n%s", "cli-status-node", out)
	}
}

// TestCLITokenIssue_PrintsJWT starts a node with an API key and JWT secret,
// runs `mrmi token issue`, and asserts the output contains a valid JWT prefix.
func TestCLITokenIssue_PrintsJWT(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "cli-token-node"
	cfg.Node.Region = "RS"
	cfg.API.APIKey = "test-api-key"
	cfg.API.JWTSecret = "test-jwt-secret"

	base, _ := launchNode(t, cfg, audit.New())

	root := moduleRoot(t)
	cmd := exec.Command("go", "run", "./cmd/mrmi",
		"token", "issue",
		"--url", base,
		"--api-key", "test-api-key",
		"--scope", "read",
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mrmi token issue: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Token:") {
		t.Errorf("expected 'Token:' in output\ngot:\n%s", outStr)
	}
	if !strings.Contains(outStr, "ey") {
		t.Errorf("expected JWT (contains 'ey') in output\ngot:\n%s", outStr)
	}
}
