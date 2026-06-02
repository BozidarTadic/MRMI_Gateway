package core

import (
	"context"
	"testing"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/dedup"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/schema"
)

func newTestGatewayWithAudit(t *testing.T) (*Gateway, *audit.Log) {
	t.Helper()
	cfg := config.DefaultConfigForProfile("balanced")
	auditLog := audit.New()
	engine, err := policy.NewEngine(cfg, auditLog, nil)
	if err != nil {
		t.Fatalf("policy engine: %v", err)
	}
	gw := NewGateway(cfg, engine, auditLog, dedup.New(cfg.Profile.DedupTTL), nil)
	return gw, auditLog
}

// TestGateway_V08Envelope_NoSchemaType_AcceptedAndLoggedAsMessaging verifies
// that a v0.8 envelope omitting schema_type is accepted and stored in the
// audit log with schema_type="messaging" (ADR-015 backward-compat guarantee).
func TestGateway_V08Envelope_NoSchemaType_AcceptedAndLoggedAsMessaging(t *testing.T) {
	gw, auditLog := newTestGatewayWithAudit(t)

	_, err := gw.SendEnvelope(context.Background(), SendRequest{
		Envelope: Envelope{
			IdempotencyKey:  "v08-no-schema",
			SenderRegion:    "RS",
			RecipientRegion: "RU",
			Payload:         []byte("hello"),
			// SchemaType intentionally omitted (v0.8 envelope)
		},
	})
	if err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}

	entries := auditLog.Entries()
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
	last := entries[len(entries)-1]
	if last.SchemaType != schema.Messaging {
		t.Fatalf("expected audit SchemaType=%q, got %q", schema.Messaging, last.SchemaType)
	}
}

// TestGateway_ExplicitMessaging_IdenticalToDefault verifies that an envelope
// with schema_type="messaging" set explicitly produces the same audit outcome
// as one with the field omitted entirely.
func TestGateway_ExplicitMessaging_IdenticalToDefault(t *testing.T) {
	gwDefault, auditDefault := newTestGatewayWithAudit(t)
	gwExplicit, auditExplicit := newTestGatewayWithAudit(t)

	send := func(gw *Gateway, key, schemaType string) {
		t.Helper()
		_, err := gw.SendEnvelope(context.Background(), SendRequest{
			Envelope: Envelope{
				IdempotencyKey:  key,
				SenderRegion:    "RS",
				RecipientRegion: "RU",
				Payload:         []byte("hello"),
				SchemaType:      schemaType,
			},
		})
		if err != nil {
			t.Fatalf("SendEnvelope: %v", err)
		}
	}

	send(gwDefault, "default-key", "")
	send(gwExplicit, "explicit-key", schema.Messaging)

	entriesDefault := auditDefault.Entries()
	entriesExplicit := auditExplicit.Entries()

	if len(entriesDefault) == 0 || len(entriesExplicit) == 0 {
		t.Fatal("expected audit entries for both gateways")
	}

	eDefault := entriesDefault[len(entriesDefault)-1]
	eExplicit := entriesExplicit[len(entriesExplicit)-1]

	if eDefault.SchemaType != eExplicit.SchemaType {
		t.Fatalf("schema_type mismatch: default=%q explicit=%q", eDefault.SchemaType, eExplicit.SchemaType)
	}
	if eDefault.Decision != eExplicit.Decision {
		t.Fatalf("decision mismatch: default=%q explicit=%q", eDefault.Decision, eExplicit.Decision)
	}
}
