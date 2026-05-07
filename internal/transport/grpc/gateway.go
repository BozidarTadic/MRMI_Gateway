package grpctransport

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

type Gateway struct {
	cfg    config.Config
	policy *policy.Engine
	audit  *audit.Log
	dedup  *dedup.Index
}

func NewGateway(cfg config.Config, policyEngine *policy.Engine, auditLog *audit.Log, dedupIndex *dedup.Index) *Gateway {
	return &Gateway{
		cfg:    cfg,
		policy: policyEngine,
		audit:  auditLog,
		dedup:  dedupIndex,
	}
}

func (g *Gateway) SendEnvelope(ctx context.Context, request *SendEnvelopeRequest) (*SendEnvelopeResponse, error) {
	_ = ctx

	if request.Envelope.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	if g.dedup.SeenOrAdd(request.Envelope.IdempotencyKey) {
		g.audit.Append(g.cfg, audit.DecisionDuplicate,
			request.Envelope.SenderRegion, request.Envelope.RecipientRegion)
		return &SendEnvelopeResponse{
			Decision:      "DUPLICATE",
			Reason:        "idempotency_key already processed",
			Profile:       g.cfg.Profile.Name,
			NodeID:        g.cfg.Node.NodeID,
			AuditRootHash: g.audit.RootHash(),
		}, nil
	}

	result := g.policy.Evaluate(policy.Request{
		SenderRegion:    request.Envelope.SenderRegion,
		RecipientRegion: request.Envelope.RecipientRegion,
		TrustTier:       request.Envelope.TrustTier,
	})

	return &SendEnvelopeResponse{
		Decision:      string(result.Decision),
		Reason:        result.Reason,
		Profile:       result.Profile,
		NodeID:        g.cfg.Node.NodeID,
		AuditRootHash: g.audit.RootHash(),
	}, nil
}

func (g *Gateway) GetNodeInfo(ctx context.Context, request *GetNodeInfoRequest) (*GetNodeInfoResponse, error) {
	_, _ = ctx, request
	return &GetNodeInfoResponse{
		NodeID:        g.cfg.Node.NodeID,
		Region:        g.cfg.Node.Region,
		ApplicableLaw: g.cfg.Node.ApplicableLaw,
		Profile:       g.cfg.Profile.Name,
	}, nil
}
