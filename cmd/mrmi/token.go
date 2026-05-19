package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func cmdToken(args []string) error {
	if len(args) < 1 || args[0] != "issue" {
		usageToken(os.Stderr)
		return fmt.Errorf("subcommand required: issue")
	}
	return cmdTokenIssue(args[1:])
}

func cmdTokenIssue(args []string) error {
	return tokenIssue(os.Stdout, args)
}

func tokenIssue(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("token issue", flag.ContinueOnError)
	rawURL := fs.String("url", "", "Node HTTP address, e.g. http://localhost:8080 (required)")
	apiKey := fs.String("api-key", "", "X-MRMI-Key value for authentication (required)")
	scope := fs.String("scope", "read", "Token scope: read or operator")
	ttl := fs.Duration("ttl", time.Hour, "Token TTL as a Go duration, e.g. 24h")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rawURL == "" {
		return fmt.Errorf("--url is required")
	}
	if *apiKey == "" {
		return fmt.Errorf("--api-key is required")
	}
	if *scope != "read" && *scope != "operator" {
		return fmt.Errorf("--scope must be read or operator")
	}

	ttlMinutes := int(ttl.Minutes())
	if ttlMinutes <= 0 {
		ttlMinutes = 60
	}

	reqBody, err := json.Marshal(map[string]any{
		"scope":       *scope,
		"ttl_minutes": ttlMinutes,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	c := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, *rawURL+"/api/v1/token", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MRMI-Key", *apiKey)

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("POST /api/v1/token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, trimNewline(body))
	}

	var result struct {
		Token     string `json:"token"`
		Scope     string `json:"scope"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	expires := time.Unix(result.ExpiresAt, 0).UTC().Format(time.RFC3339)
	fmt.Fprintf(w, "Token:   %s\n", result.Token)
	fmt.Fprintf(w, "Scope:   %s\n", result.Scope)
	fmt.Fprintf(w, "Expires: %s\n", expires)
	return nil
}

func usageToken(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  mrmi token issue --url <http-addr> --api-key <key> [--scope <read|operator>] [--ttl <duration>]
`)
}
