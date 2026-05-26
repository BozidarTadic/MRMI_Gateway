package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/server"
)

// decodeError reads the response body and returns the "error" field from a
// JSON error envelope {"error": "..."}.
func decodeError(t *testing.T, body io.Reader) string {
	t.Helper()
	var env struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("response is not a JSON error envelope: %v", err)
	}
	return env.Error
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.DefaultBalancedConfig()
	cfg.API.APIKey = "test-key"
	cfg.API.JWTSecret = "test-jwt-secret"
	srv := server.NewHTTPServer(cfg, server.Deps{})
	return httptest.NewServer(srv.Handler)
}

func TestErrorShape_Unauthorized(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/peers"},
		{http.MethodGet, "/api/v1/apps"},
		{http.MethodPost, "/api/v1/peers/register"},
		{http.MethodPost, "/api/v1/config/reload"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ts.URL+ep.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("expected application/json Content-Type, got %q", ct)
			}
			msg := decodeError(t, resp.Body)
			if msg == "" {
				t.Fatal("expected non-empty error message in JSON envelope")
			}
		})
	}
}

func TestErrorShape_ServiceUnavailable(t *testing.T) {
	ts := newTestServer(t) // no real deps wired → 503 for dep-requiring endpoints
	defer ts.Close()

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/audit/latest", ""},
		{http.MethodPost, "/api/v1/envelopes", `{"idempotency_key":"k1"}`},
		{http.MethodPost, "/api/v1/config/reload", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var bodyReader io.Reader
			if ep.body != "" {
				bodyReader = strings.NewReader(ep.body)
			} else {
				bodyReader = strings.NewReader("{}")
			}
			req, _ := http.NewRequest(ep.method, ts.URL+ep.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-MRMI-Key", "test-key")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("expected application/json Content-Type, got %q", ct)
			}
			msg := decodeError(t, resp.Body)
			if msg == "" {
				t.Fatal("expected non-empty error message in JSON envelope")
			}
		})
	}
}

func TestErrorShape_BadRequest(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Malformed JSON → 400 with JSON error envelope
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/peers/register",
		strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MRMI-Key", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}
	msg := decodeError(t, resp.Body)
	if msg != "invalid request body" {
		t.Fatalf("expected 'invalid request body', got %q", msg)
	}
}

func TestErrorShape_PeersRegister_DistinguishesErrors(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	cases := []struct {
		name        string
		body        string
		wantMsg     string
	}{
		{"malformed JSON", "not-json", "invalid request body"},
		{"missing addr", `{"node_id":"x"}`, "addr is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/peers/register",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-MRMI-Key", "test-key")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
			msg := decodeError(t, resp.Body)
			if msg != tc.wantMsg {
				t.Fatalf("expected %q, got %q", tc.wantMsg, msg)
			}
		})
	}
}

func TestErrorShape_TokenEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Missing API key → 401 JSON error
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/token",
		strings.NewReader(`{"scope":"read","ttl_minutes":10}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}
	msg := decodeError(t, resp.Body)
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}
