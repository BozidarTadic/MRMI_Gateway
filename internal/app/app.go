package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

func Run(ctx context.Context, cfg config.Config) error {
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		return fmt.Errorf("create policy engine: %w", err)
	}

	dedupIndex := dedup.New(cfg.Profile.DedupTTL)
	go runPurge(ctx, dedupIndex)

	httpServer := server.NewHTTPServer(cfg, engine, auditLog)
	grpcServer, err := grpctransport.NewServer(cfg.Network.GRPCListenAddr, grpctransport.NewGateway(cfg, engine, auditLog, dedupIndex))
	if err != nil {
		return fmt.Errorf("create grpc server: %w", err)
	}

	errCh := make(chan error, 2)

	go func() {
		errCh <- httpServer.ListenAndServe()
	}()
	go func() {
		errCh <- grpcServer.Serve()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Network.ShutdownTimeout)
		defer cancel()
		if err := grpcServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runPurge(ctx context.Context, idx *dedup.Index) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idx.Purge()
		}
	}
}
