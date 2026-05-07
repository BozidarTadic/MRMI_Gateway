// Package integration contains multi-node end-to-end tests.
// Each test spins up two in-process gateway nodes on random ports and
// exercises the full stack: config → policy engine → dedup → audit log → gRPC.
package integration

import (
	"context"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// node bundles the live components of a running gateway node so tests
// can inspect audit state directly.
type node struct {
	client   *grpctransport.Client
	auditLog *audit.Log
}

// startNode wires all gateway components, binds gRPC on a random port,
// and registers cleanup with t. Returns an active gRPC client and the
// shared audit log.
func startNode(t *testing.T, cfg config.Config) node {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		t.Fatalf("startNode %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	srv, err := grpctransport.NewServer(
		":0",
		grpctransport.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL)),
	)
	if err != nil {
		t.Fatalf("startNode %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, srv.Addr())
	if err != nil {
		t.Fatalf("startNode %s: dial %s: %v", cfg.Node.NodeID, srv.Addr(), err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return node{client: client, auditLog: auditLog}
}

func rsConfig() config.Config {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeID = "rs-node-01"
	cfg.Node.Region = "RS"
	cfg.Node.ApplicableLaw = "RS-GDPR"
	cfg.Policy.Outbound.AllowTo = []string{"RU", "BY", "KZ", "AM"}
	return cfg
}

func ruConfig() config.Config {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeID = "ru-node-01"
	cfg.Node.Region = "RU"
	cfg.Node.ApplicableLaw = "RU-PDL"
	cfg.Policy.Outbound.AllowTo = []string{"RS", "BY", "KZ", "AM"}
	return cfg
}

// TestTwoNodeLocalCorridor is the primary two-node integration test.
// It covers:
//   - RS node approves an RS→RU envelope and returns an audit root
//   - Replaying the same idempotency key returns DUPLICATE and advances the audit root
//   - RU node has an independent dedup state: the same key is NOT a duplicate on RU
//   - RU node correctly processes envelopes per its own policy
func TestTwoNodeLocalCorridor(t *testing.T) {
	rs := startNode(t, rsConfig())
	ru := startNode(t, ruConfig())

	ctx := context.Background()

	// ── RS node: first send RS→RU ─────────────────────────────────────────────
	req := &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "corridor-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	}

	resp1, err := rs.client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("RS first send: %v", err)
	}
	if resp1.Decision != "ALLOW" {
		t.Fatalf("RS first send: expected ALLOW, got %q (%s)", resp1.Decision, resp1.Reason)
	}
	if resp1.NodeID != "rs-node-01" {
		t.Fatalf("RS first send: expected NodeID rs-node-01, got %q", resp1.NodeID)
	}
	rootAfterAllow := resp1.AuditRootHash
	if rootAfterAllow == "" {
		t.Fatal("RS first send: expected non-empty audit root hash")
	}

	// ── RS node: replay — must be DUPLICATE, audit root advances ─────────────
	resp2, err := rs.client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("RS replay: %v", err)
	}
	if resp2.Decision != "DUPLICATE" {
		t.Fatalf("RS replay: expected DUPLICATE, got %q (%s)", resp2.Decision, resp2.Reason)
	}
	if resp2.AuditRootHash == rootAfterAllow {
		t.Fatal("RS replay: audit root must advance after DUPLICATE decision is logged")
	}

	// Verify the RS audit chain is intact after both entries.
	if err := rs.auditLog.Verify(); err != nil {
		t.Fatalf("RS audit chain broken: %v", err)
	}
	if entries := rs.auditLog.Entries(); len(entries) != 2 {
		t.Fatalf("RS audit: expected 2 entries (ALLOW + DUPLICATE), got %d", len(entries))
	}

	// ── RU node: same idempotency key — independent dedup, must be ALLOW ─────
	// The RU node has never seen "corridor-001"; its dedup state is separate.
	ruReq := &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "corridor-001", // same key, different node
			SenderRegion:    "RS",
			RecipientRegion: "RS", // RU's allow_to contains RS
		},
	}

	resp3, err := ru.client.SendEnvelope(ctx, ruReq)
	if err != nil {
		t.Fatalf("RU send: %v", err)
	}
	if resp3.Decision != "ALLOW" {
		t.Fatalf("RU send: expected ALLOW (independent dedup), got %q (%s)", resp3.Decision, resp3.Reason)
	}
	if resp3.NodeID != "ru-node-01" {
		t.Fatalf("RU send: expected NodeID ru-node-01, got %q", resp3.NodeID)
	}

	// ── RU node: policy enforced — denied region returns DENY ────────────────
	ruDenied, err := ru.client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "corridor-002",
			SenderRegion:    "RU",
			RecipientRegion: "US", // not in RU's allow_to
		},
	})
	if err != nil {
		t.Fatalf("RU denied send: %v", err)
	}
	if ruDenied.Decision != "DENY" {
		t.Fatalf("RU denied send: expected DENY for US, got %q", ruDenied.Decision)
	}
}

// TestTwoNodeAuditRootsAreIndependent confirms that two nodes sharing no state
// produce completely independent Merkle chains.
func TestTwoNodeAuditRootsAreIndependent(t *testing.T) {
	rs := startNode(t, rsConfig())
	ru := startNode(t, ruConfig())

	ctx := context.Background()

	send := func(client *grpctransport.Client, key, to string) string {
		t.Helper()
		resp, err := client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
			Envelope: grpctransport.Envelope{
				IdempotencyKey:  key,
				SenderRegion:    "RS",
				RecipientRegion: to,
			},
		})
		if err != nil {
			t.Fatalf("send %q→%q: %v", key, to, err)
		}
		return resp.AuditRootHash
	}

	rsRoot := send(rs.client, "ind-001", "RU")
	ruRoot := send(ru.client, "ind-001", "RS")

	if rsRoot == ruRoot {
		t.Fatal("expected independent audit roots; RS and RU roots must not match after separate sends")
	}
	if err := rs.auditLog.Verify(); err != nil {
		t.Fatalf("RS audit chain: %v", err)
	}
	if err := ru.auditLog.Verify(); err != nil {
		t.Fatalf("RU audit chain: %v", err)
	}
}
