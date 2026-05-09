package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Node    NodeConfig
	Profile ProfileConfig
	Policy  PolicyConfig
	Network NetworkConfig
}

type NodeConfig struct {
	NodeID        string
	Region        string
	OperatorID    string
	PolicyVersion string
	ApplicableLaw string
	SignedBy      string
}

type ProfileConfig struct {
	Name             string
	DedupTTL         time.Duration
	TimingJitterMax  time.Duration
	PaddingBucket    int
	DummyTrafficRate time.Duration
}

type PolicyConfig struct {
	Outbound OutboundPolicy
	Inbound  InboundPolicy
	Audit    AuditPolicy
}

type OutboundPolicy struct {
	AllowTo      []string
	DenyTo       []string
	StoreLocally bool
}

type InboundPolicy struct {
	MinTrustTier uint32
}

type AuditPolicy struct {
	LogAllDecisions  bool
	LogBackend       string
	ExportToOperator bool
	DNSTXTPublish    bool
	DNSTXTInterval   time.Duration
	HTTPSWellKnown   bool
}

type NetworkConfig struct {
	GRPCListenAddr  string
	HTTPListenAddr  string
	MetricsAddr     string
	ShutdownTimeout time.Duration
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultBalancedConfig(), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.IsDir() {
		return Config{}, fmt.Errorf("config path %q is a directory", path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".toml" {
		return Config{}, fmt.Errorf("unsupported config format %q: expected .toml", ext)
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	var raw rawTOML
	if _, err := toml.NewDecoder(file).Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("parse toml: %w", err)
	}

	cfg := DefaultConfigForProfile(raw.profileName())
	raw.apply(&cfg)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.Node.NodeID) == "":
		return errors.New("node.node_id is required")
	case strings.TrimSpace(c.Node.Region) == "":
		return errors.New("node.region is required")
	case strings.TrimSpace(c.Node.OperatorID) == "":
		return errors.New("node.operator_id is required")
	case strings.TrimSpace(c.Node.PolicyVersion) == "":
		return errors.New("node.policy_version is required")
	case strings.TrimSpace(c.Node.ApplicableLaw) == "":
		return errors.New("node.applicable_law is required")
	case !strings.HasPrefix(c.Node.SignedBy, "ed25519:"):
		return errors.New("node.signed_by must use ed25519:<public-key> format")
	case strings.TrimSpace(c.Profile.Name) == "":
		return errors.New("profile.name is required")
	case c.Profile.DedupTTL <= 0:
		return errors.New("profile.dedup_ttl must be positive")
	case strings.TrimSpace(c.Network.HTTPListenAddr) == "":
		return errors.New("network.http_listen_addr is required")
	case strings.TrimSpace(c.Network.GRPCListenAddr) == "":
		return errors.New("network.grpc_listen_addr is required")
	}

	return nil
}

// rawTOML mirrors the TOML file structure and is decoded directly by the library.
// Duration fields are kept as integers (seconds/milliseconds/hours) matching the
// TOML key names; apply() converts them to time.Duration.
type rawTOML struct {
	Node struct {
		NodeID        string `toml:"node_id"`
		Region        string `toml:"region"`
		OperatorID    string `toml:"operator_id"`
		PolicyVersion string `toml:"policy_version"`
		ApplicableLaw string `toml:"applicable_law"`
		SignedBy      string `toml:"signed_by"`
	} `toml:"node"`

	Profile struct {
		Name string `toml:"name"`
	} `toml:"profile"`

	ProfileOverride struct {
		PaddingBucketBytes         int `toml:"padding_bucket_bytes"`
		TimingJitterMaxMs          int `toml:"timing_jitter_max_ms"`
		DedupTTLH                  int `toml:"dedup_ttl_h"`
		DummyTrafficIntervalSecond int `toml:"dummy_traffic_interval_s"`
	} `toml:"profile_override"`

	Policy struct {
		Outbound struct {
			AllowTo      []string `toml:"allow_to"`
			DenyTo       []string `toml:"deny_to"`
			StoreLocally *bool    `toml:"store_locally"`
		} `toml:"outbound"`
		Inbound struct {
			MinTrustTier *uint32 `toml:"min_trust_tier"`
		} `toml:"inbound"`
		Audit struct {
			LogAllDecisions *bool  `toml:"log_all_decisions"`
			LogBackend      string `toml:"log_backend"`
			ExportOperator  *bool  `toml:"export_to_operator"`
			DNSPublish      *bool  `toml:"dns_txt_publish"`
			DNSIntervalS    int    `toml:"dns_txt_interval_s"`
			HTTPSWellKnown  *bool  `toml:"https_well_known"`
		} `toml:"audit"`
	} `toml:"policy"`

	Network struct {
		ListenAddr       string `toml:"listen_addr"`
		GRPCPort         int    `toml:"grpc_port"`
		HTTPListenAddr   string `toml:"http_listen_addr"`
		HTTPPort         int    `toml:"http_port"`
		MetricsAddr      string `toml:"metrics_addr"`
		MetricsPort      int    `toml:"metrics_port"`
		ShutdownTimeoutS int    `toml:"shutdown_timeout_s"`
	} `toml:"network"`
}

