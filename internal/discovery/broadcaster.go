package discovery

import (
	"context"
	"time"

	"MRMI_Gateway/internal/dedup"
)

const staleness = 30 * time.Second

// Request mirrors the proto DiscoveryRequest for inter-node fan-out.
type Request struct {
	QueryHash    string
	QueryType    string
	OriginNodeID string
	OriginAppID  string
	HopLimit     uint32
	RequestID    string
	Timestamp    int64 // unix_ms
}

// Response mirrors the proto DiscoveryResponse returned by peers.
type Response struct {
	NodeID       string
	AppID        string
	OpaqueToken  string
	DisplayHint  string
	MatchType    string
	TokenExpires int64 // unix_ms
}

// PeerClient is implemented by the gRPC transport client for discovery calls.
type PeerClient interface {
	BroadcastDiscovery(ctx context.Context, req *Request) (*Response, error)
	Close() error
}

// PeerDialer dials a peer at the given address and returns a PeerClient.
type PeerDialer func(ctx context.Context, addr string) (PeerClient, error)

// Broadcaster fans out a Request to all configured peers, enforcing hop_limit,
// dedup, and staleness checks, then aggregates the responses.
type Broadcaster struct {
	peers  map[string]string // peer key → dial address
	dedup  *dedup.Index
	dialer PeerDialer
}

func New(peers map[string]string, dedupIndex *dedup.Index, dialer PeerDialer) *Broadcaster {
	return &Broadcaster{peers: peers, dedup: dedupIndex, dialer: dialer}
}

// Broadcast forwards the request to all peers and returns aggregated responses.
// Returns nil when the request is stale, a duplicate, or hop_limit is 0.
func (b *Broadcaster) Broadcast(ctx context.Context, req Request) []Response {
	nowMs := time.Now().UnixMilli()
	if req.Timestamp > 0 && nowMs-req.Timestamp > int64(staleness/time.Millisecond) {
		return nil
	}

	if b.dedup.SeenOrAdd(req.RequestID) {
		return nil
	}

	if req.HopLimit == 0 {
		return nil
	}

	forward := req
	forward.HopLimit--

	var results []Response
	for _, addr := range b.peers {
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		client, err := b.dialer(dialCtx, addr)
		cancel()
		if err != nil {
			continue
		}
		resp, err := client.BroadcastDiscovery(ctx, &forward)
		_ = client.Close()
		if err != nil || resp == nil {
			continue
		}
		if resp.OpaqueToken != "" {
			results = append(results, *resp)
		}
	}
	return results
}
