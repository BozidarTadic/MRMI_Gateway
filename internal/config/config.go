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
	TLS     TLSConfig
}

// TLSConfig holds paths to certificate material for inter-node mTLS.
// Both server and client use the same cert/key/CA (node identity model).
type TLSConfig struct {
	Cert     string // path to PEM-encoded node certificate
	Key      string // path to PEM-encoded node private key
	CA       string // path to PEM-encoded CA that signed peer certificates
	Insecure bool   // skip TLS entirely — development only, never true in production
}

type NodeConfig struct {
	NodeID        string
	NodeScope     string // "regional" | "alliance" | "global"
	Region        string // physical region; required for regional and global nodes
	Regions       []string // served regions; required for alliance nodes
	AllianceID    string   // legal agreement reference; required for alliance nodes
	Disclaimer    string   // e.g. "no-data-residency-claims"; used for global nodes
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
	Routing  RoutingPolicy
}

type RoutingPolicy struct {
	// AllowVia restricts which node tiers this node will route through.
	// Empty means all tiers are allowed. Example: ["regional", "alliance"]
	AllowVia []string
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

// PeerConfig describes a single peer node. For regional peers the map key is the
// region code (e.g. "RU"). For alliance peers the key is the alliance_id; Regions
// lists every region the peer serves. For global peers the key is any unique name.
type PeerConfig struct {
	Addr      string   // gRPC dial address
	NodeScope string   // "regional" | "alliance" | "global"
	Regions   []string // non-empty only for alliance peers
}

type NetworkConfig struct {
	GRPCListenAddr  string
	HTTPListenAddr  string
	MetricsAddr     string
	ShutdownTimeout time.Duration
	Peers           map[string]PeerConfig
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
	case strings.TrimSpace(c.Node.NodeScope) == "":
		return errors.New("node.node_scope is required")
	case c.Node.NodeScope != "regional" && c.Node.NodeScope != "alliance" && c.Node.NodeScope != "global":
		return fmt.Errorf("node.node_scope %q is invalid: must be regional, alliance, or global", c.Node.NodeScope)
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

	switch c.Node.NodeScope {
	case "regional", "global":
		if strings.TrimSpace(c.Node.Region) == "" {
			return fmt.Errorf("node.region is required for %s nodes", c.Node.NodeScope)
		}
	case "alliance":
		if len(c.Node.Regions) == 0 {
			return errors.New("node.regions is required for alliance nodes")
		}
		if strings.TrimSpace(c.Node.AllianceID) == "" {
			return errors.New("node.alliance_id is required for alliance nodes")
		}
	}

	for _, tier := range c.Policy.Routing.AllowVia {
		if tier != "regional" && tier != "alliance" && tier != "global" {
			return fmt.Errorf("policy.routing.allow_via: %q is not a valid tier", tier)
		}
	}

	for key, peer := range c.Network.Peers {
		if strings.TrimSpace(peer.Addr) == "" {
			return fmt.Errorf("peers[%q].addr is empty", key)
		}
		if peer.NodeScope != "" && peer.NodeScope != "regional" && peer.NodeScope != "alliance" && peer.NodeScope != "global" {
			return fmt.Errorf("peers[%q].node_scope %q is invalid: must be regional, alliance, or global", key, peer.NodeScope)
		}
		if peer.NodeScope == "alliance" && len(peer.Regions) == 0 {
			return fmt.Errorf("peers[%q].regions is required for alliance peers", key)
		}
	}

	return nil
}

// rawTOML mirrors the TOML file structure and is decoded directly by the library.
// Duration fields are kept as integers (seconds/milliseconds/hours) matching the
// TOML key names; apply() converts them to time.Duration.
type rawTOML struct {
	Node struct {
		NodeID        string   `toml:"node_id"`
		NodeScope     string   `toml:"node_scope"`
		Region        string   `toml:"region"`
		Regions       []string `toml:"regions"`
		AllianceID    string   `toml:"alliance_id"`
		Disclaimer    string   `toml:"disclaimer"`
		OperatorID    string   `toml:"operator_id"`
		PolicyVersion string   `toml:"policy_version"`
		ApplicableLaw string   `toml:"applicable_law"`
		SignedBy      string   `toml:"signed_by"`
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
		Routing struct {
			AllowVia []string `toml:"allow_via"`
		} `toml:"routing"`
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

	Peers map[string]struct {
		Addr      string   `toml:"addr"`
		NodeScope string   `toml:"node_scope"`
		Regions   []string `toml:"regions"`
	} `toml:"peers"`

	TLS struct {
		Cert     string `toml:"cert"`
		Key      string `toml:"key"`
		CA       string `toml:"ca"`
		Insecure bool   `toml:"insecure"`
	} `toml:"tls"`
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
	if v := strings.TrimSpace(r.Node.NodeScope); v != "" {
		cfg.Node.NodeScope = v
	}
	if v := strings.TrimSpace(r.Node.Region); v != "" {
		cfg.Node.Region = v
	}
	if r.Node.Regions != nil {
		cfg.Node.Regions = r.Node.Regions
	}
	if v := strings.TrimSpace(r.Node.AllianceID); v != "" {
		cfg.Node.AllianceID = v
	}
	if v := strings.TrimSpace(r.Node.Disclaimer); v != "" {
		cfg.Node.Disclaimer = v
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
	if r.Policy.Routing.AllowVia != nil {
		cfg.Policy.Routing.AllowVia = r.Policy.Routing.AllowVia
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

	if v := strings.TrimSpace(r.TLS.Cert); v != "" {
		cfg.TLS.Cert = v
	}
	if v := strings.TrimSpace(r.TLS.Key); v != "" {
		cfg.TLS.Key = v
	}
	if v := strings.TrimSpace(r.TLS.CA); v != "" {
		cfg.TLS.CA = v
	}
	cfg.TLS.Insecure = r.TLS.Insecure

	if r.Peers != nil {
		cfg.Network.Peers = make(map[string]PeerConfig, len(r.Peers))
		for k, v := range r.Peers {
			cfg.Network.Peers[k] = PeerConfig{
				Addr:      v.Addr,
				NodeScope: v.NodeScope,
				Regions:   v.Regions,
			}
		}
	}
}
