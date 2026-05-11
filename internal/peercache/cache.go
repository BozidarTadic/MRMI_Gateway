package peercache

import "sync"

type Entry struct {
	RootHash  string `json:"root_hash"`
	Timestamp int64  `json:"timestamp"`
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func New() *Cache {
	return &Cache{entries: make(map[string]Entry)}
}

func (c *Cache) Store(nodeID, rootHash string, timestamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[nodeID] = Entry{RootHash: rootHash, Timestamp: timestamp}
}

func (c *Cache) All() map[string]Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]Entry, len(c.entries))
	for k, v := range c.entries {
		out[k] = v
	}
	return out
}
