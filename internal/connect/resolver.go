package connect

import (
	"slices"
	"strings"
	"sync/atomic"

	"MRMI_Gateway/internal/config"
)

type Status string

const (
	StatusAccepted Status = "ACCEPTED"
	StatusPending  Status = "PENDING"
	StatusDenied   Status = "DENIED"
)

// Request carries the inputs to a connect resolution.
type Request struct {
	OriginNodeID   string
	OriginAppID    string
	RecipientAppID string
	// MutualDiscovery reports whether the origin node has an active opaque token
	// issued by this node (i.e. the origin previously discovered us). Populated by
	// the caller using token.Store.HasActiveToken.
	MutualDiscovery bool
}

// Resolver evaluates ConnectRequests against [policy.connect] configuration.
type Resolver struct {
	cfg atomic.Pointer[config.Config]
}

func New(cfg config.Config) *Resolver {
	r := &Resolver{}
	r.cfg.Store(&cfg)
	return r
}

// Reload atomically replaces the config used for resolution.
func (r *Resolver) Reload(cfg config.Config) {
	r.cfg.Store(&cfg)
}

// Resolve returns the accept/deny decision for a ConnectRequest.
func (r *Resolver) Resolve(req Request) Status {
	cfg := *r.cfg.Load()
	mode := strings.ToUpper(strings.TrimSpace(cfg.Policy.Connect.AutoAccept))
	if mode == "" {
		mode = "MANUAL"
	}

	switch mode {
	case "AUTO_ALL":
		return StatusAccepted
	case "AUTO_WHITELIST":
		if slices.Contains(cfg.Policy.Connect.TrustedNodes, req.OriginNodeID) {
			return StatusAccepted
		}
		return StatusPending
	case "AUTO_MUTUAL":
		// Accept only when mutual discovery is confirmed: the origin node previously
		// received a token from this node (HasActiveToken) and is now connecting.
		if req.MutualDiscovery {
			return StatusAccepted
		}
		return StatusPending
	default: // MANUAL
		return StatusPending
	}
}