func (r rawTOML) profileName() string {
	if name := strings.TrimSpace(r.Profile.Name); name != "" {
		return name
	}
	return "balanced"
}

func (r rawTOML) apply(cfg *Config) {
	if v := strings.TrimSpace(r.Node.NodeID); v != "" {
		cfg.Node.NodeID = v
	}
	if v := strings.TrimSpace(r.Node.Region); v != "" {
		cfg.Node.Region = v
	}
	if v := strings.TrimSpace(r.Node.OperatorID); v != "" {
		cfg.Node.OperatorID = v
	}
	if v := strings.TrimSpace(r.Node.PolicyVersion); v != "" {
		cfg.Node.PolicyVersion = v
	}
	if v := strings.TrimSpace(r.Node.ApplicableLaw); v != "" {
		cfg.Node.ApplicableLaw = v
	}
	if v := strings.TrimSpace(r.Node.SignedBy); v != "" {
		cfg.Node.SignedBy = v
	}

	cfg.Profile.Name = r.profileName()

	if r.ProfileOverride.PaddingBucketBytes > 0 {
		cfg.Profile.PaddingBucket = r.ProfileOverride.PaddingBucketBytes
	}
	if r.ProfileOverride.TimingJitterMaxMs > 0 {
		cfg.Profile.TimingJitterMax = time.Duration(r.ProfileOverride.TimingJitterMaxMs) * time.Millisecond
	}
	if r.ProfileOverride.DedupTTLH > 0 {
		cfg.Profile.DedupTTL = time.Duration(r.ProfileOverride.DedupTTLH) * time.Hour
	}
	if r.ProfileOverride.DummyTrafficIntervalSecond > 0 {
		cfg.Profile.DummyTrafficRate = time.Duration(r.ProfileOverride.DummyTrafficIntervalSecond) * time.Second
	}

	if r.Policy.Outbound.AllowTo != nil {
		cfg.Policy.Outbound.AllowTo = r.Policy.Outbound.AllowTo
	}
	if r.Policy.Outbound.DenyTo != nil {
		cfg.Policy.Outbound.DenyTo = r.Policy.Outbound.DenyTo
	}
	if r.Policy.Outbound.StoreLocally != nil {
		cfg.Policy.Outbound.StoreLocally = *r.Policy.Outbound.StoreLocally
	}
	if r.Policy.Inbound.MinTrustTier != nil {
		cfg.Policy.Inbound.MinTrustTier = *r.Policy.Inbound.MinTrustTier
	}
	if r.Policy.Audit.LogAllDecisions != nil {
		cfg.Policy.Audit.LogAllDecisions = *r.Policy.Audit.LogAllDecisions
	}
	if v := strings.TrimSpace(r.Policy.Audit.LogBackend); v != "" {
		cfg.Policy.Audit.LogBackend = v
	}
	if r.Policy.Audit.ExportOperator != nil {
		cfg.Policy.Audit.ExportToOperator = *r.Policy.Audit.ExportOperator
	}
	if r.Policy.Audit.DNSPublish != nil {
		cfg.Policy.Audit.DNSTXTPublish = *r.Policy.Audit.DNSPublish
	}
	if r.Policy.Audit.DNSIntervalS > 0 {
		cfg.Policy.Audit.DNSTXTInterval = time.Duration(r.Policy.Audit.DNSIntervalS) * time.Second
	}
	if r.Policy.Audit.HTTPSWellKnown != nil {
		cfg.Policy.Audit.HTTPSWellKnown = *r.Policy.Audit.HTTPSWellKnown
	}

	if v := strings.TrimSpace(r.Network.ListenAddr); v != "" {
		cfg.Network.GRPCListenAddr = v
	} else if r.Network.GRPCPort > 0 {
		cfg.Network.GRPCListenAddr = fmt.Sprintf(":%d", r.Network.GRPCPort)
	}
	if v := strings.TrimSpace(r.Network.HTTPListenAddr); v != "" {
		cfg.Network.HTTPListenAddr = v
	} else if r.Network.HTTPPort > 0 {
		cfg.Network.HTTPListenAddr = fmt.Sprintf(":%d", r.Network.HTTPPort)
	}
	if v := strings.TrimSpace(r.Network.MetricsAddr); v != "" {
		cfg.Network.MetricsAddr = v
	} else if r.Network.MetricsPort > 0 {
		cfg.Network.MetricsAddr = fmt.Sprintf(":%d", r.Network.MetricsPort)
	}
	if r.Network.ShutdownTimeoutS > 0 {
		cfg.Network.ShutdownTimeout = time.Duration(r.Network.ShutdownTimeoutS) * time.Second
	}
}
