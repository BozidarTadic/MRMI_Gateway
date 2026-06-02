package schema

// Hl7Fhir is the schema_type value for HL7 FHIR R4 health data messages.
//
// Health data mandates hardcoded jurisdiction constraints (ADR-015):
// - allow_tiers = ["regional"]   — hl7fhir envelopes must not transit alliance or global nodes.
// - require_profile = "strict"   — the node must run the strict compliance profile.
//
// These constraints are enforced by the policy engine unconditionally and cannot
// be relaxed via TOML configuration.
const Hl7Fhir = "hl7fhir"
