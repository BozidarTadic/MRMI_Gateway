package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"MRMI_Gateway/internal/config"
)

type Decision string

const (
	DecisionAllow     Decision = "ALLOW"
	DecisionDeny      Decision = "DENY"
	DecisionDuplicate Decision = "DUPLICATE"
	DecisionDummy     Decision = "ALLOW/DUMMY"
)

type Entry struct {
	Seq             uint64   `json:"seq"`
	Timestamp       int64    `json:"timestamp"`
	Decision        Decision `json:"decision"`
	Reason          string   `json:"reason,omitempty"`
	TrustTier       uint32   `json:"trust_tier"`
	SenderRegion    string   `json:"sender_region"`
	RecipientRegion string   `json:"recipient_region"`
	PolicyVersion   string   `json:"policy_version"`
	Profile         string   `json:"profile"`
	ApplicableLaw   string   `json:"applicable_law"`
	DedupTTLHours   uint64   `json:"dedup_ttl_hours"`
	NodeScope       string   `json:"node_scope"`
	AllianceID      string   `json:"alliance_id"`
	NodeRegion      string   `json:"node_region"`
	PreviousHash    string   `json:"previous_hash"`
	EntryHash       string   `json:"entry_hash"`
}

type Log struct {
	mu      sync.RWMutex
	entries []Entry
	root    string
}

func New() *Log {
	return &Log{
		root: zeroHash(),
	}
}

func (l *Log) Append(cfg config.Config, decision Decision, reason string, trustTier uint32, senderRegion, recipientRegion string) Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	prevHash := l.root
	entry := Entry{
		Seq:             uint64(len(l.entries) + 1),
		Timestamp:       time.Now().UnixMilli(),
		Decision:        decision,
		Reason:          reason,
		TrustTier:       trustTier,
		SenderRegion:    senderRegion,
		RecipientRegion: recipientRegion,
		PolicyVersion:   cfg.Node.PolicyVersion,
		Profile:         cfg.Profile.Name,
		ApplicableLaw:   cfg.Node.ApplicableLaw,
		DedupTTLHours:   uint64(cfg.Profile.DedupTTL / time.Hour),
		NodeScope:       cfg.Node.NodeScope,
		AllianceID:      cfg.Node.AllianceID,
		NodeRegion:      cfg.Node.Region,
		PreviousHash:    prevHash,
	}
	entry.EntryHash = hashEntry(entry)

	l.entries = append(l.entries, entry)
	l.root = entry.EntryHash

	return entry
}

func (l *Log) RootHash() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.root
}

func (l *Log) Entries() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Recent returns the last n entries in reverse chronological order (newest first).
// If the log has fewer than n entries all entries are returned.
func (l *Log) Recent(n int) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := len(l.entries)
	if n <= 0 || total == 0 {
		return nil
	}
	if n > total {
		n = total
	}
	out := make([]Entry, n)
	for i := 0; i < n; i++ {
		out[i] = l.entries[total-1-i]
	}
	return out
}

func (l *Log) Verify() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	expectedPrev := zeroHash()
	for i, entry := range l.entries {
		if entry.PreviousHash != expectedPrev {
			return fmt.Errorf("entry %d previous hash mismatch", i+1)
		}
		if hashEntry(entry) != entry.EntryHash {
			return fmt.Errorf("entry %d hash mismatch", i+1)
		}
		expectedPrev = entry.EntryHash
	}

	if len(l.entries) == 0 && l.root != zeroHash() {
		return fmt.Errorf("empty log root mismatch")
	}
	if len(l.entries) > 0 && l.root != l.entries[len(l.entries)-1].EntryHash {
		return fmt.Errorf("root hash mismatch")
	}

	return nil
}

// WriteJSON serialises all current entries to path as a JSON array.
// The resulting file can be verified offline with VerifyFile.
func (l *Log) WriteJSON(path string) error {
	l.mu.RLock()
	entries := make([]Entry, len(l.entries))
	copy(entries, l.entries)
	l.mu.RUnlock()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entries)
}

// VerifyFile reads a JSON log file produced by WriteJSON, recomputes every
// entry hash, and checks that the Merkle chain is intact.
// Returns the final root hash on success.
func VerifyFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var entries []Entry
	if err := json.NewDecoder(f).Decode(&entries); err != nil {
		return "", fmt.Errorf("parse log file: %w", err)
	}

	expectedPrev := zeroHash()
	for i, entry := range entries {
		if entry.PreviousHash != expectedPrev {
			return "", fmt.Errorf("entry %d previous hash mismatch", i+1)
		}
		if hashEntry(entry) != entry.EntryHash {
			return "", fmt.Errorf("entry %d hash mismatch", i+1)
		}
		expectedPrev = entry.EntryHash
	}

	if len(entries) == 0 {
		return zeroHash(), nil
	}
	return entries[len(entries)-1].EntryHash, nil
}

func hashEntry(entry Entry) string {
	payload := struct {
		Seq             uint64   `json:"seq"`
		Timestamp       int64    `json:"timestamp"`
		Decision        Decision `json:"decision"`
		Reason          string   `json:"reason,omitempty"`
		TrustTier       uint32   `json:"trust_tier"`
		SenderRegion    string   `json:"sender_region"`
		RecipientRegion string   `json:"recipient_region"`
		PolicyVersion   string   `json:"policy_version"`
		Profile         string   `json:"profile"`
		ApplicableLaw   string   `json:"applicable_law"`
		DedupTTLHours   uint64   `json:"dedup_ttl_hours"`
		NodeScope       string   `json:"node_scope"`
		AllianceID      string   `json:"alliance_id"`
		NodeRegion      string   `json:"node_region"`
		PreviousHash    string   `json:"previous_hash"`
	}{
		Seq:             entry.Seq,
		Timestamp:       entry.Timestamp,
		Decision:        entry.Decision,
		Reason:          entry.Reason,
		TrustTier:       entry.TrustTier,
		SenderRegion:    entry.SenderRegion,
		RecipientRegion: entry.RecipientRegion,
		PolicyVersion:   entry.PolicyVersion,
		Profile:         entry.Profile,
		ApplicableLaw:   entry.ApplicableLaw,
		DedupTTLHours:   entry.DedupTTLHours,
		NodeScope:       entry.NodeScope,
		AllianceID:      entry.AllianceID,
		NodeRegion:      entry.NodeRegion,
		PreviousHash:    entry.PreviousHash,
	}

	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func zeroHash() string {
	return "sha256:" + stringsOfZeros(64)
}

func stringsOfZeros(length int) string {
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = '0'
	}
	return string(buf)
}
