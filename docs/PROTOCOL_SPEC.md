# MRMI Protocol Specification

**Multi-Regional Multi-App Interlock — Normative Protocol Document**

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| Spec ID     | MRMI-SPEC-001                                               |
| Version     | 0.1                                                         |
| Status      | NORMATIVE                                                   |
| Author      | Božidar Tadić                                               |
| ADR Basis   | MRMI-ADR-001 v0.9                                           |
| Ref Impl    | Go — `github.com/tadicbb/MRMI_Gateway`                     |

> The Go reference implementation is the ground truth for any ambiguity during v0.1. This document reflects what the code does, not aspirational behaviour.

---

## 1. Purpose and Scope

MRMI is a **federation protocol** for sovereignty-preserving, policy-enforced, auditable message routing between regional computing environments. Any language runtime may implement a compatible MRMI node provided it satisfies the normative requirements in this document.

**In scope:** envelope contract, policy engine interface, audit log format, inter-node transport (mTLS), schema type declaration contract, compliance conformance levels.

**Out of scope:** payload semantics, application-layer protocols, operator legal compliance, SDK design.

---

## 2. Normative Language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are used as defined in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 3. Envelope Contract

An MRMI envelope is the atomic unit of transport. The canonical encoding is Protocol Buffers v3 as defined in `proto/mrmi/v1/contracts.proto`.

### 3.1 Field Table

| # | Field              | Type      | Required | Description |
|---|--------------------|-----------|----------|-------------|
| 1 | `idempotency_key`  | string    | **REQUIRED** | Stable caller-chosen key for at-least-once delivery. A compliant node MUST deduplicate on this key within the configured TTL. |
| 2 | `sender_identity`  | bytes     | OPTIONAL | Opaque identity bytes. Used by the sending application; the node MUST NOT interpret or log the content. |
| 3 | `recipient_identity` | bytes   | OPTIONAL | Opaque identity bytes for the intended recipient. Same opacity guarantee as `sender_identity`. |
| 4 | `sender_region`    | string    | **REQUIRED** | ISO 3166-1 alpha-2 code of the sending node's jurisdiction. Used for policy evaluation. |
| 5 | `recipient_region` | string    | **REQUIRED** | ISO 3166-1 alpha-2 code of the intended destination jurisdiction. Used for routing and policy evaluation. |
| 6 | `trust_tier`       | uint32    | **REQUIRED** | Declared trust level of the sender. `0` = anonymous. A compliant node MUST reject envelopes whose `trust_tier` is below the configured minimum. |
| 7 | `sequence_number`  | uint64    | OPTIONAL | Per-session monotonically increasing counter for out-of-order detection. A value of `0` disables sequence validation for this envelope. |
| 8 | `payload`          | bytes     | OPTIONAL | Opaque, end-to-end encrypted application payload. A compliant node **MUST NOT** decode, inspect, or log payload content. |
| 9 | `padded_to`        | uint32    | OPTIONAL | Set by the forwarding node. Records the padded payload size in bytes. MUST be zero when no padding was applied. |
| 10 | `timestamp`       | int64     | OPTIONAL | Envelope creation time as Unix milliseconds. Used for staleness checks on discovery requests. |
| 11 | `signature`       | bytes     | OPTIONAL | Ed25519 signature over the canonical payload (see §3.2). A node configured with a verification key MUST reject envelopes with an invalid or missing signature. |
| 12 | `schema_type`     | string    | OPTIONAL | Domain adapter identifier. Empty field MUST be treated as `"messaging"`. See §6 for the full schema type contract. |
| 13 | `schema_version`  | string    | OPTIONAL | Semver string for the schema. Propagated verbatim to the audit log. |

### 3.2 Envelope Signature

When envelope signing is enabled, the sender MUST sign the **canonical payload** using Ed25519 and place the 64-byte signature in `envelope.signature`.

**Canonical payload** is the JSON encoding of the following fields in the following order, with no additional whitespace:

