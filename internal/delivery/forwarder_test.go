package delivery

import (
	"context"
	"errors"
	"testing"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

func peers(entries map[string]config.PeerConfig) config.Config {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Network.Peers = entries
	return cfg
}

func TestPeersFor_RegionalDirectMatch(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Addr: "ru:7777", NodeScope: "regional"},
		"BY": {Addr: "by:7777", NodeScope: "regional"},
	})
	f := NewForwarder(cfg, nil, nil)

	got := f.PeersFor("RU")
	if len(got) != 1 || got[0].Addr != "ru:7777" {
		t.Fatalf("expected [ru:7777], got %v", got)
	}
}

func TestPeersFor_AllianceFallback(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"eaeu-01": {Addr: "eaeu:7777", NodeScope: "alliance", Regions: []string{"BY", "KZ", "AM"}},
	})
	f := NewForwarder(cfg, nil, nil)

	got := f.PeersFor("KZ")
	if len(got) != 1 || got[0].Addr != "eaeu:7777" {
		t.Fatalf("expected [eaeu:7777], got %v", got)
	}
}

func TestPeersFor_GlobalFallback(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"global-01": {Addr: "global:7777", NodeScope: "global"},
	})
	f := NewForwarder(cfg, nil, nil)

	got := f.PeersFor("DE") // no regional or alliance peer for DE
	if len(got) != 1 || got[0].Addr != "global:7777" {
		t.Fatalf("expected [global:7777], got %v", got)
	}
}

func TestPeersFor_TierPreferenceOrder(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU":        {Addr: "ru:7777", NodeScope: "regional"},
		"eaeu-01":   {Addr: "eaeu:7777", NodeScope: "alliance", Regions: []string{"RU", "BY", "KZ"}},
		"global-01": {Addr: "global:7777", NodeScope: "global"},
	})
	f := NewForwarder(cfg, nil, nil)

	got := f.PeersFor("RU")
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(got), got)
	}
	// Regional must be first, global last
	if got[0].NodeScope != "regional" {
		t.Errorf("first candidate must be regional, got %q", got[0].NodeScope)
	}
	if got[len(got)-1].NodeScope != "global" {
		t.Errorf("last candidate must be global, got %q", got[len(got)-1].NodeScope)
	}
}

func TestPeersFor_AllowViaFiltersGlobal(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU":        {Addr: "ru:7777", NodeScope: "regional"},
		"global-01": {Addr: "global:7777", NodeScope: "global"},
	})
	cfg.Policy.Routing.AllowVia = []string{"regional", "alliance"}
	f := NewForwarder(cfg, nil, nil)

	got := f.PeersFor("RU")
	for _, p := range got {
		if p.NodeScope == "global" {
			t.Fatal("global peer must be filtered out by allow_via")
		}
	}
	if len(got) != 1 || got[0].Addr != "ru:7777" {
		t.Fatalf("expected only regional peer, got %v", got)
	}
}

func TestPeersFor_NoPeers(t *testing.T) {
	f := NewForwarder(config.DefaultConfigForProfile("balanced"), nil, nil)
	if got := f.PeersFor("RU"); len(got) != 0 {
		t.Fatalf("expected no peers, got %v", got)
	}
}

func TestForward_SuccessOnFirstPeer(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Addr: "ru:7777", NodeScope: "regional"},
	})

	var called string
	f := NewForwarder(cfg, nil, func(_ context.Context, addr string, _ core.Envelope) (string, error) {
		called = addr
		return "peer-root-hash", nil
	})

	peerRoot, err := f.Forward(context.Background(), core.Envelope{RecipientRegion: "RU"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if called != "ru:7777" {
		t.Fatalf("expected ru:7777 to be called, got %q", called)
	}
	if peerRoot != "peer-root-hash" {
		t.Fatalf("expected peer root hash, got %q", peerRoot)
	}
}

func singleAttempt() RetryPolicy {
	return RetryPolicy{MaxAttempts: 1, BaseDelay: 0, Multiplier: 1, Cap: 0}
}

func TestForward_FallsBackToNextTier(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU":        {Addr: "ru:7777", NodeScope: "regional"},
		"global-01": {Addr: "global:7777", NodeScope: "global"},
	})

	f := NewForwarder(cfg, nil, func(_ context.Context, addr string, _ core.Envelope) (string, error) {
		if addr == "ru:7777" {
			return "", errors.New("unreachable")
		}
		return "global-root-hash", nil
	})
	f.retryPolicy = singleAttempt()

	peerRoot, err := f.Forward(context.Background(), core.Envelope{RecipientRegion: "RU"})
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if peerRoot != "global-root-hash" {
		t.Fatalf("expected global fallback root hash, got %q", peerRoot)
	}
}

func TestForward_WritesToDLQOnExhaustion(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Addr: "ru:7777", NodeScope: "regional"},
	})

	dlq := NewDLQ()
	f := NewForwarder(cfg, dlq, func(_ context.Context, _ string, _ core.Envelope) (string, error) {
		return "", errors.New("always fails")
	})
	f.retryPolicy = singleAttempt()

	_, err := f.Forward(context.Background(), core.Envelope{RecipientRegion: "RU"})
	if err == nil {
		t.Fatal("expected error when all peers fail")
	}
	if dlq.Size() != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", dlq.Size())
	}
}

func TestForward_NoPeerReturnsError(t *testing.T) {
	f := NewForwarder(config.DefaultConfigForProfile("balanced"), NewDLQ(), nil)
	_, err := f.Forward(context.Background(), core.Envelope{RecipientRegion: "XX"})
	if err == nil {
		t.Fatal("expected error when no peer available")
	}
}
