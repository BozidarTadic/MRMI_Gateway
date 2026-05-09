package delivery

import (
	"sync"
	"time"

	"MRMI_Gateway/internal/core"
)

// DLQEntry holds a forwarding attempt that exhausted all retries.
type DLQEntry struct {
	Envelope        core.Envelope
	PeerAddr        string
	Attempts        int
	LastErr         error
	FirstSeenUnix   int64
	LastAttemptUnix int64
}

// DLQ is an in-memory dead-letter queue for envelopes that could not be forwarded.
type DLQ struct {
	mu      sync.Mutex
	entries []DLQEntry
}

// NewDLQ returns an empty DLQ.
func NewDLQ() *DLQ {
	return &DLQ{}
}

// Append adds an entry to the queue.
func (d *DLQ) Append(e DLQEntry) {
	if e.FirstSeenUnix == 0 {
		e.FirstSeenUnix = time.Now().Unix()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = append(d.entries, e)
}

// Entries returns a snapshot of all current entries.
func (d *DLQ) Entries() []DLQEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DLQEntry, len(d.entries))
	copy(out, d.entries)
	return out
}

// Remove deletes the entry at the given index.
// The index must be within [0, Size()).
func (d *DLQ) Remove(index int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if index < 0 || index >= len(d.entries) {
		return
	}
	d.entries = append(d.entries[:index], d.entries[index+1:]...)
}

// Size returns the number of entries currently in the queue.
func (d *DLQ) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}
