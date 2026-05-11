package bbolt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"MRMI_Gateway/internal/store"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDedup_NewKeyNotSeen(t *testing.T) {
	s := openTemp(t)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || seen {
		t.Fatalf("expected false for new key, seen=%v err=%v", seen, err)
	}
}

func TestDedup_SeenWithinTTL(t *testing.T) {
	s := openTemp(t)
	s.Deduped("key1", time.Minute)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || !seen {
		t.Fatalf("expected true for repeated key, seen=%v err=%v", seen, err)
	}
}

func TestDedup_ExpiredKeyNotSeen(t *testing.T) {
	s := openTemp(t)
	s.Deduped("key1", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || seen {
		t.Fatalf("expected false for expired key, seen=%v err=%v", seen, err)
	}
}

func TestDedup_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s1, _ := Open(dir)
	s1.Deduped("persistent-key", time.Hour)
	s1.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	seen, _ := s2.Deduped("persistent-key", time.Hour)
	if !seen {
		t.Fatal("expected key to persist across reopen")
	}
}

func TestDLQ_PushListDelete(t *testing.T) {
	s := openTemp(t)
	e := store.DLQEntry{ID: "dlq-001", IdempotencyKey: "env-001", PeerAddr: "peer:7777", Attempts: 3}
	if err := s.DLQPush(e); err != nil {
		t.Fatal(err)
	}
	list, err := s.DLQList()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d err=%v", len(list), err)
	}
	if list[0].ID != "dlq-001" {
		t.Fatalf("unexpected entry ID %q", list[0].ID)
	}
	if err := s.DLQDelete("dlq-001"); err != nil {
		t.Fatal(err)
	}
	list, _ = s.DLQList()
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestAudit_AppendAndLatest(t *testing.T) {
	s := openTemp(t)
	for i := uint64(1); i <= 5; i++ {
		s.AuditAppend(store.AuditEntry{Seq: i, Decision: "ALLOW", SenderRegion: "RS"})
	}
	entries, err := s.AuditLatest(3)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d err=%v", len(entries), err)
	}
	// Should be descending order (latest first)
	if entries[0].Seq != 5 {
		t.Fatalf("expected seq=5 first, got %d", entries[0].Seq)
	}
}

func TestCRL_PutGetList(t *testing.T) {
	s := openTemp(t)
	e := store.CRLEntry{NodeID: "node-bad", Reason: "compromised", Signatures: [][]byte{[]byte("sig1"), []byte("sig2")}}
	if err := s.CRLPut(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.CRLGet("node-bad")
	if err != nil || got == nil {
		t.Fatalf("CRLGet: got=%v err=%v", got, err)
	}
	if got.Reason != "compromised" {
		t.Fatalf("unexpected reason %q", got.Reason)
	}
	list, err := s.CRLList()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 CRL entry, got %d err=%v", len(list), err)
	}
	// Not found
	notFound, _ := s.CRLGet("unknown-node")
	if notFound != nil {
		t.Fatal("expected nil for unknown node")
	}
}

func TestDLQ_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s1, _ := Open(dir)
	s1.DLQPush(store.DLQEntry{ID: "dlq-persist", IdempotencyKey: "env-x"})
	s1.Close()

	s2, _ := Open(dir)
	defer s2.Close()
	list, _ := s2.DLQList()
	if len(list) != 1 || list[0].ID != "dlq-persist" {
		t.Fatalf("DLQ did not persist across reopen: %v", list)
	}
}

func TestOpen_CreatesDBFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(filepath.Join(dir, "mrmi.db")); err != nil {
		t.Fatal("expected mrmi.db to exist")
	}
}
