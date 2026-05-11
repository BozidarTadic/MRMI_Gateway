package config

import (
	"time"

	"MRMI_Gateway/internal/version"
)

func DefaultBalancedConfig() Config {
	return DefaultConfigForProfile("balanced")
}

func DefaultConfigForProfile(name string) Config {
	cfg := Config{
		Node: NodeConfig{
			NodeID:            "rs-node-01",
			NodeScope:         "regional",
			Region:            "RS",
			OperatorID:        "example-operator",
			PolicyVersion:     version.App,
			ApplicableLaw:     "RS-GDPR",
			SignedBy:          "ed25519:REPLACE_ME",
			DiscoveryTokenTTL: 5 * time.Minute,
		},
		Policy: PolicyConfig{
			Outbound: OutboundPolicy{
				AllowTo:      []string{"RU", "BY", "KZ", "AM"},
				DenyTo:       []string{},
				StoreLocally: true,
			},
			Inbound: InboundPolicy{
				MinTrustTier: 0,
			},
			Audit: AuditPolicy{
				LogBackend:       "local-merkle",
				LogAllDecisions:  true,
				ExportToOperator: false,
				DNSTXTPublish:    true,
				DNSTXTInterval:   6 * time.Hour,
				HTTPSWellKnown:   true,
			},
			Discovery: DiscoveryPolicy{
				AppIsolation: "SAME_APP_ONLY",
			},
			Connect: ConnectPolicy{
				AutoAccept: "MANUAL",
			},
		},
		Network: NetworkConfig{
			GRPCListenAddr:  ":7777",
			HTTPListenAddr:  ":8080",
			MetricsAddr:     ":9090",
			ShutdownTimeout: 5 * time.Second,
		},
	}

	switch name {
	case "strict":
		cfg.Profile = ProfileConfig{
			Name:             "strict",
			DedupTTL:         72 * time.Hour,
			TimingJitterMax:  500 * time.Millisecond,
			PaddingBucket:    16 * 1024,
			DummyTrafficRate: 5 * time.Second,
		}
		cfg.Policy.Audit.DNSTXTInterval = time.Hour
	case "performance":
		cfg.Profile = ProfileConfig{
			Name:             "performance",
			DedupTTL:         time.Hour,
			TimingJitterMax:  0,
			PaddingBucket:    0,
			DummyTrafficRate: 0,
		}
		cfg.Policy.Audit.LogAllDecisions = false
		cfg.Policy.Audit.ExportToOperator = false
		cfg.Policy.Audit.DNSTXTPublish = false
		cfg.Policy.Audit.DNSTXTInterval = 0
		cfg.Policy.Audit.HTTPSWellKnown = false
		cfg.Policy.Outbound.StoreLocally = false
	default:
		cfg.Profile = ProfileConfig{
			Name:             "balanced",
			DedupTTL:         24 * time.Hour,
			TimingJitterMax:  100 * time.Millisecond,
			PaddingBucket:    4 * 1024,
			DummyTrafficRate: 60 * time.Second,
		}
	}

	return cfg
}
