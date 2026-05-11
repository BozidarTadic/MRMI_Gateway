// Package integration contains multi-node end-to-end tests.
// Each test spins up two in-process gateway nodes on random ports and
// exercises the full stack: config → policy engine → dedup → audit log → gRPC.
package integration

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/policy"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// node bundles the live components of a running gateway node so tests
// can inspect audit state directly.
type node struct {
	client   *grpctransport.Client
	auditLog *audit.Log
	addr     string // dialable address (localhost:PORT)
}

// nodeWithDLQ extends node with a DLQ reference for forwarding tests.
type nodeWithDLQ struct {
	client   *grpctransport.Client
	auditLog *audit.Log
	dlq      *delivery.DLQ
}

// startNode wires all gateway components (no forwarder), binds gRPC on a random
// port, and registers cleanup with t.
func startNode(t *testing.T, cfg config.Config) node {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("startNode %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	srv, err := grpctransport.NewServer(":0", grpctransport.NewAdapter(gw), nil)
	if err != nil {
		t.Fatalf("startNode %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("startNode %s: parse addr %q: %v", cfg.Node.NodeID, srv.Addr(), err)
	}
	dialAddr := fmt.Sprintf("localhost:%s", port)

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, nil)
	if err != nil {
		t.Fatalf("startNode %s: dial %s: %v", cfg.Node.NodeID, dialAddr, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return node{client: client, auditLog: auditLog, addr: dialAddr}
}

// startNodeFwd wires gateway components with a live forwarder, binds gRPC on a
// random port, and registers cleanup with t. send is the transport function used
// by the forwarder; retryPolicy controls backoff and attempt limits.
func startNodeFwd(t *testing.T, cfg config.Config, retryPolicy delivery.RetryPolicy, send func(context.Context, string, core.Envelope) (string, error)) nodeWithDLQ {
	t.Helper()

	dlq := delivery.NewDLQ()
	fwd := delivery.NewForwarderWithPolicy(cfg, dlq, nil, send, retryPolicy)

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("startNodeFwd %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), fwd)
	srv, err := grpctransport.NewServer(":0", grpctransport.NewAdapter(gw), nil)
	if err != nil {
		t.Fatalf("startNodeFwd %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("startNodeFwd %s: parse addr: %v", cfg.Node.NodeID, err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, fmt.Sprintf("localhost:%s", port), nil)
	if err != nil {
		t.Fatalf("startNodeFwd %s: dial: %v", cfg.Node.NodeID, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return nodeWithDLQ{client: client, auditLog: auditLog, dlq: dlq}
}

// grpcSend returns a send function that dials addr and forwards env via gRPC,
// returning the peer's audit root hash on success.
func grpcSend(ctx context.Context, addr string, env core.Envelope) (string, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := grpctransport.Dial(dialCtx, addr, nil)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}
	defer c.Close()
	resp, err := c.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:    env.IdempotencyKey,
			SenderIdentity:    env.SenderIdentity,
			RecipientIdentity: env.RecipientIdentity,
			SenderRegion:      env.SenderRegion,
			RecipientRegion:   env.RecipientRegion,
			TrustTier:         env.TrustTier,
			SequenceNumber:    env.SequenceNumber,
			Payload:           env.Payload,
			PaddedTo:          env.PaddedTo,
			Timestamp:         env.Timestamp,
			Signature:         env.Signature,
		},
	})
	if err != nil {
		return "", err
	}
	if resp.Decision == "DENY" {
		return "", fmt.Errorf("peer denied: %s", resp.Reason)
	}
	return resp.AuditRootHash, nil
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

// TestTwoNodeForwardingCorridor verifies that an ALLOW decision on RS triggers
// forwarding to RU and that RU's audit log gains an entry.
func TestTwoNodeForwardingCorridor(t *testing.T) {
	// RU accepts envelopes addressed to itself; add "RU" to its allow_to so the
	// policy engine does not deny the forwarded envelope.
	ruCfg := ruConfig()
	ruCfg.Policy.Outbound.AllowTo = append(ruCfg.Policy.Outbound.AllowTo, "RU")
	ruCfg.Profile.TimingJitterMax = 0 // no jitter in tests
	ru := startNode(t, ruCfg)

	rsCfg := rsConfig()
	rsCfg.Profile.TimingJitterMax = 0 // no jitter in tests
	rsCfg.Network.Peers = map[string]config.PeerConfig{
		"RU": {Addr: ru.addr, NodeScope: "regional"},
	}

	fastPolicy := delivery.RetryPolicy{MaxAttempts: 1, BaseDelay: 0, Multiplier: 1, Cap: time.Millisecond}
	rs := startNodeFwd(t, rsCfg, fastPolicy, grpcSend)

	ctx := context.Background()
	resp, err := rs.client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "corridor-fwd-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err != nil {
		t.Fatalf("RS SendEnvelope: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW, got %q (%s)", resp.Decision, resp.Reason)
	}

	// RU must have received and processed the forwarded envelope.
	if entries := ru.auditLog.Entries(); len(entries) != 1 {
		t.Fatalf("expected 1 entry in RU audit log after forwarding, got %d", len(entries))
	}

	// RS response must carry the peer's audit root hash.
	if resp.PeerAuditRootHash == "" {
		t.Fatal("expected non-empty PeerAuditRootHash in RS response")
	}
}

// TestDLQAfterExhaustedRetries verifies that forwarding failures exhaust retries
// and write entries to the DLQ while the local node still returns ALLOW.
func TestDLQAfterExhaustedRetries(t *testing.T) {
	rsCfg := rsConfig()
	rsCfg.Profile.TimingJitterMax = 0
	rsCfg.Network.Peers = map[string]config.PeerConfig{
		"RU": {Addr: "localhost:1", NodeScope: "regional"},
	}

	// Immediately-failing send; no network I/O needed.
	alwaysFail := func(_ context.Context, _ string, _ core.Envelope) (string, error) {
		return "", fmt.Errorf("unreachable")
	}
	fastPolicy := delivery.RetryPolicy{MaxAttempts: 1, BaseDelay: 0, Multiplier: 1, Cap: time.Millisecond}
	rs := startNodeFwd(t, rsCfg, fastPolicy, alwaysFail)

	ctx := context.Background()
	const n = 3
	for i := 0; i < n; i++ {
		resp, err := rs.client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
			Envelope: grpctransport.Envelope{
				IdempotencyKey:  fmt.Sprintf("dlq-%03d", i),
				SenderRegion:    "RS",
				RecipientRegion: "RU",
			},
		})
		if err != nil {
			t.Fatalf("send dlq-%03d: unexpected gRPC error: %v", i, err)
		}
		// Local policy still ALLOWs; forwarding failure does not change the local decision.
		if resp.Decision != "ALLOW" {
			t.Fatalf("send dlq-%03d: expected ALLOW (local policy), got %q", i, resp.Decision)
		}
	}

	if got := rs.dlq.Size(); got < n {
		t.Fatalf("expected DLQ to have ≥%d entries after %d failed forwards, got %d", n, n, got)
	}
}
