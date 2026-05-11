package crl

import (
	"sync"
	"time"
)

// Entry represents a revocation record for a node.
// It becomes effective (node is revoked) when len(Signatures) >= 2.
type Entry struct {
	NodeID     string
	Reason     string
	RevokedAt  time.Time
	Signatures [][]byte
}

// Store is a thread-safe in-memory CRL store.
type Store struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

func New() *Store {
	return &Store{entries: make(map[string]*Entry)}
}

// Revoke adds a signature to the CRL entry for nodeID. Creates the entry if absent.
func (s *Store) Revoke(nodeID, reason string, sig []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[nodeID]
	if !ok {
		e = &Entry{NodeID: nodeID, Reason: reason, RevokedAt: time.Now()}
		s.entries[nodeID] = e
	}
	for _, existing := range e.Signatures {
		if string(existing) == string(sig) {
			return // deduplicate
		}
	}
	e.Signatures = append(e.Signatures, sig)
}

// IsRevoked returns true when nodeID has ≥2 corroborating signatures.
func (s *Store) IsRevoked(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.entries[nodeID]
	return ok && len(e.Signatures) >= 2
}

// Entries returns a snapshot of all CRL entries.
func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		sigs := make([][]byte, len(e.Signatures))
		copy(sigs, e.Signatures)
		out = append(out, Entry{
			NodeID:     e.NodeID,
			Reason:     e.Reason,
			RevokedAt:  e.RevokedAt,
			Signatures: sigs,
		})
	}
	return out
}

// Merge integrates entries received from a peer. Existing signatures are deduplicated.
func (s *Store) Merge(entries []Entry) {
	for _, e := range entries {
		for _, sig := range e.Signatures {
			s.Revoke(e.NodeID, e.Reason, sig)
		}
	}
}
