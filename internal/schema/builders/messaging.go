// Package builders provides convenience constructors that pre-populate the
// schema_type and schema_version fields for each built-in domain adapter
// (ADR-015). Callers still need to set SenderRegion, RecipientRegion,
// IdempotencyKey, and Payload on the returned Envelope.
package builders

import (
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/schema"
)

// NewMessagingEnvelope returns an Envelope pre-configured for the default
// messaging adapter. This is the backward-compatible domain used by all v0.8
// envelopes that omit schema_type.
func NewMessagingEnvelope(payload []byte) *core.Envelope {
	return &core.Envelope{
		Payload:       payload,
		SchemaType:    schema.Messaging,
		SchemaVersion: "1.0.0",
	}
}
