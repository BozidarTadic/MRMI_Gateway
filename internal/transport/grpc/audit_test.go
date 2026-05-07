package grpctransport

import (
	"context"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

// setupAuditServer creates a server that shares an audit.Log with the test.
// Returns the connected client, the audit log, and a cleanup func.
func setupAuditServer(t *testing.T, cfg config.Config) (*Client, *audit.Log) {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}

	srv, err := NewServer(":0", NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL)))
	if err != nil {
		t.Fatalf("create grpc server: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(dialCtx, srv.listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client, auditLog
}

func TestAudit_AllowDecisionWrittenToLog(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	client, auditLog := setupAuditServer(t, cfg)
	ctx := context.Background()

	zeroRoot := auditLog.RootHash()

	resp, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "audit-allow-1",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW, got %q", resp.Decision)
	}

	entries := auditLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Decision != audit.DecisionAllow {
		t.Fatalf("expected ALLOW entry, got %q", entries[0].Decision)
	}
	if resp.AuditRootHash == zeroRoot {
		t.Fatal("audit root hash must change after ALLOW decision")
	}
	if resp.AuditRootHash != auditLog.RootHash() {
		t.Fatalf("response root hash %q does not match log root %q", resp.AuditRootHash, auditLog.RootHash())
	}
}

func TestAudit_DenyDecisionWrittenToLog(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	client, auditLog := setupAuditServer(t, cfg)
	ctx := context.Background()

	zeroRoot := auditLog.RootHash()

	resp, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "audit-deny-1",
			SenderRegion:    "RS",
			RecipientRegion: "US", // not in allow_to
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Decision != "DENY" {
		t.Fatalf("expected DENY, got %q", resp.Decision)
	}

	entries := auditLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Decision != audit.DecisionDeny {
		t.Fatalf("expected DENY entry, got %q", entries[0].Decision)
	}
	if resp.AuditRootHash == zeroRoot {
		t.Fatal("audit root hash must change after DENY decision")
	}
	if resp.AuditRootHash != auditLog.RootHash() {
		t.Fatalf("response root hash %q does not match log root %q", resp.AuditRootHash, auditLog.RootHash())
	}
}

func TestAudit_DuplicateDecisionWrittenToLog(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	client, auditLog := setupAuditServer(t, cfg)
	ctx := context.Background()

	req := &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "audit-dup-1",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	}

	first, err := client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if first.Decision != "ALLOW" {
		t.Fatalf("first send: expected ALLOW, got %q", first.Decision)
	}
	rootAfterAllow := auditLog.RootHash()

	second, err := client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	if second.Decision != "DUPLICATE" {
		t.Fatalf("second send: expected DUPLICATE, got %q", second.Decision)
	}

	entries := auditLog.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}
	if entries[1].Decision != audit.DecisionDuplicate {
		t.Fatalf("expected DUPLICATE entry, got %q", entries[1].Decision)
	}
	if second.AuditRootHash == rootAfterAllow {
		t.Fatal("audit root hash must change after DUPLICATE decision")
	}
	if second.AuditRootHash != auditLog.RootHash() {
		t.Fatalf("response root hash %q does not match log root %q", second.AuditRootHash, auditLog.RootHash())
	}
}

func TestAudit_LogRemainsVerifiableAfterMixedDecisions(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	client, auditLog := setupAuditServer(t, cfg)
	ctx := context.Background()

	sends := []struct {
		key, to    string
		wantDecide string
	}{
		{"mix-1", "RU", "ALLOW"},
		{"mix-2", "US", "DENY"},
		{"mix-1", "RU", "DUPLICATE"}, // replay of mix-1
	}

	for _, s := range sends {
		resp, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
			Envelope: Envelope{
				IdempotencyKey:  s.key,
				SenderRegion:    "RS",
				RecipientRegion: s.to,
			},
		})
		if err != nil {
			t.Fatalf("send %q: %v", s.key, err)
		}
		if resp.Decision != s.wantDecide {
			t.Fatalf("send %q: expected %s, got %q", s.key, s.wantDecide, resp.Decision)
		}
		if resp.AuditRootHash != auditLog.RootHash() {
			t.Fatalf("send %q: response root hash out of sync with log", s.key)
		}
	}

	if len(auditLog.Entries()) != 3 {
		t.Fatalf("expected 3 audit entries, got %d", len(auditLog.Entries()))
	}
	if err := auditLog.Verify(); err != nil {
		t.Fatalf("audit log verification failed: %v", err)
	}
}
