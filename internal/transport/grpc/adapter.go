package grpctransport

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/connect"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/discovery"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/peerdiscovery"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/session"
	"MRMI_Gateway/internal/token"
)

// gatewayAdapter implements GatewayService by delegating to core.Gateway.
// It is the only place that translates between gRPC transport types and domain types.
type gatewayAdapter struct {
	gw            *core.Gateway
	seqRecv       *session.Tracker
	verifyKey     ed25519.PublicKey      // nil = skip verification (insecure mode)
	peerCache     *peercache.Cache       // nil = no gossip storage
	tokenStore    *token.Store           // nil = discovery/connect disabled
	broadcaster   *discovery.Broadcaster // nil = no peer fan-out
	connectRes    *connect.Resolver      // nil = always PENDING
	policyEng     *policy.Engine         // nil = no isolation check
	peerRegistry  *peerdiscovery.Registry // nil = no dynamic peer discovery
	nodeCfg       config.Config
}

// NewAdapter wraps a core.Gateway so it satisfies the GatewayService interface.
func NewAdapter(gw *core.Gateway) GatewayService {
	return &gatewayAdapter{gw: gw, seqRecv: session.New()}
}

// NewAdapterWithVerify wraps a core.Gateway with a public key for signature verification.
func NewAdapterWithVerify(gw *core.Gateway, verifyKey ed25519.PublicKey) GatewayService {
	return &gatewayAdapter{gw: gw, seqRecv: session.New(), verifyKey: verifyKey}
}

// NewAdapterFull wraps a core.Gateway with optional signature verification and
// an optional peer cache for root hash gossip storage.
func NewAdapterFull(gw *core.Gateway, verifyKey ed25519.PublicKey, peerCache *peercache.Cache) GatewayService {
	return &gatewayAdapter{gw: gw, seqRecv: session.New(), verifyKey: verifyKey, peerCache: peerCache}
}

// DiscoveryDeps groups the Sprint-7/8 dependencies for discovery, connect, and peer exchange.
type DiscoveryDeps struct {
	TokenStore   *token.Store
	Broadcaster  *discovery.Broadcaster
	ConnectRes   *connect.Resolver
	PolicyEng    *policy.Engine
	PeerRegistry *peerdiscovery.Registry
	NodeCfg      config.Config
}

// NewAdapterWithDiscovery wraps a core.Gateway with all Sprint-7 discovery/connect
// dependencies in addition to the standard gossip cache.
func NewAdapterWithDiscovery(gw *core.Gateway, verifyKey ed25519.PublicKey, peerCache *peercache.Cache, deps DiscoveryDeps) GatewayService {
	return &gatewayAdapter{
		gw:           gw,
		seqRecv:      session.New(),
		verifyKey:    verifyKey,
		peerCache:    peerCache,
		tokenStore:   deps.TokenStore,
		broadcaster:  deps.Broadcaster,
		connectRes:   deps.ConnectRes,
		policyEng:    deps.PolicyEng,
		peerRegistry: deps.PeerRegistry,
		nodeCfg:      deps.NodeCfg,
	}
}

