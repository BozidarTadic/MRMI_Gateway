package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"MRMI_Gateway/internal/config"
)

type Decision string

const (
	DecisionAllow     Decision = "ALLOW"
	DecisionDeny      Decision = "DENY"
	DecisionDuplicate Decision = "DUPLICATE"
)

type Entry struct {
	Seq             uint64   `json:"seq"`
	Timestamp       int64    `json:"timestamp"`
	Decision        Decision `json:"decision"`
	SenderRegion    string   `json:"sender_region"`
	RecipientRegion string   `json:"recipient_region"`
	PolicyVersion   string   `json:"policy_version"`
	Profile         string   `json:"profile"`
	ApplicableLaw   string   `json:"applicable_law"`
	DedupTTLHours   uint64   `json:"dedup_ttl_hours"`
	NodeScope       string   `json:"node_scope"`        // "regional" | "alliance" | "global"
	AllianceID      string   `json:"alliance_id"`       // non-empty only for alliance nodes
	NodeRegion      string   `json:"node_region"`       // physical region of the node
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

func (l *Log) Append(cfg config.Config, decision Decision, senderRegion, recipientRegion string) Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	prevHash := l.root
	entry := Entry{
		Seq:             uint64(len(l.entries) + 1),
		Timestamp:       time.Now().UnixMilli(),
		Decision:        decision,
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

func hashEntry(entry Entry) string {
	payload := struct {
		Seq             uint64   `json:"seq"`
		Timestamp       int64    `json:"timestamp"`
		Decision        Decision `json:"decision"`
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
