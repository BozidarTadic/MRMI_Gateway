package transit

import (
	"testing"
	"time"

	"MRMI_Gateway/internal/core"
)

func env(key string) core.Envelope {
	return core.Envelope{IdempotencyKey: key}
}

func TestPut_And_Pending(t *testing.T) {
	c := New(30 * time.Second)
	c.Put(env("a"), "peer:7777")
	c.Put(env("b"), "peer:7778")
	if c.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", c.Len())
	}
	pending := c.Pending()
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}
}

func TestDrain_ReturnsOnlyExpired(t *testing.T) {
	c := New(30 * time.Second)
	c.Put(env("live"), "peer:7777")

	// Manually inject an already-expired entry.
	c.mu.Lock()
	c.entries["expired"] = Entry{
		Env:      env("expired"),
		PeerAddr: "peer:7778",
		Expires:  time.Now().Add(-time.Second),
	}
	c.mu.Unlock()

	drained := c.Drain()
	if len(drained) != 1 {
		t.Fatalf("expected 1 drained entry, got %d", len(drained))
	}
	if drained[0].Env.IdempotencyKey != "expired" {
		t.Fatalf("expected expired entry, got %q", drained[0].Env.IdempotencyKey)
	}
	// Live entry must still be in the cache.
	if c.Len() != 1 {
		t.Fatalf("expected 1 remaining entry after drain, got %d", c.Len())
	}
}

func TestDelete_RemovesEntry(t *testing.T) {
	c := New(30 * time.Second)
	c.Put(env("k"), "peer:7777")
	c.Delete("k")
	if c.Len() != 0 {
		t.Fatal("expected empty cache after delete")
	}
}

func TestTTL_CapAtMaxTTL(t *testing.T) {
	c := New(10 * time.Minute) // well above 60 s cap
	if c.ttl != maxTTL {
		t.Fatalf("expected TTL capped at %s, got %s", maxTTL, c.ttl)
	}
}

func TestTTL_ZeroDefaults(t *testing.T) {
	c := New(0)
	if c.ttl != 30*time.Second {
		t.Fatalf("expected 30 s default, got %s", c.ttl)
	}
}

func TestPut_EvictsOldestWhenFull(t *testing.T) {
	c := New(30 * time.Second)
	// Fill to capacity.
	for i := 0; i < maxEntries; i++ {
		c.Put(core.Envelope{IdempotencyKey: string(rune('a' + i%26)) + string(rune('0'+i/26))}, "peer:7777")
	}
	// One more should trigger eviction — size must stay at maxEntries.
	c.Put(env("overflow"), "peer:7777")
	if c.Len() != maxEntries {
		t.Fatalf("expected %d entries after overflow, got %d", maxEntries, c.Len())
	}
}
