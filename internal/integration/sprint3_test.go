package integration

// Sprint 3 integration tests.
// Each test wires gateway components directly (no app.Run) to exercise
// the full stack with Sprint 3 features: Ed25519 signing, trust tier
// audit, CRL revocation, and dummy traffic.

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/policy"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// startNodeWithCRL starts a node with a caller-supplied CRL store so tests
// can inject revocation entries before sending envelopes.
func startNodeWithCRL(t *testing.T, cfg config.Config, crlStore *crl.Store) node {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, crlStore)
	if err != nil {
		t.Fatalf("startNodeWithCRL %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	srv, err := grpctransport.NewServer(":0", grpctransport.NewAdapter(gw), nil)
	if err != nil {
		t.Fatalf("startNodeWithCRL %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("startNodeWithCRL: parse addr: %v", err)
	}
	dialAddr := fmt.Sprintf("localhost:%s", port)

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, nil)
	if err != nil {
		t.Fatalf("startNodeWithCRL %s: dial: %v", cfg.Node.NodeID, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return node{client: client, auditLog: auditLog, addr: dialAddr}
}

// startNodeWithVerify starts a node that enforces Ed25519 signature verification
// on every inbound envelope using the provided public key.
func startNodeWithVerify(t *testing.T, cfg config.Config, verifyKey []byte) (node, *audit.Log) {
	t.Helper()

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("startNodeWithVerify %s: policy engine: %v", cfg.Node.NodeID, err)
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	adapter := grpctransport.NewAdapterWithVerify(gw, verifyKey)
	srv, err := grpctransport.NewServer(":0", adapter, nil)
	if err != nil {
		t.Fatalf("startNodeWithVerify %s: grpc server: %v", cfg.Node.NodeID, err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("startNodeWithVerify: parse addr: %v", err)
	}
	dialAddr := fmt.Sprintf("localhost:%s", port)

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, nil)
	if err != nil {
		t.Fatalf("startNodeWithVerify %s: dial: %v", cfg.Node.NodeID, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return node{client: client, auditLog: auditLog, addr: dialAddr}, auditLog
}

// TestSigning_ValidSignatureAllowed verifies that an envelope with a valid
// Ed25519 signature passes verification and is processed normally.
func TestSigning_ValidSignatureAllowed(t *testing.T) {
	priv, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cfg := rsConfig()
	n, _ := startNodeWithVerify(t, cfg, pub)

	env := core.Envelope{
		IdempotencyKey:  "sign-valid-001",
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		Timestamp:       time.Now().UnixMilli(),
	}
	sig := identity.Sign(priv, env)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  env.IdempotencyKey,
			SenderRegion:    env.SenderRegion,
			RecipientRegion: env.RecipientRegion,
			Timestamp:       env.Timestamp,
			Signature:       sig,
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW with valid signature, got %q (%s)", resp.Decision, resp.Reason)
	}
}

// TestSigning_TamperedPayloadRejected verifies that an envelope whose payload
// has been modified after signing is rejected with INVALID_SIGNATURE.
func TestSigning_TamperedPayloadRejected(t *testing.T) {
	priv, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cfg := rsConfig()
	n, _ := startNodeWithVerify(t, cfg, pub)

	env := core.Envelope{
		IdempotencyKey:  "sign-tamper-001",
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		Payload:         []byte("original"),
		Timestamp:       time.Now().UnixMilli(),
	}
	sig := identity.Sign(priv, env)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  env.IdempotencyKey,
			SenderRegion:    env.SenderRegion,
			RecipientRegion: env.RecipientRegion,
			Payload:         []byte("tampered"), // payload changed after signing
			Timestamp:       env.Timestamp,
			Signature:       sig,
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "DENY" {
		t.Fatalf("expected DENY for tampered payload, got %q", resp.Decision)
	}
	if resp.Reason != "INVALID_SIGNATURE" {
		t.Fatalf("expected reason INVALID_SIGNATURE, got %q", resp.Reason)
	}
}

// TestSigning_MissingSignatureRejected verifies that an envelope with no
// signature is rejected when the node enforces verification.
func TestSigning_MissingSignatureRejected(t *testing.T) {
	_, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cfg := rsConfig()
	n, _ := startNodeWithVerify(t, cfg, pub)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "sign-missing-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "DENY" {
		t.Fatalf("expected DENY for missing signature, got %q", resp.Decision)
	}
	if resp.Reason != "INVALID_SIGNATURE" {
		t.Fatalf("expected reason INVALID_SIGNATURE, got %q", resp.Reason)
	}
}

// TestTrustTier_BelowMinimum_AuditEntry verifies that a T0 envelope sent to a
// node with min_trust_tier=1 produces a DENY audit entry with the structured
// reason constant and the sender's trust tier value.
func TestTrustTier_BelowMinimum_AuditEntry(t *testing.T) {
	cfg := ruConfig()
	cfg.Policy.Inbound.MinTrustTier = 1 // require at least T1

	n := startNode(t, cfg)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "tier-deny-001",
			SenderRegion:    "RS",
			RecipientRegion: "RS",
			TrustTier:       0, // T0 — below minimum
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "DENY" {
		t.Fatalf("expected DENY, got %q (%s)", resp.Decision, resp.Reason)
	}
	if resp.Reason != policy.ReasonTrustTierBelowMinimum {
		t.Fatalf("expected reason %q, got %q", policy.ReasonTrustTierBelowMinimum, resp.Reason)
	}

	entries := n.auditLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Decision != audit.DecisionDeny {
		t.Fatalf("audit entry decision: expected DENY, got %q", e.Decision)
	}
	if e.Reason != policy.ReasonTrustTierBelowMinimum {
		t.Fatalf("audit entry reason: expected %q, got %q", policy.ReasonTrustTierBelowMinimum, e.Reason)
	}
	if e.TrustTier != 0 {
		t.Fatalf("audit entry trust_tier: expected 0, got %d", e.TrustTier)
	}
}

// TestTrustTier_AtMinimum_Allowed verifies that an envelope meeting the minimum
// trust tier is allowed through.
func TestTrustTier_AtMinimum_Allowed(t *testing.T) {
	cfg := ruConfig()
	cfg.Policy.Inbound.MinTrustTier = 1

	n := startNode(t, cfg)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "tier-allow-001",
			SenderRegion:    "RS",
			RecipientRegion: "RS",
			TrustTier:       1, // exactly at minimum
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW at min tier, got %q (%s)", resp.Decision, resp.Reason)
	}
}

// TestCRL_RevokedNodeDenied verifies that an envelope from a node with ≥2
// CRL signatures is denied with reason NODE_REVOKED.
func TestCRL_RevokedNodeDenied(t *testing.T) {
	store := crl.New()
	store.Revoke("rs-node-01", "compromised", []byte("sig-alpha"))
	store.Revoke("rs-node-01", "compromised", []byte("sig-beta"))

	if !store.IsRevoked("rs-node-01") {
		t.Fatal("precondition: rs-node-01 must be revoked before test")
	}

	cfg := ruConfig()
	n := startNodeWithCRL(t, cfg, store)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "crl-deny-001",
			SenderNodeID:    "rs-node-01", // the revoked node
			SenderRegion:    "RS",
			RecipientRegion: "RS",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "DENY" {
		t.Fatalf("expected DENY for revoked node, got %q (%s)", resp.Decision, resp.Reason)
	}
	if resp.Reason != "NODE_REVOKED" {
		t.Fatalf("expected reason NODE_REVOKED, got %q", resp.Reason)
	}
}

// TestCRL_SingleSigNotRevoked verifies that a single-signature CRL entry does
// not revoke the node — quorum requires ≥2 signatures.
func TestCRL_SingleSigNotRevoked(t *testing.T) {
	store := crl.New()
	store.Revoke("rs-node-01", "suspected", []byte("only-one-sig"))

	cfg := ruConfig()
	n := startNodeWithCRL(t, cfg, store)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "crl-onesig-001",
			SenderNodeID:    "rs-node-01",
			SenderRegion:    "RS",
			RecipientRegion: "RS",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision == "DENY" && resp.Reason == "NODE_REVOKED" {
		t.Fatal("single-sig CRL entry must not revoke node")
	}
}

// TestCRL_Merge_PropagatesRevocation verifies that CRL entries received via
// Merge (gossip) become effective as expected.
func TestCRL_Merge_PropagatesRevocation(t *testing.T) {
	// Source store has two signatures — simulates a peer that has collected quorum.
	source := crl.New()
	source.Revoke("by-node-01", "malicious", []byte("peer-sig-1"))
	source.Revoke("by-node-01", "malicious", []byte("peer-sig-2"))

	// Local store receives gossip.
	local := crl.New()
	local.Merge(source.Entries())

	cfg := ruConfig()
	n := startNodeWithCRL(t, cfg, local)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "crl-gossip-001",
			SenderNodeID:    "by-node-01",
			SenderRegion:    "BY",
			RecipientRegion: "RS",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != "DENY" || resp.Reason != "NODE_REVOKED" {
		t.Fatalf("expected NODE_REVOKED after gossip merge, got %q (%s)", resp.Decision, resp.Reason)
	}
}

// TestDummyTraffic_AuditEntry verifies that an envelope with IsDummy=true is
// accepted and logged as ALLOW/DUMMY without reaching policy evaluation.
func TestDummyTraffic_AuditEntry(t *testing.T) {
	// Use a config that would deny regular envelopes to this destination,
	// confirming that IsDummy bypasses policy evaluation entirely.
	cfg := ruConfig()
	cfg.Policy.Inbound.MinTrustTier = 3 // would deny T0 envelopes

	n := startNode(t, cfg)

	resp, err := n.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey: "dummy-001",
			SenderRegion:   "RU",
			// IsDummy skips policy — MinTrustTier=3 would otherwise deny this
			TrustTier: 0,
			IsDummy:   true,
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope dummy: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW for dummy envelope, got %q (%s)", resp.Decision, resp.Reason)
	}

	entries := n.auditLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry for dummy, got %d", len(entries))
	}
	if entries[0].Decision != audit.DecisionDummy {
		t.Fatalf("expected ALLOW/DUMMY audit decision, got %q", entries[0].Decision)
	}
}

