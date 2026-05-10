package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/dnstxt"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	"MRMI_Gateway/internal/tlsutil"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

func Run(ctx context.Context, cfg config.Config) error {
	tlsCert := tlsutil.TLSConfig{
		Cert:     cfg.TLS.Cert,
		Key:      cfg.TLS.Key,
		CA:       cfg.TLS.CA,
		Insecure: cfg.TLS.Insecure,
	}
	serverTLS, err := tlsutil.LoadServerTLS(tlsCert)
	if err != nil {
		return fmt.Errorf("load server TLS: %w", err)
	}
	clientTLS, err := tlsutil.LoadClientTLS(tlsCert)
	if err != nil {
		return fmt.Errorf("load client TLS: %w", err)
	}

	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog)
	if err != nil {
		return fmt.Errorf("create policy engine: %w", err)
	}

	dedupIndex := dedup.New(cfg.Profile.DedupTTL)
	go runPurge(ctx, dedupIndex)

	if cfg.Policy.Audit.DNSTXTPublish {
		if cfg.Policy.Audit.DNSTXTInterval == 0 {
			log.Printf("[dnstxt] dns_txt_publish=true but dns_txt_interval_s is 0, skipping publisher")
		} else {
			log.Printf("[dnstxt] no DNS provider configured; audit root hash will be emitted to stdout")
			p := dnstxt.New(cfg.Node.NodeID, cfg.Policy.Audit.DNSTXTInterval, os.Stdout)
			go p.Run(ctx, auditLog.RootHash)
		}
	}

	dlq := delivery.NewDLQ()
	var fwd core.Forwarder
	if len(cfg.Network.Peers) > 0 {
		fwd = delivery.NewForwarder(cfg, dlq, func(ctx context.Context, addr string, env core.Envelope) error {
			dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			client, err := grpctransport.Dial(dialCtx, addr, clientTLS)
			if err != nil {
				return fmt.Errorf("dial %s: %w", addr, err)
			}
			defer client.Close()
			resp, err := client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
				Envelope: grpctransport.Envelope{
					IdempotencyKey:    env.IdempotencyKey,
					SenderIdentity:    env.SenderIdentity,
					RecipientIdentity: env.RecipientIdentity,
					SenderRegion:      env.SenderRegion,
					RecipientRegion:   env.RecipientRegion,
					TrustTier:         env.TrustTier,
					SequenceNumber:    env.SequenceNumber,
					Payload:           env.Payload,
					PaddedTo:          env.PaddedTo,
					Timestamp:         env.Timestamp,
					Signature:         env.Signature,
				},
			})
			if err != nil {
				return err
			}
			if resp.Decision == "DENY" {
				return fmt.Errorf("peer denied envelope: %s", resp.Reason)
			}
			return nil
		})
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedupIndex, fwd)

	httpServer := server.NewHTTPServer(cfg, engine, auditLog)
	grpcServer, err := grpctransport.NewServer(cfg.Network.GRPCListenAddr, grpctransport.NewAdapter(gw), serverTLS)
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
