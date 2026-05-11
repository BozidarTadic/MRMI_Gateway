// Package transit implements the in-memory relay buffer described in ADR §4.3.
// Envelopes that exhausted immediate forwarding retries are held here for a
// configurable TTL (≤ 60 s) before being promoted to the DLQ. Data is never
// persisted to disk, satisfying the "no data-at-rest outside origin region"
// constraint.
package transit

import (
	"sync"
	"time"

	"MRMI_Gateway/internal/core"
)

const (
	maxEntries = 1000
	maxTTL     = 60 * time.Second
)

// Entry is one envelope held in the transit buffer.
type Entry struct {
	Env      core.Envelope
	PeerAddr string
	Expires  time.Time
}

// Cache is a bounded in-memory store keyed by idempotency key.
// Safe for concurrent use.
type Cache struct {
	mu      sync.Mutex
	entries map[string]Entry
	ttl     time.Duration
}

// New returns a Cache with the given TTL, capped at maxTTL.
// A zero or negative TTL is set to 30 s.
func New(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}
	return &Cache{entries: make(map[string]Entry), ttl: ttl}
}

// Put inserts or replaces an entry. If the cache is at capacity the entry with
// the earliest expiry is evicted to make room.
func (c *Cache) Put(env core.Envelope, peerAddr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxEntries {
		c.evictOldest()
	}
	c.entries[env.IdempotencyKey] = Entry{
		Env:      env,
		PeerAddr: peerAddr,
		Expires:  time.Now().Add(c.ttl),
	}
}

// Pending returns all entries whose TTL has not yet expired.
func (c *Cache) Pending() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	out := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		if !now.After(e.Expires) {
			out = append(out, e)
		}
	}
	return out
}

// Drain returns and removes all entries whose TTL has expired.
func (c *Cache) Drain() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	var out []Entry
	for k, e := range c.entries {
		if now.After(e.Expires) {
			out = append(out, e)
			delete(c.entries, k)
		}
	}
	return out
}

// Delete removes a single entry by idempotency key.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len returns the current number of buffered entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestExp time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.Expires.Before(oldestExp) {
			oldestKey = k
			oldestExp = e.Expires
		}
	}
	delete(c.entries, oldestKey)
}
