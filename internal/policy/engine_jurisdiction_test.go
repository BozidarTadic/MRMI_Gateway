package policy

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func jurisdictionConfig() config.Config {
	cfg := baseConfig()
	cfg.Node.NodeScope = "regional"
	cfg.Node.Region = "RS"
	cfg.Policy.Outbound.AllowTo = []string{"RU", "RS", "BY", "KZ", "AM"}
	cfg.Policy.JurisdictionIsolation.Rules = []config.JurisdictionIsolationRule{
		{
			SchemaType:         "iso20022",
			AllowTiers:         []string{"regional"},
			AllowJurisdictions: []string{"RU", "RS", "BY"},
			RequireProfile:     "strict",
		},
		{
			SchemaType: "messaging",
			AllowTiers: []string{"regional", "alliance", "global"},
		},
		{
			SchemaType: "custom:gov-rs-doc-exchange",
			AllowTiers: []string{"regional"},
		},
	}
	cfg.Profile.Name = "strict"
	return cfg
}

func TestJurisdictionIsolation_ISO20022_AllowedOnRegionalNode(t *testing.T) {
	engine, _ := newEngine(t, jurisdictionConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "iso20022",
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for iso20022 on regional node in allowed jurisdictions, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestJurisdictionIsolation_ISO20022_BlockedOnAllianceNode(t *testing.T) {
	cfg := jurisdictionConfig()
	cfg.Node.NodeScope = "alliance"
	cfg.Node.Region = ""
	cfg.Node.Regions = []string{"RU", "RS"}
	cfg.Node.AllianceID = "TEST-ALLIANCE"

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "iso20022",
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for iso20022 on alliance node, got %q", result.Decision)
	}
	if result.Reason != ReasonJurisdictionIsolationTier {
		t.Fatalf("expected reason %q, got %q", ReasonJurisdictionIsolationTier, result.Reason)
	}
}

func TestJurisdictionIsolation_ISO20022_BlockedOnGlobalNode(t *testing.T) {
	cfg := jurisdictionConfig()
	cfg.Node.NodeScope = "global"

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "iso20022",
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for iso20022 on global node, got %q", result.Decision)
	}
	if result.Reason != ReasonJurisdictionIsolationTier {
		t.Fatalf("expected reason %q, got %q", ReasonJurisdictionIsolationTier, result.Reason)
	}
}

func TestJurisdictionIsolation_ISO20022_BlockedOutsideAllowedJurisdictions(t *testing.T) {
	engine, _ := newEngine(t, jurisdictionConfig())

	// KZ is in outbound.AllowTo but not in iso20022 allow_jurisdictions — reaches isolation check.
	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "KZ",
		TrustTier:       0,
		SchemaType:      "iso20022",
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for iso20022 to non-allowed jurisdiction, got %q", result.Decision)
	}
	if result.Reason != ReasonJurisdictionIsolationRegion {
		t.Fatalf("expected reason %q, got %q", ReasonJurisdictionIsolationRegion, result.Reason)
	}
}

func TestJurisdictionIsolation_ISO20022_BlockedWhenProfileMismatch(t *testing.T) {
	cfg := jurisdictionConfig()
	cfg.Profile.Name = "balanced" // rule requires "strict"

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "iso20022",
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for iso20022 with wrong profile, got %q", result.Decision)
	}
	if result.Reason != ReasonJurisdictionIsolationProfile {
		t.Fatalf("expected reason %q, got %q", ReasonJurisdictionIsolationProfile, result.Reason)
	}
}

func TestJurisdictionIsolation_Messaging_UnrestrictedOnAnyTier(t *testing.T) {
	for _, scope := range []string{"regional", "alliance", "global"} {
		cfg := jurisdictionConfig()
		cfg.Node.NodeScope = scope
		if scope == "alliance" {
			cfg.Node.Region = ""
			cfg.Node.Regions = []string{"RU", "RS"}
			cfg.Node.AllianceID = "TEST-ALLIANCE"
		} else {
			cfg.Node.Region = "RS"
		}

		engine, _ := newEngine(t, cfg)

		result := engine.Evaluate(Request{
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			TrustTier:       0,
			SchemaType:      "messaging",
		})

		if result.Decision != DecisionAllow {
			t.Fatalf("scope %q: expected ALLOW for messaging, got %q (%s)", scope, result.Decision, result.Reason)
		}
	}
}

func TestJurisdictionIsolation_EmptySchemaType_TreatedAsMessaging(t *testing.T) {
	engine, _ := newEngine(t, jurisdictionConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "", // should default to "messaging"
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for empty schema_type (treated as messaging), got %q (%s)", result.Decision, result.Reason)
	}
}

func TestJurisdictionIsolation_CustomAdapter_RegionalOnly(t *testing.T) {
	engine, _ := newEngine(t, jurisdictionConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "custom:gov-rs-doc-exchange",
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for custom adapter on regional node, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestJurisdictionIsolation_CustomAdapter_BlockedOnGlobalNode(t *testing.T) {
	cfg := jurisdictionConfig()
	cfg.Node.NodeScope = "global"

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "custom:gov-rs-doc-exchange",
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for custom adapter on global node, got %q", result.Decision)
	}
	if result.Reason != ReasonJurisdictionIsolationTier {
		t.Fatalf("expected reason %q, got %q", ReasonJurisdictionIsolationTier, result.Reason)
	}
}

func TestJurisdictionIsolation_NoRules_AllowsNonHealthDomains(t *testing.T) {
	cfg := baseConfig()
	cfg.Node.NodeScope = "global"

	engine, _ := newEngine(t, cfg)

	// hl7fhir is excluded: it has ADR-015 hardcoded constraints (regional+strict only)
	// and is covered by TestHl7Fhir_* tests.
	for _, schemaType := range []string{"iso20022", "messaging", "custom:anything"} {
		result := engine.Evaluate(Request{
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			TrustTier:       0,
			SchemaType:      schemaType,
		})
		if result.Decision != DecisionAllow {
			t.Fatalf("schema_type %q: expected ALLOW with no isolation rules, got %q", schemaType, result.Decision)
		}
	}
}

func TestJurisdictionIsolation_Hl7Fhir_AllowedOnRegionalStrictNode_NoTomlRule(t *testing.T) {
	// jurisdictionConfig() → NodeScope=regional, Profile=strict.
	// hl7fhir hardcoded constraints are satisfied; no TOML rule defined → ALLOW.
	engine, _ := newEngine(t, jurisdictionConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
		SchemaType:      "hl7fhir",
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for hl7fhir on regional+strict node, got %q (%s)", result.Decision, result.Reason)
	}
}
