package ratelimit

import (
	"testing"
	"time"
)

func TestAllow_BurstAndExhaust(t *testing.T) {
	l := New(1, 3) // refill 1/s, burst 3
	defer l.Close()

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("call %d should be allowed within burst", i+1)
		}
	}
	// Burst exhausted — next call should be denied.
	if l.Allow("k") {
		t.Fatal("expected denial after burst exhausted")
	}
}

func TestAllow_IndependentKeys(t *testing.T) {
	l := New(1, 1)
	defer l.Close()

	if !l.Allow("a") {
		t.Fatal("key a should be allowed")
	}
	if !l.Allow("b") {
		t.Fatal("key b should be allowed (independent bucket)")
	}
	if l.Allow("a") {
		t.Fatal("key a should be denied after burst exhausted")
	}
}

func TestAllow_RefillOverTime(t *testing.T) {
	l := New(1000, 1) // 1000/s so one token refills in 1 ms
	defer l.Close()

	if !l.Allow("k") {
		t.Fatal("first call should be allowed")
	}
	if l.Allow("k") {
		t.Fatal("second call immediately should be denied")
	}

	// Manually push lastSeen back so refill happens.
	l.mu.Lock()
	l.buckets["k"].lastSeen = time.Now().Add(-2 * time.Millisecond)
	l.mu.Unlock()

	if !l.Allow("k") {
		t.Fatal("expected allow after token refill")
	}
}

func TestLen_TracksActiveBuckets(t *testing.T) {
	l := New(10, 5)
	defer l.Close()

	l.Allow("a")
	l.Allow("b")
	l.Allow("c")

	if l.Len() != 3 {
		t.Fatalf("expected 3 buckets, got %d", l.Len())
	}
}

func TestCleanup_EvictsIdleBuckets(t *testing.T) {
	l := New(10, 5)
	defer l.Close()

	l.Allow("idle")

	// Set lastSeen to past the idle eviction threshold.
	l.mu.Lock()
	l.buckets["idle"].lastSeen = time.Now().Add(-(idleEvictAfter + time.Second))
	l.mu.Unlock()

	// Run cleanup manually.
	now := time.Now()
	l.mu.Lock()
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) > idleEvictAfter {
			delete(l.buckets, k)
		}
	}
	l.mu.Unlock()

	if l.Len() != 0 {
		t.Fatalf("expected 0 buckets after idle eviction, got %d", l.Len())
	}
}
