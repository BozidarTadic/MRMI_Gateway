package core

import (
	"context"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

// captureForwarder records the last envelope passed to Forward.
type captureForwarder struct {
	env Envelope
}

func (f *captureForwarder) Forward(_ context.Context, env Envelope) (string, error) {
	f.env = env
	return "mock:7777", nil
}

func newTestGateway(t *testing.T, profileName string) (*Gateway, *captureForwarder) {
	t.Helper()
	cfg := config.DefaultConfigForProfile(profileName)
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	fwd := &captureForwarder{}
	gw := NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), fwd)
	return gw, fwd
}

func sendAllow(t *testing.T, gw *Gateway, payload []byte) {
	t.Helper()
	_, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "test-key",
			SenderRegion:    "RS",
			RecipientRegion: "RU", // in default AllowTo list
			Payload:         payload,
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
}

// ── Padding tests ─────────────────────────────────────────────────────────────

func TestPadding_BalancedProfile(t *testing.T) {
	gw, fwd := newTestGateway(t, "balanced") // bucket = 4096
	payload := make([]byte, 1000)

	sendAllow(t, gw, payload)

	if fwd.env.PaddedTo != 4096 {
		t.Errorf("expected PaddedTo=4096, got %d", fwd.env.PaddedTo)
	}
	if len(fwd.env.Payload) != 4096 {
		t.Errorf("expected len(Payload)=4096, got %d", len(fwd.env.Payload))
	}
	// Original content preserved at the front
	for i, b := range payload {
		if fwd.env.Payload[i] != b {
			t.Fatalf("payload byte %d corrupted", i)
		}
	}
}

func TestPadding_StrictProfile(t *testing.T) {
	gw, fwd := newTestGateway(t, "strict") // bucket = 16384
	payload := make([]byte, 1000)

	sendAllow(t, gw, payload)

	if fwd.env.PaddedTo != 16384 {
		t.Errorf("expected PaddedTo=16384, got %d", fwd.env.PaddedTo)
	}
	if len(fwd.env.Payload) != 16384 {
		t.Errorf("expected len(Payload)=16384, got %d", len(fwd.env.Payload))
	}
}

func TestPadding_PerformanceProfile(t *testing.T) {
	gw, fwd := newTestGateway(t, "performance") // bucket = 0 (disabled)
	payload := []byte("hello")

	sendAllow(t, gw, payload)

	if fwd.env.PaddedTo != 0 {
		t.Errorf("expected PaddedTo=0 for performance profile, got %d", fwd.env.PaddedTo)
	}
	if len(fwd.env.Payload) != len(payload) {
		t.Errorf("expected payload unchanged, got len=%d", len(fwd.env.Payload))
	}
}

func TestPadding_AlreadyAtBucketBoundary(t *testing.T) {
	gw, fwd := newTestGateway(t, "balanced") // bucket = 4096
	payload := make([]byte, 4096)

	sendAllow(t, gw, payload)

	if fwd.env.PaddedTo != 4096 {
		t.Errorf("expected PaddedTo=4096, got %d", fwd.env.PaddedTo)
	}
	if len(fwd.env.Payload) != 4096 {
		t.Errorf("expected len(Payload)=4096, got %d", len(fwd.env.Payload))
	}
}

func TestPadding_ExceedsBucketBoundary(t *testing.T) {
	gw, fwd := newTestGateway(t, "balanced") // bucket = 4096
	payload := make([]byte, 4097)

	sendAllow(t, gw, payload)

	if fwd.env.PaddedTo != 8192 {
		t.Errorf("expected PaddedTo=8192, got %d", fwd.env.PaddedTo)
	}
	if len(fwd.env.Payload) != 8192 {
		t.Errorf("expected len(Payload)=8192, got %d", len(fwd.env.Payload))
	}
}

func TestPadding_EmptyPayloadSkipped(t *testing.T) {
	gw, fwd := newTestGateway(t, "balanced")

	sendAllow(t, gw, nil)

	if fwd.env.PaddedTo != 0 {
		t.Errorf("expected PaddedTo=0 for empty payload, got %d", fwd.env.PaddedTo)
	}
}

// ── Jitter tests ──────────────────────────────────────────────────────────────

func TestJitter_ZeroMaxSkipped(t *testing.T) {
	// performance profile has TimingJitterMax = 0; forward must complete immediately
	gw, fwd := newTestGateway(t, "performance")

	start := time.Now()
	sendAllow(t, gw, []byte("x"))
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("expected no jitter delay, took %v", elapsed)
	}
	if fwd.env.IdempotencyKey == "" {
		t.Error("expected forwarder to be called")
	}
}

func TestJitter_CancelledContextSkipsForward(t *testing.T) {
	gw, fwd := newTestGateway(t, "strict") // jitter up to 500ms

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := gw.SendEnvelope(ctx, SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "cancel-key",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("hello"),
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope returned error: %v", err)
	}
	// Forwarder must not have been called — cancelled context aborts jitter
	if fwd.env.IdempotencyKey != "" {
		t.Error("forwarder must not be called when context is already cancelled")
	}
}
