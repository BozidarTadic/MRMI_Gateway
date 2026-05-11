package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"MRMI_Gateway/internal/store"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	s := &Store{
		client: goredis.NewClient(&goredis.Options{Addr: mr.Addr()}),
		prefix: "test:",
	}
	t.Cleanup(func() { s.Close() })
	return s, mr
}

func TestDedup_NewKeyNotSeen(t *testing.T) {
	s, _ := newTestStore(t)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || seen {
		t.Fatalf("expected false for new key, seen=%v err=%v", seen, err)
	}
}

func TestDedup_SeenWithinTTL(t *testing.T) {
	s, _ := newTestStore(t)
	s.Deduped("key1", time.Minute)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || !seen {
		t.Fatalf("expected true for repeated key, seen=%v err=%v", seen, err)
	}
}

func TestDedup_ExpiredKeyNotSeen(t *testing.T) {
	s, mr := newTestStore(t)
	s.Deduped("key1", time.Second)
	mr.FastForward(2 * time.Second)
	seen, err := s.Deduped("key1", time.Minute)
	if err != nil || seen {
		t.Fatalf("expected false after TTL expiry, seen=%v err=%v", seen, err)
	}
}

func TestDLQ_PushListDelete(t *testing.T) {
	s, _ := newTestStore(t)
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
	s, _ := newTestStore(t)
	for i := uint64(1); i <= 5; i++ {
		s.AuditAppend(store.AuditEntry{Seq: i, Decision: "ALLOW", SenderRegion: "RS"})
	}
	entries, err := s.AuditLatest(3)
	if err != nil || len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d err=%v", len(entries), err)
	}
	// Descending order: seq 5, 4, 3
	if entries[0].Seq != 5 {
		t.Fatalf("expected seq=5 first, got %d", entries[0].Seq)
	}
}

func TestCRL_PutGetList(t *testing.T) {
	s, _ := newTestStore(t)
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
	notFound, _ := s.CRLGet("unknown-node")
	if notFound != nil {
		t.Fatal("expected nil for unknown node")
	}
}

func TestKeyPrefixIsolation(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()

	s1 := &Store{client: goredis.NewClient(&goredis.Options{Addr: mr.Addr()}), prefix: "node-a:"}
	s2 := &Store{client: goredis.NewClient(&goredis.Options{Addr: mr.Addr()}), prefix: "node-b:"}
	defer s1.Close()
	defer s2.Close()

	s1.DLQPush(store.DLQEntry{ID: "e1", IdempotencyKey: "k1"})
	list, _ := s2.DLQList()
	if len(list) != 0 {
		t.Fatal("key prefix isolation failed: node-b sees node-a's DLQ")
	}
}
