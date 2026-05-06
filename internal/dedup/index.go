package dedup

import (
	"sync"
	"time"
)

// Index is a thread-safe in-memory store tracking idempotency keys within a TTL window.
type Index struct {
	mu      sync.Mutex
	entries map[string]time.Time
	ttl     time.Duration
}

func New(ttl time.Duration) *Index {
	return &Index{
		entries: make(map[string]time.Time),
		ttl:     ttl,
	}
}

// SeenOrAdd returns true if key was already registered within its TTL window.
// If not seen (or expired), it registers the key and returns false.
// The check and registration are atomic.
func (idx *Index) SeenOrAdd(key string) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	now := time.Now()
	if exp, ok := idx.entries[key]; ok && now.Before(exp) {
		return true
	}
	idx.entries[key] = now.Add(idx.ttl)
	return false
}

// Purge removes entries whose TTL has expired. Call periodically to bound memory.
func (idx *Index) Purge() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	now := time.Now()
	for key, exp := range idx.entries {
		if now.After(exp) {
			delete(idx.entries, key)
		}
	}
}
