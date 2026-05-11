package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"

	"MRMI_Gateway/internal/core"
)

var ErrInvalidSignature = errors.New("invalid envelope signature")
var ErrMissingSignature = errors.New("envelope signature is required")

// GenerateKey generates a new Ed25519 key pair.
func GenerateKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	return priv, pub, err
}

// Sign signs the canonical form of env with privKey and returns the signature.
// The Signature field of env is excluded from the signed payload.
func Sign(privKey ed25519.PrivateKey, env core.Envelope) []byte {
	return ed25519.Sign(privKey, canonicalPayload(env))
}

// Verify checks that sig is a valid Ed25519 signature over env by pubKey.
func Verify(pubKey ed25519.PublicKey, env core.Envelope, sig []byte) error {
	if len(sig) == 0 {
		return ErrMissingSignature
	}
	if !ed25519.Verify(pubKey, canonicalPayload(env), sig) {
		return ErrInvalidSignature
	}
	return nil
}

// canonicalPayload produces a deterministic JSON encoding of envelope fields,
// excluding the Signature field itself.
func canonicalPayload(env core.Envelope) []byte {
	payload := struct {
		IdempotencyKey    string `json:"idempotency_key"`
		SenderRegion      string `json:"sender_region"`
		RecipientRegion   string `json:"recipient_region"`
		TrustTier         uint32 `json:"trust_tier"`
		SequenceNumber    uint64 `json:"sequence_number"`
		Payload           []byte `json:"payload"`
		PaddedTo          uint32 `json:"padded_to"`
		Timestamp         int64  `json:"timestamp"`
	}{
		IdempotencyKey:  env.IdempotencyKey,
		SenderRegion:    env.SenderRegion,
		RecipientRegion: env.RecipientRegion,
		TrustTier:       env.TrustTier,
		SequenceNumber:  env.SequenceNumber,
		Payload:         env.Payload,
		PaddedTo:        env.PaddedTo,
		Timestamp:       env.Timestamp,
	}
	raw, _ := json.Marshal(payload)
	return raw
}
