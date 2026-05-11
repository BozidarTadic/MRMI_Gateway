package dummy

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

// Generator emits synthetic dummy envelopes at the profile-defined interval
// to prevent traffic analysis from distinguishing idle periods from active ones.
type Generator struct {
	cfg     config.Config
	counter atomic.Uint64
}

func New(cfg config.Config) *Generator {
	return &Generator{cfg: cfg}
}

// Run starts the dummy traffic loop until ctx is cancelled.
// send is called with each synthetic envelope. Envelopes have IsDummy=true
// so the receiving node can identify and skip policy evaluation.
func (g *Generator) Run(ctx context.Context, peers []config.PeerConfig, send func(core.Envelope)) {
	interval := g.cfg.Profile.DummyTrafficRate
	if interval <= 0 || len(peers) == 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, peer := range peers {
				g.counter.Add(1)
				env := core.Envelope{
					IdempotencyKey:  fmt.Sprintf("dummy-%s-%d", g.cfg.Node.NodeID, g.counter.Load()),
					SenderRegion:    g.cfg.Node.Region,
					RecipientRegion: peer.Region,
					TrustTier:       0,
					Timestamp:       time.Now().UnixMilli(),
					IsDummy:         true,
				}
				if g.cfg.Profile.PaddingBucket > 0 {
					env.Payload = make([]byte, g.cfg.Profile.PaddingBucket)
					env.PaddedTo = uint32(g.cfg.Profile.PaddingBucket)
				}
				send(env)
			}
		}
	}
}
