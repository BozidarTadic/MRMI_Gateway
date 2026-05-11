package grpctransport

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/peercache"
	"MRMI_Gateway/internal/session"
)

// gatewayAdapter implements GatewayService by delegating to core.Gateway.
// It is the only place that translates between gRPC transport types and domain types.
type gatewayAdapter struct {
	gw        *core.Gateway
	seqRecv   *session.Tracker
	verifyKey ed25519.PublicKey // nil = skip verification (insecure mode)
	peerCache *peercache.Cache  // nil = no gossip storage
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
