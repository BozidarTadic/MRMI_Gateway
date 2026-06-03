package dedup

import (
	"hash/fnv"
	"sync"
	"time"
)

const numShards = 16

type shard struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

// Index is a thread-safe in-memory store tracking idempotency keys within a TTL window.
// Keys are distributed across 16 independent shards to reduce lock contention under concurrent load.
type Index struct {
	shards [numShards]shard
	ttl    time.Duration
}

func New(ttl time.Duration) *Index {
	idx := &Index{ttl: ttl}
	for i := range idx.shards {
		idx.shards[i].entries = make(map[string]time.Time)
	}
	return idx
}

func (idx *Index) shardFor(key string) *shard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &idx.shards[h.Sum32()&(numShards-1)]
}

// SeenOrAdd returns true if key was already registered within its TTL window.
// If not seen (or expired), it registers the key and returns false.
// The check and registration are atomic within the shard.
func (idx *Index) SeenOrAdd(key string) bool {
	s := idx.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if exp, ok := s.entries[key]; ok && now.Before(exp) {
		return true
	}
	s.entries[key] = now.Add(idx.ttl)
	return false
}

// SeenOrAddWithTTL is like SeenOrAdd but uses ttl instead of the index default.
func (idx *Index) SeenOrAddWithTTL(key string, ttl time.Duration) bool {
	s := idx.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if exp, ok := s.entries[key]; ok && now.Before(exp) {
		return true
	}
	s.entries[key] = now.Add(ttl)
	return false
}

// Purge removes entries whose TTL has expired from all shards. Call periodically to bound memory.
func (idx *Index) Purge() {
	now := time.Now()
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.Lock()
		for key, exp := range s.entries {
			if now.After(exp) {
				delete(s.entries, key)
			}
		}
		s.mu.Unlock()
	}
}

// Len returns the total number of live (non-expired) entries across all shards.
func (idx *Index) Len() int {
	now := time.Now()
	n := 0
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.Lock()
		for _, exp := range s.entries {
			if now.Before(exp) {
				n++
			}
		}
		s.mu.Unlock()
	}
	return n
}
