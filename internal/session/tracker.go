package session

import (
	"fmt"
	"sync"
)

// Tracker maintains per-sender monotonic sequence numbers.
// Out-of-order delivery is warned but not rejected in v0.1 (gap tolerance).
type Tracker struct {
	mu   sync.Mutex
	next map[string]uint64 // senderID → next expected seq
}

func New() *Tracker {
	return &Tracker{next: make(map[string]uint64)}
}

// NextSeq returns the next sequence number for senderID and advances the counter.
func (t *Tracker) NextSeq(senderID string) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	seq := t.next[senderID] + 1
	t.next[senderID] = seq
	return seq
}

// Validate checks that seq is the expected next value for senderID.
// Returns nil on success. Returns a non-fatal warning error on out-of-order
// delivery — callers should log but not reject the envelope.
func (t *Tracker) Validate(senderID string, seq uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	expected := t.next[senderID] + 1
	if seq == expected {
		t.next[senderID] = seq
		return nil
	}
	// advance regardless to avoid permanently stalling on a gap
	if seq > t.next[senderID] {
		t.next[senderID] = seq
	}
	return fmt.Errorf("out-of-order envelope from %s: got seq %d, expected %d", senderID, seq, expected)
}
