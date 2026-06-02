package delivery

import (
	"context"
	"errors"
	"testing"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/schema"
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
	f := NewForwarder(cfg, nil, nil, nil)

	got := f.PeersFor("RU")
	if len(got) != 1 || got[0].Addr != "ru:7777" {
		t.Fatalf("expected [ru:7777], got %v", got)
	}
}

func TestPeersFor_AllianceFallback(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"eaeu-01": {Addr: "eaeu:7777", NodeScope: "alliance", Regions: []string{"BY", "KZ", "AM"}},
	})
	f := NewForwarder(cfg, nil, nil, nil)

	got := f.PeersFor("KZ")
	if len(got) != 1 || got[0].Addr != "eaeu:7777" {
		t.Fatalf("expected [eaeu:7777], got %v", got)
	}
}

func TestPeersFor_GlobalFallback(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"global-01": {Addr: "global:7777", NodeScope: "global"},
	})
	f := NewForwarder(cfg, nil, nil, nil)

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
	f := NewForwarder(cfg, nil, nil, nil)

	got := f.PeersFor("RU")
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(got), got)
	}
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
	f := NewForwarder(cfg, nil, nil, nil)

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
	f := NewForwarder(config.DefaultConfigForProfile("balanced"), nil, nil, nil)
	if got := f.PeersFor("RU"); len(got) != 0 {
		t.Fatalf("expected no peers, got %v", got)
	}
}

func TestForward_SuccessOnFirstPeer(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Addr: "ru:7777", NodeScope: "regional"},
	})

	var called string
	f := NewForwarder(cfg, nil, nil, func(_ context.Context, addr string, _ core.Envelope) (string, error) {
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

	f := NewForwarder(cfg, nil, nil, func(_ context.Context, addr string, _ core.Envelope) (string, error) {
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
	f := NewForwarder(cfg, dlq, nil, func(_ context.Context, _ string, _ core.Envelope) (string, error) {
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
	f := NewForwarder(config.DefaultConfigForProfile("balanced"), NewDLQ(), nil, nil)
	_, err := f.Forward(context.Background(), core.Envelope{RecipientRegion: "XX"})
	if err == nil {
		t.Fatal("expected error when no peer available")
	}
}

func TestForward_Iso20022_OutsideCutoffWindow_GoesToDLQ(t *testing.T) {
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Region: "RU", Addr: "ru:7777", NodeScope: "regional"},
	})
	// Window 08:00-09:00 UTC — use a time that is definitely outside (23:00).
	// We test the corridor RS-RU.
	cfg.SchemaRegistry.Iso20022.CutoffWindows = map[string]config.CutoffWindow{
		"RS-RU": {Open: "08:00", Close: "09:00", TZ: "UTC"},
	}

	dlq := NewDLQ()
	f := NewForwarder(cfg, dlq, nil, func(_ context.Context, _ string, _ core.Envelope) (string, error) {
		return "ok", nil
	})

	env := core.Envelope{
		IdempotencyKey:  "iso-cutoff-test",
		SchemaType:      schema.Iso20022,
		SenderRegion:    "RS",
		RecipientRegion: "RU",
	}

	// Override the corridor check for testing by using a window that is always closed
	// at any point in the day (open == close makes no valid window, but we rely on
	// the fact that 08:00-09:00 is a narrow window almost always outside).
	// A more deterministic approach: configure a past-only window by checking if
	// the forwarder correctly DLQs when IsInWindow returns false.
	// For a portable test we use the closed window trick: Open=Close means
	// nothing is inside. Actually with our logic open<close means [open,close) —
	// if open==close the window is empty. Let's just set 00:00-00:01.
	cfg.SchemaRegistry.Iso20022.CutoffWindows = map[string]config.CutoffWindow{
		"RS-RU": {Open: "00:00", Close: "00:01", TZ: "UTC"},
	}
	f = NewForwarder(cfg, dlq, nil, func(_ context.Context, _ string, _ core.Envelope) (string, error) {
		return "ok", nil
	})

	_, err := f.Forward(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when outside cutoff window")
	}

	if dlq.Size() == 0 {
		t.Fatal("expected DLQ entry when outside cutoff window")
	}
	entry := dlq.Entries()[0]
	if entry.Reason != "outside_cutoff_window" {
		t.Fatalf("expected reason %q, got %q", "outside_cutoff_window", entry.Reason)
	}
	if entry.NextOpenUnix == 0 {
		t.Fatal("expected NextOpenUnix to be set")
	}
}

func TestForward_Iso20022_InsideCutoffWindow_Proceeds(t *testing.T) {
	// Window covering the full day: 00:00-23:59 — always inside.
	cfg := peers(map[string]config.PeerConfig{
		"RU": {Region: "RU", Addr: "ru:7777", NodeScope: "regional"},
	})
	cfg.SchemaRegistry.Iso20022.CutoffWindows = map[string]config.CutoffWindow{
		"RS-RU": {Open: "00:00", Close: "23:59", TZ: "UTC"},
	}

	dlq := NewDLQ()
	f := NewForwarder(cfg, dlq, nil, func(_ context.Context, _ string, _ core.Envelope) (string, error) {
		return "peer-hash", nil
	})

	_, err := f.Forward(context.Background(), core.Envelope{
		IdempotencyKey:  "iso-inside-window",
		SchemaType:      schema.Iso20022,
		SenderRegion:    "RS",
		RecipientRegion: "RU",
	})
	if err != nil {
		t.Fatalf("expected success inside window, got: %v", err)
	}
	if dlq.Size() != 0 {
		t.Fatal("expected no DLQ entry when inside window")
	}
}

func TestReorderByHint_MovesMatchToFront(t *testing.T) {
	peers := []config.PeerConfig{
		{Region: "RU", Addr: "ru:7777", NodeScope: "regional"},
		{Region: "BANKDE22", Addr: "de:7777", NodeScope: "global"},
		{Region: "global-01", Addr: "global:7777", NodeScope: "global"},
	}

	got := reorderByHint(peers, "BANKDE22")
	if got[0].Region != "BANKDE22" {
		t.Fatalf("expected BANKDE22 first, got %q", got[0].Region)
	}
	if got[1].Region != "RU" {
		t.Fatalf("expected RU second, got %q", got[1].Region)
	}
}

func TestReorderByHint_NoMatch_Unchanged(t *testing.T) {
	peerList := []config.PeerConfig{
		{Region: "RU", Addr: "ru:7777", NodeScope: "regional"},
		{Region: "global-01", Addr: "global:7777", NodeScope: "global"},
	}
	got := reorderByHint(peerList, "NOTEXIST")
	if got[0].Region != "RU" {
		t.Fatalf("expected RU first (unchanged), got %q", got[0].Region)
	}
}
