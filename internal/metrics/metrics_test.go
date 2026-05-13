package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"MRMI_Gateway/internal/metrics"
)

func newReg(dlq, transit, peers int) *metrics.Registry {
	return metrics.New(
		func() int { return dlq },
		func() int { return transit },
		func() int { return peers },
	)
}

func scrape(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
	return string(body)
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("metrics output missing %q\ngot:\n%s", want, body)
	}
}

func TestAllCounterStartsAtZero(t *testing.T) {
	body := scrape(t, newReg(0, 0, 0))
	assertContains(t, body, "mrmi_envelope_allow_total 0")
	assertContains(t, body, "mrmi_envelope_duplicate_total 0")
	assertContains(t, body, "mrmi_rate_limit_deny_total 0")
	assertContains(t, body, "mrmi_dlq_len 0")
	assertContains(t, body, "mrmi_transit_cache_len 0")
	assertContains(t, body, "mrmi_peer_count 0")
}

func TestIncAllow(t *testing.T) {
	reg := newReg(0, 0, 0)
	reg.IncAllow()
	reg.IncAllow()
	reg.IncAllow()
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_envelope_allow_total 3")
}

func TestIncDuplicate(t *testing.T) {
	reg := newReg(0, 0, 0)
	reg.IncDuplicate()
	reg.IncDuplicate()
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_envelope_duplicate_total 2")
}

func TestIncRateLimitDeny(t *testing.T) {
	reg := newReg(0, 0, 0)
	reg.IncRateLimitDeny()
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_rate_limit_deny_total 1")
}

func TestIncDenyWithReason(t *testing.T) {
	reg := newReg(0, 0, 0)
	reg.IncDeny("POLICY_DENY")
	reg.IncDeny("POLICY_DENY")
	reg.IncDeny("TRUST_TIER_BELOW_MINIMUM")
	body := scrape(t, reg)
	assertContains(t, body, `mrmi_envelope_deny_total{reason="POLICY_DENY"} 2`)
	assertContains(t, body, `mrmi_envelope_deny_total{reason="TRUST_TIER_BELOW_MINIMUM"} 1`)
}

func TestGaugesFromGetters(t *testing.T) {
	reg := newReg(7, 3, 5)
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_dlq_len 7")
	assertContains(t, body, "mrmi_transit_cache_len 3")
	assertContains(t, body, "mrmi_peer_count 5")
}

func TestNilGettersDefaultToZero(t *testing.T) {
	reg := metrics.New(nil, nil, nil)
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_dlq_len 0")
	assertContains(t, body, "mrmi_transit_cache_len 0")
	assertContains(t, body, "mrmi_peer_count 0")
}

func TestTypeAndHelpLinesPresent(t *testing.T) {
	body := scrape(t, newReg(0, 0, 0))
	for _, metric := range []string{
		"mrmi_envelope_allow_total",
		"mrmi_envelope_deny_total",
		"mrmi_envelope_duplicate_total",
		"mrmi_dlq_len",
		"mrmi_transit_cache_len",
		"mrmi_rate_limit_deny_total",
		"mrmi_peer_count",
	} {
		assertContains(t, body, "# HELP "+metric)
		assertContains(t, body, "# TYPE "+metric)
	}
}

func TestConcurrentIncrements(t *testing.T) {
	reg := newReg(0, 0, 0)
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			reg.IncAllow()
			reg.IncDuplicate()
			reg.IncDeny("RACE_REASON")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	body := scrape(t, reg)
	assertContains(t, body, "mrmi_envelope_allow_total 100")
	assertContains(t, body, "mrmi_envelope_duplicate_total 100")
	assertContains(t, body, `mrmi_envelope_deny_total{reason="RACE_REASON"} 100`)
}
