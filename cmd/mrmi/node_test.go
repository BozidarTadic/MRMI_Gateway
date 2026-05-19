package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockServer(t *testing.T, path string, body any) (*httptest.Server, nodeOpts) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal mock body: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, nodeOpts{url: srv.URL}
}

func TestNodeStatus_PrintsFields(t *testing.T) {
	_, opts := mockServer(t, "/api/v1/status", map[string]any{
		"node_id":        "rs-node-01",
		"region":         "RS",
		"node_scope":     "regional",
		"profile":        "balanced",
		"applicable_law": "RS-GDPR",
		"app_version":    "0.1.0",
		"adr_version":    "0.8",
		"uptime_seconds": 3661,
	})

	var buf bytes.Buffer
	if err := nodeStatus(&buf, opts); err != nil {
		t.Fatalf("nodeStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"rs-node-01", "RS", "regional", "balanced", "RS-GDPR", "0.1.0", "0.8", "1h1m1s"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestNodePeers_PrintsTable(t *testing.T) {
	_, opts := mockServer(t, "/api/v1/peers", []map[string]any{
		{"node_id": "ru-node-01", "addr": "10.0.0.2:9090", "node_scope": "regional", "region": "RU", "source": "config"},
	})

	var buf bytes.Buffer
	if err := nodePeers(&buf, opts); err != nil {
		t.Fatalf("nodePeers: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"NODE_ID", "ru-node-01", "10.0.0.2:9090", "regional", "RU", "config"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestNodePeers_EmptyList(t *testing.T) {
	_, opts := mockServer(t, "/api/v1/peers", []map[string]any{})

	var buf bytes.Buffer
	if err := nodePeers(&buf, opts); err != nil {
		t.Fatalf("nodePeers empty: %v", err)
	}
	if !strings.Contains(buf.String(), "NODE_ID") {
		t.Error("expected header row even with empty peer list")
	}
}

func TestNodeDLQ_PrintsTable(t *testing.T) {
	_, opts := mockServer(t, "/api/v1/dlq", []map[string]any{
		{
			"index": 0, "envelope_id": "key-abc", "peer_addr": "10.0.0.2:9090",
			"attempts": 3, "first_seen_unix": 1700000000, "last_error": "connection refused",
		},
	})

	var buf bytes.Buffer
	if err := nodeDLQ(&buf, opts); err != nil {
		t.Fatalf("nodeDLQ: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"INDEX", "key-abc", "10.0.0.2:9090", "3", "connection refused"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestNodeApps_PrintsTable(t *testing.T) {
	_, opts := mockServer(t, "/api/v1/apps", []map[string]any{
		{"app_id": "myapp", "webhook_url": "https://example.com/hook"},
	})

	var buf bytes.Buffer
	if err := nodeApps(&buf, opts); err != nil {
		t.Fatalf("nodeApps: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"APP_ID", "myapp", "https://example.com/hook"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestNodeStatus_RequiresURL(t *testing.T) {
	if err := cmdNodeStatus([]string{}); err == nil {
		t.Fatal("expected error when --url is missing")
	}
}

func TestCmdNode_UnknownSubcommand(t *testing.T) {
	if err := cmdNode([]string{"invalid"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestCmdNode_NoSubcommand(t *testing.T) {
	if err := cmdNode([]string{}); err == nil {
		t.Fatal("expected error when no subcommand given")
	}
}

func TestNodeStatus_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	err := nodeStatus(&buf, nodeOpts{url: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected HTTP 401 error, got: %v", err)
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		secs int64
		want string
	}{
		{0, "0s"},
		{45, "45s"},
		{90, "1m30s"},
		{3661, "1h1m1s"},
		{7200, "2h0m0s"},
	}
	for _, c := range cases {
		got := formatUptime(c.secs)
		if got != c.want {
			t.Errorf("formatUptime(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestAPIGet_SendsAuthHeaders(t *testing.T) {
	var gotToken, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("Authorization")
		gotKey = r.Header.Get("X-MRMI-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	opts := nodeOpts{url: srv.URL, token: "my-jwt", apiKey: "my-key"}
	if _, err := apiGet(opts, "/"); err != nil {
		t.Fatalf("apiGet: %v", err)
	}
	if gotToken != "Bearer my-jwt" {
		t.Errorf("Authorization = %q, want %q", gotToken, "Bearer my-jwt")
	}
	if gotKey != "my-key" {
		t.Errorf("X-MRMI-Key = %q, want %q", gotKey, "my-key")
	}
}
