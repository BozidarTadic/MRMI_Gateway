package grpctransport

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"google.golang.org/grpc/connectivity"
)

// ConnPool keeps one persistent gRPC connection per peer address,
// eliminating per-RPC dial overhead on the forwarding hot path.
// Safe for concurrent use.
type ConnPool struct {
	mu     sync.Mutex
	conns  map[string]*Client
	tlsCfg *tls.Config
}

// NewConnPool creates a pool backed by the given TLS config (nil = insecure).
func NewConnPool(tlsCfg *tls.Config) *ConnPool {
	return &ConnPool{
		conns:  make(map[string]*Client),
		tlsCfg: tlsCfg,
	}
}

// Client returns a live, pooled client for addr, dialing only on the first call
// or after the previous connection entered the Shutdown state.
// Callers must not close the returned client; the pool manages lifetime.
func (p *ConnPool) Client(ctx context.Context, addr string) (*Client, error) {
	p.mu.Lock()
	c := p.conns[addr]
	if c != nil && c.conn.GetState() != connectivity.Shutdown {
		p.mu.Unlock()
		return c, nil
	}
	if c != nil {
		c.pooled = false
		_ = c.conn.Close()
		delete(p.conns, addr)
	}
	p.mu.Unlock()

	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fresh, err := Dial(dialCtx, addr, p.tlsCfg)
	if err != nil {
		return nil, err
	}
	fresh.pooled = true

	p.mu.Lock()
	if existing := p.conns[addr]; existing != nil {
		// Another goroutine dialed concurrently; use theirs.
		p.mu.Unlock()
		fresh.pooled = false
		_ = fresh.conn.Close()
		return existing, nil
	}
	p.conns[addr] = fresh
	p.mu.Unlock()
	return fresh, nil
}

// Close closes all pooled connections. Call once at node shutdown.
func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for addr, c := range p.conns {
		c.pooled = false
		_ = c.conn.Close()
		delete(p.conns, addr)
	}
}
