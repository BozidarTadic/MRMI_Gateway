package builders

import (
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/schema"
)

// NewIso20022Envelope returns an Envelope pre-configured for the ISO 20022
// financial adapter. If bicPrefix is non-empty it is stored as RoutingHint so
// the forwarder can prefer a matching peer (ADR-016 §3.2).
func NewIso20022Envelope(payload []byte, bicPrefix string) *core.Envelope {
	env := &core.Envelope{
		Payload:       payload,
		SchemaType:    schema.Iso20022,
		SchemaVersion: "1.0.0",
	}
	if bicPrefix != "" {
		env.RoutingHint = bicPrefix
	}
	return env
}
