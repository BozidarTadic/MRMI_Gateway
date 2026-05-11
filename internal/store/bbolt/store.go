package bbolt

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"MRMI_Gateway/internal/store"
)

var (
	bucketDedup  = []byte("dedup")
	bucketDLQ    = []byte("dlq")
	bucketAudit  = []byte("audit")
	bucketCRL    = []byte("crl")
)

// Store is a bbolt-backed implementation of store.NodeStore.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) a bbolt database at dir/mrmi.db.
func Open(dir string) (*Store, error) {
	path := filepath.Join(dir, "mrmi.db")
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("bbolt open %s: %w", path, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketDedup, bucketDLQ, bucketAudit, bucketCRL} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("bbolt init buckets: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Deduped returns true if key was seen within ttl. Atomic check-and-set.
func (s *Store) Deduped(key string, ttl time.Duration) (bool, error) {
	now := time.Now()
	var seen bool
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDedup)
		raw := b.Get([]byte(key))
		if raw != nil {
			exp := int64(binary.BigEndian.Uint64(raw))
			if now.UnixNano() < exp {
				seen = true
				return nil
			}
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(now.Add(ttl).UnixNano()))
		return b.Put([]byte(key), buf)
	})
	return seen, err
}

// DLQPush appends a DLQ entry keyed by entry.ID.
func (s *Store) DLQPush(entry store.DLQEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketDLQ).Put([]byte(entry.ID), data)
	})
}

// DLQList returns all DLQ entries.
func (s *Store) DLQList() ([]store.DLQEntry, error) {
	var out []store.DLQEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketDLQ).ForEach(func(_, v []byte) error {
			var e store.DLQEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, e)
			return nil
		})
	})
	return out, err
}

// DLQDelete removes the entry with the given id.
func (s *Store) DLQDelete(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketDLQ).Delete([]byte(id))
	})
}

// AuditAppend appends an audit entry using its Seq as the key.
func (s *Store) AuditAppend(entry store.AuditEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		key := seqKey(entry.Seq)
		return tx.Bucket(bucketAudit).Put(key, data)
	})
}

// AuditLatest returns the latest n audit entries in descending order.
func (s *Store) AuditLatest(n int) ([]store.AuditEntry, error) {
	var out []store.AuditEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketAudit).Cursor()
		for k, v := c.Last(); k != nil && len(out) < n; k, v = c.Prev() {
			var e store.AuditEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// CRLPut stores or replaces the CRL entry for entry.NodeID.
func (s *Store) CRLPut(entry store.CRLEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketCRL).Put([]byte(entry.NodeID), data)
	})
}

// CRLGet retrieves the CRL entry for nodeID. Returns nil if not found.
func (s *Store) CRLGet(nodeID string) (*store.CRLEntry, error) {
	var out *store.CRLEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketCRL).Get([]byte(nodeID))
		if v == nil {
			return nil
		}
		var e store.CRLEntry
		if err := json.Unmarshal(v, &e); err != nil {
			return err
		}
		out = &e
		return nil
	})
	return out, err
}

// CRLList returns all CRL entries.
func (s *Store) CRLList() ([]store.CRLEntry, error) {
	var out []store.CRLEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketCRL).ForEach(func(_, v []byte) error {
			var e store.CRLEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, e)
			return nil
		})
	})
	return out, err
}

func seqKey(seq uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, seq)
	return b
}
