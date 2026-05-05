package grpctransport

import (
	"context"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/policy"
)

func TestClientServerRoundTrip(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	cfg.Network.GRPCListenAddr = "127.0.0.1:17777"

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}

	server, err := NewServer(cfg.Network.GRPCListenAddr, NewGateway(cfg, engine, auditLog))
	if err != nil {
		t.Fatalf("create grpc server: %v", err)
	}

	go func() {
		_ = server.Serve()
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := Dial(ctx, cfg.Network.GRPCListenAddr)
	if err != nil {
		t.Fatalf("dial grpc server: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	response, err := client.SendEnvelope(ctx, &SendEnvelopeRequest{
		Envelope: Envelope{
			IdempotencyKey:  "test-1",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			TrustTier:       0,
		},
	})
	if err != nil {
		t.Fatalf("send envelope: %v", err)
	}

	if response.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW, got %q", response.Decision)
	}
	if response.AuditRootHash == "" {
		t.Fatalf("expected audit root hash")
	}
}
