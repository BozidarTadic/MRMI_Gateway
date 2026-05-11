package policy

import (
	"testing"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
)

func TestEngine_Reload_AppliesNewAllowList(t *testing.T) {
	cfg := baseConfig()
	engine, _ := newEngine(t, cfg)

	// Initially RS→US is denied.
	if r := engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "US"}); r.Decision != DecisionDeny {
		t.Fatalf("expected DENY before reload, got %q", r.Decision)
	}

	// Reload a config that allows US.
	newCfg := baseConfig()
	newCfg.Policy.Outbound.AllowTo = append(newCfg.Policy.Outbound.AllowTo, "US")
	if err := engine.Reload(newCfg); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if r := engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "US"}); r.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW after reload, got %q (%s)", r.Decision, r.Reason)
	}
}

func TestEngine_Reload_InvalidConfigRejected(t *testing.T) {
	cfg := baseConfig()
	engine, _ := newEngine(t, cfg)

	bad := config.Config{} // fails Validate() — no node_id, etc.
	if err := engine.Reload(bad); err == nil {
		t.Fatal("expected Reload to return error for invalid config")
	}

	// Engine must still use the old config.
	if r := engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "RU"}); r.Decision != DecisionAllow {
		t.Fatalf("old config must be preserved after rejected reload; got %q", r.Decision)
	}
}

func TestEngine_Reload_AuditLogPreserved(t *testing.T) {
	cfg := baseConfig()
	log := audit.New()
	engine, err := NewEngine(cfg, log, nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "RU"})

	newCfg := baseConfig()
	newCfg.Policy.Outbound.AllowTo = []string{"RU", "US"}
	if err := engine.Reload(newCfg); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	engine.Evaluate(Request{SenderRegion: "RS", RecipientRegion: "US"})

	if len(log.Entries()) != 2 {
		t.Fatalf("expected 2 audit entries across reload, got %d", len(log.Entries()))
	}
}
