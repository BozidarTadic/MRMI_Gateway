package session

import (
	"testing"
)

func TestTracker_NextSeq_StartsAtOne(t *testing.T) {
	tr := New()
	if got := tr.NextSeq("alice"); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestTracker_NextSeq_Increments(t *testing.T) {
	tr := New()
	for i := uint64(1); i <= 5; i++ {
		if got := tr.NextSeq("alice"); got != i {
			t.Fatalf("step %d: expected %d, got %d", i, i, got)
		}
	}
}

func TestTracker_NextSeq_IndependentSenders(t *testing.T) {
	tr := New()
	tr.NextSeq("alice")
	tr.NextSeq("alice")
	if got := tr.NextSeq("bob"); got != 1 {
		t.Fatalf("expected bob to start at 1, got %d", got)
	}
}

func TestTracker_Validate_InOrder(t *testing.T) {
	tr := New()
	for i := uint64(1); i <= 3; i++ {
		if err := tr.Validate("alice", i); err != nil {
			t.Fatalf("seq %d: unexpected error: %v", i, err)
		}
	}
}

func TestTracker_Validate_OutOfOrder_ReturnsError(t *testing.T) {
	tr := New()
	_ = tr.Validate("alice", 1)
	_ = tr.Validate("alice", 2)
	if err := tr.Validate("alice", 10); err == nil {
		t.Fatal("expected out-of-order warning, got nil")
	}
}

func TestTracker_Validate_OutOfOrder_DoesNotBlockFutureSeqs(t *testing.T) {
	tr := New()
	_ = tr.Validate("alice", 1)
	_ = tr.Validate("alice", 5) // gap — advances to 5
	// next expected is 6
	if err := tr.Validate("alice", 6); err != nil {
		t.Fatalf("expected seq 6 to be accepted after gap, got: %v", err)
	}
}

func TestTracker_Validate_NewSenderAcceptsOne(t *testing.T) {
	tr := New()
	if err := tr.Validate("carol", 1); err != nil {
		t.Fatalf("expected seq 1 from new sender to pass, got: %v", err)
	}
}
