package audit

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func TestLogVerify(t *testing.T) {
	log := New()
	cfg := config.DefaultBalancedConfig()

	log.Append(cfg, DecisionAllow, "RS", "RU")
	log.Append(cfg, DecisionDeny, "RS", "US")

	if err := log.Verify(); err != nil {
		t.Fatalf("expected audit log verification to pass, got %v", err)
	}
}
