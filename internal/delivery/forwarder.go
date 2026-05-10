package delivery

import (
	"context"
	"fmt"
	"slices"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

// Forwarder selects the best peer for a given recipient region and forwards
// the envelope. Path selection follows ADR-006 tier preference:
// Regional → Alliance → Global → DLQ.
type Forwarder struct {
	cfg         config.Config
	dlq         *DLQ
	retryPolicy RetryPolicy
	send        func(ctx context.Context, addr string, env core.Envelope) error
}

// NewForwarder creates a Forwarder. send is the transport function (typically a
// gRPC call). dlq receives entries when all peers and all retries are exhausted.
func NewForwarder(cfg config.Config, dlq *DLQ, send func(ctx context.Context, addr string, env core.Envelope) error) *Forwarder {
	return &Forwarder{cfg: cfg, dlq: dlq, retryPolicy: DefaultRetryPolicy(), send: send}
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
// the default retry policy per peer. The envelope is written to the DLQ only after
// all peers and all their retries are exhausted. Satisfies core.Forwarder.
func (f *Forwarder) Forward(ctx context.Context, env core.Envelope) (string, error) {
	peers := f.PeersFor(env.RecipientRegion)
	if len(peers) == 0 {
		return "", fmt.Errorf("no peer available for region %q", env.RecipientRegion)
	}

	for _, peer := range peers {
		addr := peer.Addr
		err := SendWithRetry(ctx, func() error {
			return f.send(ctx, addr, env)
		}, f.retryPolicy, nil, DLQEntry{Envelope: env, PeerAddr: addr})
		if err == nil {
			return addr, nil
		}
	}

	if f.dlq != nil {
		f.dlq.Append(DLQEntry{Envelope: env, PeerAddr: peers[0].Addr})
	}
	return "", fmt.Errorf("all peers exhausted for region %q", env.RecipientRegion)
}