```json
{
  "idempotency_key": "...",
  "sender_region": "...",
  "recipient_region": "...",
  "trust_tier": 0,
  "sequence_number": 0,
  "payload": "...",
  "padded_to": 0,
  "timestamp": 0
}
```

`payload` is encoded as a Base64 string within JSON. The `signature` field itself is **excluded** from the signed payload. A compliant verifying node MUST use this exact structure; deviating implementations are not compatible.

### 3.3 Schema Type Normalization

A compliant node MUST normalize `schema_type` **before** policy evaluation and **before** writing the audit entry:

- Empty string → `"messaging"`
- Whitespace-only string → `"messaging"`
- Any other value → unchanged

This normalization is the backward-compatibility guarantee for v0.8 envelopes that predate the Schema Registry.

---

## 4. Policy Engine Interface

### 4.1 Inputs

```
PolicyRequest {
    sender_node_id:    string   // node that forwarded this envelope; used for CRL check
    sender_region:     string   // ISO 3166-1 alpha-2
    recipient_region:  string   // ISO 3166-1 alpha-2
    trust_tier:        uint32
    schema_type:       string   // already normalized (see §3.3)
    schema_version:    string
}
```

### 4.2 Outputs

```
PolicyResult {
    decision:  ALLOW | DENY | DUPLICATE
    reason:    string   // machine-readable code (see §4.4)
    profile:   string   // active compliance profile name
}
```

### 4.3 Evaluation Order

A compliant policy engine MUST evaluate rules in the following order. The **first matching denial** terminates evaluation and MUST be returned immediately; no further rules are evaluated.

1. **Deduplication** — If `idempotency_key` was seen within the dedup TTL, return `DUPLICATE / DUPLICATE_IDEMPOTENCY_KEY`. This check MUST precede all others.

2. **CRL check** — If `sender_node_id` appears in the Certificate Revocation List, return `DENY / NODE_REVOKED`.

3. **Trust tier minimum** — If `trust_tier < config.policy.inbound.min_trust_tier`, return `DENY / TRUST_TIER_BELOW_MINIMUM`.

4. **Outbound deny list** — If `recipient_region` is in `config.policy.outbound.deny_to`, return `DENY / RECIPIENT_REGION_DENIED`.

