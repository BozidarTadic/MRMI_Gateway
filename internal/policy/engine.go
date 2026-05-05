package policy

import (
	"slices"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
)

type Engine struct {
	cfg   config.Config
	audit *audit.Log
}

type Decision string

const (
	DecisionAllow Decision = "ALLOW"
	DecisionDeny  Decision = "DENY"
)

type Request struct {
	SenderRegion    string
	RecipientRegion string
	TrustTier       uint32
}

type Result struct {
	Decision Decision
	Reason   string
	Profile  string
}

func NewEngine(cfg config.Config, auditLog *audit.Log) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Engine{cfg: cfg, audit: auditLog}, nil
}

func (e *Engine) Evaluate(req Request) Result {
	if req.TrustTier < e.cfg.Policy.Inbound.MinTrustTier {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "trust tier below minimum",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	if slices.Contains(e.cfg.Policy.Outbound.DenyTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "recipient region denied by policy",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	if len(e.cfg.Policy.Outbound.AllowTo) > 0 && !slices.Contains(e.cfg.Policy.Outbound.AllowTo, req.RecipientRegion) {
		result := Result{
			Decision: DecisionDeny,
			Reason:   "recipient region not present in allow list",
			Profile:  e.cfg.Profile.Name,
		}
		e.appendAudit(result, req)
		return result
	}

	result := Result{
		Decision: DecisionAllow,
		Reason:   "policy accepted request",
		Profile:  e.cfg.Profile.Name,
	}
	e.appendAudit(result, req)
	return result
}

func (e *Engine) appendAudit(result Result, req Request) {
	if e.audit == nil {
		return
	}

	decision := audit.Decision(result.Decision)
	e.audit.Append(e.cfg, decision, req.SenderRegion, req.RecipientRegion)
}
