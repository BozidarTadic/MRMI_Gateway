package core

import (
	"context"
	"errors"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

// ErrEmptyIdempotencyKey is returned when SendEnvelope receives a request with no idempotency key.
var ErrEmptyIdempotencyKey = errors.New("idempotency_key is required")

type Decision string

const (
	DecisionAllow     Decision = "ALLOW"
	DecisionDeny      Decision = "DENY"
	DecisionDuplicate Decision = "DUPLICATE"
)

type Envelope struct {
	IdempotencyKey    string
	SenderIdentity    []byte
	RecipientIdentity []byte
	SenderRegion      string
	RecipientRegion   string
	TrustTier         uint32
	SequenceNumber    uint64
	Payload           []byte
	PaddedTo          uint32
	Timestamp         int64
	Signature         []byte
}

type SendRequest struct {
	Envelope Envelope
}

type SendResponse struct {
	Decision      Decision
	Reason        string
	Profile       string
	NodeID        string
	AuditRootHash string
}

type NodeInfo struct {
	NodeID        string
	Region        string
	ApplicableLaw string
	Profile       string
}

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

func (g *Gateway) SendEnvelope(_ context.Context, req SendRequest) (SendResponse, error) {
	if req.Envelope.IdempotencyKey == "" {
		return SendResponse{}, ErrEmptyIdempotencyKey
	}

	if g.dedup.SeenOrAdd(req.Envelope.IdempotencyKey) {
		g.audit.Append(g.cfg, audit.DecisionDuplicate,
			req.Envelope.SenderRegion, req.Envelope.RecipientRegion)
		return SendResponse{
			Decision:      DecisionDuplicate,
			Reason:        "idempotency_key already processed",
			Profile:       g.cfg.Profile.Name,
			NodeID:        g.cfg.Node.NodeID,
			AuditRootHash: g.audit.RootHash(),
		}, nil
	}

	result := g.policy.Evaluate(policy.Request{
		SenderRegion:    req.Envelope.SenderRegion,
		RecipientRegion: req.Envelope.RecipientRegion,
		TrustTier:       req.Envelope.TrustTier,
	})

	return SendResponse{
		Decision:      Decision(result.Decision),
		Reason:        result.Reason,
		Profile:       result.Profile,
		NodeID:        g.cfg.Node.NodeID,
		AuditRootHash: g.audit.RootHash(),
	}, nil
}

func (g *Gateway) GetNodeInfo(_ context.Context) (NodeInfo, error) {
	return NodeInfo{
		NodeID:        g.cfg.Node.NodeID,
		Region:        g.cfg.Node.Region,
		ApplicableLaw: g.cfg.Node.ApplicableLaw,
		Profile:       g.cfg.Profile.Name,
	}, nil
}