5. **Outbound allow list** — If `config.policy.outbound.allow_to` is non-empty and `recipient_region` is not in the list (and the envelope is not destined for this node's own region), return `DENY / RECIPIENT_REGION_NOT_IN_ALLOW_LIST`.

6. **Jurisdiction isolation** — Evaluate `config.policy.jurisdiction_isolation.rules` in order (see §4.5). On first matching schema_type:
   - If `node_scope` is not in `allow_tiers` (when `allow_tiers` is non-empty), return `DENY / JURISDICTION_ISOLATION_TIER_DENIED`.
   - If `sender_region` or `recipient_region` is not in `allow_jurisdictions` (when `allow_jurisdictions` is non-empty), return `DENY / JURISDICTION_ISOLATION_REGION_DENIED`.
   - If `require_profile` is set and the active profile name does not match (case-insensitive), return `DENY / JURISDICTION_ISOLATION_PROFILE_MISMATCH`.

7. **Allow** — If no denial matched, return `ALLOW / POLICY_ACCEPTED`.

### 4.4 Standard Reason Codes

| Code | Decision | Trigger |
|------|----------|---------|
| `POLICY_ACCEPTED` | ALLOW | No rule denied the envelope |
| `DUPLICATE_IDEMPOTENCY_KEY` | DUPLICATE | Key seen within dedup TTL |
| `NODE_REVOKED` | DENY | sender_node_id is in the CRL |
| `TRUST_TIER_BELOW_MINIMUM` | DENY | trust_tier < configured minimum |
| `RECIPIENT_REGION_DENIED` | DENY | recipient_region in deny_to |
| `RECIPIENT_REGION_NOT_IN_ALLOW_LIST` | DENY | allow_to is non-empty and region not listed |
| `JURISDICTION_ISOLATION_TIER_DENIED` | DENY | Node tier not in rule's allow_tiers |
| `JURISDICTION_ISOLATION_REGION_DENIED` | DENY | Region not in rule's allow_jurisdictions |
| `JURISDICTION_ISOLATION_PROFILE_MISMATCH` | DENY | Active profile does not match require_profile |
| `INVALID_SIGNATURE` | DENY | Ed25519 signature verification failed |
| `DUMMY_TRAFFIC` | ALLOW | IsDummy flag set; logged but not forwarded |

### 4.5 Jurisdiction Isolation Rules

Rules in `policy.jurisdiction_isolation` are evaluated in **declaration order**. First matching `schema_type` wins; remaining rules are skipped. When no rule matches the schema_type, the envelope is allowed.

An empty `allow_tiers` list means all node tiers are permitted.  
An empty `allow_jurisdictions` list means all jurisdictions are permitted.  
An empty `require_profile` means no profile constraint applies.

---

## 5. Audit Log Format

A compliant node MUST append an immutable audit entry for **every** policy decision, including ALLOW, DENY, DUPLICATE, and dummy traffic decisions.

### 5.1 Entry Structure

The canonical encoding is JSON. Fields marked `omitempty` are omitted when empty.

| Field              | Type   | omitempty | Description |
|--------------------|--------|-----------|-------------|
| `seq`              | uint64 | no | Monotonically increasing sequence number, starting at 1. |
| `timestamp`        | int64  | no | Decision time as Unix milliseconds. |
| `decision`         | string | no | `"ALLOW"` \| `"DENY"` \| `"DUPLICATE"` \| `"ALLOW/DUMMY"` |
| `reason`           | string | yes | Machine-readable reason code (see §4.4). |
| `trust_tier`       | uint32 | no | Trust tier from the envelope. |
| `sender_region`    | string | no | Sender region from the envelope. |
| `recipient_region` | string | no | Recipient region from the envelope. |
| `policy_version`   | string | no | Active policy version string. |
| `profile`          | string | no | Active compliance profile name. |
| `applicable_law`   | string | no | Legal framework declared in node config. |
| `dedup_ttl_hours`  | uint64 | no | Dedup TTL of the active profile in hours. |
| `node_scope`       | string | no | `"regional"` \| `"alliance"` \| `"global"` |
| `alliance_id`      | string | no | Non-empty only for alliance nodes. |
| `node_region`      | string | no | Physical region of the node. |
| `schema_type`      | string | yes | Normalized schema type. |
| `schema_version`   | string | yes | Schema version from the envelope. |
| `previous_hash`    | string | no | Hash of the previous entry (see §5.2). |
| `entry_hash`       | string | no | Hash of this entry (see §5.2). |

### 5.2 Merkle Chaining Algorithm

A compliant node MUST maintain a hash chain over audit entries. The chain guarantees tamper evidence: altering any historical entry invalidates all subsequent entry hashes.

**Initial state:** The root hash before the first entry is:

```
"sha256:0000000000000000000000000000000000000000000000000000000000000000"
```
(the string `"sha256:"` followed by 64 ASCII zero characters)

**Entry hash computation:**

1. Construct a JSON object containing the following fields (using the same `omitempty` rules as the entry):

   ```
   seq, timestamp, decision, reason, trust_tier, sender_region,
   recipient_region, policy_version, profile, applicable_law,
   dedup_ttl_hours, node_scope, alliance_id, node_region,
   schema_type, schema_version, previous_hash
   ```

   The field `entry_hash` is **excluded** from this object.

2. Marshal the object to JSON with no additional whitespace or key reordering beyond what the implementation's standard JSON library produces.

3. Compute SHA-256 of the marshalled bytes.

4. Format as `"sha256:" + hex.EncodeToString(digest)`.

**Chain rule:** Each entry's `previous_hash` MUST equal the `entry_hash` of the immediately preceding entry, or the initial state for the first entry. A verifier MUST walk the chain in `seq` order and reject any entry where this invariant is violated.

**Root hash:** After appending an entry, the node's **root hash** is the `entry_hash` of the last appended entry. The root hash is published via the management API and gossip.

---

## 6. Schema Type Declaration Contract

### 6.1 Sender Responsibility

The sender declares `schema_type` to communicate the payload domain to the transport layer. This declaration:

- Enables `jurisdiction_isolation` policy evaluation (§4.5)
- Is recorded verbatim in the audit entry
- Does **not** change how the node handles the payload bytes

The declaration is a routing and policy hint. The node does not validate that the payload actually conforms to the declared schema.

### 6.2 Node Opacity Guarantee

A compliant MRMI node MUST NOT:

- Decode, parse, or inspect `envelope.payload`
- Reject an envelope based on payload content
- Log any payload content in the audit entry

The schema type is the only domain metadata the node is permitted to act on.

### 6.3 Valid Schema Types

| Value | Description | Since |
|-------|-------------|-------|
| `messaging` | Default messaging envelope. Active in v0.1. | v0.8 |
| `iso20022` | ISO 20022 financial messaging. | v0.2 |
| `hl7fhir` | HL7 FHIR healthcare data exchange. | v0.3 |
| `edifact` | UN/EDIFACT trade documents. | v0.3 |
| `custom:<id>` | Operator-registered adapter. `<id>` MUST be non-empty. | v0.1 |

A compliant node MUST accept envelopes with any `schema_type` string it does not recognise; unknown types are permitted unless a `jurisdiction_isolation` rule explicitly denies them.

---

## 7. Inter-Node Transport (mTLS)

### 7.1 gRPC over TLS 1.3

Inter-node communication MUST use gRPC. In production environments, nodes MUST establish mutual TLS (mTLS). The minimum TLS version is **TLS 1.3**.

The same X.509 certificate is used for both the server listener and the outbound client connections. This is the **node identity model**: a node's certificate IS its identity on the wire.

### 7.2 Certificate Requirements

- Server TLS mode: `RequireAndVerifyClientCert` — the server MUST demand and verify a client certificate.
- CA: All participating nodes in a federation MUST be signed by the same Certificate Authority (or a CA chain trusted by all peers).
- The CA certificate is configured at `[tls] ca = "..."` and applies to both server verification and client verification.

### 7.3 Insecure Mode

A node MAY be started without TLS (`[tls] insecure = true`). This mode MUST NOT be used in production. A node in insecure mode is not a compliant production node.

### 7.4 gRPC Service Definition

The normative service interface is:

```protobuf
service GatewayService {
  rpc SendEnvelope(SendEnvelopeRequest)   returns (SendEnvelopeResponse);
  rpc GetNodeInfo(GetNodeInfoRequest)     returns (GetNodeInfoResponse);
  rpc BroadcastDiscovery(DiscoveryRequest) returns (DiscoveryResponse);
  rpc Connect(ConnectRequest)             returns (ConnectAck);
}
```

`SendEnvelope` and `GetNodeInfo` are REQUIRED. `BroadcastDiscovery` and `Connect` are OPTIONAL (federated discovery, introduced in v0.2).

---

## 8. Node Tiers

A compliant node MUST declare exactly one of three tiers:

| Tier | `node_scope` | Region requirement | Alliance requirement |
|------|-------------|--------------------|--------------------|
| Regional | `"regional"` | Single `region` (e.g. `"RS"`) | None |
| Alliance | `"alliance"` | `regions` list (≥1 entry) | `alliance_id` required |
| Global | `"global"` | Single `region` | None |

The tier is used by `jurisdiction_isolation` rules to restrict which tiers certain schema types may transit (§4.5).

---

## 9. Delivery Guarantee

### 9.1 Tier-Preference Routing

When forwarding an envelope, a compliant node MUST attempt peers in the following preference order:

1. Regional peers whose `region` matches `recipient_region`
2. Alliance peers whose `regions` list includes `recipient_region`
3. Global peers

### 9.2 Transit Cache and Dead-Letter Queue

When all delivery attempts fail, the envelope MUST be written to a transit cache with TTL ≤ 60 seconds. A background process MUST drain the transit cache into the Dead-Letter Queue (DLQ) for operator inspection. Transit cache items that expire before a drain attempt are discarded.

### 9.3 Idempotency

`idempotency_key` MUST be stable across retries. The receiving node deduplicates on this key within the configured TTL. A `DUPLICATE` decision is not an error; the sender SHOULD treat it as a successful delivery confirmation.

---

## 10. Compliance Conformance

### 10.1 REQUIRED (Core Compliance)

A node claiming MRMI compatibility MUST implement:

1. Accept envelopes via the `SendEnvelope` gRPC RPC.
2. Normalize empty `schema_type` to `"messaging"` before policy evaluation.
3. Evaluate the policy engine in the exact order defined in §4.3.
4. Append a Merkle-chained audit entry for every decision, including DUPLICATE and ALLOW/DUMMY.
5. Compute entry hashes using the algorithm in §5.2.
6. Enforce deduplication by `idempotency_key` within the configured TTL.
7. Implement the node tier model (`regional` / `alliance` / `global`).
8. Expose `GetNodeInfo` to return the node's identity and active profile.
9. Use TLS 1.3 minimum for all inter-node gRPC connections in production.

### 10.2 RECOMMENDED

10. Verify Ed25519 envelope signatures when a verification key is configured.
11. Implement the transit cache and DLQ for failed deliveries.
12. Publish the audit root hash via the `.well-known/mrmi-audit` endpoint.
13. Support hot-reload of the TOML configuration without restart.

### 10.3 OPTIONAL

- Management REST API (`/api/v1/*`)
- SSE envelope inbox stream (`/api/v1/stream`)
- Dashboard SPA
- Webhook notifications to registered applications
- Dynamic peer gossip (`ExchangePeers` RPC)
- Federated user discovery (`BroadcastDiscovery` / `Connect` RPCs)
- Persistent storage backends (bbolt, Redis)
- DNS TXT root hash publication
- Dummy traffic generation

---

## 11. Compatibility Statement

### 11.1 Wire Compatibility

A compliant node MUST accept any `Envelope` message that is valid Protocol Buffers v3 binary encoding of the message defined in §3.1, regardless of which optional fields are present. Unknown fields MUST be silently ignored (proto3 forward-compatibility rule).

### 11.2 Version Negotiation

There is no protocol-level version negotiation in v0.1. Version compatibility is managed at the operator level via the `policy_version` node config field, which is recorded in every audit entry.

### 11.3 Backward Compatibility with v0.8

All envelopes produced by MRMI Gateway v0.8 or earlier (which predate the Schema Registry) MUST be accepted. The normalization rule in §3.3 provides this guarantee: an absent `schema_type` is treated as `"messaging"`, and the `"messaging"` adapter imposes no jurisdiction constraints by default.

---

## 12. Reference Implementation

The Go reference implementation is the authoritative source during v0.1. When this document is ambiguous, the behaviour of the Go implementation takes precedence.

| Component | Source Path |
|-----------|-------------|
| Envelope (Go type) | `internal/core/gateway.go` |
| Policy engine | `internal/policy/engine.go` |
| Audit log | `internal/audit/log.go` |
| Schema normalization | `internal/schema/messaging.go` |
| mTLS | `internal/tlsutil/tls.go` |
| Ed25519 signing | `internal/identity/identity.go` |
| Proto contract | `proto/mrmi/v1/contracts.proto` |
| TOML config | `internal/config/config.go` |
