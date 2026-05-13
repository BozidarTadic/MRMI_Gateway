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
	"MRMI_Gateway/internal/connect"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/discovery"
	"MRMI_Gateway/internal/dnstxt"
	"MRMI_Gateway/internal/dummy"
	"MRMI_Gateway/internal/hotreload"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/inbox"
	"MRMI_Gateway/internal/metrics"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/peerdiscovery"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/registry"
	"MRMI_Gateway/internal/server"
	"MRMI_Gateway/internal/session"
	storebb "MRMI_Gateway/internal/store/bbolt"
	storeredis "MRMI_Gateway/internal/store/redis"
	"MRMI_Gateway/internal/ratelimit"
	"MRMI_Gateway/internal/tlsutil"
	"MRMI_Gateway/internal/token"
	"MRMI_Gateway/internal/transit"
	"MRMI_Gateway/internal/trustdecay"
	grpctransport "MRMI_Gateway/internal/transport/grpc"
	"MRMI_Gateway/internal/webhook"
)

// Run starts the gateway node. configPath is the path to the loaded config file;
// pass an empty string when config was loaded from defaults (hot-reload is skipped).
func Run(ctx context.Context, cfg config.Config, configPath string) error {
	signingKey, _, err := identity.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate signing key: %w", err)
	}
	log.Printf("[identity] using ephemeral Ed25519 signing key — set signing_key in [tls] for a persistent key")

	// Persistent store: bbolt, Redis, or nil (in-memory fallback).
	switch cfg.Storage.Backend {
	case "bbolt":
		dir := cfg.Storage.Path
		if dir == "" {
			dir = "/var/lib/mrmi"
		}
		s, err := storebb.Open(dir)
		if err != nil {
			return fmt.Errorf("open bbolt store: %w", err)
		}
		defer s.Close()
		log.Printf("[store] bbolt backend at %s/mrmi.db", dir)
		_ = s // store integration wired via NodeStore interface (future: inject into dedup/DLQ/CRL)
	case "redis":
		prefix := cfg.Storage.KeyPrefix
		if prefix == "" {
			prefix = "mrmi:"
		}
		s, err := storeredis.New(cfg.Storage.RedisURL, prefix)
		if err != nil {
			return fmt.Errorf("open redis store: %w", err)
		}
		defer s.Close()
		log.Printf("[store] redis backend at %s (prefix %s)", cfg.Storage.RedisURL, prefix)
		_ = s
	default:
		log.Printf("[store] using in-memory storage (no persistence)")
	}

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

	// Transit cache: buffer failed forwards briefly before DLQ (ADR §4.3).
	var tc *transit.Cache
	if cfg.Profile.TransitCacheTTL > 0 {
		tc = transit.New(cfg.Profile.TransitCacheTTL)
		log.Printf("[transit] cache enabled (TTL %s)", cfg.Profile.TransitCacheTTL)
		go runTransitRetry(ctx, tc, dlq, auditLog, cfg)
	}

	// Metrics: build registry with gauge readers for DLQ, transit cache, and peer registry.
	// peerRegistry is created later; use a stable pointer via closure.
	var peerRegistryRef *peerdiscovery.Registry
	metricsReg := metrics.New(
		dlq.Size,
		func() int {
			if tc != nil {
				return tc.Len()
			}
			return 0
		},
		func() int {
			if peerRegistryRef != nil {
				return len(peerRegistryRef.Known())
			}
			return 0
		},
	)

	if cfg.Network.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metricsReg.Handler())
		metricsSrv := &http.Server{Addr: cfg.Network.MetricsAddr, Handler: mux}
		go func() {
			log.Printf("[metrics] serving /metrics on %s", cfg.Network.MetricsAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("[metrics] server error: %v", err)
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsSrv.Shutdown(shutdownCtx)
		}()
	}

	var fwd core.Forwarder
	if len(cfg.Network.Peers) > 0 {
		fwd = delivery.NewForwarder(cfg, dlq, tc, func(ctx context.Context, addr string, env core.Envelope) (string, error) {
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
	gw.SetOnDeny(metricsReg.IncDeny)
	gw.SetOnDuplicate(metricsReg.IncDuplicate)

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
		metricsReg.IncAllow()
		msgInbox.Publish(inbox.Event{
			IdempotencyKey:  env.IdempotencyKey,
			SenderRegion:    env.SenderRegion,
			RecipientRegion: env.RecipientRegion,
			TrustTier:       env.TrustTier,
			Payload:         env.Payload,
			Timestamp:       env.Timestamp,
		})
	})

	// Webhook notifier: fire-and-forget POST to registered app URLs on ALLOW.
	if len(cfg.Apps) > 0 {
		gw.SetNotifier(webhook.New(cfg))
	}

	// Registry: user discovery and connect protocol.
	reg := registry.New(cfg)

	// Runtime peers: populated via POST /api/v1/peers/register.
	runtimePeers := server.NewRuntimePeers()

	// Runtime apps: registered via POST /api/v1/apps/register.
	runtimeApps := server.NewRuntimeApps()

	// Config reload callback: re-reads file and applies to engine.
	var onReload func() error
	var onConfigSave func(config.Config) error
	if configPath != "" {
		onReload = func() error {
			newCfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			return engine.Reload(newCfg)
		}
		onConfigSave = func(newCfg config.Config) error {
			if err := newCfg.Validate(); err != nil {
				return err
			}
			return engine.Reload(newCfg)
		}
	}

	httpServer := server.NewHTTPServer(cfg, server.ServerDeps{
		Engine:         engine,
		Audit:          auditLog,
		PrivKey:        signingKey,
		Peers:          peerCache,
		Gateway:        gw,
		DLQ:            dlq,
		CRL:            crlStore,
		Inbox:          msgInbox,
		Registry:       reg,
		RuntimePeers:   runtimePeers,
		RuntimeApps:    runtimeApps,
		OnConfigReload: onReload,
		OnConfigSave:   onConfigSave,
	})

	// Sprint 7: opaque token store + background purge.
	tokenStore := token.New()
	go runTokenPurge(ctx, tokenStore)

	// Sprint 7: discovery broadcaster — fan out BroadcastDiscovery to peers.
	var broadcaster *discovery.Broadcaster
	if len(cfg.Network.Peers) > 0 {
		peers := make(map[string]string, len(cfg.Network.Peers))
		for k, p := range cfg.Network.Peers {
			peers[k] = p.Addr
		}
		discoveryDedup := dedup.New(30 * time.Second)
		broadcaster = discovery.New(peers, discoveryDedup, func(dialCtx context.Context, addr string) (discovery.PeerClient, error) {
			c, err := grpctransport.Dial(dialCtx, addr, clientTLS)
			if err != nil {
				return nil, err
			}
			return c.AsDiscoveryClient(), nil
		})
	}

	// Sprint 7: connect resolver.
	connectRes := connect.New(cfg)

	// Sprint 8: dynamic peer discovery registry + gossip.
	peerRegistry := peerdiscovery.New()
	peerRegistryRef = peerRegistry // expose to metrics gauge closure
	gossipInterval := cfg.Network.PeerGossipInterval
	if gossipInterval <= 0 {
		gossipInterval = 120 * time.Second
	}
	// Seed registry with static peers from config.
	for _, p := range cfg.Network.Peers {
		peerRegistry.Announce(peerdiscovery.PeerInfo{
			NodeID:    p.Region, // use region as key for static peers
			Addr:      p.Addr,
			NodeScope: p.NodeScope,
			Region:    p.Region,
		})
	}
	go runPeerGossip(ctx, cfg, peerRegistry, clientTLS, gossipInterval)

	discoveryLimiter := ratelimit.New(10, 20) // 10 req/s, burst 20 per origin node
	defer discoveryLimiter.Close()

	discoveryDeps := grpctransport.DiscoveryDeps{
		TokenStore:       tokenStore,
		Broadcaster:      broadcaster,
		ConnectRes:       connectRes,
		PolicyEng:        engine,
		PeerRegistry:     peerRegistry,
		DiscoveryLimiter: discoveryLimiter,
		Metrics:          metricsReg,
		NodeCfg:          cfg,
	}

	var adapter grpctransport.GatewayService
	adapter = grpctransport.NewAdapterWithDiscovery(gw, nil, peerCache, discoveryDeps)

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

func runPeerGossip(ctx context.Context, cfg config.Config, reg *peerdiscovery.Registry, clientTLS *tls.Config, interval time.Duration) {
	staleAge := 5 * interval

	dialAndExchange := func(addr string) {
		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		client, err := grpctransport.Dial(dialCtx, addr, clientTLS)
		if err != nil {
			log.Printf("[gossip] dial %s: %v", addr, err)
			return
		}
		defer client.Close()

		known := reg.Known()
		peers := make([]grpctransport.PeerEntry, 0, len(known))
		for _, p := range known {
			peers = append(peers, grpctransport.PeerEntry{
				NodeID:    p.NodeID,
				Addr:      p.Addr,
				NodeScope: p.NodeScope,
				Region:    p.Region,
				LastSeen:  p.LastSeen.Unix(),
			})
		}
		resp, err := client.ExchangePeers(ctx, &grpctransport.PeerListRequest{
			SenderNodeID: cfg.Node.NodeID,
			KnownPeers:   peers,
		})
		if err != nil {
			log.Printf("[gossip] exchange peers %s: %v", addr, err)
			return
		}
		for _, p := range resp.Peers {
			reg.Announce(peerdiscovery.PeerInfo{
				NodeID:    p.NodeID,
				Addr:      p.Addr,
				NodeScope: p.NodeScope,
				Region:    p.Region,
			})
		}
	}

	// Bootstrap: dial seed nodes once at startup.
	for _, addr := range cfg.Network.BootstrapNodes {
		dialAndExchange(addr)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reg.EvictStale(staleAge)
			for _, p := range reg.Known() {
				go dialAndExchange(p.Addr)
			}
		}
	}
}

func runTokenPurge(ctx context.Context, s *token.Store) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Purge()
		}
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

func runTransitRetry(ctx context.Context, tc *transit.Cache, dlq *delivery.DLQ, _ *audit.Log, _ config.Config) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, e := range tc.Drain() {
				dlq.Append(delivery.DLQEntry{Envelope: e.Env, PeerAddr: e.PeerAddr})
			}
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
