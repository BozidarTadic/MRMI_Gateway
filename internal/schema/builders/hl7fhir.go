package builders

import (
	"MRMI_Gateway/internal/core"
	"MRMI_Gateway/internal/schema"
)

// NewHl7FhirEnvelope returns an Envelope pre-configured for the HL7 FHIR R4
// health-data adapter. The policy engine will enforce ADR-015 hardcoded
// constraints (regional node + strict profile) on any envelope with this type.
func NewHl7FhirEnvelope(payload []byte) *core.Envelope {
	return &core.Envelope{
		Payload:       payload,
		SchemaType:    schema.Hl7Fhir,
		SchemaVersion: "1.0.0",
	}
}
