// Package ratelimit provides a per-key token bucket rate limiter.
// Buckets are lazily created and evicted after an idle period to bound memory.
package ratelimit

import (
	"sync"
	"time"
)

const (
	defaultRate     = 10.0 // tokens per second
	defaultBurst    = 20
	idleEvictAfter  = 5 * time.Minute
	cleanupInterval = time.Minute
)

type bucket struct {
	tokens   float64
	capacity float64
	rate     float64 // tokens added per second
	lastSeen time.Time
}

func (b *bucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Limiter is a thread-safe, per-key token bucket limiter.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	burst    float64
	stopOnce sync.Once
	stop     chan struct{}
}

// New creates a Limiter that allows up to burst calls per key, refilling at
// rate tokens per second. If rate or burst are ≤ 0 the defaults are used.
func New(rate float64, burst int) *Limiter {
	if rate <= 0 {
		rate = defaultRate
	}
	if burst <= 0 {
		burst = defaultBurst
	}
	l := &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   float64(burst),
		stop:    make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow returns true if the key is within its rate limit.
func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, capacity: l.burst, rate: l.rate, lastSeen: now}
		l.buckets[key] = b
	}
	return b.allow(now)
}

// Len returns the number of live buckets (for testing/metrics).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Close stops the background cleanup goroutine.
func (l *Limiter) Close() {
	l.stopOnce.Do(func() { close(l.stop) })
}

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for k, b := range l.buckets {
				if now.Sub(b.lastSeen) > idleEvictAfter {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}
}
