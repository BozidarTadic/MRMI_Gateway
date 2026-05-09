package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"MRMI_Gateway/internal/core"
)

var errTransient = errors.New("transient failure")

func TestSendWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	send := func() error { calls++; return nil }

	err := SendWithRetry(context.Background(), send, DefaultRetryPolicy(), nil, DLQEntry{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestSendWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	calls := 0
	send := func() error {
		calls++
		if calls < 2 {
			return errTransient
		}
		return nil
	}

	policy := RetryPolicy{BaseDelay: time.Millisecond, Multiplier: 2, Cap: time.Second, MaxAttempts: 5}
	err := SendWithRetry(context.Background(), send, policy, nil, DLQEntry{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestSendWithRetry_ExhaustionWritesToDLQ(t *testing.T) {
	send := func() error { return errTransient }

	dlq := NewDLQ()
	policy := RetryPolicy{BaseDelay: time.Millisecond, Multiplier: 1, Cap: time.Millisecond, MaxAttempts: 3}
	entry := DLQEntry{Envelope: core.Envelope{IdempotencyKey: "exhaust-1"}, PeerAddr: "peer:7777"}

	err := SendWithRetry(context.Background(), send, policy, dlq, entry)
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
	if dlq.Size() != 1 {
		t.Fatalf("expected 1 DLQ entry after exhaustion, got %d", dlq.Size())
	}
	stored := dlq.Entries()[0]
	if stored.Attempts != 3 {
		t.Fatalf("expected Attempts=3, got %d", stored.Attempts)
	}
	if stored.LastErr != errTransient {
		t.Fatalf("expected LastErr=errTransient, got %v", stored.LastErr)
	}
}

func TestSendWithRetry_ContextCancelStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	send := func() error {
		calls++
		if calls == 1 {
			cancel() // cancel after first failure
		}
		return errTransient
	}

	policy := RetryPolicy{BaseDelay: 10 * time.Millisecond, Multiplier: 2, Cap: time.Second, MaxAttempts: 10}
	err := SendWithRetry(ctx, send, policy, nil, DLQEntry{})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if calls > 2 {
		t.Fatalf("expected at most 2 calls after cancel, got %d", calls)
	}
}

func TestSendWithRetry_NilDLQNoopOnExhaustion(t *testing.T) {
	send := func() error { return errTransient }
	policy := RetryPolicy{BaseDelay: time.Millisecond, Multiplier: 1, Cap: time.Millisecond, MaxAttempts: 2}

	// must not panic with nil DLQ
	err := SendWithRetry(context.Background(), send, policy, nil, DLQEntry{})
	if err == nil {
		t.Fatal("expected error")
	}
}
