package trustdecay

import (
	"context"
	"log"
	"sync"
	"time"
)

// Timer tracks the last cross-validation timestamp per peer node and reduces
// the effective trust tier when no validation has occurred within DecayWindow.
type Timer struct {
	mu           sync.RWMutex
	lastValidated map[string]time.Time
	DecayWindow  time.Duration // default 30 days
	checkInterval time.Duration
}

// New creates a Timer with the given decay window.
func New(decayWindow time.Duration) *Timer {
	if decayWindow <= 0 {
		decayWindow = 30 * 24 * time.Hour
	}
	return &Timer{
		lastValidated: make(map[string]time.Time),
		DecayWindow:   decayWindow,
		checkInterval: time.Hour,
	}
}

// RecordValidation records a cross-validation event for peerID.
func (t *Timer) RecordValidation(peerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastValidated[peerID] = time.Now()
}

// EffectiveTier returns the tier to use for routing. If peerID has not been
// cross-validated within DecayWindow, the tier is reduced by 1 (floor 0).
func (t *Timer) EffectiveTier(peerID string, announcedTier uint32) uint32 {
	t.mu.RLock()
	last, known := t.lastValidated[peerID]
	t.mu.RUnlock()

	if !known || time.Since(last) > t.DecayWindow {
		if announcedTier > 0 {
			return announcedTier - 1
		}
		return 0
	}
	return announcedTier
}

// Run starts the background decay check loop until ctx is cancelled.
func (t *Timer) Run(ctx context.Context) {
	ticker := time.NewTicker(t.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.mu.RLock()
			for peerID, last := range t.lastValidated {
				if time.Since(last) > t.DecayWindow {
					log.Printf("[trustdecay] peer %s has not been cross-validated in %s — effective trust tier reduced", peerID, t.DecayWindow)
				}
			}
			t.mu.RUnlock()
		}
	}
}
