package builders

import (
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/schema"
)

// NewEdifactEnvelope returns an Envelope pre-configured for the UN/EDIFACT
// logistics and supply-chain adapter. Default policy applies regional+alliance
// nodes and balanced profile unless the operator overrides via TOML (ADR-015).
func NewEdifactEnvelope(payload []byte) *core.Envelope {
	return &core.Envelope{
		Payload:       payload,
		SchemaType:    schema.Edifact,
		SchemaVersion: "1.0.0",
	}
}
