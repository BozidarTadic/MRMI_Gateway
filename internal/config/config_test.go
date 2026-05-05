package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultBalancedConfigIsValid(t *testing.T) {
	cfg := DefaultBalancedConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default config to be valid, got %v", err)
	}
}

func TestLoadBalancedConfigFromFile(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "node.balanced.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if cfg.Node.NodeID != "rs-node-01" {
		t.Fatalf("expected node_id rs-node-01, got %q", cfg.Node.NodeID)
	}
	if cfg.Profile.Name != "balanced" {
		t.Fatalf("expected profile balanced, got %q", cfg.Profile.Name)
	}
	if cfg.Network.GRPCListenAddr != "0.0.0.0:7777" {
		t.Fatalf("expected grpc listen addr 0.0.0.0:7777, got %q", cfg.Network.GRPCListenAddr)
	}
	if cfg.Network.MetricsAddr != ":9090" {
		t.Fatalf("expected metrics addr :9090, got %q", cfg.Network.MetricsAddr)
	}
	if cfg.Policy.Audit.DNSTXTInterval != 6*time.Hour {
		t.Fatalf("expected dns interval 6h, got %v", cfg.Policy.Audit.DNSTXTInterval)
	}
}

func TestLoadConfigAppliesProfileOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strict.toml")
	content := []byte(`
[node]
node_id = "ru-node-01"
region = "RU"
operator_id = "ru-operator"
policy_version = "1.0.0"
applicable_law = "RU-152FZ"
signed_by = "ed25519:TEST"

[profile]
name = "strict"

[profile_override]
timing_jitter_max_ms = 250
dedup_ttl_h = 48

[network]
listen_addr = "0.0.0.0:7777"
metrics_port = 9091
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if cfg.Profile.TimingJitterMax != 250*time.Millisecond {
		t.Fatalf("expected jitter override 250ms, got %v", cfg.Profile.TimingJitterMax)
	}
	if cfg.Profile.DedupTTL != 48*time.Hour {
		t.Fatalf("expected dedup ttl override 48h, got %v", cfg.Profile.DedupTTL)
	}
	if cfg.Network.HTTPListenAddr != ":8080" {
		t.Fatalf("expected default http addr :8080, got %q", cfg.Network.HTTPListenAddr)
	}
}
