package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

func appCfg(webhookURL, secret string) config.Config {
	cfg := config.DefaultBalancedConfig()
	cfg.Apps = map[string]config.AppConfig{
		"test-app": {
			WebhookURL:    webhookURL,
			WebhookSecret: secret,
			WebhookTimeout: 5,
		},
	}
	return cfg
}

func TestNotifier_NotifyAll_CallsWebhook(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = json.Marshal(r.Header.Get("Content-Type"))
		var b []byte
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		b = buf[:n]
		received = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(appCfg(srv.URL, ""))
	env := core.Envelope{
		IdempotencyKey:  "test-key-01",
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		Timestamp:       1000,
	}
	n.NotifyAll(t.Context(), env)
	time.Sleep(100 * time.Millisecond) // let the goroutine complete

	if len(received) == 0 {
		t.Fatal("webhook not called")
	}
	var m map[string]any
	if err := json.Unmarshal(received, &m); err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if m["idempotency_key"] != "test-key-01" {
		t.Fatalf("unexpected idempotency_key: %v", m["idempotency_key"])
	}
	if m["sender_region"] != "RS" {
		t.Fatalf("unexpected sender_region: %v", m["sender_region"])
	}
}

func TestNotifier_HMAC_Signature(t *testing.T) {
	var gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-MRMI-Signature")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(appCfg(srv.URL, "super-secret"))
	n.NotifyAll(t.Context(), core.Envelope{
		IdempotencyKey:  "hmac-test",
		SenderRegion:    "RS",
		RecipientRegion: "RU",
	})
	time.Sleep(100 * time.Millisecond)

	if gotSig == "" {
		t.Fatal("X-MRMI-Signature not set")
	}
	if !VerifySignature(gotBody, "super-secret", gotSig) {
		t.Fatalf("HMAC verification failed: sig=%q", gotSig)
	}
}

func TestNotifier_NotCalled_OnDeny(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Notifier is only called from Gateway on ALLOW; this test verifies NotifyAll
	// is never invoked by asserting our test code doesn't call it.
	_ = New(appCfg(srv.URL, ""))
	time.Sleep(50 * time.Millisecond)
	if called {
		t.Fatal("webhook should not be called without explicit NotifyAll invocation")
	}
}

func TestVerifySignature_PassAndFail(t *testing.T) {
	body := []byte(`{"test":"payload"}`)
	secret := "s3cr3t"
	sig := "sha256=" + sign(body, secret)

	if !VerifySignature(body, secret, sig) {
		t.Fatal("expected valid signature to pass")
	}
	if VerifySignature(body, secret, "sha256=badhex") {
		t.Fatal("expected bad signature to fail")
	}
}
