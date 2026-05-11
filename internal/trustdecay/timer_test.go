package trustdecay

import (
	"context"
	"testing"
	"time"
)

func TestEffectiveTier_FreshValidation_Preserved(t *testing.T) {
	tr := New(time.Hour)
	tr.RecordValidation("peer-ru")
	if got := tr.EffectiveTier("peer-ru", 2); got != 2 {
		t.Fatalf("expected tier 2, got %d", got)
	}
}

func TestEffectiveTier_StaleValidation_Decays(t *testing.T) {
	tr := New(time.Millisecond) // tiny window so it's always stale
	tr.RecordValidation("peer-ru")
	time.Sleep(5 * time.Millisecond)
	if got := tr.EffectiveTier("peer-ru", 2); got != 1 {
		t.Fatalf("expected decayed tier 1, got %d", got)
	}
}

func TestEffectiveTier_UnknownPeer_Decays(t *testing.T) {
	tr := New(time.Hour)
	if got := tr.EffectiveTier("unknown", 3); got != 2 {
		t.Fatalf("expected decayed tier 2 for unknown peer, got %d", got)
	}
}

func TestEffectiveTier_T0_FloorZero(t *testing.T) {
	tr := New(time.Millisecond)
	tr.RecordValidation("peer-ru")
	time.Sleep(5 * time.Millisecond)
	if got := tr.EffectiveTier("peer-ru", 0); got != 0 {
		t.Fatalf("expected floor 0, got %d", got)
	}
}

func TestRun_StopsOnCancel(t *testing.T) {
	tr := New(time.Hour)
	tr.checkInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tr.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}
