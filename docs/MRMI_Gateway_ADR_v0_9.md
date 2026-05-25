# MRMI Gateway — Architecture Decision Record v0.9

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                         |
|------------|-------------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                                  |
| Status     | PROPOSED                                                                      |
| Author     | Božidar Tadić                                                                 |
| Version    | 0.9 — Universal Federation Protocol: Schema Registry (ADR-015), Protocol Specification, Domain Adapters, Financial Corridors (ADR-016). Primary focus remains cross-border messaging. |
| Supersedes | v0.8                                                                          |
| Covers App | v0.1 (target) · v0.2–v1.0 (roadmap)                                          |
| Created    | 2025                                                                          |

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*
>
> *"A universal federation protocol does not own the data it carries — it only guarantees that data reaches the right jurisdiction, intact."*

---

## Goal of v0.9

The goal of v0.9 is to make MRMI Gateway **universally adaptable** — not only for messaging applications, but for any regulated cross-domain data corridor that requires the same guarantees: sovereignty-preserving routing, verifiable audit, and compliance enforcement at the transport layer.

**Cross-border messaging remains the primary focus and the production target for v0.1.** The `messaging` adapter is the only adapter active by default. All other adapters (`iso20022`, `hl7fhir`, `edifact`) are roadmap items.

What v0.9 changes is the *architecture*: instead of hardwiring messaging as the only domain MRMI knows about, the Schema Registry gives any domain a formal way to declare itself and receive appropriate policy treatment — without any changes to the transport layer. The transport layer stays payload-agnostic, as it has been since v0.1.

The analogy: Apache Kafka started as a messaging system and became universal infrastructure. MRMI follows the same path — messaging first, universal architecture from v0.9 onward.

## What was decided in v0.9

| Area | Change |
|---|---|
| **Schema Layer (ADR-015)** | Formal Schema Registry introduced. Domain adapters: `messaging` (default, active), `iso20022` (v0.2), `hl7fhir` (v0.3), `edifact` (v0.3), `custom` (v0.1). Gateway validates envelope only; payload semantics belong to the domain. |
| **Protocol Specification (§1.3)** | MRMI formally repositioned as a protocol, not only a gateway implementation. Envelope contract is implementation-independent. Any language may build a compatible node. |
| **Financial Corridors (ADR-016)** | ISO 20022 domain adapter architecture defined. Settlement finality, cutoff times, idempotency guarantees. Scoped to v0.2 — not shipped in v0.1. |
| **Jurisdiction Policy (ADR-005 ext)** | `jurisdiction_isolation` policy: payload type may restrict node tiers and jurisdictions independently of sender/recipient region policy. Needed to safely colocate sensitive domains (financial, health) with messaging on the same node. |

ADR-001 through ADR-014 are carried forward from v0.8 without modification.

---

## §1.3 — Protocol vs. Implementation (NEW)

Starting with v0.9, MRMI is formally repositioned as a **protocol specification**, not only a gateway implementation. The Go binary is the reference implementation. The protocol itself is implementation-independent.

A Java shop, a Python ML pipeline, or a financial institution running JVM can implement a fully compatible MRMI node without touching the Go codebase. The envelope contract, the policy engine interface (ADR-005), and the audit log format are the normative protocol surface.

**This does not change the primary focus.** The Go reference implementation and the messaging use case remain the active development target. Protocol independence is an architectural property that lowers the barrier for adoption in other language ecosystems — it is not a signal that MRMI is pivoting away from messaging.

The analogy is TCP/IP: the protocol is defined independently of any OS kernel. MRMI follows the same principle. Alternative implementations are explicitly welcome and will be tracked in the ecosystem registry.

---

## ADR-015 — Schema Registry (NEW)

### Problem

MRMI Gateway has always been payload-agnostic — the envelope carries an opaque encrypted blob. This is correct and intentional. However, as the protocol matures beyond its initial messaging focus, two problems emerge:

1. **Out-of-band domain agreements:** every non-messaging integration requires operators to agree on payload types outside the protocol. There is no formal mechanism for declaring "this envelope carries a financial message" vs. "this is a healthcare record."
2. **Undifferentiated policy:** some domains (financial, healthcare) require stricter jurisdiction constraints than messaging. Without domain metadata at the envelope level, the policy engine cannot apply domain-specific rules.

The Schema Registry solves both problems while preserving the transport layer's payload opacity.

**Important:** The problem this solves is architectural readiness, not a shift in primary focus. Messaging is the default domain and the active use case. The registry exists so that when a second domain adopts MRMI, it has a first-class path — not an out-of-band workaround.

### Decision

Introduce a Schema Registry as a first-class MRMI component at the **Schema Layer**, above Transport and below Application.

### Three-Layer Architecture

```
┌──────────────────────────────────────────────────────────┐
│  APPLICATION LAYER                                       │
│  Business logic, domain semantics, user-facing features  │
└────────────────────────┬─────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────┐
│  SCHEMA LAYER  (new in v0.9)                             │
│  Schema Registry: domain adapter registration            │
│  Adapters: messaging | iso20022 | hl7fhir | edifact      │
│  Gateway validates envelope; payload stays opaque        │
└────────────────────────┬─────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────┐
│  TRANSPORT LAYER  (unchanged from v0.8)                  │
│  Routing · Delivery · Policy · Audit · mTLS · Revocation │
└──────────────────────────────────────────────────────────┘
```

