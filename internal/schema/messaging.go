// Package schema provides schema-type constants and normalization helpers for
// the ADR-015 domain adapter model.
package schema

import "strings"

// Messaging is the built-in default domain adapter. All v0.8 envelopes that
// omit schema_type are treated as messaging — this is the backward-compatibility
// guarantee described in ADR-015.
const Messaging = "messaging"

// Normalize returns t unchanged when it is non-empty, or Messaging for v0.8
// envelopes that omit the schema_type field entirely. It requires no TOML
// configuration and activates automatically for every gateway instance.
func Normalize(t string) string {
	if strings.TrimSpace(t) == "" {
		return Messaging
	}
	return t
}
