package schema

// Edifact is the schema_type value for UN/EDIFACT logistics and supply-chain messages.
//
// Default jurisdiction constraints (ADR-015) — operator may override via TOML
// [policy.jurisdiction_isolation] rule for "edifact":
//   - allow_tiers = ["regional", "alliance"]  — global nodes blocked by default
//   - require_profile = "balanced"            — performance profile is insufficient
//
// Unlike hl7fhir, these defaults can be relaxed by a matching TOML rule.
const Edifact = "edifact"
