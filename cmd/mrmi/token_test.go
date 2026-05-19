package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func tokenServer(t *testing.T, wantScope string, ttlMinutes int, expiresAt int64, signed string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/token" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Scope      string `json:"scope"`
			TTLMinutes int    `json:"ttl_minutes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Scope != wantScope || req.TTLMinutes != ttlMinutes {
			http.Error(w, "unexpected params", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      signed,
			"scope":      req.Scope,
			"expires_at": expiresAt,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTokenIssue_PrintsOutput(t *testing.T) {
	exp := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC).Unix()
	srv := tokenServer(t, "operator", 60, exp, "eyJhbGciOiJIUzI1NiJ9.test.sig")

	var buf bytes.Buffer
	err := tokenIssue(&buf, []string{
		"--url", srv.URL,
		"--api-key", "secret",
		"--scope", "operator",
	})
	if err != nil {
		t.Fatalf("tokenIssue: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"eyJhbGciOiJIUzI1NiJ9.test.sig", "operator", "2026-05-20T10:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestTokenIssue_DefaultScopeRead(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	srv := tokenServer(t, "read", 60, exp, "tok123")

	var buf bytes.Buffer
	err := tokenIssue(&buf, []string{"--url", srv.URL, "--api-key", "key"})
	if err != nil {
		t.Fatalf("tokenIssue default scope: %v", err)
	}
	if !strings.Contains(buf.String(), "tok123") {
		t.Errorf("output missing token:\n%s", buf.String())
	}
}

func TestTokenIssue_CustomTTL(t *testing.T) {
	exp := time.Now().Add(24 * time.Hour).Unix()
	srv := tokenServer(t, "read", 1440, exp, "tok-ttl")

	var buf bytes.Buffer
	err := tokenIssue(&buf, []string{"--url", srv.URL, "--api-key", "key", "--ttl", "24h"})
	if err != nil {
		t.Fatalf("tokenIssue custom ttl: %v", err)
	}
}

func TestTokenIssue_RequiresURL(t *testing.T) {
	var buf bytes.Buffer
	if err := tokenIssue(&buf, []string{"--api-key", "key"}); err == nil {
		t.Fatal("expected error when --url is missing")
	}
}

func TestTokenIssue_RequiresAPIKey(t *testing.T) {
	var buf bytes.Buffer
	if err := tokenIssue(&buf, []string{"--url", "http://localhost:8080"}); err == nil {
		t.Fatal("expected error when --api-key is missing")
	}
}

func TestTokenIssue_InvalidScope(t *testing.T) {
	var buf bytes.Buffer
	if err := tokenIssue(&buf, []string{"--url", "http://localhost:8080", "--api-key", "key", "--scope", "admin"}); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestTokenIssue_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	err := tokenIssue(&buf, []string{"--url", srv.URL, "--api-key", "wrong"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected HTTP 401 error, got: %v", err)
	}
}

func TestCmdToken_NoSubcommand(t *testing.T) {
	if err := cmdToken([]string{}); err == nil {
		t.Fatal("expected error when no subcommand given")
	}
}

func TestCmdToken_UnknownSubcommand(t *testing.T) {
	if err := cmdToken([]string{"revoke"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}
