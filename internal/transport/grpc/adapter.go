package grpctransport

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"MRMI_Gateway/internal/core"
)

// gatewayAdapter implements GatewayService by delegating to core.Gateway.
// It is the only place that translates between gRPC transport types and domain types.
type gatewayAdapter struct {
	gw *core.Gateway
}

// NewAdapter wraps a core.Gateway so it satisfies the GatewayService interface.
func NewAdapter(gw *core.Gateway) GatewayService {
	return &gatewayAdapter{gw: gw}
}

func (a *gatewayAdapter) SendEnvelope(ctx context.Context, req *SendEnvelopeRequest) (*SendEnvelopeResponse, error) {
	resp, err := a.gw.SendEnvelope(ctx, core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:    req.Envelope.IdempotencyKey,
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
		},
	})
	if err != nil {
		if errors.Is(err, core.ErrEmptyIdempotencyKey) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &SendEnvelopeResponse{
		Decision:      string(resp.Decision),
		Reason:        resp.Reason,
		Profile:       resp.Profile,
		NodeID:        resp.NodeID,
		AuditRootHash: resp.AuditRootHash,
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
