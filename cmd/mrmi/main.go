// Command mrmi provides operator tooling for MRMI Gateway nodes.
//
// Usage:
//
//	mrmi keygen --output <path>
//	mrmi audit verify --local --log <path>
//	mrmi audit verify --dns --node <node_id>
//	mrmi audit verify --https --url <url> [--pubkey <path>]
//	mrmi -version
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/identity"
	"MRMI_Gateway/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "-version", "--version", "version":
		fmt.Printf("mrmi %s (ADR %s)\n", version.App, version.ADR)
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	case "audit":
		if len(os.Args) < 3 || os.Args[2] != "verify" {
			usageAuditVerify(os.Stderr)
			os.Exit(1)
		}
		err = cmdAuditVerify(os.Args[3:])
	default:
		usage(os.Stderr)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// cmdKeygen generates an Ed25519 key pair. The private key is written as a
// PKCS8 PEM file; the public key (PKIX PEM) is printed to stdout.
func cmdKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	output := fs.String("output", "", "Path to write private key PEM (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return fmt.Errorf("--output is required")
	}

	priv, pub, err := identity.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(*output, privPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	fmt.Printf("Private key written to: %s\n\nPublic key:\n%s", *output, pubPEM)
	return nil
}

// cmdAuditVerify dispatches to the appropriate verify subcommand.
func cmdAuditVerify(args []string) error {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	localFlag := fs.Bool("local", false, "Verify a local JSON log file")
	logPath := fs.String("log", "", "Path to JSON log file (with --local)")
	dnsFlag := fs.Bool("dns", false, "Verify via DNS TXT record")
	nodeID := fs.String("node", "", "Node ID for DNS lookup (with --dns)")
	httpsFlag := fs.Bool("https", false, "Verify via HTTPS well-known endpoint")
	rawURL := fs.String("url", "", "URL to fetch (with --https)")
	pubkeyPath := fs.String("pubkey", "", "Path to Ed25519 public key PEM for signature check (optional, with --https)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	switch {
	case *localFlag:
		return verifyLocal(*logPath)
	case *dnsFlag:
		return verifyDNS(*nodeID)
	case *httpsFlag:
		return verifyHTTPS(*rawURL, *pubkeyPath)
	default:
		usageAuditVerify(os.Stderr)
		return fmt.Errorf("one of --local, --dns, or --https is required")
	}
}

// verifyLocal reads a JSON log file, recomputes the Merkle chain, and reports PASS or FAIL.
func verifyLocal(logPath string) error {
	if logPath == "" {
		return fmt.Errorf("--log is required with --local")
	}
	rootHash, err := audit.VerifyFile(logPath)
	if err != nil {
		fmt.Printf("FAIL  %s\n      %v\n", logPath, err)
		return err
	}
	fmt.Printf("PASS  %s\n      root: %s\n", logPath, rootHash)
	return nil
}

// verifyDNS looks up the _mrmi-audit.<node_id> TXT record and prints the result.
func verifyDNS(nodeID string) error {
	if nodeID == "" {
		return fmt.Errorf("--node is required with --dns")
	}
	dnsName := "_mrmi-audit." + nodeID
	txts, err := net.LookupTXT(dnsName)
	if err != nil {
		fmt.Printf("FAIL  %s\n      DNS lookup error: %v\n", dnsName, err)
		return err
	}
	for _, txt := range txts {
		if !strings.HasPrefix(txt, "v=1 ") {
			continue
		}
		fields := parseTXTFields(txt)
		root, ok := fields["root"]
		if !ok {
			continue
		}
		fmt.Printf("PASS  %s\n      root: %s\n", dnsName, root)
		if ts, ok := fields["ts"]; ok {
			fmt.Printf("      ts:   %s\n", ts)
		}
		return nil
	}
	fmt.Printf("FAIL  %s\n      no valid v=1 MRMI TXT record found\n", dnsName)
	return fmt.Errorf("no valid record at %s", dnsName)
}

// auditWellKnown mirrors the JSON shape returned by /.well-known/mrmi-audit.
type auditWellKnown struct {
	ADRVersion    string `json:"adr_version"`
	AppVersion    string `json:"app_version"`
	ApplicableLaw string `json:"applicable_law"`
	NodeID        string `json:"node_id"`
	RootHash      string `json:"root_hash"`
	Timestamp     int64  `json:"timestamp"`
	Version       int    `json:"version"`
	Signature     string `json:"signature"`
}

// verifyHTTPS GETs the well-known audit endpoint and optionally verifies the
// Ed25519 signature when --pubkey is provided.
func verifyHTTPS(rawURL, pubkeyPath string) error {
	if rawURL == "" {
		return fmt.Errorf("--url is required with --https")
	}

	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(rawURL)
	if err != nil {
		fmt.Printf("FAIL  %s\n      HTTP error: %v\n", rawURL, err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("FAIL  %s\n      HTTP %d\n", rawURL, resp.StatusCode)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var wk auditWellKnown
	if err := json.Unmarshal(body, &wk); err != nil {
		fmt.Printf("FAIL  %s\n      parse error: %v\n", rawURL, err)
		return err
	}

	fmt.Printf("PASS  %s\n", rawURL)
	fmt.Printf("      node:  %s\n", wk.NodeID)
	fmt.Printf("      root:  %s\n", wk.RootHash)
	fmt.Printf("      law:   %s\n", wk.ApplicableLaw)

	if pubkeyPath != "" {
		if err := checkHTTPSSig(wk, pubkeyPath); err != nil {
			fmt.Printf("SIG   FAIL  %v\n", err)
			return err
		}
		fmt.Printf("SIG   PASS\n")
	}
	return nil
}

// checkHTTPSSig verifies the Ed25519 signature in the well-known response.
// The canonical payload is the JSON of all fields except "signature",
// using the same struct layout as the server's auditSignPayload.
func checkHTTPSSig(wk auditWellKnown, pubkeyPath string) error {
	pemData, err := os.ReadFile(pubkeyPath)
	if err != nil {
		return fmt.Errorf("read pubkey: %w", err)
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("no PEM block in pubkey file")
	}
	pubRaw, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse pubkey: %w", err)
	}
	pub, ok := pubRaw.(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("key is not Ed25519")
	}

	// Canonical payload must match the server's auditSignPayload struct exactly.
	canonical, err := json.Marshal(struct {
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
	if err != nil {
		return err
	}

	if !strings.HasPrefix(wk.Signature, "ed25519:") {
		return fmt.Errorf("unexpected signature format %q", wk.Signature)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(wk.Signature, "ed25519:"))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(pub, canonical, sigBytes) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func parseTXTFields(txt string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Fields(txt) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `mrmi — MRMI Gateway operator CLI

Usage:
  mrmi keygen --output <path>
  mrmi audit verify --local --log <path>
  mrmi audit verify --dns --node <node_id>
  mrmi audit verify --https --url <url> [--pubkey <path>]
  mrmi -version
`)
}

func usageAuditVerify(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  mrmi audit verify --local --log <path>
  mrmi audit verify --dns --node <node_id>
  mrmi audit verify --https --url <url> [--pubkey <path>]
`)
}
