package store

import (
	"time"
)

// DLQEntry is a forwarding attempt that exhausted all retries.
type DLQEntry struct {
	ID              string `json:"id"`
	IdempotencyKey  string `json:"idempotency_key"`
	PeerAddr        string `json:"peer_addr"`
	Attempts        int    `json:"attempts"`
	LastErr         string `json:"last_err"`
	FirstSeenUnix   int64  `json:"first_seen_unix"`
	LastAttemptUnix int64  `json:"last_attempt_unix"`
	Payload         []byte `json:"payload,omitempty"`
}

// AuditEntry mirrors the audit log row persisted to the store.
type AuditEntry struct {
	Seq             uint64 `json:"seq"`
	TimestampUnixMS int64  `json:"timestamp_unix_ms"`
	Decision        string `json:"decision"`
	SenderRegion    string `json:"sender_region"`
	RecipientRegion string `json:"recipient_region"`
	PolicyVersion   string `json:"policy_version"`
	Profile         string `json:"profile"`
	ApplicableLaw   string `json:"applicable_law"`
	Reason          string `json:"reason"`
}

// CRLEntry stores a certificate revocation record.
type CRLEntry struct {
	NodeID     string   `json:"node_id"`
	Reason     string   `json:"reason"`
	RevokedAt  int64    `json:"revoked_at_unix"`
	Signatures [][]byte `json:"signatures"`
}

// NodeStore is the persistence interface shared by all storage backends.
// Implementations must be safe for concurrent use.
type NodeStore interface {
	// Deduped returns true if the key was seen within its TTL window.
	// If not seen, it registers the key and returns false. Atomic.
	Deduped(key string, ttl time.Duration) (bool, error)

	// DLQ operations
	DLQPush(entry DLQEntry) error
	DLQList() ([]DLQEntry, error)
	DLQDelete(id string) error

	// Audit log operations
	AuditAppend(entry AuditEntry) error
	AuditLatest(n int) ([]AuditEntry, error)

	// CRL operations
	CRLPut(entry CRLEntry) error
	CRLGet(nodeID string) (*CRLEntry, error)
	CRLList() ([]CRLEntry, error)

	Close() error
}
