package policy

import (
	"slices"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/crl"
)

type Engine struct {
	cfg      config.Config
	audit    *audit.Log
	crlStore *crl.Store
}

type Decision string

const (
	DecisionAllow Decision = "ALLOW"
	DecisionDeny  Decision = "DENY"
)

const ReasonTrustTierBelowMinimum = "TRUST_TIER_BELOW_MINIMUM"

type Request struct {
	SenderNodeID    string
	SenderRegion    string
	RecipientRegion string
	TrustTier       uint32
}

type Result struct {
	Decision Decision
	Reason   string
	Profile  string
}

func NewEngine(cfg config.Config, auditLog *audit.Log, crlStore *crl.Store) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Engine{cfg: cfg, audit: auditLog, crlStore: crlStore}, nil
}

func (e *Engine) Evaluate(req Request) Result {
	if e.crlStore != nil && e.crlStore.IsRevoked(req.SenderNodeID) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "NODE_REVOKED",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	if req.TrustTier < e.cfg.Policy.Inbound.MinTrustTier {
		result := Result{
			Decision: DecisionDeny,
			Reason:   ReasonTrustTierBelowMinimum,
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	if slices.Contains(e.cfg.Policy.Outbound.DenyTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "RECIPIENT_REGION_DENIED",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	if len(e.cfg.Policy.Outbound.AllowTo) > 0 && !slices.Contains(e.cfg.Policy.Outbound.AllowTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "RECIPIENT_REGION_NOT_IN_ALLOW_LIST",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	result := Result{
		Decision: DecisionAllow,
		Reason:   "POLICY_ACCEPTED",
		Profile:  e.cfg.Profile.Name,
	}
	e.appendAudit(result, req)
	return result
}

func (e *Engine) appendAudit(result Result, req Request) {
	if e.audit == nil {
		return
	}

	e.audit.Append(e.cfg, audit.Decision(result.Decision), result.Reason, req.TrustTier, req.SenderRegion, req.RecipientRegion)
}
