package token

import (
	"testing"
	"time"
)

func TestIssueAndResolve(t *testing.T) {
	s := New()
	tok, err := s.Issue("app-a", "node-x", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appID, nodeID, err := s.Resolve(tok)
	if err != nil {
		t.Fatalf("expected resolve to succeed: %v", err)
	}
	if appID != "app-a" || nodeID != "node-x" {
		t.Fatalf("unexpected metadata: appID=%q nodeID=%q", appID, nodeID)
	}
}

func TestEvictPreventsReuse(t *testing.T) {
	s := New()
	tok, _ := s.Issue("app-a", "node-x", time.Minute)
	s.Evict(tok)
	_, _, err := s.Resolve(tok)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after evict, got %v", err)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	s := New()
	tok, _ := s.Issue("app-a", "node-x", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, _, err := s.Resolve(tok)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after expiry, got %v", err)
	}
}

func TestInvalidTokenRejected(t *testing.T) {
	s := New()
	_, _, err := s.Resolve("not-a-real-token")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for invalid token, got %v", err)
	}
}

func TestHasActiveToken(t *testing.T) {
	s := New()
	if s.HasActiveToken("node-x") {
		t.Fatal("expected false before any token issued")
	}
	tok, _ := s.Issue("app-a", "node-x", time.Minute)
	if !s.HasActiveToken("node-x") {
		t.Fatal("expected true after issue")
	}
	s.Evict(tok)
	if s.HasActiveToken("node-x") {
		t.Fatal("expected false after evict")
	}
}

func TestConcurrentIssueResolve(t *testing.T) {
	s := New()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			tok, _ := s.Issue("app", "node", time.Minute)
			s.Resolve(tok) //nolint:errcheck
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		s.Purge()
	}
	<-done
}

func TestPurgeRemovesExpired(t *testing.T) {
	s := New()
	s.Issue("app", "node", time.Millisecond) //nolint:errcheck
	time.Sleep(5 * time.Millisecond)
	s.Purge()
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 entries after purge, got %d", n)
	}
}
