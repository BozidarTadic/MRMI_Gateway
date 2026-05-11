package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/dnstxt"
	"MRMI_Gateway/internal/dummy"
	"MRMI_Gateway/internal/hotreload"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/inbox"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/server"
	"MRMI_Gateway/internal/session"
	"MRMI_Gateway/internal/trustdecay"
	"MRMI_Gateway/internal/tlsutil"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
)

// Run starts the gateway node. configPath is the path to the loaded config file;
// pass an empty string when config was loaded from defaults (hot-reload is skipped).
func Run(ctx context.Context, cfg config.Config, configPath string) error {
	signingKey, _, err := identity.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate signing key: %w", err)
	}
	log.Printf("[identity] using ephemeral Ed25519 signing key — set signing_key in [tls] for a persistent key")

	seqSend := session.New()

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
	crlStore := crl.New()
	engine, err := policy.NewEngine(cfg, auditLog, crlStore)
	if err != nil {
		return fmt.Errorf("create policy engine: %w", err)
	}

	decayTimer := trustdecay.New(30 * 24 * time.Hour)
	go decayTimer.Run(ctx)

	dedupIndex := dedup.New(cfg.Profile.DedupTTL)
	go runPurge(ctx, dedupIndex)

	if cfg.Node.ApplicableLaw == "NONE" && cfg.Profile.Name != "performance" {
		log.Printf("[warn] applicable_law is NONE on a %s profile node — set a real legal framework before production use", cfg.Profile.Name)
	}

	if cfg.Policy.Audit.DNSTXTPublish {
		if cfg.Policy.Audit.DNSTXTInterval == 0 {
			log.Printf("[dnstxt] dns_txt_publish=true but dns_txt_interval_s is 0, skipping publisher")
		} else {
			log.Printf("[dnstxt] no DNS provider configured; audit root hash will be emitted to stdout")
			p := dnstxt.New(cfg.Node.NodeID, cfg.Node.ApplicableLaw, cfg.Policy.Audit.DNSTXTInterval, os.Stdout)
			go p.Run(ctx, auditLog.RootHash)
		}
	}

	// Hot-reload: watch config file for changes and apply new policy atomically.
	if configPath != "" {
		lastVersion := cfg.Node.PolicyVersion
		watcher := hotreload.New()
		go watcher.Watch(ctx, configPath, func(newCfg config.Config) {
			if newCfg.Node.PolicyVersion == lastVersion {
				log.Printf("[hotreload] warning: policy_version unchanged (%s) — update policy_version when changing policy", lastVersion)
			}
			if err := engine.Reload(newCfg); err != nil {
				log.Printf("[hotreload] rejected invalid config: %v", err)
				return
			}
			lastVersion = newCfg.Node.PolicyVersion
			log.Printf("[hotreload] policy reloaded: version %s", newCfg.Node.PolicyVersion)
		})
	}

	dlq := delivery.NewDLQ()
	var fwd core.Forwarder
	if len(cfg.Network.Peers) > 0 {
		fwd = delivery.NewForwarder(cfg, dlq, func(ctx context.Context, addr string, env core.Envelope) (string, error) {
			env.SequenceNumber = seqSend.NextSeq(env.RecipientRegion)
			env.Signature = identity.Sign(signingKey, env)

			dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			client, err := grpctransport.Dial(dialCtx, addr, clientTLS)
			if err != nil {
				return "", fmt.Errorf("dial %s: %w", addr, err)
			}
			defer client.Close()
			resp, err := client.SendEnvelope(ctx, &grpctransport.SendEnvelopeRequest{
				Envelope: grpctransport.Envelope{
					IdempotencyKey:    env.IdempotencyKey,
					SenderNodeID:      cfg.Node.NodeID,
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
					IsDummy:           env.IsDummy,
				},
			})
			if err != nil {
				return "", err
			}
			if resp.Decision == "DENY" {
				return "", fmt.Errorf("peer denied envelope: %s", resp.Reason)
			}
			return resp.AuditRootHash, nil
		})
	}

	gw := core.NewGateway(cfg, engine, auditLog, dedupIndex, fwd)

	if len(cfg.Network.Peers) > 0 {
		peers := make([]config.PeerConfig, 0, len(cfg.Network.Peers))
		for _, p := range cfg.Network.Peers {
			peers = append(peers, p)
		}
		gen := dummy.New(cfg)
		go gen.Run(ctx, peers, func(env core.Envelope) {
			_, _ = gw.SendEnvelope(ctx, core.SendRequest{Envelope: env})
		})
	}

	// Peer cache and root hash gossip.
	var peerCache *peercache.Cache
	if cfg.Policy.Audit.RootHashGossip {
		peerCache = peercache.New()
		if cfg.Policy.Audit.DNSTXTInterval > 0 && len(cfg.Network.Peers) > 0 {
			go runGossip(ctx, cfg, auditLog, clientTLS)
		}
	}

	// Inbox: fan-out broadcaster for SSE clients.
	msgInbox := inbox.New()
	gw.SetOnAllow(func(env core.Envelope) {
		msgInbox.Publish(inbox.Event{
			IdempotencyKey:  env.IdempotencyKey,
			SenderRegion:    env.SenderRegion,
			RecipientRegion: env.RecipientRegion,
			TrustTier:       env.TrustTier,
			Payload:         env.Payload,
			Timestamp:       env.Timestamp,
		})
	})

	httpServer := server.NewHTTPServer(cfg, server.ServerDeps{
		Engine:  engine,
		Audit:   auditLog,
		PrivKey: signingKey,
		Peers:   peerCache,
		Gateway: gw,
		DLQ:     dlq,
		CRL:     crlStore,
		Inbox:   msgInbox,
	})

	var adapter grpctransport.GatewayService
	if peerCache != nil {
		adapter = grpctransport.NewAdapterFull(gw, nil, peerCache)
	} else {
		adapter = grpctransport.NewAdapter(gw)
	}

	grpcServer, err := grpctransport.NewServer(cfg.Network.GRPCListenAddr, adapter, serverTLS)
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

func runGossip(ctx context.Context, cfg config.Config, auditLog *audit.Log, clientTLS *tls.Config) {
	ticker := time.NewTicker(cfg.Policy.Audit.DNSTXTInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rootHash := auditLog.RootHash()
			ts := time.Now().Unix()
			for _, peer := range cfg.Network.Peers {
				dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				client, err := grpctransport.Dial(dialCtx, peer.Addr, clientTLS)
				cancel()
				if err != nil {
					log.Printf("[gossip] dial %s: %v", peer.Addr, err)
					continue
				}
				_, _ = client.ShareRootHash(ctx, &grpctransport.RootHashMessage{
					NodeID:    cfg.Node.NodeID,
					RootHash:  rootHash,
					Timestamp: ts,
				})
				_ = client.Close()
			}
		}
	}
}
