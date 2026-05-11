package registry

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func cfg() config.Config {
	c := config.DefaultBalancedConfig()
	c.Apps = map[string]config.AppConfig{
		"rs-app": {
			AutoAccept: "auto_mutual",
			Users: map[string]config.UserConfig{
				"user-marko": {DisplayHint: "Marko Petrović", Region: "RS"},
				"user-ana":   {DisplayHint: "Ana Jović", Region: "RS"},
			},
		},
	}
	return c
}

func TestDiscover_ByDisplayHint(t *testing.T) {
	r := New(cfg())
	results := r.Discover("marko", "display_hint")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].UserID != "user-marko" {
		t.Fatalf("unexpected user_id: %q", results[0].UserID)
	}
	if results[0].OpaqueToken == "" {
		t.Fatal("opaque_token must not be empty")
	}
}

func TestDiscover_ByAppID(t *testing.T) {
	r := New(cfg())
	results := r.Discover("rs-app", "app_id")
	if len(results) != 2 {
		t.Fatalf("expected 2 results for app_id query, got %d", len(results))
	}
}

func TestDiscover_NoMatch(t *testing.T) {
	r := New(cfg())
	results := r.Discover("nobody", "display_hint")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestConnect_AcceptedViaAutoMutual(t *testing.T) {
	r := New(cfg())
	// RU is in cfg allow_to list.
	results := r.Discover("marko", "")
	if len(results) == 0 {
		t.Fatal("no discovery results")
	}
	token := results[0].OpaqueToken

	res := r.Connect(token, "ru-user-01", "RU")
	if res.Status != "ACCEPTED" {
		t.Fatalf("expected ACCEPTED, got %q", res.Status)
	}
	if res.SessionID == "" {
		t.Fatal("session_id must not be empty")
	}
}

func TestConnect_PendingForNonAllowedRegion(t *testing.T) {
	r := New(cfg())
	results := r.Discover("marko", "")
	token := results[0].OpaqueToken

	res := r.Connect(token, "us-user", "US")
	if res.Status != "PENDING" {
		t.Fatalf("expected PENDING, got %q", res.Status)
	}
}

func TestConnect_TokenOneTimeUse(t *testing.T) {
	r := New(cfg())
	results := r.Discover("marko", "")
	token := results[0].OpaqueToken

	r.Connect(token, "ru-user-01", "RU")
	res2 := r.Connect(token, "ru-user-01", "RU")
	if res2.Status != "DENIED" {
		t.Fatalf("expected DENIED on reuse, got %q", res2.Status)
	}
}

func TestConnect_InvalidToken(t *testing.T) {
	r := New(cfg())
	res := r.Connect("not-a-real-token", "ru-user", "RU")
	if res.Status != "DENIED" {
		t.Fatalf("expected DENIED for invalid token, got %q", res.Status)
	}
}
