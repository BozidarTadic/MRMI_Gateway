package dnstxt

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Publisher emits DNS TXT record values for audit root hash publication.
// Format: v=1 ts=<unix> root=<root_hash> node=<node_id>
type Publisher struct {
	nodeID   string
	interval time.Duration
	out      io.Writer
}

// New creates a Publisher that writes to out on each interval tick.
func New(nodeID string, interval time.Duration, out io.Writer) *Publisher {
	return &Publisher{nodeID: nodeID, interval: interval, out: out}
}

// Run emits the DNS TXT value on each interval tick until ctx is cancelled.
// rootHash is called on each tick to get the current audit root hash.
func (p *Publisher) Run(ctx context.Context, rootHash func() string) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(p.out, "v=1 ts=%d root=%s node=%s\n",
				time.Now().Unix(), rootHash(), p.nodeID)
		}
	}
}
