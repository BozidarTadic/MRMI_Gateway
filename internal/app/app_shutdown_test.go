package app_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"MRMI_Gateway/internal/app"
	"MRMI_Gateway/internal/config"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.LogLevel = "error"
	cfg.Network.HTTPListenAddr = freeAddr(t)
	cfg.Network.GRPCListenAddr = freeAddr(t)
	cfg.Network.MetricsAddr = ""
	cfg.Network.ShutdownTimeout = 3 * time.Second
	cfg.TLS.Insecure = true
	cfg.Policy.Audit.DNSTXTPublish = false
	cfg.Policy.Audit.RootHashGossip = false
	return cfg
}

func TestRun_GracefulShutdown(t *testing.T) {
	cfg := testConfig(t)
	httpAddr := cfg.Network.HTTPListenAddr

	ctx, cancel := context.WithCancel(context.Background())

	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx, cfg, "")
	}()

	// Wait for HTTP to become ready.
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + httpAddr + "/healthz") //nolint:noctx
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("server did not become ready within 5s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("app.Run returned non-nil error after clean shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app.Run did not return within 5s of cancellation")
	}
}
