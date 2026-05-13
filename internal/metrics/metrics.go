// Package metrics implements a hand-rolled Prometheus text-format exposition
// server. No external Prometheus client dependency is used — only net/http and
// sync/atomic from the standard library.
package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// Registry holds all counters and gauge readers for the node.
// Create one with New, wire counter increments via Inc* methods, and serve
// /metrics with Handler().
type Registry struct {
	allowTotal         atomic.Int64
	duplicateTotal     atomic.Int64
	rateLimitDenyTotal atomic.Int64

	denyMu       sync.Mutex
	denyByReason map[string]*atomic.Int64

	getDLQLen     func() int // nil → 0
	getTransitLen func() int // nil → 0
	getPeerCount  func() int // nil → 0
}

// New returns a Registry. The three getter functions are called on every
// /metrics request to read current gauge values; pass nil to report 0.
func New(getDLQLen, getTransitLen, getPeerCount func() int) *Registry {
	return &Registry{
		denyByReason:  make(map[string]*atomic.Int64),
		getDLQLen:     getDLQLen,
		getTransitLen: getTransitLen,
		getPeerCount:  getPeerCount,
	}
}

// IncAllow increments mrmi_envelope_allow_total.
func (r *Registry) IncAllow() { r.allowTotal.Add(1) }

// IncDuplicate increments mrmi_envelope_duplicate_total.
func (r *Registry) IncDuplicate() { r.duplicateTotal.Add(1) }

// IncRateLimitDeny increments mrmi_rate_limit_deny_total.
func (r *Registry) IncRateLimitDeny() { r.rateLimitDenyTotal.Add(1) }

// IncDeny increments mrmi_envelope_deny_total for the given reason label.
func (r *Registry) IncDeny(reason string) {
	r.denyMu.Lock()
	c, ok := r.denyByReason[reason]
	if !ok {
		c = new(atomic.Int64)
		r.denyByReason[reason] = c
	}
	r.denyMu.Unlock()
	c.Add(1)
}

// Handler returns an http.Handler that serves all metrics in Prometheus text
// exposition format (version 0.0.4).
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintf(w, "# HELP mrmi_envelope_allow_total Envelopes with ALLOW decision\n")
		fmt.Fprintf(w, "# TYPE mrmi_envelope_allow_total counter\n")
		fmt.Fprintf(w, "mrmi_envelope_allow_total %d\n\n", r.allowTotal.Load())

		fmt.Fprintf(w, "# HELP mrmi_envelope_deny_total Envelopes with DENY decision\n")
		fmt.Fprintf(w, "# TYPE mrmi_envelope_deny_total counter\n")
		r.denyMu.Lock()
		denyCopy := make(map[string]int64, len(r.denyByReason))
		for reason, c := range r.denyByReason {
			denyCopy[reason] = c.Load()
		}
		r.denyMu.Unlock()
		for reason, n := range denyCopy {
			fmt.Fprintf(w, "mrmi_envelope_deny_total{reason=%q} %d\n", reason, n)
		}
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP mrmi_envelope_duplicate_total Envelopes rejected as duplicates\n")
		fmt.Fprintf(w, "# TYPE mrmi_envelope_duplicate_total counter\n")
		fmt.Fprintf(w, "mrmi_envelope_duplicate_total %d\n\n", r.duplicateTotal.Load())

		dlqLen := readGauge(r.getDLQLen)
		fmt.Fprintf(w, "# HELP mrmi_dlq_len Current DLQ depth\n")
		fmt.Fprintf(w, "# TYPE mrmi_dlq_len gauge\n")
		fmt.Fprintf(w, "mrmi_dlq_len %d\n\n", dlqLen)

		transitLen := readGauge(r.getTransitLen)
		fmt.Fprintf(w, "# HELP mrmi_transit_cache_len Current transit cache depth\n")
		fmt.Fprintf(w, "# TYPE mrmi_transit_cache_len gauge\n")
		fmt.Fprintf(w, "mrmi_transit_cache_len %d\n\n", transitLen)

		fmt.Fprintf(w, "# HELP mrmi_rate_limit_deny_total Discovery requests rejected by rate limiter\n")
		fmt.Fprintf(w, "# TYPE mrmi_rate_limit_deny_total counter\n")
		fmt.Fprintf(w, "mrmi_rate_limit_deny_total %d\n\n", r.rateLimitDenyTotal.Load())

		peerCount := readGauge(r.getPeerCount)
		fmt.Fprintf(w, "# HELP mrmi_peer_count Live peers in registry\n")
		fmt.Fprintf(w, "# TYPE mrmi_peer_count gauge\n")
		fmt.Fprintf(w, "mrmi_peer_count %d\n", peerCount)
	})
}

func readGauge(fn func() int) int {
	if fn == nil {
		return 0
	}
	return fn()
}
