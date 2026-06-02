package policy

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func edifactRequest() Request {
	return Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "edifact",
	}
}

func TestEdifact_AcceptedOnRegionalNodeBalancedProfile(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for edifact on regional+balanced node, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEdifact_AcceptedOnAllianceNodeBalancedProfile(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "alliance"
	cfg.Node.Region = ""
	cfg.Node.Regions = []string{"RS", "RU"}
	cfg.Node.AllianceID = "TEST-ALLIANCE"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for edifact on alliance+balanced node, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEdifact_AcceptedOnRegionalNodeStrictProfile(t *testing.T) {
	// strict ≥ balanced — must not be blocked by the profile check.
	cfg := config.DefaultConfigForProfile("strict")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for edifact on regional+strict node, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEdifact_RejectedOnGlobalNodeByDefault(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for edifact on global node (default), got %q", result.Decision)
	}
	if result.Reason != ReasonEdifactNodeTierDenied {
		t.Fatalf("expected reason %q, got %q", ReasonEdifactNodeTierDenied, result.Reason)
	}
}

func TestEdifact_AllowedOnGlobalNodeWhenOperatorOverrides(t *testing.T) {
	// Operator explicitly defines a TOML rule that permits global tier.
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}
	cfg.Policy.JurisdictionIsolation.Rules = []config.JurisdictionIsolationRule{
		{
			SchemaType: "edifact",
			AllowTiers: []string{"regional", "alliance", "global"},
		},
	}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for edifact on global node with TOML override, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEdifact_RejectedOnPerformanceProfile(t *testing.T) {
	cfg := config.DefaultConfigForProfile("performance")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for edifact with performance profile, got %q", result.Decision)
	}
	if result.Reason != ReasonEdifactRequiresBalancedProfile {
		t.Fatalf("expected reason %q, got %q", ReasonEdifactRequiresBalancedProfile, result.Reason)
	}
}

func TestEdifact_NodeTierCheckedBeforeProfile(t *testing.T) {
	// Global node + performance profile: tier check fires first.
	cfg := config.DefaultConfigForProfile("performance")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(edifactRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY, got %q", result.Decision)
	}
	if result.Reason != ReasonEdifactNodeTierDenied {
		t.Fatalf("tier check must fire before profile check; got reason %q", result.Reason)
	}
}

func TestEdifact_NonEdifactSchemaUnaffected(t *testing.T) {
	// messaging on a global performance node must still ALLOW — edifact defaults
	// must not bleed into other schema types.
	cfg := config.DefaultConfigForProfile("performance")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "messaging",
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("messaging must not be affected by edifact defaults, got %q (%s)", result.Decision, result.Reason)
	}
}