// TestDummyTraffic_NotForwarded verifies that a dummy envelope is not forwarded
// to a peer node — it only produces a local audit entry.
func TestDummyTraffic_NotForwarded(t *testing.T) {
	ruCfg := ruConfig()
	ruCfg.Policy.Outbound.AllowTo = append(ruCfg.Policy.Outbound.AllowTo, "RU")
	ru := startNode(t, ruCfg)

	rsCfg := rsConfig()
	rsCfg.Profile.TimingJitterMax = 0
	rsCfg.Network.Peers = map[string]config.PeerConfig{
		"RU": {Addr: ru.addr, NodeScope: "regional"},
	}

	fastPolicy := delivery.RetryPolicy{MaxAttempts: 1, BaseDelay: 0, Multiplier: 1, Cap: time.Millisecond}
	rs := startNodeFwd(t, rsCfg, fastPolicy, grpcSend)

	_, err := rs.client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey: "dummy-fwd-001",
			SenderRegion:   "RS",
			IsDummy:        true,
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope dummy: %v", err)
	}

	// RU must not have received the dummy envelope.
	if got := len(ru.auditLog.Entries()); got != 0 {
		t.Fatalf("dummy envelope must not be forwarded to peer; RU audit has %d entries", got)
	}

	// RS must have one ALLOW/DUMMY audit entry.
	entries := rs.auditLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry on RS for dummy, got %d", len(entries))
	}
	if entries[0].Decision != audit.DecisionDummy {
		t.Fatalf("expected ALLOW/DUMMY on RS, got %q", entries[0].Decision)
	}
}