func (a *gatewayAdapter) SendEnvelope(ctx context.Context, req *SendEnvelopeRequest) (*SendEnvelopeResponse, error) {
	if req.Envelope.SequenceNumber > 0 {
		if err := a.seqRecv.Validate(req.Envelope.SenderRegion, req.Envelope.SequenceNumber); err != nil {
			log.Printf("[session] %v", err)
		}
	}

	if a.verifyKey != nil {
		env := core.Envelope{
			IdempotencyKey:  req.Envelope.IdempotencyKey,
			SenderRegion:    req.Envelope.SenderRegion,
			RecipientRegion: req.Envelope.RecipientRegion,
			TrustTier:       req.Envelope.TrustTier,
			SequenceNumber:  req.Envelope.SequenceNumber,
			Payload:         req.Envelope.Payload,
			PaddedTo:        req.Envelope.PaddedTo,
			Timestamp:       req.Envelope.Timestamp,
		}
		if err := identity.Verify(a.verifyKey, env, req.Envelope.Signature); err != nil {
			return &SendEnvelopeResponse{
				Decision: "DENY",
				Reason:   "INVALID_SIGNATURE",
			}, nil
		}
	}

	resp, err := a.gw.SendEnvelope(ctx, core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:    req.Envelope.IdempotencyKey,
			SenderNodeID:      req.Envelope.SenderNodeID,
			SenderIdentity:    req.Envelope.SenderIdentity,
			RecipientIdentity: req.Envelope.RecipientIdentity,
			SenderRegion:      req.Envelope.SenderRegion,
			RecipientRegion:   req.Envelope.RecipientRegion,
			TrustTier:         req.Envelope.TrustTier,
			SequenceNumber:    req.Envelope.SequenceNumber,
			Payload:           req.Envelope.Payload,
			PaddedTo:          req.Envelope.PaddedTo,
			Timestamp:         req.Envelope.Timestamp,
			Signature:         req.Envelope.Signature,
			IsDummy:           req.Envelope.IsDummy,
		},
	})
	if err != nil {
		if errors.Is(err, core.ErrEmptyIdempotencyKey) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &SendEnvelopeResponse{
		Decision:          string(resp.Decision),
		Reason:            resp.Reason,
		Profile:           resp.Profile,
		NodeID:            resp.NodeID,
		AuditRootHash:     resp.AuditRootHash,
		PeerAuditRootHash: resp.PeerAuditRootHash,
	}, nil
}

