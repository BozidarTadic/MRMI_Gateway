package policy

import (
	"testing"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
)

func baseConfig() config.Config {
	cfg := config.DefaultBalancedConfig()
	// Deterministic baseline: RS allows RU/BY, denies nothing, min tier 0.
	cfg.Policy.Outbound.AllowTo = []string{"RU", "BY"}
	cfg.Policy.Outbound.DenyTo = []string{}
	cfg.Policy.Inbound.MinTrustTier = 0
	return cfg
}

func newEngine(t *testing.T, cfg config.Config) (*Engine, *audit.Log) {
	t.Helper()
	log := audit.New()
	engine, err := NewEngine(cfg, log)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine, log
}

func TestEvaluate_AllowedRegion(t *testing.T) {
	engine, _ := newEngine(t, baseConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       0,
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEvaluate_DeniedRegionNotInAllowList(t *testing.T) {
	engine, _ := newEngine(t, baseConfig())

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "US",
		TrustTier:       0,
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY, got %q", result.Decision)
	}
	if result.Reason == "" {
		t.Fatal("expected non-empty deny reason")
	}
}

func TestEvaluate_DeniedRegionInDenyList(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Outbound.DenyTo = []string{"RU"}

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU", // in allow_to AND deny_to — deny wins
		TrustTier:       0,
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY, got %q", result.Decision)
	}
}

func TestEvaluate_TrustTierBelowMinimumRejected(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Inbound.MinTrustTier = 2

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       1, // below MinTrustTier=2
	})

	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for trust tier below minimum, got %q", result.Decision)
	}
	if result.Reason != "trust tier below minimum" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
}

func TestEvaluate_TrustTierAtMinimumAllowed(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Inbound.MinTrustTier = 2

	engine, _ := newEngine(t, cfg)

	result := engine.Evaluate(Request{
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       2, // exactly at MinTrustTier
	})

	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW at minimum trust tier, got %q (%s)", result.Decision, result.Reason)
	}
}

func TestEvaluate_EmptyAllowListPermitsAnyRegion(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Outbound.AllowTo = []string{}
	cfg.Policy.Outbound.DenyTo = []string{}

	engine, _ := newEngine(t, cfg)

	for _, region := range []string{"US", "CN", "DE", "RS"} {
		result := engine.Evaluate(Request{
			SenderRegion:    "RS",
			RecipientRegion: region,
			TrustTier:       0,
		})
		if result.Decision != DecisionAllow {
			t.Fatalf("region %q: expected ALLOW with empty allow list, got %q", region, result.Decision)
		}
	}
}

func TestEvaluate_DuplicateHandledBeforePolicyLayer(t *testing.T) {
	// The policy engine itself has no dedup concept — duplicate suppression happens
	// in the gateway layer before Evaluate is called. This test confirms the engine
	// treats two identical requests independently (both get the same policy result).
	engine, _ := newEngine(t, baseConfig())

	req := Request{SenderRegion: "RS", RecipientRegion: "RU", TrustTier: 0}

	first := engine.Evaluate(req)
	second := engine.Evaluate(req)

	if first.Decision != DecisionAllow || second.Decision != DecisionAllow {
		t.Fatalf("expected both ALLOW; got %q, %q", first.Decision, second.Decision)
	}
}

func TestEvaluate_EachDecisionWritesToAuditLog(t *testing.T) {
	engine, log := newEngine(t, baseConfig())

	engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "RU", TrustTier: 0}) // ALLOW
	engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "US", TrustTier: 0}) // DENY (not in allow_to)

	cfg := baseConfig()
	cfg.Policy.Inbound.MinTrustTier = 1
	tierEngine, tierLog := newEngine(t, cfg)
	tierEngine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "RU", TrustTier: 0}) // DENY (trust tier)

	entries := log.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}
	if entries[0].Decision != audit.DecisionAllow {
		t.Fatalf("entry 0: expected ALLOW, got %q", entries[0].Decision)
	}
	if entries[1].Decision != audit.DecisionDeny {
		t.Fatalf("entry 1: expected DENY, got %q", entries[1].Decision)
	}

	tierEntries := tierLog.Entries()
	if len(tierEntries) != 1 {
		t.Fatalf("expected 1 trust-tier audit entry, got %d", len(tierEntries))
	}
	if tierEntries[0].Decision != audit.DecisionDeny {
		t.Fatalf("trust-tier entry: expected DENY, got %q", tierEntries[0].Decision)
	}

	if err := log.Verify(); err != nil {
		t.Fatalf("audit log chain broken: %v", err)
	}
}
