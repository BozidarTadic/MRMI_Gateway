package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/identity"
)

func TestKeygen_WritesValidKeyPair(t *testing.T) {
	path := t.TempDir() + "/node.key"
	if err := cmdKeygen([]string{"--output", path}); err != nil {
		t.Fatalf("cmdKeygen: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatal("expected PRIVATE KEY PEM block")
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse PKCS8: %v", err)
	}
	if _, ok := raw.(ed25519.PrivateKey); !ok {
		t.Fatal("key is not Ed25519")
	}
}

func TestKeygen_GeneratedKeyRoundTrips(t *testing.T) {
	path := t.TempDir() + "/node.key"
	if err := cmdKeygen([]string{"--output", path}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	block, _ := pem.Decode(data)
	raw, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
	priv := raw.(ed25519.PrivateKey)
	pub := priv.Public().(ed25519.PublicKey)

	msg := []byte("mrmi test message")
	sig := ed25519.Sign(priv, msg)
	if !ed25519.Verify(pub, msg, sig) {
		t.Fatal("sign/verify round trip failed with generated key pair")
	}
}

func TestAuditVerifyLocal_Pass(t *testing.T) {
	log := audit.New()
	cfg := config.DefaultBalancedConfig()
	log.Append(cfg, audit.DecisionAllow, "POLICY_ACCEPTED", 0, "RS", "RU")
	log.Append(cfg, audit.DecisionDeny, "RECIPIENT_REGION_NOT_IN_ALLOW_LIST", 0, "RS", "US")

	path := t.TempDir() + "/log.json"
	if err := log.WriteJSON(path); err != nil {
		t.Fatal(err)
	}
	if err := verifyLocal(path); err != nil {
		t.Fatalf("verifyLocal on valid log: %v", err)
	}
}

func TestAuditVerifyLocal_FailsOnMissingFile(t *testing.T) {
	if err := verifyLocal(t.TempDir() + "/missing.json"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestAuditVerifyLocal_FailsOnTamperedLog(t *testing.T) {
	log := audit.New()
	cfg := config.DefaultBalancedConfig()
	log.Append(cfg, audit.DecisionAllow, "POLICY_ACCEPTED", 0, "RS", "RU")

	path := t.TempDir() + "/log.json"
	_ = log.WriteJSON(path)

	// Mutate the decision field so the stored hash no longer matches.
	data, _ := os.ReadFile(path)
	tampered := strings.Replace(string(data), `"ALLOW"`, `"DENY"`, 1)
	_ = os.WriteFile(path, []byte(tampered), 0644)

	if err := verifyLocal(path); err == nil {
		t.Fatal("expected verifyLocal to detect tamper")
	}
}

// TestCheckHTTPSSig_ValidSignature verifies that checkHTTPSSig passes when
// the signature was produced by the node's private key.
func TestCheckHTTPSSig_ValidSignature(t *testing.T) {
	priv, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	wk := auditWellKnown{
		ADRVersion:    "0.8",
		AppVersion:    "0.1.0",
		ApplicableLaw: "RS-GDPR",
		NodeID:        "rs-node-01",
		RootHash:      "sha256:abc123",
		Timestamp:     1000000,
		Version:       1,
	}

	// Build the same canonical payload the server signs.
	canonical, _ := json.Marshal(struct {
		ADRVersion    string `json:"adr_version"`
		AppVersion    string `json:"app_version"`
		ApplicableLaw string `json:"applicable_law"`
		NodeID        string `json:"node_id"`
		RootHash      string `json:"root_hash"`
		Timestamp     int64  `json:"timestamp"`
		Version       int    `json:"version"`
	}{
		ADRVersion:    wk.ADRVersion,
		AppVersion:    wk.AppVersion,
		ApplicableLaw: wk.ApplicableLaw,
		NodeID:        wk.NodeID,
		RootHash:      wk.RootHash,
		Timestamp:     wk.Timestamp,
		Version:       wk.Version,
	})
	sigBytes := ed25519.Sign(priv, canonical)
	wk.Signature = "ed25519:" + base64.StdEncoding.EncodeToString(sigBytes)

	pubBytes, _ := x509.MarshalPKIXPublicKey(pub)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	pubPath := t.TempDir() + "/pub.pem"
	_ = os.WriteFile(pubPath, pubPEM, 0644)

	if err := checkHTTPSSig(wk, pubPath); err != nil {
		t.Fatalf("checkHTTPSSig with valid sig: %v", err)
	}
}

// TestCheckHTTPSSig_InvalidSignature verifies that checkHTTPSSig rejects a
// signature that does not match the payload.
func TestCheckHTTPSSig_InvalidSignature(t *testing.T) {
	_, pub, err := identity.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	// 64 zero bytes is a syntactically valid but wrong signature.
	badSig := base64.StdEncoding.EncodeToString(make([]byte, 64))
	wk := auditWellKnown{
		ADRVersion: "0.8", AppVersion: "0.1.0", ApplicableLaw: "RS-GDPR",
		NodeID: "rs-node-01", RootHash: "sha256:abc123", Timestamp: 1000000,
		Version: 1, Signature: "ed25519:" + badSig,
	}

	pubBytes, _ := x509.MarshalPKIXPublicKey(pub)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	pubPath := t.TempDir() + "/pub.pem"
	_ = os.WriteFile(pubPath, pubPEM, 0644)

	if err := checkHTTPSSig(wk, pubPath); err == nil {
		t.Fatal("expected checkHTTPSSig to fail with invalid signature")
	}
}
