package delivery

import (
	"context"
	"fmt"
	"slices"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/transit"
)

// Forwarder selects the best peer for a given recipient region and forwards
// the envelope. Path selection follows ADR-006 tier preference:
// Regional → Alliance → Global → Transit Cache → DLQ.
type Forwarder struct {
	cfg          config.Config
	dlq          *DLQ
	transitCache *transit.Cache // nil when transit cache is disabled
	retryPolicy  RetryPolicy
	send         func(ctx context.Context, addr string, env core.Envelope) (peerRootHash string, err error)
}

// NewForwarder creates a Forwarder with the default ADR-007 retry policy.
// send is the transport function (typically a gRPC call) that returns the
// peer's audit root hash on success. dlq receives entries when all peers and
// all retries are exhausted. tc may be nil to disable the transit buffer.
func NewForwarder(cfg config.Config, dlq *DLQ, tc *transit.Cache, send func(ctx context.Context, addr string, env core.Envelope) (string, error)) *Forwarder {
	return &Forwarder{cfg: cfg, dlq: dlq, transitCache: tc, retryPolicy: DefaultRetryPolicy(), send: send}
}

// NewForwarderWithPolicy creates a Forwarder with a custom retry policy.
// Useful in tests to avoid multi-second backoff delays.
func NewForwarderWithPolicy(cfg config.Config, dlq *DLQ, tc *transit.Cache, send func(ctx context.Context, addr string, env core.Envelope) (string, error), policy RetryPolicy) *Forwarder {
	return &Forwarder{cfg: cfg, dlq: dlq, transitCache: tc, retryPolicy: policy, send: send}
}

// PeersFor returns candidate peers for recipientRegion in tier-preference order:
// Regional → Alliance → Global. Tiers excluded by policy.routing.allow_via are
// filtered out. If allow_via is empty all tiers are permitted.
func (f *Forwarder) PeersFor(recipientRegion string) []config.PeerConfig {
	allowVia := f.cfg.Policy.Routing.AllowVia

	var regional, alliance, global []config.PeerConfig
	for key, peer := range f.cfg.Network.Peers {
		if len(allowVia) > 0 && !slices.Contains(allowVia, peer.NodeScope) {
			continue
		}
		switch peer.NodeScope {
		case "regional":
			if key == recipientRegion {
				regional = append(regional, peer)
			}
		case "alliance":
			if slices.Contains(peer.Regions, recipientRegion) {
				alliance = append(alliance, peer)
			}
		case "global":
			global = append(global, peer)
		}
	}

	return append(append(regional, alliance...), global...)
}

// Forward attempts delivery to candidate peers in tier-preference order, applying
// the configured retry policy per peer. The envelope is written to the DLQ only
// after all peers and all their retries are exhausted. Satisfies core.Forwarder.
func (f *Forwarder) Forward(ctx context.Context, env core.Envelope) (string, error) {
	peers := f.PeersFor(env.RecipientRegion)
	if len(peers) == 0 {
		return "", fmt.Errorf("no peer available for region %q", env.RecipientRegion)
	}

	for _, peer := range peers {
		addr := peer.Addr
		var peerRootHash string
		err := SendWithRetry(ctx, func() error {
			var sendErr error
			peerRootHash, sendErr = f.send(ctx, addr, env)
			return sendErr
		}, f.retryPolicy, nil, DLQEntry{Envelope: env, PeerAddr: addr})
		if err == nil {
			return peerRootHash, nil
		}
	}

	// Transit cache buffers the envelope for a short TTL before DLQ promotion,
	// giving transient peer failures a second delivery window (ADR §4.3).
	if f.transitCache != nil {
		f.transitCache.Put(env, peers[0].Addr)
		return "", fmt.Errorf("all peers exhausted for region %q; buffered in transit cache", env.RecipientRegion)
	}
	if f.dlq != nil {
		f.dlq.Append(DLQEntry{Envelope: env, PeerAddr: peers[0].Addr})
	}
	return "", fmt.Errorf("all peers exhausted for region %q", env.RecipientRegion)
}
