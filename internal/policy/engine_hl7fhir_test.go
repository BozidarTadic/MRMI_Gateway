package policy

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func hl7fhirRequest() Request {
	return Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "hl7fhir",
	}
}

func TestHl7Fhir_AcceptedOnRegionalStrictNode(t *testing.T) {
	cfg := config.DefaultConfigForProfile("strict")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for hl7fhir on regional+strict node, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestHl7Fhir_RejectedOnGlobalNode(t *testing.T) {
	cfg := config.DefaultConfigForProfile("strict")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for hl7fhir on global node, got %q", result.Decision)
	}
	if result.Reason != ReasonHl7FhirNonRegionalNode {
		t.Fatalf("expected reason %q, got %q", ReasonHl7FhirNonRegionalNode, result.Reason)
	}
}

func TestHl7Fhir_RejectedOnAllianceNode(t *testing.T) {
	cfg := config.DefaultConfigForProfile("strict")
	cfg.Node.NodeScope = "alliance"
	cfg.Node.Region = ""
	cfg.Node.Regions = []string{"RS", "RU"}
	cfg.Node.AllianceID = "TEST-ALLIANCE"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for hl7fhir on alliance node, got %q", result.Decision)
	}
	if result.Reason != ReasonHl7FhirNonRegionalNode {
		t.Fatalf("expected reason %q, got %q", ReasonHl7FhirNonRegionalNode, result.Reason)
	}
}

func TestHl7Fhir_RejectedOnBalancedProfile(t *testing.T) {
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for hl7fhir with balanced profile, got %q", result.Decision)
	}
	if result.Reason != ReasonHl7FhirRequiresStrictProfile {
		t.Fatalf("expected reason %q, got %q", ReasonHl7FhirRequiresStrictProfile, result.Reason)
	}
}

func TestHl7Fhir_RejectedOnPerformanceProfile(t *testing.T) {
	cfg := config.DefaultConfigForProfile("performance")
	cfg.Node.NodeScope = "regional"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for hl7fhir with performance profile, got %q", result.Decision)
	}
	if result.Reason != ReasonHl7FhirRequiresStrictProfile {
		t.Fatalf("expected reason %q, got %q", ReasonHl7FhirRequiresStrictProfile, result.Reason)
	}
}

func TestHl7Fhir_NodeScopeCheckedBeforeProfile(t *testing.T) {
	// Global node + balanced profile: node scope check fires first.
	cfg := config.DefaultConfigForProfile("balanced")
	cfg.Node.NodeScope = "global"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)
	result := engine.Evaluate(hl7fhirRequest())

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY, got %q", result.Decision)
	}
	if result.Reason != ReasonHl7FhirNonRegionalNode {
		t.Fatalf("node scope check must fire before profile check; got reason %q", result.Reason)
	}
}

func TestHl7Fhir_AdditionalTomlRulesStillApply(t *testing.T) {
	// Regional + strict node satisfies hl7fhir constraints, but a TOML
	// jurisdiction isolation rule further restricts the allowed jurisdictions.
	cfg := config.DefaultConfigForProfile("strict")
	cfg.Node.NodeScope = "regional"
	cfg.Node.Region = "RS"
	cfg.Policy.Outbound.AllowTo = []string{"RU", "DE"}
	cfg.Policy.JurisdictionIsolation.Rules = []config.JurisdictionIsolationRule{
		{
			SchemaType:         "hl7fhir",
			AllowJurisdictions: []string{"RS", "RU"}, // DE not in list
		},
	}

	engine, _ := newEngine(t, cfg)

	// RU is in the TOML allow list → ALLOW.
	if r := engine.Evaluate(hl7fhirRequest()); r.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for RU in hl7fhir jurisdiction list, got %q (%s)", r.Decision, r.Reason)
	}

	// DE is allowed by outbound routing but not by the hl7fhir TOML rule → DENY.
	restricted := hl7fhirRequest()
	restricted.RecipientRegion = "DE"
	if r := engine.Evaluate(restricted); r.Decision != DecisionDeny {
		t.Fatalf("expected DENY for DE not in hl7fhir jurisdiction list, got %q", r.Decision)
	}
}

func TestHl7Fhir_NonHealthSchemaUnaffected(t *testing.T) {
	// Messaging on a global non-strict node must still ALLOW — hl7fhir constraints
	// must not bleed into other schema types.
	cfg := config.DefaultConfigForProfile("balanced")
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
		t.Fatalf("messaging on global node must not be affected by hl7fhir constraints, got %q (%s)", result.Decision, result.Reason)
	}
}
