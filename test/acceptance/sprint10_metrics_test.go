package acceptance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/crl"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/delivery"
	"MRMI_Gateway/internal/metrics"
	"MRMI_Gateway/internal/policy"
)

// startMetricsNode starts a gateway with a live /metrics endpoint.
// Returns the base metrics URL (e.g. "http://127.0.0.1:PORT") and the gateway.
// Uses httptest.NewServer so the server is guaranteed to be listening before returning.
func startMetricsNode(t *testing.T) (metricsBase string, gw *core.Gateway, reg *metrics.Registry) {
	t.Helper()

	cfg := config.DefaultBalancedConfig()
	cfg.Node.NodeID = "metrics-test-node"
	cfg.Node.Region = "RS"
	cfg.Policy.Outbound.AllowTo = []string{"RU"}

	auditLog := audit.New()
	crlStore := crl.New()
	engine, err := policy.NewEngine(cfg, auditLog, crlStore)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}

	// dlq wires the DLQ-depth gauge; no entries expected in these tests (gauge stays 0).
	dlq := delivery.NewDLQ()
	reg = metrics.New(dlq.Size, nil, nil)

	gw = core.NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	gw.SetOnAllow(func(_ core.Envelope) { reg.IncAllow() })
	gw.SetOnDeny(func(reason string) { reg.IncDeny(reason) })
	gw.SetOnDuplicate(func() { reg.IncDuplicate() })

	srv := httptest.NewServer(reg.Handler())
	t.Cleanup(srv.Close)

	return srv.URL, gw, reg
}

func scrapeMetrics(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("scrape /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
	return string(body)
}

func assertMetric(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("expected metrics to contain %q\ngot:\n%s", want, body)
	}
}

// TestMetrics_AllowCounter verifies that ALLOW decisions increment mrmi_envelope_allow_total.
func TestMetrics_AllowCounter(t *testing.T) {
	base, gw, _ := startMetricsNode(t)

	for i := 0; i < 3; i++ {
		_, err := gw.SendEnvelope(context.Background(), core.SendRequest{
			Envelope: core.Envelope{
				IdempotencyKey:  fmt.Sprintf("allow-key-%d", i),
				SenderRegion:    "RS",
				RecipientRegion: "RU",
				TrustTier:       1,
				Timestamp:       time.Now().UnixMilli(),
			},
		})
		if err != nil {
			t.Fatalf("SendEnvelope %d: %v", i, err)
		}
	}

	body := scrapeMetrics(t, base)
	assertMetric(t, body, "mrmi_envelope_allow_total 3")
}

// TestMetrics_DenyCounter verifies that DENY decisions increment mrmi_envelope_deny_total with reason label.
func TestMetrics_DenyCounter(t *testing.T) {
	base, gw, _ := startMetricsNode(t)

	// Send to a blocked region to trigger a DENY.
	_, err := gw.SendEnvelope(context.Background(), core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "deny-key-1",
			SenderRegion:    "RS",
			RecipientRegion: "US", // not in AllowTo
			TrustTier:       1,
			Timestamp:       time.Now().UnixMilli(),
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	body := scrapeMetrics(t, base)
	if !strings.Contains(body, "mrmi_envelope_deny_total{reason=") {
		t.Errorf("expected deny counter with reason label\ngot:\n%s", body)
	}
}

// TestMetrics_DuplicateCounter verifies that duplicate envelopes increment mrmi_envelope_duplicate_total.
func TestMetrics_DuplicateCounter(t *testing.T) {
	base, gw, _ := startMetricsNode(t)

	env := core.SendRequest{
		Envelope: core.Envelope{
			IdempotencyKey:  "dup-key-1",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			TrustTier:       1,
			Timestamp:       time.Now().UnixMilli(),
		},
	}

	if _, err := gw.SendEnvelope(context.Background(), env); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := gw.SendEnvelope(context.Background(), env); err != nil {
		t.Fatalf("duplicate send: %v", err)
	}

	body := scrapeMetrics(t, base)
	assertMetric(t, body, "mrmi_envelope_allow_total 1")
	assertMetric(t, body, "mrmi_envelope_duplicate_total 1")
}

// TestMetrics_HelpAndTypeLines verifies all required # HELP / # TYPE lines are present.
func TestMetrics_HelpAndTypeLines(t *testing.T) {
	base, _, _ := startMetricsNode(t)
	body := scrapeMetrics(t, base)

	for _, metric := range []string{
		"mrmi_envelope_allow_total",
		"mrmi_envelope_deny_total",
		"mrmi_envelope_duplicate_total",
		"mrmi_dlq_len",
		"mrmi_transit_cache_len",
		"mrmi_rate_limit_deny_total",
		"mrmi_peer_count",
	} {
		assertMetric(t, body, "# HELP "+metric)
		assertMetric(t, body, "# TYPE "+metric)
	}
}

// TestMetrics_ContentType verifies the response carries the Prometheus text format content type.
func TestMetrics_ContentType(t *testing.T) {
	base, _, _ := startMetricsNode(t)
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "0.0.4") {
		t.Errorf("Content-Type = %q, want text/plain; version=0.0.4", ct)
	}
}