### Envelope — new fields

```protobuf
message Envelope {
  // ... existing fields (v0.8) ...
  string schema_type    = 12; // "messaging" | "iso20022" | "hl7fhir" | "edifact" | "custom:<id>"
  string schema_version = 13; // semver — "1.0.0"
  bytes  payload        = 8;  // opaque, E2E encrypted — unchanged
}
```

The gateway reads `schema_type` only to: (1) apply `jurisdiction_isolation` policy, (2) record it in the audit log entry. It does **not** decode or validate the payload.

### Built-in Domain Adapters

| `schema_type` | Version | Description | Target |
|---|---|---|---|
| `messaging` | 1.0.0 | Default. Chat/notifications. Backward-compatible with all v0.8 envelopes. | v0.1 |
| `iso20022` | 1.0.0 | Financial messages. Triggers financial policy extensions (ADR-016). | v0.2 |
| `hl7fhir` | 1.0.0 | Healthcare. FHIR R4. Jurisdiction policy: strict only. | v0.3 |
| `edifact` | 1.0.0 | Logistics. UN/EDIFACT. Supply chain corridors. | v0.3 |
| `custom:` | any | Operator-registered. No built-in policy extensions. | v0.1 |

### Custom Adapter Registration

```toml
[schema_registry]
[[schema_registry.adapters]]
type               = "custom:gov-doc-exchange"
version            = "1.0.0"
description        = "Government document exchange — RS Ministry of Interior"
jurisdiction_policy = "strict"
allow_tiers        = ["regional"]
contact            = "ops@example.rs"
```

### Audit Log additions

```
AuditEntry {
  // ... existing fields (v0.8) ...
  schema_type:    string  // e.g. "iso20022"
  schema_version: string  // e.g. "1.0.0"
}
```

> **Design Principle:** "The gateway never opens the payload. `schema_type` is a routing and policy hint declared by the sender — not a content inspection mechanism."

---

## ADR-005 Extension — jurisdiction_isolation Policy (NEW)

v0.8 policy engine controls routing based on sender/recipient region. v0.9 adds `jurisdiction_isolation`: a payload type may independently restrict which node tiers and jurisdictions it transits.

**Use case:** A financial institution uses MRMI for both chat (`messaging`) and payment (`iso20022`). Chat may transit global nodes; payments must only transit regional nodes in declared jurisdictions.

```toml
[policy.jurisdiction_isolation]

[[policy.jurisdiction_isolation.rules]]
schema_type         = "iso20022"
allow_tiers         = ["regional"]
allow_jurisdictions = ["RU", "RS", "BY"]
require_profile     = "strict"

[[policy.jurisdiction_isolation.rules]]
schema_type         = "messaging"
allow_tiers         = ["regional", "alliance", "global"]  # default — unrestricted
```

---

## ADR-016 — Financial Domain Adapter ISO 20022 (NEW, v0.2)

### Context

Financial messaging (SWIFT MT/MX, ISO 20022) is structurally identical to general messaging at the transport level. MRMI's transport layer already provides the required primitives. ADR-016 adds financial-specific policy extensions.

**Target:** Not replacement of SWIFT or CIPS. A sovereignty-preserving federation layer for corridors where SWIFT access is restricted or commercially undesirable.

### Financial Policy Extensions

| Extension | Specification |
|---|---|
| **Settlement Finality** | Once `iso20022` message reaches ACK from destination node, it is marked final. No replay or cancellation via MRMI. |
| **Cutoff Times** | Per-corridor processing windows. Messages outside window held in DLQ until next window opens — not rejected. |
| **Idempotency TTL** | `iso20022` defaults to 72h dedup TTL regardless of operator profile. |
| **BIC Routing Hint** | Optional `routing_hint` field: BIC prefix (first 6 chars) for path optimisation. Never used for identity. |
| **Audit Retention** | `retain_long=true` flag. Operators must configure Merkle log retention ≥ 7 years for financial corridors. |

### TOML

```toml
[schema_registry.adapters.iso20022]
enabled             = true
dedup_ttl_h         = 72
settlement_finality = true
retain_long         = true

[schema_registry.adapters.iso20022.cutoff_windows."RU-RS"]
open  = "08:00"
close = "17:00"
tz    = "Europe/Belgrade"
```

### Compliance Note

> The `iso20022` adapter provides technical infrastructure for financial message routing. It does not constitute a payment system, a banking licence, or a money transmission service. Operators must obtain all applicable regulatory approvals. Legal accountability rests entirely with the operator.

---

## Milestones — v0.9 Additions

### v0.1 (Schema Registry)

