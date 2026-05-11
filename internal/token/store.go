package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("token not found or expired")

type entry struct {
	appID        string
	originNodeID string // node that requested discovery (who the token is issued to)
	expires      time.Time
}

// Store issues single-use opaque tokens backed by SHA-256 hashes.
// The plaintext token is returned once at issue time and never stored.
type Store struct {
	mu      sync.Mutex
	entries map[string]entry // SHA-256(token) → entry
}

func New() *Store {
	return &Store{entries: make(map[string]entry)}
}

// Issue generates a cryptographically random 32-byte token, stores only its SHA-256
// hash with the given TTL, and returns the hex-encoded plaintext token.
func (s *Store) Issue(appID, originNodeID string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	hash := hashToken(token)

	s.mu.Lock()
	s.entries[hash] = entry{appID: appID, originNodeID: originNodeID, expires: time.Now().Add(ttl)}
	s.mu.Unlock()

	return token, nil
}

// Resolve looks up the token by its SHA-256 hash. Returns ErrNotFound if the token
// does not exist or has expired.
func (s *Store) Resolve(token string) (appID, originNodeID string, err error) {
	hash := hashToken(token)

	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[hash]
	if !ok || time.Now().After(e.expires) {
		delete(s.entries, hash)
		return "", "", ErrNotFound
	}
	return e.appID, e.originNodeID, nil
}

// Evict removes the token immediately (called after a successful handshake so the
// token cannot be replayed).
func (s *Store) Evict(token string) {
	hash := hashToken(token)
	s.mu.Lock()
	delete(s.entries, hash)
	s.mu.Unlock()
}

// HasActiveToken reports whether originNodeID has at least one unexpired token
// issued by this store. Used by AUTO_MUTUAL connect resolution.
func (s *Store) HasActiveToken(originNodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, e := range s.entries {
		if e.originNodeID == originNodeID && now.Before(e.expires) {
			return true
		}
	}
	return false
}

// Purge removes entries whose TTL has expired. Call periodically to bound memory.
func (s *Store) Purge() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for hash, e := range s.entries {
		if now.After(e.expires) {
			delete(s.entries, hash)
		}
	}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
