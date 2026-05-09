package delivery

import (
	"testing"

	"MRMI_Gateway/internal/core"
)

func TestDLQ_AppendAndSize(t *testing.T) {
	d := NewDLQ()
	if d.Size() != 0 {
		t.Fatalf("expected empty DLQ, got size %d", d.Size())
	}

	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k1"}, PeerAddr: "localhost:7777"})
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k2"}, PeerAddr: "localhost:7778"})

	if d.Size() != 2 {
		t.Fatalf("expected size 2, got %d", d.Size())
	}
}

func TestDLQ_EntriesReturnsCopy(t *testing.T) {
	d := NewDLQ()
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k1"}})

	entries := d.Entries()
	entries[0].PeerAddr = "mutated"

	// mutation of snapshot must not affect the queue
	stored := d.Entries()
	if stored[0].PeerAddr == "mutated" {
		t.Fatal("Entries() must return a copy, not a reference to internal slice")
	}
}

func TestDLQ_Remove(t *testing.T) {
	d := NewDLQ()
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k1"}})
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k2"}})
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k3"}})

	d.Remove(1) // remove k2

	entries := d.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(entries))
	}
	if entries[0].Envelope.IdempotencyKey != "k1" || entries[1].Envelope.IdempotencyKey != "k3" {
		t.Fatalf("unexpected entries after remove: %v", entries)
	}
}

func TestDLQ_RemoveOutOfBoundsIsNoOp(t *testing.T) {
	d := NewDLQ()
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k1"}})

	d.Remove(-1)
	d.Remove(5)

	if d.Size() != 1 {
		t.Fatalf("out-of-bounds Remove must be a no-op, got size %d", d.Size())
	}
}

func TestDLQ_FirstSeenSetOnAppend(t *testing.T) {
	d := NewDLQ()
	d.Append(DLQEntry{Envelope: core.Envelope{IdempotencyKey: "k1"}})

	entries := d.Entries()
	if entries[0].FirstSeenUnix == 0 {
		t.Fatal("expected FirstSeenUnix to be set automatically on Append")
	}
}
