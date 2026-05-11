package hotreload

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"MRMI_Gateway/internal/config"
)

// validTOML returns a minimal TOML config that passes config.Validate().
func validTOML(policyVersion string) []byte {
	return []byte(fmt.Sprintf(`
[node]
node_id        = "rs-node-01"
node_scope     = "regional"
region         = "RS"
operator_id    = "ops"
policy_version = %q
applicable_law = "RS-GDPR"
signed_by      = "ed25519:REPLACE_ME"

[network]
http_listen_addr = ":8080"
grpc_port        = 7777
`, policyVersion))
}

// TestWatcher_FiresOnFileChange writes an initial config, then overwrites with
// a new version and asserts onChange is called within 3 seconds.
func TestWatcher_FiresOnFileChange(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/node.toml"

	if err := os.WriteFile(path, validTOML("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fired := make(chan config.Config, 1)
	w := New()
	go w.Watch(ctx, path, func(cfg config.Config) {
		select {
		case fired <- cfg:
		default:
		}
	})

	// Give the watcher time to record the initial mtime.
	time.Sleep(700 * time.Millisecond)

	// Write a new config with a different policy_version.
	if err := os.WriteFile(path, validTOML("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-fired:
		if cfg.Node.PolicyVersion != "v2" {
			t.Fatalf("expected policy_version v2, got %q", cfg.Node.PolicyVersion)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not fire within 3 seconds of file change")
	}
}

// TestWatcher_ExitsOnCancel verifies the goroutine stops when ctx is cancelled.
func TestWatcher_ExitsOnCancel(t *testing.T) {
	path := t.TempDir() + "/node.toml"
	if err := os.WriteFile(path, validTOML("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := New()
	go func() {
		w.Watch(ctx, path, func(_ config.Config) {})
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not exit after context cancel")
	}
}

// TestWatcher_InvalidConfigNotForwarded verifies that a bad TOML write does not
// call onChange (the old config is preserved at the caller's side).
func TestWatcher_InvalidConfigNotForwarded(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/node.toml"

	if err := os.WriteFile(path, validTOML("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	called := make(chan struct{}, 1)
	w := New()
	go w.Watch(ctx, path, func(_ config.Config) {
		select {
		case called <- struct{}{}:
		default:
		}
	})

	time.Sleep(700 * time.Millisecond)

	// Write invalid TOML.
	if err := os.WriteFile(path, []byte("not valid toml :::"), 0644); err != nil {
		t.Fatal(err)
	}

	// onChange must NOT be called for invalid config.
	select {
	case <-called:
		t.Fatal("onChange was called for invalid config — must not happen")
	case <-time.After(1500 * time.Millisecond):
		// expected: nothing fired
	}
}
