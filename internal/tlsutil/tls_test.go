package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCerts writes a self-signed CA + leaf cert/key pair to dir.
// Returns (certPath, keyPath, caPath).
func generateTestCerts(t *testing.T, dir string) (certPath, keyPath, caPath string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-leaf"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	caPath = filepath.Join(dir, "ca.pem")

	writePEM(t, certPath, "CERTIFICATE", leafDER)
	writeKeyPEM(t, keyPath, leafKey)
	writePEM(t, caPath, "CERTIFICATE", caDER)

	return certPath, keyPath, caPath
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatalf("encode pem %s: %v", path, err)
	}
}

func writeKeyPEM(t *testing.T, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	writePEM(t, path, "EC PRIVATE KEY", der)
}

func TestLoadServerTLS_Insecure(t *testing.T) {
	cfg := TLSConfig{Insecure: true}
	tlsCfg, err := LoadServerTLS(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg != nil {
		t.Fatal("expected nil tls.Config when insecure=true")
	}
}

func TestLoadClientTLS_Insecure(t *testing.T) {
	cfg := TLSConfig{Insecure: true}
	tlsCfg, err := LoadClientTLS(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg != nil {
		t.Fatal("expected nil tls.Config when insecure=true")
	}
}

func TestLoadServerTLS_Empty(t *testing.T) {
	tlsCfg, err := LoadServerTLS(TLSConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg != nil {
		t.Fatal("expected nil tls.Config when no paths set")
	}
}

func TestLoadClientTLS_Empty(t *testing.T) {
	tlsCfg, err := LoadClientTLS(TLSConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg != nil {
		t.Fatal("expected nil tls.Config when no paths set")
	}
}

func TestLoadServerTLS_WithCerts(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := generateTestCerts(t, dir)

	tlsCfg, err := LoadServerTLS(TLSConfig{Cert: certPath, Key: keyPath, CA: caPath})
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
}

func TestLoadClientTLS_WithCerts(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := generateTestCerts(t, dir)

	tlsCfg, err := LoadClientTLS(TLSConfig{Cert: certPath, Key: keyPath, CA: caPath})
	if err != nil {
		t.Fatalf("LoadClientTLS: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
}

func TestLoadServerTLS_MissingKeyReturnsError(t *testing.T) {
	dir := t.TempDir()
	certPath, _, _ := generateTestCerts(t, dir)

	_, err := LoadServerTLS(TLSConfig{Cert: certPath}) // Key missing
	if err == nil {
		t.Fatal("expected error when key is missing")
	}
}

func TestLoadServerTLS_InvalidCAReturnsError(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := generateTestCerts(t, dir)

	badCA := filepath.Join(dir, "bad.pem")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write bad CA: %v", err)
	}

	_, err := LoadServerTLS(TLSConfig{Cert: certPath, Key: keyPath, CA: badCA})
	if err == nil {
		t.Fatal("expected error for invalid CA PEM")
	}
}