func (a *gatewayAdapter) GetNodeInfo(ctx context.Context, req *GetNodeInfoRequest) (*GetNodeInfoResponse, error) {
	info, err := a.gw.GetNodeInfo(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &GetNodeInfoResponse{
		NodeID:        info.NodeID,
		NodeScope:     info.NodeScope,
		Region:        info.Region,
		ApplicableLaw: info.ApplicableLaw,
		Profile:       info.Profile,
	}, nil
}

func (a *gatewayAdapter) ShareRootHash(_ context.Context, req *RootHashMessage) (*RootHashAck, error) {
	if a.peerCache != nil {
		a.peerCache.Store(req.NodeID, req.RootHash, req.Timestamp)
	}
	return &RootHashAck{Accepted: true}, nil
}

func (a *gatewayAdapter) BroadcastDiscovery(ctx context.Context, req *DiscoveryRequest) (*DiscoveryResponse, error) {
	if a.tokenStore == nil {
		return &DiscoveryResponse{}, nil
	}

	// Enforce app isolation policy before searching local registry.
	if a.policyEng != nil {
		// Find the first app_id this node serves and evaluate isolation.
		var nodeAppID string
		for appID := range a.nodeCfg.Apps {
			nodeAppID = appID
			break
		}
		result := a.policyEng.EvaluateDiscovery(policy.DiscoveryRequest{
			OriginAppID: req.OriginAppID,
			NodeAppID:   nodeAppID,
		})
		if result.Decision == policy.DecisionDeny {
			return &DiscoveryResponse{}, nil
		}
	}

	// Fan out to peers via the broadcaster (handles hop_limit, dedup, staleness).
	if a.broadcaster != nil {
		dreq := discovery.Request{
			QueryHash:    req.QueryHash,
			QueryType:    req.QueryType,
			OriginNodeID: req.OriginNodeID,
			OriginAppID:  req.OriginAppID,
			HopLimit:     req.HopLimit,
			RequestID:    req.RequestID,
			Timestamp:    req.Timestamp,
		}
		forwarded := a.broadcaster.Broadcast(ctx, dreq)
		if len(forwarded) > 0 {
			// Return the first peer match; the HTTP discovery API aggregates all.
			r := forwarded[0]
			return &DiscoveryResponse{
				NodeID:       r.NodeID,
				AppID:        r.AppID,
				OpaqueToken:  r.OpaqueToken,
				DisplayHint:  r.DisplayHint,
				MatchType:    r.MatchType,
				TokenExpires: r.TokenExpires,
			}, nil
		}
	}

	// Search this node's own registered users.
	for appID, app := range a.nodeCfg.Apps {
		for _, u := range app.Users {
			// Simple exact match on display_hint for now; query_hash matching
			// requires the App to pre-hash its identifier — left to the App layer.
			ttl := a.nodeCfg.Node.DiscoveryTokenTTL
			if ttl <= 0 {
				ttl = 5 * time.Minute
			}
			tok, err := a.tokenStore.Issue(appID, req.OriginNodeID, ttl)
			if err != nil {
				log.Printf("[discovery] issue token: %v", err)
				continue
			}
			expires := time.Now().Add(ttl).UnixMilli()
			return &DiscoveryResponse{
				NodeID:       a.nodeCfg.Node.NodeID,
				AppID:        appID,
				OpaqueToken:  tok,
				DisplayHint:  u.DisplayHint,
				MatchType:    "exact",
				TokenExpires: expires,
			}, nil
		}
	}

	return &DiscoveryResponse{}, nil
}

func (a *gatewayAdapter) Connect(_ context.Context, req *ConnectRequest) (*ConnectAck, error) {
	if a.tokenStore == nil {
		return &ConnectAck{Status: string(connect.StatusDenied), Reason: "discovery not enabled"}, nil
	}

	appID, originNodeID, err := a.tokenStore.Resolve(req.OpaqueToken)
	if err != nil {
		return &ConnectAck{Status: string(connect.StatusDenied), Reason: "invalid or expired token"}, nil
	}

	// Verify the token was issued for this requester.
	if originNodeID != req.OriginNodeID {
		return &ConnectAck{Status: string(connect.StatusDenied), Reason: "token not issued for this node"}, nil
	}

	a.tokenStore.Evict(req.OpaqueToken)

	var resolvedStatus connect.Status
	if a.connectRes != nil {
		mutual := a.tokenStore.HasActiveToken(req.OriginNodeID)
		resolvedStatus = a.connectRes.Resolve(connect.Request{
			OriginNodeID:    req.OriginNodeID,
			OriginAppID:     req.OriginAppID,
			RecipientAppID:  appID,
			MutualDiscovery: mutual,
		})
	} else {
		resolvedStatus = connect.StatusPending
	}

	ack := &ConnectAck{Status: string(resolvedStatus)}
	if resolvedStatus == connect.StatusAccepted {
		ack.SessionID = uuid.NewString()
		ack.ExpiresAt = time.Now().Add(24 * time.Hour).UnixMilli()
	}
	return ack, nil
}

func (a *gatewayAdapter) ExchangePeers(_ context.Context, req *PeerListRequest) (*PeerListResponse, error) {
	if a.peerRegistry != nil {
		// Merge the sender's known peers into our registry.
		for _, p := range req.KnownPeers {
			a.peerRegistry.Announce(peerdiscovery.PeerInfo{
				NodeID:    p.NodeID,
				Addr:      p.Addr,
				NodeScope: p.NodeScope,
				Region:    p.Region,
			})
		}
		// Also record the sender itself.
		if req.SenderNodeID != "" && req.SenderNodeID != a.nodeCfg.Node.NodeID {
			for _, p := range req.KnownPeers {
				if p.NodeID == req.SenderNodeID {
					a.peerRegistry.Announce(peerdiscovery.PeerInfo{
						NodeID:    p.NodeID,
						Addr:      p.Addr,
						NodeScope: p.NodeScope,
						Region:    p.Region,
					})
				}
			}
		}
	}

	// Return our known peer list.
	var peers []PeerEntry
	if a.peerRegistry != nil {
		for _, p := range a.peerRegistry.Known() {
			peers = append(peers, PeerEntry{
				NodeID:    p.NodeID,
				Addr:      p.Addr,
				NodeScope: p.NodeScope,
				Region:    p.Region,
				LastSeen:  p.LastSeen.Unix(),
			})
		}
	}
	return &PeerListResponse{Peers: peers}, nil
}
