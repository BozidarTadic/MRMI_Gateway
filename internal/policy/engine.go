package policy

import (
	"slices"
	"strings"
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
const ReasonAppIsolationViolation = "APP_ISOLATION_VIOLATION"

// DiscoveryRequest is the input to EvaluateDiscovery.
type DiscoveryRequest struct {
	OriginAppID string // app_id from the incoming BroadcastDiscovery request
	NodeAppID   string // app_id this node serves (from config.Apps key)
}

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

	// allow_to is an outbound routing constraint; skip it when the envelope is
	// already at its final destination (recipient == this node's region).
	isLocalDestination := cfg.Node.Region != "" && req.RecipientRegion == cfg.Node.Region
	if !isLocalDestination && len(cfg.Policy.Outbound.AllowTo) > 0 && !slices.Contains(cfg.Policy.Outbound.AllowTo, req.RecipientRegion) {
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

// EvaluateDiscovery enforces app_id isolation policy before forwarding a
// BroadcastDiscovery request.
func (e *Engine) EvaluateDiscovery(req DiscoveryRequest) Result {
	cfg := *e.cfg.Load()
	isolation := strings.ToUpper(strings.TrimSpace(cfg.Policy.Discovery.AppIsolation))
	if isolation == "" {
		isolation = "SAME_APP_ONLY"
	}

	var allowed bool
	switch isolation {
	case "OPEN":
		allowed = true
	case "WHITELIST":
		allowed = slices.Contains(cfg.Policy.Discovery.AllowedAppIDs, req.OriginAppID)
	default: // SAME_APP_ONLY
		allowed = strings.EqualFold(req.OriginAppID, req.NodeAppID)
	}

	if !allowed {
		return Result{
			Decision: DecisionDeny,
			Reason:   ReasonAppIsolationViolation,
			Profile:  cfg.Profile.Name,
		}
	}
	return Result{
		Decision: DecisionAllow,
		Reason:   "DISCOVERY_POLICY_ACCEPTED",
		Profile:  cfg.Profile.Name,
	}
}

func (e *Engine) appendAudit(cfg config.Config, result Result, req Request) {
	if e.audit == nil {
		return
	}
	e.audit.Append(cfg, audit.Decision(result.Decision), result.Reason, req.TrustTier, req.SenderRegion, req.RecipientRegion)
}
