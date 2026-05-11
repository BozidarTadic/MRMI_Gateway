package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"MRMI_Gateway/internal/store"
)

// Store is a Redis-backed implementation of store.NodeStore.
type Store struct {
	client *goredis.Client
	prefix string // key namespace, e.g. "mrmi:"
}

// New connects to a Redis server and returns a Store.
// prefix is prepended to every key so multiple nodes can share one Redis instance.
func New(redisURL, prefix string) (*Store, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis parse url: %w", err)
	}
	if prefix == "" {
		prefix = "mrmi:"
	}
	return &Store{client: goredis.NewClient(opts), prefix: prefix}, nil
}

func (s *Store) Close() error { return s.client.Close() }

func (s *Store) key(parts ...string) string {
	k := s.prefix
	for _, p := range parts {
		k += p
	}
	return k
}

// Deduped uses SET NX EX for atomic check-and-set.
func (s *Store) Deduped(key string, ttl time.Duration) (bool, error) {
	ctx := context.Background()
	ok, err := s.client.SetNX(ctx, s.key("dedup:", key), 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil // SetNX returns true when key was set (i.e. NOT seen before)
}

// DLQPush stores a DLQ entry as JSON in a Redis hash.
func (s *Store) DLQPush(entry store.DLQEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.client.HSet(context.Background(), s.key("dlq"), entry.ID, data).Err()
}

// DLQList returns all DLQ entries.
func (s *Store) DLQList() ([]store.DLQEntry, error) {
	m, err := s.client.HGetAll(context.Background(), s.key("dlq")).Result()
	if err != nil {
		return nil, err
	}
	out := make([]store.DLQEntry, 0, len(m))
	for _, v := range m {
		var e store.DLQEntry
		if err := json.Unmarshal([]byte(v), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// DLQDelete removes the DLQ entry with the given id.
func (s *Store) DLQDelete(id string) error {
	return s.client.HDel(context.Background(), s.key("dlq"), id).Err()
}

// AuditAppend appends an audit entry to a Redis list.
func (s *Store) AuditAppend(entry store.AuditEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.client.RPush(context.Background(), s.key("audit"), data).Err()
}

// AuditLatest returns the latest n entries from the audit list in descending order.
func (s *Store) AuditLatest(n int) ([]store.AuditEntry, error) {
	ctx := context.Background()
	// LRANGE with negative indices: -n..-1 gives the last n elements
	vals, err := s.client.LRange(ctx, s.key("audit"), int64(-n), -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]store.AuditEntry, 0, len(vals))
	for i := len(vals) - 1; i >= 0; i-- { // reverse for descending order
		var e store.AuditEntry
		if err := json.Unmarshal([]byte(vals[i]), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// CRLPut stores a CRL entry in a Redis hash keyed by node_id.
func (s *Store) CRLPut(entry store.CRLEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.client.HSet(context.Background(), s.key("crl"), entry.NodeID, data).Err()
}

// CRLGet retrieves the CRL entry for nodeID. Returns nil if not found.
func (s *Store) CRLGet(nodeID string) (*store.CRLEntry, error) {
	v, err := s.client.HGet(context.Background(), s.key("crl"), nodeID).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e store.CRLEntry
	if err := json.Unmarshal([]byte(v), &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// CRLList returns all CRL entries.
func (s *Store) CRLList() ([]store.CRLEntry, error) {
	m, err := s.client.HGetAll(context.Background(), s.key("crl")).Result()
	if err != nil {
		return nil, err
	}
	out := make([]store.CRLEntry, 0, len(m))
	for _, v := range m {
		var e store.CRLEntry
		if err := json.Unmarshal([]byte(v), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
