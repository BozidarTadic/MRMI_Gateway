package peerdiscovery

import (
	"sync"
	"time"
)

// PeerInfo describes a discovered peer node.
type PeerInfo struct {
	NodeID    string    `json:"node_id"`
	Addr      string    `json:"addr"`
	NodeScope string    `json:"node_scope"`
	Region    string    `json:"region"`
	LastSeen  time.Time `json:"last_seen"`
}

// Registry is a thread-safe in-memory store of known peers discovered via gossip.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]PeerInfo // keyed by NodeID
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{peers: make(map[string]PeerInfo)}
}

// Announce adds or refreshes a peer. LastSeen is set to now.
func (r *Registry) Announce(peer PeerInfo) {
	peer.LastSeen = time.Now()
	r.mu.Lock()
	r.peers[peer.NodeID] = peer
	r.mu.Unlock()
}

// Evict removes a peer by NodeID.
func (r *Registry) Evict(nodeID string) {
	r.mu.Lock()
	delete(r.peers, nodeID)
	r.mu.Unlock()
}

// Known returns a snapshot of all known peers.
func (r *Registry) Known() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PeerInfo, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, p)
	}
	return out
}

// EvictStale removes peers whose LastSeen is older than maxAge.
func (r *Registry) EvictStale(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.peers {
		if p.LastSeen.Before(cutoff) {
			delete(r.peers, id)
		}
	}
}
