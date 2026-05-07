package grpctransport

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

// startTestServer binds on a random port and returns the address and a connected client.
// The server and client are shut down via t.Cleanup.
func startTestServer(t *testing.T) (addr string, client *Client) {
	t.Helper()

	cfg := config.DefaultBalancedConfig()
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}

	srv, err := NewServer(":0", NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL)))
	if err != nil {
		t.Fatalf("create grpc server: %v", err)
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	addr = srv.listener.Addr().String()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dialCancel()

	c, err := Dial(dialCtx, addr)
	if err != nil {
		t.Fatalf("dial grpc server at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return addr, c
}

func TestSendEnvelope_DuplicateKeyReturnsDuplicate(t *testing.T) {
	_, client := startTestServer(t)
	ctx := context.Background()

	req := &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "key-dup-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			TrustTier:       0,
		},
	}

	first, err := client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if first.Decision != "ALLOW" {
		t.Fatalf("first send: expected ALLOW, got %q", first.Decision)
	}
	if first.AuditRootHash == "" {
		t.Fatal("first send: expected non-empty audit root hash")
	}

	second, err := client.SendEnvelope(ctx, req)
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	if second.Decision != "DUPLICATE" {
		t.Fatalf("second send: expected DUPLICATE, got %q", second.Decision)
	}
	if second.AuditRootHash == "" {
		t.Fatal("second send: expected non-empty audit root hash")
	}
}

func TestSendEnvelope_EmptyIdempotencyKeyRejected(t *testing.T) {
	_, client := startTestServer(t)
	ctx := context.Background()

	_, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err == nil {
		t.Fatal("expected error for empty idempotency_key, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestSendEnvelope_DistinctKeysNotDuplicated(t *testing.T) {
	_, client := startTestServer(t)
	ctx := context.Background()

	for _, key := range []string{"key-a", "key-b"} {
		resp, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
			Envelope: Envelope{
				IdempotencyKey:  key,
				SenderRegion:    "RS",
				RecipientRegion: "RU",
			},
		})
		if err != nil {
			t.Fatalf("send %q: %v", key, err)
		}
		if resp.Decision != "ALLOW" {
			t.Fatalf("send %q: expected ALLOW, got %q", key, resp.Decision)
		}
	}
}
