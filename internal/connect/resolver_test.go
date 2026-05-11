package connect

import (
	"testing"

	"MRMI_Gateway/internal/config"
)

func cfg(autoAccept string, trusted ...string) config.Config {
	c := config.DefaultBalancedConfig()
	c.Policy.Connect = config.ConnectPolicy{
		AutoAccept:   autoAccept,
		TrustedNodes: trusted,
	}
	return c
}

func TestManual_AlwaysPending(t *testing.T) {
	r := New(cfg("MANUAL"))
	if s := r.Resolve(Request{OriginNodeID: "node-x"}); s != StatusPending {
		t.Fatalf("expected PENDING, got %s", s)
	}
}

func TestAutoAll_AlwaysAccepted(t *testing.T) {
	r := New(cfg("AUTO_ALL"))
	if s := r.Resolve(Request{OriginNodeID: "any-node"}); s != StatusAccepted {
		t.Fatalf("expected ACCEPTED, got %s", s)
	}
}

func TestAutoWhitelist_AcceptsTrusted(t *testing.T) {
	r := New(cfg("AUTO_WHITELIST", "node-trusted"))
	if s := r.Resolve(Request{OriginNodeID: "node-trusted"}); s != StatusAccepted {
		t.Fatalf("expected ACCEPTED for trusted node, got %s", s)
	}
}

func TestAutoWhitelist_PendingForUnknown(t *testing.T) {
	r := New(cfg("AUTO_WHITELIST", "node-trusted"))
	if s := r.Resolve(Request{OriginNodeID: "node-unknown"}); s != StatusPending {
		t.Fatalf("expected PENDING for unknown node, got %s", s)
	}
}

func TestAutoMutual_AcceptsWhenMutualDiscovery(t *testing.T) {
	r := New(cfg("AUTO_MUTUAL"))
	req := Request{OriginNodeID: "node-x", MutualDiscovery: true}
	if s := r.Resolve(req); s != StatusAccepted {
		t.Fatalf("expected ACCEPTED with mutual discovery, got %s", s)
	}
}

func TestAutoMutual_PendingWhenNoMutualDiscovery(t *testing.T) {
	r := New(cfg("AUTO_MUTUAL"))
	req := Request{OriginNodeID: "node-x", MutualDiscovery: false}
	if s := r.Resolve(req); s != StatusPending {
		t.Fatalf("expected PENDING without mutual discovery, got %s", s)
	}
}

func TestDefaultMode_IsPending(t *testing.T) {
	r := New(cfg("")) // empty → MANUAL
	if s := r.Resolve(Request{OriginNodeID: "node-x"}); s != StatusPending {
		t.Fatalf("expected PENDING from default mode, got %s", s)
	}
}

func TestReload_ChangesMode(t *testing.T) {
	r := New(cfg("MANUAL"))
	if s := r.Resolve(Request{OriginNodeID: "x"}); s != StatusPending {
		t.Fatalf("pre-reload: expected PENDING, got %s", s)
	}
	newCfg := cfg("AUTO_ALL")
	r.Reload(newCfg)
	if s := r.Resolve(Request{OriginNodeID: "x"}); s != StatusAccepted {
		t.Fatalf("post-reload: expected ACCEPTED, got %s", s)
	}
}
