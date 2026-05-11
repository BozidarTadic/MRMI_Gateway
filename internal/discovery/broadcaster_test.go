package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"MRMI_Gateway/internal/dedup"
)

type fakeClient struct {
	resp *Response
	err  error
}

func (f *fakeClient) BroadcastDiscovery(_ context.Context, _ *Request) (*Response, error) {
	return f.resp, f.err
}

func (f *fakeClient) Close() error { return nil }

func fixedDialer(c PeerClient) PeerDialer {
	return func(_ context.Context, _ string) (PeerClient, error) {
		return c, nil
	}
}

func freshBroadcaster(dialer PeerDialer) *Broadcaster {
	peers := map[string]string{"peer-a": "addr-a"}
	return New(peers, dedup.New(30*time.Second), dialer)
}

func goodRequest() Request {
	return Request{
		QueryHash:    "abc",
		OriginNodeID: "node-src",
		OriginAppID:  "app-x",
		HopLimit:     3,
		RequestID:    "req-001",
		Timestamp:    time.Now().UnixMilli(),
	}
}

func TestBroadcast_ReturnsResponse(t *testing.T) {
	client := &fakeClient{resp: &Response{OpaqueToken: "tok-123"}}
	b := freshBroadcaster(fixedDialer(client))
	results := b.Broadcast(context.Background(), goodRequest())
	if len(results) != 1 || results[0].OpaqueToken != "tok-123" {
		t.Fatalf("expected 1 result with token, got %v", results)
	}
}

func TestBroadcast_HopLimitZeroDropped(t *testing.T) {
	client := &fakeClient{resp: &Response{OpaqueToken: "tok"}}
	b := freshBroadcaster(fixedDialer(client))
	req := goodRequest()
	req.HopLimit = 0
	results := b.Broadcast(context.Background(), req)
	if len(results) != 0 {
		t.Fatalf("expected no results when hop_limit=0, got %d", len(results))
	}
}

func TestBroadcast_HopLimitDecremented(t *testing.T) {
	var got *Request
	dialer := func(_ context.Context, _ string) (PeerClient, error) {
		return &captureClient{fn: func(req *Request) (*Response, error) {
			got = req
			return &Response{OpaqueToken: "tok"}, nil
		}}, nil
	}
	b := freshBroadcaster(dialer)
	req := goodRequest()
	req.HopLimit = 3
	b.Broadcast(context.Background(), req)
	if got == nil || got.HopLimit != 2 {
		t.Fatalf("expected forwarded hop_limit=2, got %v", got)
	}
}

func TestBroadcast_DuplicateRequestIDIgnored(t *testing.T) {
	client := &fakeClient{resp: &Response{OpaqueToken: "tok"}}
	b := freshBroadcaster(fixedDialer(client))
	req := goodRequest()
	b.Broadcast(context.Background(), req)
	results := b.Broadcast(context.Background(), req)
	if len(results) != 0 {
		t.Fatalf("expected 0 results on duplicate request_id, got %d", len(results))
	}
}

func TestBroadcast_StaleTimestampRejected(t *testing.T) {
	client := &fakeClient{resp: &Response{OpaqueToken: "tok"}}
	b := freshBroadcaster(fixedDialer(client))
	req := goodRequest()
	req.Timestamp = time.Now().Add(-31 * time.Second).UnixMilli()
	results := b.Broadcast(context.Background(), req)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for stale request, got %d", len(results))
	}
}

func TestBroadcast_PartialResultsOnPeerTimeout(t *testing.T) {
	peers := map[string]string{"ok": "addr-ok", "fail": "addr-fail"}
	d := dedup.New(30 * time.Second)
	dialer := func(_ context.Context, addr string) (PeerClient, error) {
		if addr == "addr-fail" {
			return nil, errors.New("timeout")
		}
		return &fakeClient{resp: &Response{OpaqueToken: "tok"}}, nil
	}
	b := New(peers, d, dialer)
	results := b.Broadcast(context.Background(), goodRequest())
	if len(results) != 1 {
		t.Fatalf("expected 1 partial result, got %d", len(results))
	}
}

func TestBroadcast_EmptyTokenResponseSkipped(t *testing.T) {
	client := &fakeClient{resp: &Response{OpaqueToken: ""}}
	b := freshBroadcaster(fixedDialer(client))
	results := b.Broadcast(context.Background(), goodRequest())
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty token response, got %d", len(results))
	}
}

// captureClient lets tests inspect the forwarded request.
type captureClient struct {
	fn func(*Request) (*Response, error)
}

func (c *captureClient) BroadcastDiscovery(_ context.Context, req *Request) (*Response, error) {
	return c.fn(req)
}
func (c *captureClient) Close() error { return nil }
