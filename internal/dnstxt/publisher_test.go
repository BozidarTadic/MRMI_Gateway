package dnstxt

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPublisher_EmitFormat(t *testing.T) {
	var buf bytes.Buffer
	p := New("test-node", 50*time.Millisecond, &buf)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	p.Run(ctx, func() string { return "sha256:abc123" })

	if buf.Len() == 0 {
		t.Fatal("expected at least one emit, got none")
	}
	line := strings.SplitN(strings.TrimSpace(buf.String()), "\n", 2)[0]
	if !strings.HasPrefix(line, "v=1 ts=") {
		t.Fatalf("unexpected format: %q", line)
	}
	if !strings.Contains(line, "root=sha256:abc123") {
		t.Fatalf("missing root hash in output: %q", line)
	}
	if !strings.Contains(line, "node=test-node") {
		t.Fatalf("missing node id in output: %q", line)
	}
}

func TestPublisher_StopsOnContextCancel(t *testing.T) {
	var buf bytes.Buffer
	p := New("test-node", time.Hour, &buf)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		p.Run(ctx, func() string { return "hash" })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
	if buf.Len() != 0 {
		t.Fatal("expected no output when cancelled before first tick")
	}
}

func TestPublisher_MultipleEmits(t *testing.T) {
	var buf bytes.Buffer
	p := New("n1", 30*time.Millisecond, &buf)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	p.Run(ctx, func() string { return "hash" })

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple emits, got %d line(s)", len(lines))
	}
}
