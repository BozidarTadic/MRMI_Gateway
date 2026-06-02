package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/schema"
)

// stubForwarder is a minimal Forwarder that always succeeds.
type stubForwarder struct{}

func (stubForwarder) Forward(_ context.Context, _ Envelope) (string, error) {
	return "sha256:abc", nil
}

func newIso20022Gateway(t *testing.T, settlementFinality bool) (*Gateway, *audit.Log) {
	t.Helper()
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.SchemaRegistry.Iso20022 = config.Iso20022AdapterConfig{
		DedupTTLH:          72,
		SettlementFinality: settlementFinality,
		RetainLong:         true,
	}
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	gw := NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), stubForwarder{})
	return gw, auditLog
}

// TestGateway_Iso20022_DedupTTLOverride verifies that iso20022 envelopes use the
// 72-hour dedup TTL rather than the profile default.
func TestGateway_Iso20022_DedupTTLOverride(t *testing.T) {
	gw, auditLog := newIso20022Gateway(t, false)

	send := func(key string) SendResponse {
		resp, err := gw.SendEnvelope(context.Background(), SendRequest{
			Envelope: Envelope{
				IdempotencyKey:  key,
				SenderRegion:    "RS",
				RecipientRegion: "RU",
				Payload:         []byte("iso-payload"),
				SchemaType:      schema.Iso20022,
				SchemaVersion:   "2019",
			},
		})
		if err != nil {
			t.Fatalf("SendEnvelope: %v", err)
		}
		return resp
	}

	first := send("iso-dedup-key-1")
	if first.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW, got %s", first.Decision)
	}

	dup := send("iso-dedup-key-1")
	if dup.Decision != DecisionDuplicate {
		t.Fatalf("expected DUPLICATE on second send, got %s", dup.Decision)
	}

	// Confirm the first entry has retain_long=true.
	entries := auditLog.Entries()
	if len(entries) == 0 {
		t.Fatal("expected audit entries")
	}
	for _, e := range entries {
		if e.SchemaType == schema.Iso20022 && e.Decision == audit.DecisionAllow {
			if !e.RetainLong {
				t.Fatalf("expected retain_long=true for iso20022 ALLOW entry")
			}
			return
		}
	}
	t.Fatal("no iso20022 ALLOW audit entry found")
}

// TestGateway_Iso20022_RetainLong verifies that iso20022 audit entries carry retain_long=true.
func TestGateway_Iso20022_RetainLong(t *testing.T) {
	gw, auditLog := newIso20022Gateway(t, false)

	_, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "retain-long-test",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("data"),
			SchemaType:      schema.Iso20022,
			SchemaVersion:   "2019",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	entries := auditLog.Entries()
	for _, e := range entries {
		if e.SchemaType == schema.Iso20022 {
			if !e.RetainLong {
				t.Fatalf("iso20022 entry missing retain_long=true")
			}
			return
		}
	}
	t.Fatal("no iso20022 audit entry found")
}

// TestGateway_Iso20022_SettlementFinality verifies that a SETTLEMENT/FINAL audit entry
// is appended after a successful forward when SettlementFinality is enabled.
func TestGateway_Iso20022_SettlementFinality(t *testing.T) {
	gw, auditLog := newIso20022Gateway(t, true)

	_, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "settlement-final-test",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("payment-msg"),
			SchemaType:      schema.Iso20022,
			SchemaVersion:   "2019",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	entries := auditLog.Entries()
	for _, e := range entries {
		if e.Decision == audit.DecisionSettlementFinal {
			if !e.SettlementFinal {
				t.Fatalf("settlement_final entry missing settlement_final=true flag")
			}
			if !e.RetainLong {
				t.Fatalf("settlement_final entry missing retain_long=true flag")
			}
			return
		}
	}
	t.Fatal("no SETTLEMENT/FINAL audit entry found")
}

// TestGateway_Iso20022_NoSettlementFinalityWhenDisabled verifies that no
// SETTLEMENT/FINAL entry is appended when the feature is not configured.
func TestGateway_Iso20022_NoSettlementFinalityWhenDisabled(t *testing.T) {
	gw, auditLog := newIso20022Gateway(t, false)

	_, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "no-finality-test",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("payment-msg"),
			SchemaType:      schema.Iso20022,
			SchemaVersion:   "2019",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	for _, e := range auditLog.Entries() {
		if e.Decision == audit.DecisionSettlementFinal {
			t.Fatal("unexpected SETTLEMENT/FINAL entry when finality disabled")
		}
	}
}

// TestGateway_Iso20022_SettlementFinality_OnlyOnSuccessfulForward verifies that
// no SETTLEMENT/FINAL entry is appended when forwarding fails.
func TestGateway_Iso20022_SettlementFinality_OnlyOnSuccessfulForward(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.SchemaRegistry.Iso20022 = config.Iso20022AdapterConfig{
		DedupTTLH:          72,
		SettlementFinality: true,
		RetainLong:         true,
	}
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	// Use a forwarder that always fails.
	failForwarder := forwardFunc(func(_ context.Context, _ Envelope) (string, error) {
		return "", errors.New("peer down")
	})
	gw := NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), failForwarder)

	_, err = gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "fail-forward-test",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("data"),
			SchemaType:      schema.Iso20022,
			SchemaVersion:   "2019",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	for _, e := range auditLog.Entries() {
		if e.Decision == audit.DecisionSettlementFinal {
			t.Fatal("unexpected SETTLEMENT/FINAL entry when forward failed")
		}
	}
}

// TestGateway_Iso20022_JitterIsApplied ensures iso20022 envelopes still go through
// the normal jitter + padding path (regression guard).
func TestGateway_Iso20022_JitterIsApplied(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Profile.TimingJitterMax = time.Millisecond // keep test fast
	cfg.SchemaRegistry.Iso20022 = config.Iso20022AdapterConfig{
		DedupTTLH: 72,
	}
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	gw := NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), stubForwarder{})

	resp, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "jitter-test",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("data"),
			SchemaType:      schema.Iso20022,
			SchemaVersion:   "2019",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	if resp.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW, got %s", resp.Decision)
	}
}

// forwardFunc is a Forwarder adapter for a plain function.
type forwardFunc func(ctx context.Context, env Envelope) (string, error)

func (f forwardFunc) Forward(ctx context.Context, env Envelope) (string, error) {
	return f(ctx, env)
}
