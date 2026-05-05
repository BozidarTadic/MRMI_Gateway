package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

	raw, err := parseTOML(file)
	if err != nil {
		return Config{}, err
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

type rawConfig struct {
	node struct {
		nodeID        string
		region        string
		operatorID    string
		policyVersion string
		applicableLaw string
		signedBy      string
	}
	profile struct {
		name string
	}
	profileOverride struct {
		paddingBucketBytes         int
		timingJitterMaxMs          int
		dedupTTLH                  int
		dummyTrafficIntervalSecond int
	}
	policy struct {
		outbound struct {
			allowTo      []string
			denyTo       []string
			storeLocally *bool
		}
		inbound struct {
			minTrustTier *uint32
		}
		audit struct {
			logAllDecisions *bool
			logBackend      string
			exportOperator  *bool
			dnsPublish      *bool
			dnsIntervalS    int
			httpsWellKnown  *bool
		}
	}
	network struct {
		listenAddr       string
		grpcPort         int
		httpListenAddr   string
		httpPort         int
		metricsAddr      string
		metricsPort      int
		shutdownTimeoutS int
	}
}

func (r rawConfig) profileName() string {
	name := strings.TrimSpace(r.profile.name)
	if name == "" {
		return "balanced"
	}
	return name
}

func (r rawConfig) apply(cfg *Config) {
	if value := strings.TrimSpace(r.node.nodeID); value != "" {
		cfg.Node.NodeID = value
	}
	if value := strings.TrimSpace(r.node.region); value != "" {
		cfg.Node.Region = value
	}
	if value := strings.TrimSpace(r.node.operatorID); value != "" {
		cfg.Node.OperatorID = value
	}
	if value := strings.TrimSpace(r.node.policyVersion); value != "" {
		cfg.Node.PolicyVersion = value
	}
	if value := strings.TrimSpace(r.node.applicableLaw); value != "" {
		cfg.Node.ApplicableLaw = value
	}
	if value := strings.TrimSpace(r.node.signedBy); value != "" {
		cfg.Node.SignedBy = value
	}

	cfg.Profile.Name = r.profileName()

	if r.profileOverride.paddingBucketBytes > 0 {
		cfg.Profile.PaddingBucket = r.profileOverride.paddingBucketBytes
	}
	if r.profileOverride.timingJitterMaxMs > 0 {
		cfg.Profile.TimingJitterMax = time.Duration(r.profileOverride.timingJitterMaxMs) * time.Millisecond
	}
	if r.profileOverride.dedupTTLH > 0 {
		cfg.Profile.DedupTTL = time.Duration(r.profileOverride.dedupTTLH) * time.Hour
	}
	if r.profileOverride.dummyTrafficIntervalSecond > 0 {
		cfg.Profile.DummyTrafficRate = time.Duration(r.profileOverride.dummyTrafficIntervalSecond) * time.Second
	}

	if r.policy.outbound.allowTo != nil {
		cfg.Policy.Outbound.AllowTo = r.policy.outbound.allowTo
	}
	if r.policy.outbound.denyTo != nil {
		cfg.Policy.Outbound.DenyTo = r.policy.outbound.denyTo
	}
	if r.policy.outbound.storeLocally != nil {
		cfg.Policy.Outbound.StoreLocally = *r.policy.outbound.storeLocally
	}
	if r.policy.inbound.minTrustTier != nil {
		cfg.Policy.Inbound.MinTrustTier = *r.policy.inbound.minTrustTier
	}
	if r.policy.audit.logAllDecisions != nil {
		cfg.Policy.Audit.LogAllDecisions = *r.policy.audit.logAllDecisions
	}
	if value := strings.TrimSpace(r.policy.audit.logBackend); value != "" {
		cfg.Policy.Audit.LogBackend = value
	}
	if r.policy.audit.exportOperator != nil {
		cfg.Policy.Audit.ExportToOperator = *r.policy.audit.exportOperator
	}
	if r.policy.audit.dnsPublish != nil {
		cfg.Policy.Audit.DNSTXTPublish = *r.policy.audit.dnsPublish
	}
	if r.policy.audit.dnsIntervalS > 0 {
		cfg.Policy.Audit.DNSTXTInterval = time.Duration(r.policy.audit.dnsIntervalS) * time.Second
	}
	if r.policy.audit.httpsWellKnown != nil {
		cfg.Policy.Audit.HTTPSWellKnown = *r.policy.audit.httpsWellKnown
	}

	if value := strings.TrimSpace(r.network.listenAddr); value != "" {
		cfg.Network.GRPCListenAddr = value
	} else if r.network.grpcPort > 0 {
		cfg.Network.GRPCListenAddr = fmt.Sprintf(":%d", r.network.grpcPort)
	}
	if value := strings.TrimSpace(r.network.httpListenAddr); value != "" {
		cfg.Network.HTTPListenAddr = value
	} else if r.network.httpPort > 0 {
		cfg.Network.HTTPListenAddr = fmt.Sprintf(":%d", r.network.httpPort)
	}
	if value := strings.TrimSpace(r.network.metricsAddr); value != "" {
		cfg.Network.MetricsAddr = value
	} else if r.network.metricsPort > 0 {
		cfg.Network.MetricsAddr = fmt.Sprintf(":%d", r.network.metricsPort)
	}
	if r.network.shutdownTimeoutS > 0 {
		cfg.Network.ShutdownTimeout = time.Duration(r.network.shutdownTimeoutS) * time.Second
	}
}

func parseTOML(file *os.File) (rawConfig, error) {
	var raw rawConfig
	var section string

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return raw, fmt.Errorf("line %d: expected key = value", lineNumber)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if err := assignValue(&raw, section, key, value); err != nil {
			return raw, fmt.Errorf("line %d: %w", lineNumber, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return raw, err
	}

	return raw, nil
}

func assignValue(raw *rawConfig, section, key, value string) error {
	switch section {
	case "node":
		switch key {
		case "node_id":
			raw.node.nodeID = parseString(value)
		case "region":
			raw.node.region = parseString(value)
		case "operator_id":
			raw.node.operatorID = parseString(value)
		case "policy_version":
			raw.node.policyVersion = parseString(value)
		case "applicable_law":
			raw.node.applicableLaw = parseString(value)
		case "signed_by":
			raw.node.signedBy = parseString(value)
		default:
			return fmt.Errorf("unsupported node key %q", key)
		}
	case "profile":
		if key != "name" {
			return fmt.Errorf("unsupported profile key %q", key)
		}
		raw.profile.name = parseString(value)
	case "profile_override":
		intValue, err := parseInt(value)
		if err != nil {
			return err
		}
		switch key {
		case "padding_bucket_bytes":
			raw.profileOverride.paddingBucketBytes = intValue
		case "timing_jitter_max_ms":
			raw.profileOverride.timingJitterMaxMs = intValue
		case "dedup_ttl_h":
			raw.profileOverride.dedupTTLH = intValue
		case "dummy_traffic_interval_s":
			raw.profileOverride.dummyTrafficIntervalSecond = intValue
		default:
			return fmt.Errorf("unsupported profile_override key %q", key)
		}
	case "policy.outbound":
		switch key {
		case "allow_to":
			values, err := parseStringArray(value)
			if err != nil {
				return err
			}
			raw.policy.outbound.allowTo = values
		case "deny_to":
			values, err := parseStringArray(value)
			if err != nil {
				return err
			}
			raw.policy.outbound.denyTo = values
		case "store_locally":
			boolValue, err := parseBool(value)
			if err != nil {
				return err
			}
			raw.policy.outbound.storeLocally = &boolValue
		default:
			return fmt.Errorf("unsupported policy.outbound key %q", key)
		}
	case "policy.inbound":
		if key != "min_trust_tier" {
			return fmt.Errorf("unsupported policy.inbound key %q", key)
		}
		intValue, err := parseInt(value)
		if err != nil {
			return err
		}
		uintValue := uint32(intValue)
		raw.policy.inbound.minTrustTier = &uintValue
	case "policy.audit":
		switch key {
		case "log_all_decisions":
			boolValue, err := parseBool(value)
			if err != nil {
				return err
			}
			raw.policy.audit.logAllDecisions = &boolValue
		case "log_backend":
			raw.policy.audit.logBackend = parseString(value)
		case "export_to_operator":
			boolValue, err := parseBool(value)
			if err != nil {
				return err
			}
			raw.policy.audit.exportOperator = &boolValue
		case "dns_txt_publish":
			boolValue, err := parseBool(value)
			if err != nil {
				return err
			}
			raw.policy.audit.dnsPublish = &boolValue
		case "dns_txt_interval_s":
			intValue, err := parseInt(value)
			if err != nil {
				return err
			}
			raw.policy.audit.dnsIntervalS = intValue
		case "https_well_known":
			boolValue, err := parseBool(value)
			if err != nil {
				return err
			}
			raw.policy.audit.httpsWellKnown = &boolValue
		default:
			return fmt.Errorf("unsupported policy.audit key %q", key)
		}
	case "network":
		switch key {
		case "listen_addr":
			raw.network.listenAddr = parseString(value)
		case "grpc_port":
			intValue, err := parseInt(value)
			if err != nil {
				return err
			}
			raw.network.grpcPort = intValue
		case "http_listen_addr":
			raw.network.httpListenAddr = parseString(value)
		case "http_port":
			intValue, err := parseInt(value)
			if err != nil {
				return err
			}
			raw.network.httpPort = intValue
		case "metrics_addr":
			raw.network.metricsAddr = parseString(value)
		case "metrics_port":
			intValue, err := parseInt(value)
			if err != nil {
				return err
			}
			raw.network.metricsPort = intValue
		case "shutdown_timeout_s":
			intValue, err := parseInt(value)
			if err != nil {
				return err
			}
			raw.network.shutdownTimeoutS = intValue
		default:
			return fmt.Errorf("unsupported network key %q", key)
		}
	default:
		return fmt.Errorf("unsupported section %q", section)
	}

	return nil
}

func stripComment(line string) string {
	inString := false
	for i, r := range line {
		switch r {
		case '"':
			inString = !inString
		case '#':
			if !inString {
				return line[:i]
			}
		}
	}
	return line
}

func parseString(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"`)
}

func parseInt(value string) (int, error) {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", value)
	}
	return intValue, nil
}

func parseBool(value string) (bool, error) {
	boolValue, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q", value)
	}
	return boolValue, nil
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("invalid array %q", value)
	}

	body := strings.TrimSpace(value[1 : len(value)-1])
	if body == "" {
		return []string{}, nil
	}

	parts := strings.Split(body, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, parseString(part))
	}

	return values, nil
}
