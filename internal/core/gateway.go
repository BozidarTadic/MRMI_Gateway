package core

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
)

// ErrEmptyIdempotencyKey is returned when SendEnvelope receives a request with no idempotency key.
var ErrEmptyIdempotencyKey = errors.New("idempotency_key is required")

// Forwarder delivers an envelope to a peer node chosen by tier-preference routing.
// Implementations must handle DLQ writes internally on exhausted retries.
type Forwarder interface {
	Forward(ctx context.Context, env Envelope) (addr string, err error)
}

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
	NodeScope     string
	Region        string
	ApplicableLaw string
	Profile       string
}

type Gateway struct {
	cfg       config.Config
	policy    *policy.Engine
	audit     *audit.Log
	dedup     *dedup.Index
	forwarder Forwarder // nil = forwarding disabled
}

// NewGateway creates a Gateway. Pass nil for forwarder to disable outbound forwarding
// (policy evaluation and audit still run normally).
func NewGateway(cfg config.Config, policyEngine *policy.Engine, auditLog *audit.Log, dedupIndex *dedup.Index, forwarder Forwarder) *Gateway {
	return &Gateway{
		cfg:       cfg,
		policy:    policyEngine,
		audit:     auditLog,
		dedup:     dedupIndex,
		forwarder: forwarder,
	}
}

func (g *Gateway) SendEnvelope(ctx context.Context, req SendRequest) (SendResponse, error) {
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

	if result.Decision == policy.DecisionAllow && g.forwarder != nil {
		if err := applyJitter(ctx, g.cfg.Profile.TimingJitterMax); err == nil {
			env := applyPadding(req.Envelope, g.cfg.Profile.PaddingBucket)
			_, _ = g.forwarder.Forward(ctx, env)
		}
	}

	return SendResponse{
		Decision:      Decision(result.Decision),
		Reason:        result.Reason,
		Profile:       result.Profile,
		NodeID:        g.cfg.Node.NodeID,
		AuditRootHash: g.audit.RootHash(),
	}, nil
}

// applyJitter sleeps a random duration in [0, max) before forwarding.
// Returns ctx.Err() if the context is cancelled during the sleep, which
// signals the caller to skip forwarding for this request.
func applyJitter(ctx context.Context, max time.Duration) error {
	if max <= 0 {
		return nil
	}
	delay := time.Duration(rand.Int64N(int64(max)))
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// applyPadding rounds the envelope payload up to the nearest bucket boundary
// and sets PaddedTo accordingly. Returns the envelope unchanged when bucket
// is zero (performance profile) or the payload is empty.
func applyPadding(env Envelope, bucket int) Envelope {
	if bucket <= 0 || len(env.Payload) == 0 {
		return env
	}
	n := len(env.Payload)
	target := ((n + bucket - 1) / bucket) * bucket
	padded := make([]byte, target)
	copy(padded, env.Payload)
	env.Payload = padded
	env.PaddedTo = uint32(target)
	return env
}

func (g *Gateway) GetNodeInfo(_ context.Context) (NodeInfo, error) {
	return NodeInfo{
		NodeID:        g.cfg.Node.NodeID,
		NodeScope:     g.cfg.Node.NodeScope,
		Region:        g.cfg.Node.Region,
		ApplicableLaw: g.cfg.Node.ApplicableLaw,
		Profile:       g.cfg.Profile.Name,
	}, nil
}
