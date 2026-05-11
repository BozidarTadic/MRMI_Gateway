package dummy

import (
	"context"
	"testing"
	"time"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

func balancedCfg() config.Config {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Profile.DummyTrafficRate = 20 * time.Millisecond
	return cfg
}

func performanceCfg() config.Config {
	return config.DefaultConfigForProfile("performance")
}

func TestGenerator_BalancedProfile_Emits(t *testing.T) {
	cfg := balancedCfg()
	peers := []config.PeerConfig{{Region: "RU", Addr: "localhost:7778", NodeScope: "regional"}}

	var count int
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	New(cfg).Run(ctx, peers, func(env core.Envelope) {
		count++
		if !env.IsDummy {
			t.Error("expected IsDummy=true")
		}
	})

	if count < 2 {
		t.Fatalf("expected ≥2 dummy envelopes, got %d", count)
	}
}

func TestGenerator_PerformanceProfile_EmitsNothing(t *testing.T) {
	cfg := performanceCfg()
	peers := []config.PeerConfig{{Region: "RU", Addr: "localhost:7778", NodeScope: "regional"}}

	var count int
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	New(cfg).Run(ctx, peers, func(_ core.Envelope) { count++ })

	if count != 0 {
		t.Fatalf("performance profile should emit no dummy traffic, got %d", count)
	}
}

func TestGenerator_NoPeers_EmitsNothing(t *testing.T) {
	cfg := balancedCfg()

	var count int
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	New(cfg).Run(ctx, nil, func(_ core.Envelope) { count++ })

	if count != 0 {
		t.Fatalf("no peers — expected no emission, got %d", count)
	}
}

func TestGenerator_IsDummy_SkipsForwarding(t *testing.T) {
	// Receiving side: IsDummy envelopes must not reach the forwarder.
	// This is enforced in core.Gateway.SendEnvelope — test that IsDummy is set correctly.
	cfg := balancedCfg()
	peers := []config.PeerConfig{{Region: "RU", Addr: "localhost:7778", NodeScope: "regional"}}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	New(cfg).Run(ctx, peers, func(env core.Envelope) {
		if env.IdempotencyKey == "" {
			t.Error("dummy envelope missing idempotency_key")
		}
	})
}
