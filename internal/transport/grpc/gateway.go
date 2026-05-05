package grpctransport

import (
	"context"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/policy"
)

type Gateway struct {
	cfg    config.Config
	policy *policy.Engine
	audit  *audit.Log
}

func NewGateway(cfg config.Config, policyEngine *policy.Engine, auditLog *audit.Log) *Gateway {
	return &Gateway{
		cfg:    cfg,
		policy: policyEngine,
		audit:  auditLog,
	}
}

func (g *Gateway) SendEnvelope(ctx context.Context, request *SendEnvelopeRequest) (*SendEnvelopeResponse, error) {
	_ = ctx

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
