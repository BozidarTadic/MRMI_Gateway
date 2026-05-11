package policy

import (
	"slices"
	"sync/atomic"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/crl"
)

type Engine struct {
	cfg      atomic.Pointer[config.Config]
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
	e := &Engine{audit: auditLog, crlStore: crlStore}
	e.cfg.Store(&cfg)
	return e, nil
}

// Reload atomically replaces the policy config. Returns an error if the new
// config fails validation; the old config is preserved on failure.
func (e *Engine) Reload(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	e.cfg.Store(&cfg)
	return nil
}

func (e *Engine) Evaluate(req Request) Result {
	cfg := *e.cfg.Load()

	if e.crlStore != nil && e.crlStore.IsRevoked(req.SenderNodeID) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "NODE_REVOKED",
			Profile:  cfg.Profile.Name,
		}
		e.appendAudit(cfg, result, req)
		return result
	}

	if req.TrustTier < cfg.Policy.Inbound.MinTrustTier {
		result := Result{
			Decision: DecisionDeny,
			Reason:   ReasonTrustTierBelowMinimum,
			Profile:  cfg.Profile.Name,
		}
		e.appendAudit(cfg, result, req)
		return result
	}

	if slices.Contains(cfg.Policy.Outbound.DenyTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "RECIPIENT_REGION_DENIED",
			Profile:  cfg.Profile.Name,
		}
		e.appendAudit(cfg, result, req)
		return result
	}

	if len(cfg.Policy.Outbound.AllowTo) > 0 && !slices.Contains(cfg.Policy.Outbound.AllowTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "RECIPIENT_REGION_NOT_IN_ALLOW_LIST",
			Profile:  cfg.Profile.Name,
		}
		e.appendAudit(cfg, result, req)
		return result
	}

	result := Result{
		Decision: DecisionAllow,
		Reason:   "POLICY_ACCEPTED",
		Profile:  cfg.Profile.Name,
	}
	e.appendAudit(cfg, result, req)
	return result
}

func (e *Engine) appendAudit(cfg config.Config, result Result, req Request) {
	if e.audit == nil {
		return
	}
	e.audit.Append(cfg, audit.Decision(result.Decision), result.Reason, req.TrustTier, req.SenderRegion, req.RecipientRegion)
}
