package dedup

import (
	"sync"
	"testing"
	"time"
)

func TestSeenOrAdd_NewKey(t *testing.T) {
	idx := New(time.Hour)
	if idx.SeenOrAdd("key-1") {
		t.Fatal("expected false for unseen key")
	}
}

func TestSeenOrAdd_DuplicateWithinTTL(t *testing.T) {
	idx := New(time.Hour)
	idx.SeenOrAdd("key-1")
	if !idx.SeenOrAdd("key-1") {
		t.Fatal("expected true for duplicate key within TTL")
	}
}

func TestSeenOrAdd_ExpiredKey(t *testing.T) {
	idx := New(10 * time.Millisecond)
	idx.SeenOrAdd("key-1")
	time.Sleep(20 * time.Millisecond)
	if idx.SeenOrAdd("key-1") {
		t.Fatal("expected false for expired key")
	}
}

func TestSeenOrAdd_IndependentKeys(t *testing.T) {
	idx := New(time.Hour)
	idx.SeenOrAdd("key-a")
	if idx.SeenOrAdd("key-b") {
		t.Fatal("key-b should not be seen after only registering key-a")
	}
}

func TestPurge_RemovesExpired(t *testing.T) {
	idx := New(10 * time.Millisecond)
	idx.SeenOrAdd("key-1")
	idx.SeenOrAdd("key-2")
	time.Sleep(20 * time.Millisecond)
	idx.Purge()

	idx.mu.Lock()
	n := len(idx.entries)
	idx.mu.Unlock()

	if n != 0 {
		t.Fatalf("expected 0 entries after purge, got %d", n)
	}
}

func TestPurge_KeepsLiveEntries(t *testing.T) {
	idx := New(time.Hour)
	idx.SeenOrAdd("key-1")
	idx.Purge()

	if !idx.SeenOrAdd("key-1") {
		t.Fatal("key-1 should still be present after purge of non-expired entries")
	}
}

func TestSeenOrAdd_Concurrent(t *testing.T) {
	idx := New(time.Hour)
	const goroutines = 50
	results := make([]bool, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i] = idx.SeenOrAdd("shared-key")
		}(i)
	}
	wg.Wait()

	falseCount := 0
	for _, seen := range results {
		if !seen {
			falseCount++
		}
	}
	if falseCount != 1 {
		t.Fatalf("exactly one goroutine should have registered the key, got %d", falseCount)
	}
}