| # | Task | Owner |
|---|---|---|
| 13 | ADR-015: Schema Registry TOML config + `schema_type`/`schema_version` proto fields | Božidar Tadić |
| 14 | Gateway: `jurisdiction_isolation` policy engine extension | Božidar Tadić |
| 15 | Audit log: `schema_type` + `schema_version` in `AuditEntry` | Božidar Tadić |
| 16 | `messaging` adapter: default, backward-compatible with all v0.8 envelopes | Božidar Tadić |
| 17 | .NET SDK v0.1: `SchemaType` enum + `schema_version` in send API | Božidar Tadić |
| 18 | Protocol Specification document: envelope contract, policy interface, audit format | Božidar Tadić |

### v0.2 (Financial Adapter)

| # | Task | Owner |
|---|---|---|
| 11 | ADR-016: `iso20022` adapter — settlement finality, cutoff windows, BIC routing hint | Božidar Tadić |
| 12 | `iso20022` policy: `dedup_ttl_h` override, `retain_long` flag, cutoff window enforcement | Božidar Tadić |
| 13 | Legal analysis: `iso20022` corridor compliance per jurisdiction (RU, RS, BY) | Božidar Tadić + counsel |
| 14 | Python SDK v0.2: `schema_type` support, `iso20022` envelope helper | Open for contributors |

### v0.3 (Domain Adapter Infrastructure)

| # | Task | Owner |
|---|---|---|
| 7 | `hl7fhir` adapter: FHIR R4 schema hint, strict-only jurisdiction policy | Open for contributors |
| 8 | `edifact` adapter: UN/EDIFACT schema hint, logistics corridor support | Open for contributors |
| 9 | SDK helpers: domain-specific envelope builders (Go + .NET) | Božidar Tadić |
| 10 | Schema Registry UI: adapter list, per-adapter policy view, custom registration | Open for contributors |

---

## Roadmap (updated v0.9)

| Version | Focus | Key Deliverables |
|---|---|---|
| **v0.1** | Core protocol + Schema Registry | Routing, policy, audit, delivery, mTLS, revocation, node tiers, hosted nodes, read-only API, seed corridor RS→RU, **Schema Registry (ADR-015)**, `jurisdiction_isolation`, `messaging` adapter, Protocol Specification |
| **v0.2** | User discovery + Financial adapter | Federated lookup, opaque token, namespace isolation, auto-accept, push webhook, write API, **ISO 20022 (ADR-016)**, cutoff windows, settlement finality |
| **v0.3** | Operator infrastructure + Domain adapters | Storage backends, cache layer, dynamic discovery, dashboard UI, **HL7/FHIR**, **EDIFACT**, domain adapter SDK helpers |
| **v1.0** | Scale + Governance | DHT routing, EU alliance node, governance model, operator certification, Schema Registry governance |
| **post-v1.0** | Extensions | Video calls, WebRTC signaling, energy sector adapter (IEC 61968/61970), government document exchange |

---

## Appendix C — Schema Registry TOML Reference

### C.1 Minimal (messaging only, backward-compatible)

```toml
# No [schema_registry] section required.
# All envelopes without schema_type default to schema_type="messaging".
```

### C.2 Financial corridor

```toml
[schema_registry]
[[schema_registry.adapters]]
type    = "iso20022"
version = "1.0.0"
enabled = true

[schema_registry.adapters.iso20022]
dedup_ttl_h         = 72
settlement_finality = true
retain_long         = true

[schema_registry.adapters.iso20022.cutoff_windows."RU-RS"]
open  = "08:00"
close = "17:00"
tz    = "Europe/Belgrade"

[policy.jurisdiction_isolation]
[[policy.jurisdiction_isolation.rules]]
schema_type         = "iso20022"
allow_tiers         = ["regional"]
allow_jurisdictions = ["RU", "RS", "BY", "KZ", "AM"]
require_profile     = "strict"
```

### C.3 Multi-domain

```toml
[schema_registry]
[[schema_registry.adapters]]
type = "messaging"; version = "1.0.0"; enabled = true

[[schema_registry.adapters]]
type = "iso20022"; version = "1.0.0"; enabled = true

[[schema_registry.adapters]]
type               = "custom:gov-rs-doc-exchange"
version            = "1.0.0"; enabled = true
jurisdiction_policy = "strict"
allow_tiers        = ["regional"]
contact            = "ops@gov.rs"
```

---

## Open Questions (v0.9 additions)

| Question | Notes |
|---|---|
| Schema Registry governance | GitHub repo (author-controlled) v0.1–v0.3. Foundation/committee for v1.0. |
| Financial corridor compliance | Legal analysis needed per jurisdiction before v0.2 launch. |
| Custom namespace `schema_type` | Flat registry vs. namespaced `custom:org.example.name`? |
| Schema validation at node boundary | Opt-in validation against JSON Schema / Protobuf descriptor? |
| ISO 20022 cutoff window precedence | Proposed: destination node's window is authoritative. |

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.8](MRMI_Gateway_ADR_v0_8.md) | **v0.9** (this file) | — |

*Also available as the original PDF: [MRMI_Gateway_ADR_v0_9.pdf](MRMI_Gateway_ADR_v0_9.pdf)*
