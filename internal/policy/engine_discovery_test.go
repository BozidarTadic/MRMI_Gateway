package policy

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func TestEvaluateDiscovery_SameAppOnly_Allows(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Discovery = config.DiscoveryPolicy{AppIsolation: "SAME_APP_ONLY"}
	engine, _ := newEngine(t, cfg)

	result := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "my-app", NodeAppID: "my-app"})
	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW, got %s: %s", result.Decision, result.Reason)
	}
}

func TestEvaluateDiscovery_SameAppOnly_Blocks(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Discovery = config.DiscoveryPolicy{AppIsolation: "SAME_APP_ONLY"}
	engine, _ := newEngine(t, cfg)

	result := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "other-app", NodeAppID: "my-app"})
	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY, got %s", result.Decision)
	}
	if result.Reason != ReasonAppIsolationViolation {
		t.Fatalf("expected APP_ISOLATION_VIOLATION, got %q", result.Reason)
	}
}

func TestEvaluateDiscovery_Whitelist_AllowsListed(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Discovery = config.DiscoveryPolicy{
		AppIsolation:  "WHITELIST",
		AllowedAppIDs: []string{"app-a", "app-b"},
	}
	engine, _ := newEngine(t, cfg)

	result := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "app-a", NodeAppID: "app-x"})
	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for whitelisted app, got %s", result.Decision)
	}
}

func TestEvaluateDiscovery_Whitelist_BlocksUnlisted(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Discovery = config.DiscoveryPolicy{
		AppIsolation:  "WHITELIST",
		AllowedAppIDs: []string{"app-a"},
	}
	engine, _ := newEngine(t, cfg)

	result := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "app-z", NodeAppID: "app-x"})
	if result.Decision != DecisionDeny {
		t.Fatalf("expected DENY for unlisted app, got %s", result.Decision)
	}
}

func TestEvaluateDiscovery_Open_AllowsAll(t *testing.T) {
	cfg := baseConfig()
	cfg.Policy.Discovery = config.DiscoveryPolicy{AppIsolation: "OPEN"}
	engine, _ := newEngine(t, cfg)

	result := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "any-app", NodeAppID: "other-app"})
	if result.Decision != DecisionAllow {
		t.Fatalf("expected ALLOW for OPEN mode, got %s", result.Decision)
	}
}

func TestEvaluateDiscovery_DefaultIsSameAppOnly(t *testing.T) {
	cfg := baseConfig()
	// AppIsolation left empty → default SAME_APP_ONLY
	engine, _ := newEngine(t, cfg)

	blocked := engine.EvaluateDiscovery(DiscoveryRequest{OriginAppID: "other-app", NodeAppID: "my-app"})
	if blocked.Decision != DecisionDeny {
		t.Fatalf("expected DENY from default isolation, got %s", blocked.Decision)
	}
}
