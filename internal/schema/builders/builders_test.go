package builders_test

import (
	"testing"

	"MRMI_Gateway/internal/schema"
	"MRMI_Gateway/internal/schema/builders"
)

func TestNewMessagingEnvelope(t *testing.T) {
	payload := []byte("hello")
	env := builders.NewMessagingEnvelope(payload)

	if env.SchemaType != schema.Messaging {
		t.Errorf("SchemaType: want %q, got %q", schema.Messaging, env.SchemaType)
	}
	if env.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion: want %q, got %q", "1.0.0", env.SchemaVersion)
	}
	if string(env.Payload) != "hello" {
		t.Errorf("Payload not propagated")
	}
	if env.RoutingHint != "" {
		t.Errorf("RoutingHint should be empty for messaging, got %q", env.RoutingHint)
	}
}

func TestNewIso20022Envelope_WithBicPrefix(t *testing.T) {
	env := builders.NewIso20022Envelope([]byte("xml"), "BANKRS")

	if env.SchemaType != schema.Iso20022 {
		t.Errorf("SchemaType: want %q, got %q", schema.Iso20022, env.SchemaType)
	}
	if env.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion: want %q, got %q", "1.0.0", env.SchemaVersion)
	}
	if env.RoutingHint != "BANKRS" {
		t.Errorf("RoutingHint: want %q, got %q", "BANKRS", env.RoutingHint)
	}
}

func TestNewIso20022Envelope_EmptyBicPrefix(t *testing.T) {
	env := builders.NewIso20022Envelope([]byte("xml"), "")

	if env.RoutingHint != "" {
		t.Errorf("empty bicPrefix should leave RoutingHint unset, got %q", env.RoutingHint)
	}
}

func TestNewHl7FhirEnvelope(t *testing.T) {
	env := builders.NewHl7FhirEnvelope([]byte("fhir-bundle"))

	if env.SchemaType != schema.Hl7Fhir {
		t.Errorf("SchemaType: want %q, got %q", schema.Hl7Fhir, env.SchemaType)
	}
	if env.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion: want %q, got %q", "1.0.0", env.SchemaVersion)
	}
	if env.RoutingHint != "" {
		t.Errorf("RoutingHint should be empty for hl7fhir, got %q", env.RoutingHint)
	}
}

func TestNewEdifactEnvelope(t *testing.T) {
	env := builders.NewEdifactEnvelope([]byte("edifact-msg"))

	if env.SchemaType != schema.Edifact {
		t.Errorf("SchemaType: want %q, got %q", schema.Edifact, env.SchemaType)
	}
	if env.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion: want %q, got %q", "1.0.0", env.SchemaVersion)
	}
	if env.RoutingHint != "" {
		t.Errorf("RoutingHint should be empty for edifact, got %q", env.RoutingHint)
	}
}
