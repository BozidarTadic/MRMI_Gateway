package crl

import (
	"testing"
)

func TestStore_SingleSig_NotRevoked(t *testing.T) {
	s := New()
	s.Revoke("node-ru", "compromised", []byte("sig-a"))
	if s.IsRevoked("node-ru") {
		t.Fatal("single signature should not revoke")
	}
}

func TestStore_TwoSigs_Revoked(t *testing.T) {
	s := New()
	s.Revoke("node-ru", "compromised", []byte("sig-a"))
	s.Revoke("node-ru", "compromised", []byte("sig-b"))
	if !s.IsRevoked("node-ru") {
		t.Fatal("two signatures should revoke")
	}
}

func TestStore_DuplicateSig_NotCounted(t *testing.T) {
	s := New()
	s.Revoke("node-ru", "compromised", []byte("sig-a"))
	s.Revoke("node-ru", "compromised", []byte("sig-a")) // duplicate
	if s.IsRevoked("node-ru") {
		t.Fatal("duplicate signature should not count toward quorum")
	}
}

func TestStore_UnknownNode_NotRevoked(t *testing.T) {
	s := New()
	if s.IsRevoked("unknown") {
		t.Fatal("unknown node should not be revoked")
	}
}

func TestStore_Merge_PropagatesEntries(t *testing.T) {
	src := New()
	src.Revoke("node-by", "bad-actor", []byte("sig-1"))
	src.Revoke("node-by", "bad-actor", []byte("sig-2"))

	dst := New()
	dst.Merge(src.Entries())

	if !dst.IsRevoked("node-by") {
		t.Fatal("merged entries should revoke node-by")
	}
}

func TestStore_Merge_DeduplicatesSignatures(t *testing.T) {
	s := New()
	s.Revoke("node-kz", "stale", []byte("sig-a"))

	entries := []Entry{{NodeID: "node-kz", Reason: "stale", Signatures: [][]byte{[]byte("sig-a"), []byte("sig-b")}}}
	s.Merge(entries)

	e := s.Entries()
	for _, entry := range e {
		if entry.NodeID == "node-kz" && len(entry.Signatures) != 2 {
			t.Fatalf("expected 2 unique signatures after merge, got %d", len(entry.Signatures))
		}
	}
	if !s.IsRevoked("node-kz") {
		t.Fatal("node-kz should be revoked after merge")
	}
}
