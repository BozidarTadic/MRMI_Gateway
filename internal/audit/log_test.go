package audit

import (
	"sync"
	"testing"
	"time"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/store"
)

func TestLogVerify(t *testing.T) {
	log := New()
	cfg := config.DefaultBalancedConfig()

	log.Append(cfg, DecisionAllow, "POLICY_ACCEPTED", 1, "RS", "RU")
	log.Append(cfg, DecisionDeny, "RECIPIENT_REGION_DENIED", 0, "RS", "US")

	if err := log.Verify(); err != nil {
		t.Fatalf("expected audit log verification to pass, got %v", err)
	}
}

// memStore is a minimal in-memory NodeStore for testing audit persistence.
type memStore struct {
	mu      sync.Mutex
	entries []store.AuditEntry
}

func (m *memStore) AuditAppend(e store.AuditEntry) error {
	m.mu.Lock()
	m.entries = append(m.entries, e)
	m.mu.Unlock()
	return nil
}

func (m *memStore) AuditLatest(n int) ([]store.AuditEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := len(m.entries)
	if n > total {
		n = total
	}
	out := make([]store.AuditEntry, n)
	for i := 0; i < n; i++ {
		out[i] = m.entries[total-1-i]
	}
	return out, nil
}

func (m *memStore) Deduped(string, time.Duration) (bool, error) { return false, nil }
func (m *memStore) DLQPush(store.DLQEntry) error                { return nil }
func (m *memStore) DLQList() ([]store.DLQEntry, error)          { return nil, nil }
func (m *memStore) DLQDelete(string) error                      { return nil }
func (m *memStore) CRLPut(store.CRLEntry) error                 { return nil }
func (m *memStore) CRLGet(string) (*store.CRLEntry, error)      { return nil, nil }
func (m *memStore) CRLList() ([]store.CRLEntry, error)          { return nil, nil }
func (m *memStore) Close() error                                { return nil }

func TestLog_AppendPersistsToStore(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	ms := &memStore{}
	log := New()
	log.SetStore(ms)

	log.Append(cfg, DecisionAllow, "POLICY_ACCEPTED", 1, "RS", "RU")
	log.Append(cfg, DecisionDeny, "BLOCKED", 0, "RS", "US")

	ms.mu.Lock()
	n := len(ms.entries)
	ms.mu.Unlock()
	if n != 2 {
		t.Fatalf("expected 2 store entries, got %d", n)
	}
	if ms.entries[0].Decision != "ALLOW" {
		t.Errorf("expected ALLOW, got %q", ms.entries[0].Decision)
	}
	if ms.entries[1].SenderRegion != "RS" {
		t.Errorf("expected RS, got %q", ms.entries[1].SenderRegion)
	}
}

func TestLog_RecentPrefersStore(t *testing.T) {
	ms := &memStore{}
	ms.entries = []store.AuditEntry{
		{Seq: 1, Decision: "ALLOW", SenderRegion: "RS", RecipientRegion: "RU"},
		{Seq: 2, Decision: "DENY", SenderRegion: "RS", RecipientRegion: "US"},
	}

	log := New()
	log.SetStore(ms)

	// In-memory is empty — Recent should draw from store.
	entries := log.Recent(10)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from store, got %d", len(entries))
	}
	if entries[0].Decision != DecisionDeny {
		t.Errorf("expected newest first (DENY), got %q", entries[0].Decision)
	}
}

func TestLog_RecentFallsBackToMemory(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	log := New()
	// No store set — must use in-memory.
	log.Append(cfg, DecisionAllow, "", 1, "RS", "RU")

	entries := log.Recent(5)
	if len(entries) != 1 {
		t.Fatalf("expected 1 in-memory entry, got %d", len(entries))
	}
}

func TestLog_RootHashUnaffectedByStore(t *testing.T) {
	cfg := config.DefaultBalancedConfig()
	ms := &memStore{}
	log := New()
	log.SetStore(ms)

	before := log.RootHash()
	log.Append(cfg, DecisionAllow, "", 1, "RS", "RU")
	after := log.RootHash()

	if before == after {
		t.Error("root hash should change after append")
	}
	if err := log.Verify(); err != nil {
		t.Fatalf("chain verify failed: %v", err)
	}
}
