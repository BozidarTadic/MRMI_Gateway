package identity

import (
	"testing"

	"MRMI_Gateway/internal/core"
)

func testEnvelope() core.Envelope {
	return core.Envelope{
		IdempotencyKey:  "key-001",
		SenderRegion:    "RS",
		RecipientRegion: "RU",
		TrustTier:       1,
		SequenceNumber:  1,
		Payload:         []byte("hello"),
		Timestamp:       1700000000000,
	}
}

func TestSignVerify_Valid(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	env := testEnvelope()
	sig := Sign(priv, env)
	if err := Verify(pub, env, sig); err != nil {
		t.Fatalf("Verify failed on valid signature: %v", err)
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	env := testEnvelope()
	sig := Sign(priv, env)

	env.Payload = []byte("tampered")
	if err := Verify(pub, env, sig); err == nil {
		t.Fatal("expected error on tampered payload, got nil")
	}
}

func TestVerify_MissingSignature(t *testing.T) {
	_, pub, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	env := testEnvelope()
	if err := Verify(pub, env, nil); err != ErrMissingSignature {
		t.Fatalf("expected ErrMissingSignature, got %v", err)
	}
}

func TestVerify_WrongKey(t *testing.T) {
	priv, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	_, pub2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	env := testEnvelope()
	sig := Sign(priv, env)
	if err := Verify(pub2, env, sig); err == nil {
		t.Fatal("expected error with wrong public key, got nil")
	}
}
