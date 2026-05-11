package integration

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/testcerts"
	"MRMI_Gateway/internal/tlsutil"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// startMTLSNode starts a gateway node with mTLS using the provided cert paths.
// It returns a client connected via mTLS and the node's audit log.
func startMTLSNode(t *testing.T, cfg config.Config, certPath, keyPath, caPath string) (*grpctransport.Client, *audit.Log) {
	t.Helper()

	serverTLS, err := tlsutil.LoadServerTLS(tlsutil.TLSConfig{
		Cert: certPath,
		Key:  keyPath,
		CA:   caPath,
	})
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)

	srv, err := grpctransport.NewServer(":0", grpctransport.NewAdapter(gw), serverTLS)
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	// Extract port from ":PORT" or "[::]:PORT" and dial localhost so the
	// client's ServerName matches the cert's DNS SAN "localhost".
	_, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("parse server addr %q: %v", srv.Addr(), err)
	}
	dialAddr := fmt.Sprintf("localhost:%s", port)

	clientTLS, err := tlsutil.LoadClientTLS(tlsutil.TLSConfig{
		Cert: certPath,
		Key:  keyPath,
		CA:   caPath,
	})
	if err != nil {
		t.Fatalf("LoadClientTLS: %v", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, clientTLS)
	if err != nil {
		t.Fatalf("dial %s: %v", dialAddr, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client, auditLog
}

func TestMTLS_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := testcerts.Generate(t, dir)

	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeID = "rs-mtls-test"
	cfg.Node.Region = "RS"
	cfg.Node.ApplicableLaw = "RS-GDPR"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	client, _ := startMTLSNode(t, cfg, certPath, keyPath, caPath)

	resp, err := client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "mtls-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope over mTLS: %v", err)
	}
	if resp.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW, got %q (%s)", resp.Decision, resp.Reason)
	}
}

func TestMTLS_InsecureClientRejected(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := testcerts.Generate(t, dir)

	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeID = "rs-mtls-reject"
	cfg.Node.Region = "RS"
	cfg.Node.ApplicableLaw = "RS-GDPR"

	serverTLS, err := tlsutil.LoadServerTLS(tlsutil.TLSConfig{
		Cert: certPath,
		Key:  keyPath,
		CA:   caPath,
	})
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	gw := core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	srv, err := grpctransport.NewServer(":0", grpctransport.NewAdapter(gw), serverTLS)
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_, port, _ := net.SplitHostPort(srv.Addr())
	dialAddr := fmt.Sprintf("localhost:%s", port)

	// Insecure client — no certificates presented
	dialCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := grpctransport.Dial(dialCtx, dialAddr, nil)
	if err != nil {
		// Connection-level rejection is also acceptable
		return
	}
	defer client.Close()

	_, err = client.SendEnvelope(context.Background(), &grpctransport.SendEnvelopeRequest{
		Envelope: grpctransport.Envelope{
			IdempotencyKey:  "mtls-reject-001",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
		},
	})
	if err == nil {
		t.Fatal("expected error: insecure client must be rejected by mTLS server")
	}
}
