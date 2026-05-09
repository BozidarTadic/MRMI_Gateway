package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig holds paths for certificate material. It matches the [tls] TOML section.
type TLSConfig struct {
	Cert     string // path to PEM-encoded certificate (server or client)
	Key      string // path to PEM-encoded private key
	CA       string // path to PEM-encoded CA certificate for verification
	Insecure bool   // skip TLS entirely — test-only; must not be true in production
}

// LoadServerTLS builds a *tls.Config suitable for a gRPC server with mTLS.
//
// Returns nil, nil when Insecure is true (no TLS applied).
// Returns an error when Cert/Key paths are provided but cannot be loaded.
func LoadServerTLS(cfg TLSConfig) (*tls.Config, error) {
	if cfg.Insecure {
		return nil, nil
	}
	if cfg.Cert == "" && cfg.Key == "" {
		return nil, nil
	}
	return buildTLSConfig(cfg, tls.RequireAndVerifyClientCert)
}

// LoadClientTLS builds a *tls.Config suitable for a gRPC client with mTLS.
//
// Returns nil, nil when Insecure is true (use insecure.NewCredentials() at the call site).
// Returns an error when Cert/Key paths are provided but cannot be loaded.
func LoadClientTLS(cfg TLSConfig) (*tls.Config, error) {
	if cfg.Insecure {
		return nil, nil
	}
	if cfg.Cert == "" && cfg.Key == "" && cfg.CA == "" {
		return nil, nil
	}
	return buildTLSConfig(cfg, tls.NoClientCert)
}

func buildTLSConfig(cfg TLSConfig, clientAuth tls.ClientAuthType) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: clientAuth,
	}

	if cfg.Cert != "" || cfg.Key != "" {
		if cfg.Cert == "" || cfg.Key == "" {
			return nil, fmt.Errorf("tlsutil: cert and key must both be set or both be empty")
		}
		cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("tlsutil: load key pair: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if cfg.CA != "" {
		pem, err := os.ReadFile(cfg.CA)
		if err != nil {
			return nil, fmt.Errorf("tlsutil: read CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("tlsutil: no valid certificates in CA file %q", cfg.CA)
		}
		tlsCfg.RootCAs = pool
		tlsCfg.ClientCAs = pool
	}

	return tlsCfg, nil
}
