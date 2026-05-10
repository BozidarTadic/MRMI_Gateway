# MRMI Gateway — Architecture Decision Record

**Multi-Regional Multi-App Interlock**

---

| Field        | Value                                                                                     |
|--------------|-------------------------------------------------------------------------------------------|
| ADR ID       | MRMI-ADR-001                                                                              |
| Status       | PROPOSED                                                                                  |
| Author       | Božidar Tadić                                                                             |
| Version      | 0.8 — Node Tier model (Regional / Alliance / Global), node_scope field, ADR-006 expanded, corridor flexibility |
| Supersedes   | v0.7                                                                                      |
| Covers App   | v0.1 (target) · v0.2–v1.0 (roadmap)                                                      |
| Created      | 2025                                                                                      |

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*

---

## Table of Contents

- [0. Changelog](#0-changelog)
- [1. Context](#1-context)
- [2. Architecture Decision Records](#2-architecture-decision-records)
- [3. Architecture Overview](#3-architecture-overview)
- [4. Data & Metadata Privacy](#4-data--metadata-privacy)
- [4.3 Data Transit Policy](#43-data-transit-policy)
- [5. Explicit Non-Goals](#5-explicit-non-goals)
- [6. Risks & Mitigations](#6-risks--mitigations)
- [7. Bootstrap Strategy](#7-bootstrap-strategy)
- [8. Open Questions](#8-open-questions)
- [9. Consequences](#9-consequences)
- [10. Operator Incentives](#10-operator-incentives)
- [11. Acceptance Criteria — v0.1](#11-acceptance-criteria--v01)
- [12. Milestones by Version](#12-milestones-by-version)
- [13. Roadmap](#13-roadmap)
- [ADR-012 — Federated User Discovery (v0.2)](#adr-012--federated-user-discovery-v02)
- [ADR-013 — Storage & Cache (v0.3)](#adr-013--storage--cache-v03)
- [ADR-014 — Management API](#adr-014--management-api)
- [Appendix A — Full TOML Configuration Examples](#appendix-a--full-toml-configuration-examples)
- [Appendix B — Operator Compliance Checklist](#appendix-b--operator-compliance-checklist)

---

## 0. Changelog

### v0.7 → v0.8

| Change | Detail |
|--------|--------|
| Node Tier model | Three deployment tiers: Regional, Alliance, Global. Not every country needs its own node. Alliance nodes serve compatible-jurisdiction groups (EU/GDPR, EAEU). Global nodes provide neutral routing for corridors without a regional operator. |
| `node_scope` field | Added to TOML config, audit log entries, and ADR-006. Auditors can see which tier handled each message. |
| ADR-006 expanded | "One node per region" replaced with three-tier topology. Fallback routing path documented. |
| Alliance node examples | EU (GDPR), BY/KZ/AM (EAEU-compatible), author-hosted as global bridge. |
| Data residency principle reinforced | node_scope affects routing only. App data always stored in App's own jurisdiction regardless of node tier. |
| Risks updated | Alliance node legal agreement risk added. |
| Roadmap updated | node_scope and tier model in v0.1 scope (TOML + audit). Alliance/Global node operator guide in v0.3. |

### v0.6 → v0.7 (summary)

Versioned roadmap, Discovery → v0.2, Storage/Cache/Node Discovery/UI → v0.3, hosted nodes (§10.4), Management API (ADR-014), Storage interface (ADR-013), milestones split by version, §13 Roadmap.

### v0.5 → v0.6 (summary)

ADR/App versioning clarification, Data Transit Policy (§4.3), Federated User Discovery Protocol (ADR-012), transit cache conditions, opaque_token model.

### v0.4 → v0.5 (summary)

Acceptance criteria, sequence diagrams, full TOML examples, `applicable_law` field, gossip load note, HTTPS fallback for DNS TXT, operator compliance checklist.

### v0.1 → v0.4 (summary)

Deployment topology, delivery semantics / idempotency, identity trust tiers (T0–T3), trust revocation (CRL + blacklist gossip + trust decay), SDK-first strategy, metadata minimisation with honest limits, Merkle audit log + DNS TXT + HTTPS fallback, signed policy configs, configuration profiles with compliance mapping, bootstrap strategy, horizontal scaling / HA split (ADR-011), operator incentives.

---

## 1. Context

Modern messaging applications operate across a fragmented global regulatory landscape. Frameworks such as Russia's **152-ФЗ**, the EU's **GDPR**, and Kazakhstan's data localisation laws impose strict requirements on where user data may reside and how it may cross borders. Existing federated protocols (Matrix, XMPP) treat cross-border data flow as an implementation concern rather than an architectural guarantee.

**MRMI Gateway** is an open-source federation middleware layer — analogous to Apache Kafka for messaging infrastructure — that any messaging application can integrate. It provides inter-region message routing, identity resolution, at-least-once delivery, policy enforcement, and verifiable audit trails, while remaining agnostic to the business logic of the application built on top of it.

> **Core insight:** Legal compliance is not a deployment concern. It is an architectural constraint that must be enforced at the transport layer — not bolted on afterwards by operators.

### 1.1 Target Use-Case — v0.1

> **Primary corridor:** Messaging between applications in **RU, BY, KZ, AM, RS** — regions with similar data sovereignty concerns, existing commercial relationships, and no current cross-border protocol enforcement. EU/US corridors are deferred to v1.0.

### 1.2 Scope

**v0.1 — In scope:**
Federation gateway core, regional node (Go), policy engine with compliance profiles, Merkle audit log + DNS TXT + HTTPS fallback, signed policy configs with `applicable_law`, identity resolution (T0–T3) + revocation (ADR-010), at-least-once delivery + DLQ + retry, horizontal scaling / HA strategy (ADR-011), .NET SDK v0.1, Go CLI client, Management API read-only (ADR-014), hosted node model.

**v0.2 — In scope:**
Federated user discovery (ADR-012), push notification webhook contract, app namespace isolation (`app_id` filtering), Management API write endpoints.

**v0.3 — In scope:**
Node dashboard UI, node settings via API, storage backends (bbolt → Redis adapter, ADR-013), cache layer, dynamic node discovery (gossip peer list), Management API UI-ready.

**v1.0 — In scope:**
DHT routing, EU/US corridor (separate legal analysis), governance model, operator certification programme.

**Out of scope (all versions):**
Full messenger UI, media storage / CDN (S3 URL in payload is sufficient), end-user authentication / KYC, billing, blockchain consensus, WebRTC signaling (extension module), full traffic anonymity (Tor-level), centralised user identity registry, video calls (deferred post-v1.0).

---

## 2. Architecture Decision Records

### ADR-001 — Standalone Service

Gateway is an independent binary. Applications integrate via SDK or direct gRPC.

| Option | Pros | Cons |
|--------|------|------|
| Embedded library | Simple integration | Couples lifecycle to app; hard to update independently |
| Sidecar (per-pod) | Cloud-native isolation | Kubernetes dependency; complex for simpler deployments |
| **Standalone service ✓** | **Full independence, any language, any platform, own release cycle** | **One additional process to operate** |

---

### ADR-002 — Core Language: Go

Gateway core in Go. SDKs published separately (ADR-009).

| Option | Pros | Cons |
|--------|------|------|
| **Go ✓** | **Single binary, native gRPC, proven in proxy space (Envoy, Caddy), low contributor barrier** | **Less familiar to .NET/JVM contributors — mitigated by SDK strategy** |
| Rust | Maximum performance, memory safety | High barrier; slower contributor growth |
| .NET / C# | Author expertise, rich ecosystem | Heavier runtime; less idiomatic for a gateway proxy |
| Java / Kotlin | Mature ecosystem | JVM startup overhead |

---

### ADR-003 — Inter-Node Protocol: gRPC + mTLS

All cross-region node-to-node communication uses gRPC with mutual TLS. Protobuf defines all contracts.

- mTLS: only registered, certificate-holding nodes may communicate
- Revoked node certificates propagated via CRL gossip (ADR-010)
- Protobuf versioning: field deprecation + reserved IDs from v0.1
- REST: health, metrics, operator tooling only

**Delivery semantics:**

| Semantic | Behaviour | Implementation |
|----------|-----------|----------------|
| **At-least-once ✓** | **Delivered minimum once; duplicates possible** | **UUID v7 idempotency key; dedup TTL per profile (72h / 24h / 1h)** |
| Exactly-once | No duplicates | Distributed transaction — too complex for v0.1 |
| At-most-once | May be lost | Unacceptable for messaging |

> **Dedup TTL per profile:** `strict = 72h` (covers delayed delivery), `balanced = 24h` (default), `performance = 1h` (dev/test). Configurable per-operator on top of profile preset.

**Retry / backoff:**
- Exponential backoff: 1s → 2s → 4s → … → max 5 min
- DLQ per node after 10 failed attempts
- Operator API to inspect and replay DLQ

**Ordering:**
Per-sender ordering preserved within a session (sequence number in envelope header). Cross-sender ordering is not guaranteed — consistent with all major federated protocols.

#### Sequence diagram — message delivery

```
App A (RU)      MRMI Node RU      MRMI Node RS      App B (RS)
    |                 |                 |                 |
    |---send()------->|                 |                 |
    |                 |--dedup check--->|                 |
    |                 |  (UUID v7)      |                 |
    |                 |--policy eval--->|                 |
    |                 |  (allow RU→RS)  |                 |
    |                 |--pad + jitter-->|                 |
    |                 |--sign envelope->|                 |
    |                 |--gRPC/mTLS----->|                 |
    |                 |                 |--dedup check    |
    |                 |                 |--policy eval    |
    |                 |                 |--write Merkle-->|
    |                 |                 |--deliver()----->|
    |                 |<----ACK---------|                 |
    |                 |--write Merkle   |                 |
    |<---ok-----------|                 |                 |
```

---

### ADR-004 — Identity Model

Federated addressing: `user@region-domain`

```
Examples:
  boza@rs.mrmi.net
  ivan@ru.mrmi.net
  asel@kz.mrmi.net
```

Directory stores **only** minimum cryptographic identity — no PII federated:
- Ed25519 public key
- Routing endpoint (gRPC address of regional gateway node)
- Region identifier
- Key expiry timestamp

**Trust tiers (ADR-008):**

| Tier | Meaning | Mechanism |
|------|---------|-----------|
| T0 — Anonymous | Key exists, no claims | Default for new identities |
| T1 — Node-verified | Regional operator confirms key ownership | Challenge-response at registration |
| T2 — Claim-signed | Signed operator attribute claims | Operator signs claims (e.g. "registered user of App X") |
| T3 — Cross-verified | Another T2 node co-signs | Web-of-Trust — inter-corridor trust |

> **Important:** Gateway does not verify real-world identity. Tier assignment is the responsibility of the regional node operator — the legal entity accountable under local law.

---

### ADR-005 — Policy Engine + Configuration Profiles

Each gateway node runs a local Policy Engine evaluated entirely in-memory — no network calls, no central policy server. Policies are TOML-declared, signed by the node operator's private key, and include an explicit `applicable_law` declaration.

**Configuration profiles with compliance mapping:**

| Parameter | `strict` | `balanced` | `performance` |
|-----------|----------|------------|---------------|
| Payload padding bucket | 16 KB | 4 KB | Disabled |
| Timing jitter | 0–500 ms | 0–100 ms | Disabled |
| Dummy traffic rate | 1 msg / 5s / peer | 1 msg / 60s / peer | Disabled |
| Dedup TTL | 72 h | 24 h | 1 h |
| Audit log scope | All decisions | All decisions | Deny-only |
| Cross-region ACK | Confirmed ACK | Confirmed ACK | Fire-and-forget |
| Merkle hash publish interval | Every 1 h | Every 6 h | Disabled |
| Approx. latency delta | +200–700 ms | +10–100 ms | ~0 ms |
| Bandwidth overhead | High | Moderate | Minimal |
| **Compliance mapping** | **GDPR Art.25 / 152-ФЗ high category** | **Standard operation** | **Internal dev/test only** |

> **Note:** Compliance mapping is indicative guidance, not legal certification. Operators must verify compliance with their own legal counsel.

**Merkle audit log — external verification:**
- Every policy decision is an immutable log entry chained via SHA-256
- Root hash published to **DNS TXT** at configurable interval (per profile)
- **HTTPS fallback:** `GET /.well-known/mrmi-audit` — JSON endpoint exposing `{ version, timestamp, root_hash, node_id, signature }`. Used when DNS TXT is unavailable or potentially compromised
- Cross-node hash gossip: opt-in — nodes share root hashes with trusted peers for cross-verifiable audit trail without exposing log content
- Profile name, `applicable_law`, dedup TTL recorded in every audit entry

```
# DNS TXT (primary):
_mrmi-audit.rs.mrmi.net  TXT  "v=1 ts=1719000000 root=sha256:AbCd1234..."

# HTTPS fallback (secondary):
GET https://rs.mrmi.net/.well-known/mrmi-audit
{
  "version": 1,
  "timestamp": 1719000000,
  "root_hash": "sha256:AbCd1234...",
  "node_id": "rs.mrmi.net",
  "applicable_law": "RS-GDPR",
  "signature": "ed25519:XyZ..."
}
```

**Audit log entry structure:**

```
AuditEntry {
  seq:              uint64   // monotonic
  timestamp:        unix_ms
  decision:         ALLOW | DENY
  sender_region:    string
  recipient_region: string
  policy_version:   string
  profile:          strict | balanced | performance
  applicable_law:   string  // e.g. "RU-152FZ", "RS-GDPR"
  dedup_ttl_h:      uint8
  prev_hash:        sha256   // chain integrity
  entry_hash:       sha256   // this entry
}
```

> **Three-layer evidence trail:** (1) signed policy config — what rules were in effect; (2) Merkle log — what decisions were made; (3) DNS TXT + HTTPS root hash — externally verifiable proof log was not tampered with. Operator remains the legally accountable party.

#### Sequence diagram — audit verification

```
Auditor           DNS / HTTPS endpoint    MRMI Node RS      Merkle Log
   |                      |                    |                 |
   |--GET root hash------->|                    |                 |
   |<--{ root_hash, sig }--|                    |                 |
   |                                            |                 |
   |--request log export----------------------->|                 |
   |                                            |--read entries-->|
   |                                            |<--entries-------|
   |<--log entries------------------------------|                 |
   |                                            |                 |
   |--verify chain (prev_hash links)            |                 |
   |--compute root from entries                 |                 |
   |--compare with published root_hash          |                 |
   |--verify node signature on root_hash        |                 |
   |                                            |                 |
   |  ✓ Log integrity confirmed                 |                 |
```

---

### ADR-006 — Deployment Topology

Three node tiers are supported. Operators declare their tier via `node_scope` in TOML. The tier affects routing decisions and audit log entries — it does not affect the protocol itself, which is identical across all tiers.

---

#### Tier 1 — Regional Node

One country, one operator, strict data localisation. The baseline model.

```
node_scope = "regional"
region     = "RU"          # single region
```

```
┌──────────────────────────────────┬───────────────────────────────┐
│  REGION: RU                      │  REGION: RS                   │
│  ┌─────────┐  ┌─────────┐        │  ┌─────────┐                 │
│  │  App A  │  │  App B  │        │  │  App C  │                 │
│  └────┬────┘  └────┬────┘        │  └────┬────┘                 │
│       └─────┬──────┘             │       │                      │
│       ┌─────▼──────┐             │  ┌────▼──────┐               │
│       │  MRMI Node  │◄──gRPC/mTLS──►│  MRMI Node│               │
│       │  scope:     │             │  │  scope:   │               │
│       │  regional   │             │  │  regional │               │
│       └─────────────┘             │  └───────────┘               │
│  152-ФЗ compliant                │  GDPR compliant              │
└──────────────────────────────────┴───────────────────────────────┘
```

**When to use:** Any country with its own operator and data sovereignty requirements. RS and RU seed nodes are both Tier 1.

---

#### Tier 2 — Alliance Node

Multiple countries with compatible legal frameworks share a single node. The node operator declares which regions it serves. App data is still stored on each App's own servers in their respective jurisdictions — the node only routes.

```
node_scope  = "alliance"
regions     = ["BY", "KZ", "AM"]     # all served regions
alliance_id = "eaeu-corridor-01"     # identifies the legal agreement group
```

```
┌─────────────────────────────────────────────────┐
│  ALLIANCE NODE — BY / KZ / AM                   │
│  (EAEU-compatible legal framework)              │
│                                                 │
│  App (BY) ──┐                                   │
│  App (KZ) ──┼──► MRMI Node ◄──gRPC/mTLS──► RS  │
│  App (AM) ──┘    scope: alliance                │
│                                                 │
│  Data stored: each App in its own country       │
│  Node stores: audit log only (operator's choice │
│               of jurisdiction for node itself)  │
└─────────────────────────────────────────────────┘
```

**When to use:**
- EU countries sharing GDPR framework — one node serves DE, FR, PL, etc.
- EAEU members (BY, KZ, AM) with compatible data localisation laws
- Any group of countries that have a legal agreement allowing shared infrastructure

**Legal requirement:** The alliance operator must have a legal basis for routing data from all declared regions. This is the operator's responsibility — not enforced by the protocol. The `alliance_id` field in audit entries provides a reference to the governing agreement.

**Key principle:** A BY/KZ operator hosting a node that RU traffic passes through is valid — because the node sees only an encrypted blob, and user data lives on the App's servers in each user's home country. The node is a postal hub, not an archive.

---

#### Tier 3 — Global / Neutral Node

A node without a specific jurisdictional claim. Used for:
- Corridors where no regional or alliance operator exists yet
- Neutral relay between incompatible regulatory zones (e.g. future RU↔EU bridge via RS)
- Author-hosted bootstrap nodes during early adoption

```
node_scope  = "global"
region      = "RS"          # physical location of the node
disclaimer  = "no-data-residency-claims"
```

```
┌─────────────────────────────────────────────────┐
│  GLOBAL NODE — RS (neutral)                     │
│                                                 │
│  Serves: any region not covered by regional/    │
│          alliance node                          │
│                                                 │
│  ◄──── RU ────►  GLOBAL NODE  ◄──── AM ────►   │
│                                                 │
│  As regional operators join: global node        │
│  routes are replaced by direct regional paths.  │
└─────────────────────────────────────────────────┘
```

**When to use:** Bootstrap phase. Author-hosted nodes are Global tier. As the ecosystem matures, Global nodes become fallback-only — most traffic routes through Regional or Alliance nodes.

**Important:** Global nodes explicitly disclaim data residency guarantees. Apps routing through a Global node must accept this in their own ToS. The audit log records `node_scope = global` so it is always transparent.

---

#### Routing path selection

When sending a message, the SDK selects the best available path:

```
1. Direct Regional node in destination region     ← preferred
2. Alliance node covering destination region      ← if no regional node
3. Global node                                    ← fallback
4. DLQ (retry later)                             ← if no path available
```

Path selection is deterministic and logged in the audit entry. Operators can restrict which tiers their node will route through via policy config:

```toml
[policy.routing]
allow_via = ["regional", "alliance"]   # refuse to route via global nodes
# allow_via = ["regional"]            # strictest: only direct regional paths
```

---

#### `node_scope` in audit log

Every audit entry records the scope of the node that processed the message:

```
AuditEntry {
  ...
  node_scope:   "regional" | "alliance" | "global"
  alliance_id:  string   // populated only for alliance nodes
  node_region:  string   // physical region of the node
  ...
}
```

This makes the routing path fully transparent to auditors and regulators.

---

### ADR-007 — Delivery Semantics

- At-least-once across regional boundaries
- Idempotency: UUID v7 per envelope; TTL per profile (72h / 24h / 1h)
- Per-sender ordering within session; no global ordering
- Retry: exponential backoff → DLQ after 10 failures
- Receiving node ACKs envelope receipt; application-level ACK out of scope

---

### ADR-009 — SDK-First Strategy

| SDK | Target | Release |
|-----|--------|---------|
| Go | Gateway core companion | v0.1 — ships with core |
| .NET / C# | Primary author stack; enterprise adoption reference | v0.1 |
| Java / Kotlin | JVM enterprise | v0.2 — open for contributor |
| Python | Scripting / ML pipeline | v0.2 — open for contributor |

---

### ADR-010 — Trust Revocation

**Mechanisms:**
- **CRL (Certificate Revocation List):** each node maintains a signed CRL, propagated to peers via gossip on update
- **Node blacklist:** published signed blacklist entry requires corroboration from ≥2 independent T2+ nodes before taking effect (prevents single-node censorship)
- **Trust decay:** no cross-validation update within 30 days → effective trust tier auto-reduced by one level
- **Key rotation:** operators must publish new Ed25519 key before expiry; failure → automatic T0 downgrade

#### Sequence diagram — revocation flow

```
Operator A        Node X (compromised)   Node Y (T2+)    Node Z (T2+)    All peers
    |                     |                   |               |               |
    |--publish BlacklistEntry { node_id:X, reason, sig_A }                   |
    |--gossip to peers------------------------------------------->           |
    |                                         |--corroborate?                 |
    |                                         |  (independent verify)         |
    |                                         |--co-sign entry--------------->|
    |                                                         |--co-sign----->|
    |                     |                                                   |
    |  quorum reached (≥2 T2+ signatures)                                    |
    |--update CRL----------------------------------------------------------->|
    |--gossip CRL to all peers---------------------------------------------->|
    |                                                                         |
    |                     |<--mTLS handshake rejected by all peers            |
    |--publish new DNS TXT root hash (CRL updated)                           |
```

> **Anti-censorship guard:** A single node cannot blacklist another unilaterally. Minimum ≥2 independent T2+ signatures required.

**Reputation scoring (lightweight, v0.1):**
- Uptime and ACK reliability → positive
- Policy violations detected by peers → negative
- Failed cross-validation → negative
- Score is local to each node; not a global consensus mechanism
- Nodes with low score may be deprioritised for routing by peers
- Full reputation system deferred to v1.0; data model defined in v0.1 for future use

---

### ADR-011 — Horizontal Scaling & High Availability

**Stateless / stateful split:**

| Component | Stateless? | Scaling Strategy |
|-----------|-----------|-----------------|
| Routing engine | ✅ Yes | Horizontal: multiple instances behind load balancer |
| Policy engine | ✅ Yes | Horizontal: config loaded at startup, no shared state |
| gRPC listener | ✅ Yes | Horizontal: each instance handles independent connections |
| Dedup index | ❌ No | Shared Redis / embedded KV; single writer or distributed lock |
| DLQ | ❌ No | Single writer per region; replicated for HA |
| Merkle audit log | ❌ No | Append-only; single writer; read replicas for verification |
| mTLS / CRL store | ❌ No | Replicated; read-heavy; updated via CRL gossip |

```
HA topology (single region):

  Load Balancer (gRPC-aware)
       │              │              │
  ┌────▼────┐  ┌──────▼────┐  ┌─────▼────┐
  │ MRMI-1  │  │  MRMI-2   │  │  MRMI-3  │  ...
  │(routing)│  │ (routing) │  │(routing) │
  └────┬────┘  └─────┬─────┘  └────┬─────┘
       └─────────────┼─────────────┘
               ┌─────▼──────┐
               │ Shared State│
               │ (Redis/KV)  │
               │ Dedup index │
               │ DLQ         │
               └─────┬──────┘
                     │
               ┌─────▼──────┐
               │ Merkle Log  │
               │(append-only)│
               └────────────┘
```

- v0.1 reference deployment: single process + embedded KV (bbolt)
- v0.2: Redis adapter; multiple routing instances
- HA architecture is defined now so that scaling is an operational decision, not a protocol change

---

### ADR-012 — Federated User Discovery *(v0.2)*

**Status: PROPOSED — scheduled for v0.2. Proto fields reserved in v0.1 schemas.**

#### Problem

A user knows a contact's identifier (phone number, email, or username) but does not know in which region or application that contact is registered.

MRMI Gateway must support cross-regional user lookup without introducing a centralised identity registry — which would create a single point of surveillance, a single point of failure, and introduce jurisdictional ambiguity incompatible with data sovereignty guarantees.

#### Design

**Discovery is app-driven, not node-driven.** Nodes broadcast the request; each connected application decides independently whether and how to respond, according to its own internal policy.

**Discovery flow:**

```
User types "@bozidar" or "+381..."
    ↓
SDK → Local Gateway Node (origin node)
    ↓
Origin node broadcasts DiscoveryRequest to all known peer nodes
  (hop_limit enforced — see scale path)
    ↓
Each peer node forwards DiscoveryRequest to its connected Apps
    ↓
Each App evaluates against its own discovery policy:
  - EXACT_MATCH_ONLY  → responds only if identifier matches exactly
  - PARTIAL_MATCH     → responds if identifier starts with / contains query
  - SILENT            → does not respond (user is not discoverable)
    ↓
App returns DiscoveryResponse: { node_id, app_id, opaque_token, display_hint }
    ↓
Origin node aggregates responses, returns list to requesting user
    ↓
User selects contact from list
    ↓
User's SDK sends ConnectRequest using opaque_token
    ↓
Remote app resolves opaque_token → real user_id (locally, never exposed)
    ↓
Connection established. opaque_token expires immediately after handshake.
```

#### Sequence diagram — discovery + connect

```
User (RS)       Node RS        Node RU        App B (RU)     App C (RU)
    |               |               |               |               |
    |--discover()-->|               |               |               |
    |               |--broadcast DiscoveryRequest-->|               |
    |               |               |--forward----->|               |
    |               |               |--forward------------------>   |
    |               |               |               |               |
    |               |               |<--{ opaque_token, hint }--    |
    |               |               |<--SILENT (no response)-----   |
    |               |<--aggregate---|               |               |
    |<--[results]---|               |               |               |
    |               |               |               |               |
    |--connect(opaque_token)------->|               |               |
    |               |               |--ConnectReq-->|               |
    |               |               |               |--resolve token|
    |               |               |               |--accept/deny  |
    |               |               |<--ConnectAck--|               |
    |<--connected---|               |               |               |
    |               |               | token expired |               |
```

#### DiscoveryRequest structure

```protobuf
message DiscoveryRequest {
  string  query_hash      = 1;  // SHA-256 of identifier — node never sees plaintext
  string  query_type      = 2;  // "phone" | "email" | "username"
  string  origin_node_id  = 3;
  string  origin_app_id   = 4;
  uint32  hop_limit       = 5;  // remaining hops; node decrements before forwarding
  string  request_id      = 6;  // UUID v7 — dedup for broadcast
  int64   timestamp       = 7;  // unix_ms — stale requests rejected (TTL: 30s)
}
```

> **Privacy note:** The node layer never sees the plaintext identifier. Only the App receives the hash and performs the local lookup. This keeps PII within the application boundary — not the federation layer.

#### DiscoveryResponse structure

```protobuf
message DiscoveryResponse {
  string  node_id        = 1;
  string  app_id         = 2;
  string  opaque_token   = 3;  // temporary — expires after handshake or TTL
  string  display_hint   = 4;  // e.g. display name or "User on App X" — app decides
  string  match_type     = 5;  // "exact" | "partial"
  int64   token_expires  = 6;  // unix_ms
}
```

#### User visibility levels

Each user controls their own discoverability within their application. The application enforces this and translates it into the response policy:

| Level | Meaning | App behaviour |
|-------|---------|--------------|
| `DISCOVERABLE` | Visible to any node | Responds to all DiscoveryRequests |
| `FRIENDS_ONLY` | Visible only if connection already exists | Responds only if sender's `origin_app_id` is in contact list |
| `HIDDEN` | Not discoverable | Always SILENT — returns no response |

#### opaque_token — privacy guarantee

- Generated by the **App**, not the node
- Maps to a real `user_id` only inside the App's own database
- Exposed only to the requesting user, never stored by any node
- Expires after handshake completion OR after a fixed TTL (suggested: 5 minutes)
- After expiry, the token cannot be replayed — a new discovery request is required

#### Auto-accept configuration

Applications may configure auto-accept for incoming ConnectRequests:

| Mode | Behaviour |
|------|-----------|
| `MANUAL` | User must confirm every incoming connection request (default) |
| `AUTO_WHITELIST` | Auto-accept if origin node is in operator's trusted node list |
| `AUTO_MUTUAL` | Auto-accept if both parties initiated discovery for each other within TTL window |
| `AUTO_ALL` | Auto-accept all incoming — suitable for public-facing bots or services |

Auto-accept is configured at the **App level**, not the node level. The node only routes the ConnectRequest.

#### Scale path

| Phase | Discovery mechanism | Condition |
|-------|--------------------|-----------| 
| v0.1 — Seed corridor | Broadcast with `hop_limit = 3` | 2–10 nodes; broadcast is trivially manageable |
| v0.2 — Corridor expansion | Broadcast with `hop_limit` configurable | 10–50 nodes; monitor broadcast volume |
| v0.3+ — DHT routing | Hash-based DHT lookup; broadcast deprecated | >50 nodes; broadcast storm risk requires transition |

> **Why not DHT from day one?** DHT adds significant complexity (partition tolerance, quorum, key placement). For the v0.1 two-node seed corridor it is pure overhead. The `query_hash` field in `DiscoveryRequest` is designed to be directly usable as a DHT key when the transition happens — no protocol change required, only the routing layer changes.

#### What discovery does NOT expose

- Plaintext phone number, email, or username — only SHA-256 hash reaches peer nodes
- Real `user_id` — only `opaque_token` is returned
- Whether a user exists on a SILENT node — no response is indistinguishable from "not found"
- The list of all users on any node — discovery is query-driven, not bulk-export

#### Compliance note

Discovery requests transit peer nodes as opaque hash queries. No plaintext PII is transmitted at the federation layer. Under 152-ФЗ framing, this is "transmission of pseudonymised routing data", not "processing of personal data" — because the node cannot reverse the hash or resolve the token. The App remains the accountable party for its own user database.

---

## 3. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                 MRMI GATEWAY — MESSAGE FLOW (v0.8)                   │
└──────────────────────────────────────────────────────────────────────┘

         TIER 1          TIER 2              TIER 1
      Regional (RU)   Alliance (BY/KZ/AM)  Regional (RS)
          │                  │                  │
     App (RU)           App (BY)            App (RS)
          │                  │                  │
    ┌─────▼─────┐      ┌─────▼──────┐     ┌────▼──────┐
    │ MRMI Node │      │ MRMI Node  │     │ MRMI Node │
    │ scope:    │◄────►│ scope:     │◄───►│ scope:    │
    │ regional  │      │ alliance   │     │ regional  │
    └─────┬─────┘      └────────────┘     └────┬──────┘
          │                                    │
          └──────────── direct ────────────────┘
                   (if policy allows)

         TIER 3 — Global node (RS hosted, neutral)
         Used as fallback when no regional/alliance path exists

Each node:
  1. Validate idempotency key (dedup TTL per profile)
  2. Check CRL (revocation)
  3. Policy engine + profile + routing tier check
  4. Apply padding + jitter
  5. Sign + forward envelope (gRPC/mTLS)
  6. Write Merkle audit entry (includes node_scope)

Merkle → DNS TXT + HTTPS (/.well-known/mrmi-audit)
CRL gossip between all peers
App data: always stored in App's own jurisdiction
```

---

## 4. Data & Metadata Privacy

### 4.1 Envelope Structure

```protobuf
// Preview — full definition in proto/mrmi/v1/envelope.proto
message Envelope {
  string  idempotency_key     = 1;  // UUID v7 — dedup
  bytes   sender_identity     = 2;  // Ed25519 public key — no PII
  bytes   recipient_identity  = 3;  // Ed25519 public key — no PII
  string  sender_region       = 4;  // routing
  string  recipient_region    = 5;  // routing
  uint32  trust_tier          = 6;  // T0–T3
  uint64  sequence_number     = 7;  // per-sender ordering
  bytes   payload             = 8;  // opaque, E2E encrypted by app
  uint32  padded_to           = 9;  // bucket: 256B / 1KB / 4KB / 16KB
  int64   timestamp           = 10; // unix_ms
  bytes   signature           = 11; // sender node signs full envelope
}
```

### 4.2 Metadata Minimisation — Honest Scope

Even with encrypted payload, metadata (who communicates with whom, when, how frequently) constitutes sensitive data under GDPR. Three transport-level mitigations are applied — all controlled by the active configuration profile:

- **Payload padding** — envelopes padded to fixed size bucket; recipient cannot infer message length
- **Timing jitter** — randomised delay before forwarding prevents timing correlation
- **Dummy traffic** — synthetic envelopes when idle; traffic analysis cannot distinguish silence from activity

> **⚠️ Known limits — read carefully:**
>
> These mitigations are **partial**. They do not provide full anonymity.
>
> Known residual attack vectors:
> 1. **Long-term graph analysis** — sustained observation of which region pairs communicate can reveal organisational relationships even without content
> 2. **Traffic volume correlation** — even with padding, sustained high-volume periods may be distinguishable
> 3. **Timing correlation at scale** — jitter provides probabilistic, not absolute, protection
>
> MRMI Gateway is **not** a replacement for Tor or similar anonymity networks. It provides meaningful metadata protection appropriate for **regulated business communication** — not for adversarial anonymity.
>
> Operators requiring stronger anonymity should route MRMI Gateway traffic through an additional anonymisation layer. This is out of scope for the core protocol.

---

### 4.3 Data Transit Policy *(new in v0.6)*

#### Core distinction

> **Data-in-transit** and **data-at-rest** are distinct legal and architectural concepts. MRMI Gateway enforces this distinction explicitly.

| Concept | Definition | MRMI position |
|---------|-----------|---------------|
| **Data-in-transit** | Data actively moving through a node en route to its destination | **Permitted** through any corridor node |
| **Data-at-rest** | Data stored persistently in a node's jurisdiction | **Prohibited** outside the user's origin region |
| **Transit cache** | Short-lived encrypted buffer held by a routing node for optimisation | **Permitted** under strict conditions (see below) |

#### Analogy

A letter passing through a foreign postal hub is transit — legally and architecturally acceptable. A postal hub retaining a copy of the letter is storage — a violation of data sovereignty. MRMI Gateway enforces that transit nodes function as postal hubs, never as archives.

#### Transit cache — permitted conditions

A transit node MAY hold an encrypted envelope in a short-lived buffer if and only if **all** of the following conditions are met:

1. **TTL is strictly enforced** — maximum 60 seconds. After TTL expiry, the buffer entry is deleted with no possibility of recovery.
2. **Payload is opaque to the transit node** — the envelope payload is E2E encrypted by the sending application. The transit node holds an encrypted blob it cannot read — it can only forward or discard.
3. **Audit trail records transit** — the audit log entry records: `{ origin_region, destination_region, transit_node_id, timestamp_in, timestamp_out_or_evicted }`. Payload content is never logged.
4. **No persistence beyond TTL** — transit buffer is in-memory only. It is not written to disk, not replicated, and not included in log exports.
5. **Transit node cannot decrypt** — this is enforced by the E2E encryption model of the application layer. MRMI Gateway does not hold decryption keys.

```
Transit cache entry lifecycle:

  Envelope arrives at transit node
        ↓
  [ encrypted blob stored in memory ]
        ↓
  Forward attempt to destination node
      ↓ success              ↓ failure (node unreachable)
  Delete immediately    Retry (exponential backoff)
                              ↓ TTL = 60s elapsed
                        Evict + audit log entry: "evicted, not delivered"
                        → DLQ if retry budget exhausted
```

#### Legal framing

Under 152-ФЗ and analogous data localisation frameworks, the distinction between "processing" and "storing" personal data is significant. MRMI Gateway's transit model is designed to qualify as **"transmission of opaque encrypted data"** rather than **"processing of personal data"**, because:

- The transit node never accesses plaintext content
- No data remains after TTL expiry
- The node operator has no technical ability to read message content

> **⚠️ This framing is architectural guidance, not legal certification.** Operators must verify the classification with their own legal counsel under their applicable law.

#### Operator configuration

```toml
[transit]
cache_enabled     = true          # default: true
cache_ttl_s       = 60            # max: 60; operator may reduce, not increase
audit_transit     = true          # always true in strict profile; configurable in balanced
```

The `strict` profile enforces `cache_ttl_s = 30` and `audit_transit = true` with no override. The `balanced` profile defaults to `cache_ttl_s = 60` with operator override downward only.

---

## 5. Explicit Non-Goals

- Full-featured messaging application
- End-user accounts, authentication, or KYC
- Media storage, CDN, or file transfer
- Replacing Matrix, XMPP, or Signal
- Full anonymity or Tor-level traffic analysis resistance
- RU↔EU or RU↔US corridors in v0.1
- Blockchain consensus
- WebRTC call signaling in core (extension module)
- Legal compliance certification — gateway is a tool; operator is accountable
- Centralised user identity registry
- Plaintext identifier storage at federation layer

---

## 6. Risks & Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Regulatory landscape changes | 🔴 HIGH | Policy engine: declarative, versioned, hot-reloadable. Profile change requires no restart. |
| Operator non-compliance / fake identity | 🔴 HIGH | mTLS + signed policy config + Merkle log + CRL gossip. Revocation (ADR-010) closes v0.3 gap. |
| Compromised operator node | 🔴 HIGH | Blacklist requires ≥2 T2+ corroborations. Trust decay auto-reduces tier after 30 days without cross-validation. |
| Network fragmentation | 🔴 HIGH | Stateless routing instances can relocate. Fallback routing via neutral-region relay. CRL gossip continues via any reachable peer. |
| Adoption paradox / cold-start | 🔴 HIGH | Bootstrap: RS+RU seed nodes. Operator incentives §10. Acceptance criteria §11 define first milestone concretely. |
| Alliance node legal agreement breakdown | 🟡 MED | `alliance_id` references external legal agreement. If agreement dissolves, alliance node reverts to global scope or shuts down. Operators must monitor agreement validity. Regional nodes unaffected. |
| Discovery broadcast storm | 🟡 MED | hop_limit enforced per request. Scale path to DHT defined (ADR-012). v0.1 scope (2 nodes) makes this a non-issue; monitored before v0.2 expansion. |
| DHT partition resilience (Phase 2) | 🟡 MED | Quorum for DHT writes. Fallback to last-known DNS seed. HA topology (ADR-011) reduces single-node risk. |
| Metadata leakage — graph analysis | 🟡 MED | §4.2 honest limits documented. Partial mitigation via padding/jitter/dummy traffic. Full anonymity explicitly out of scope. |
| Transit cache misuse | 🟡 MED | §4.3: TTL hard cap 60s, in-memory only, opaque payload, audit logged. Operator cannot extend TTL beyond cap. |
| Scaling bottleneck | 🟡 MED | ADR-011: stateless routing + shared state split. v0.1 single-node; HA documented for operators. |
| CRL gossip load at 50+ nodes | 🟡 MED | Not relevant for v0.1 (2 nodes). Bandwidth budget and update frequency to be evaluated before Phase 2 transition. |
| Protocol ossification | 🟡 MED | Protobuf deprecation + reserved IDs. Breaking changes require major version bump + migration guide. |
| Economic sustainability | 🟡 MED | §10 honest operator incentive model. No token economics. |
| Legal liability misattribution | 🟡 MED | Three-layer evidence trail. Documentation explicitly disclaims certification. `applicable_law` field in policy config. |
| Performance overhead from privacy features | 🟢 LOW | `performance` profile disables all overhead. Configurable per operator. |

---

## 7. Bootstrap Strategy

### Phase 1 — Seed Corridor (v0.1)
- Author: seed node **RS** (Belgrade) — scope: **global** (neutral bootstrap node)
- Community volunteer: seed node **RU** — scope: **regional**
- Discovery: DNS SRV — `_mrmi._tcp.rs.mrmi.net`
- Two nodes demonstrate end-to-end protocol operation
- RS node serves as global fallback until more regional nodes join

### Phase 2 — Corridor Expansion (v0.2)
- Open operator registration: BY, KZ, AM
- BY/KZ/AM may join as individual **regional** nodes or form an **alliance** node (EAEU-compatible)
- RS node transitions from global → regional as corridor matures
- Operator requirements published: hardware spec, uptime SLA, legal entity, `node_scope` declaration
- Hybrid DNS + DHT discovery; DHT partition resilience tested before transition
- CRL gossip bandwidth evaluation before expansion beyond 10 nodes
- Discovery broadcast volume monitored; DHT transition plan reviewed

### Phase 3 — Community Governance (v1.0)
- Protocol governance → foundation or community committee
- EU **alliance** node design begins — single GDPR-compliant node serving multiple EU countries
- RU↔EU corridor via RS as neutral **global** relay — separate legal analysis required
- Operator certification programme (optional)
- Discovery: DHT routing operational; broadcast deprecated

---

## 8. Open Questions

> **RESOLVED — WebRTC Signaling:** NOT part of core. Signaling metadata is more sensitive than routing metadata. Session state is architecturally distinct from stateless routing. Resolution: separate extension module with a defined Extension API (event stream contract). See Milestone #9.

> **RESOLVED — Transit vs. Storage:** Data-in-transit through a foreign node is permitted. Data-at-rest outside origin region is prohibited. Transit cache permitted under strict conditions defined in §4.3.

Open for community discussion before v0.2:

- Certificate issuance for new operators: self-signed + web-of-trust, or dedicated MRMI CA?
- Read receipts / typing indicators: cross-regional? (metadata leakage vs. UX)
- DHT transition threshold: at what node count?
- Governance model post-v1.0: foundation, committee, operator vote?
- Extension API contract: event stream interface for signaling and other extensions
- Reputation scoring algorithm: signals, weights, v1.0 full design
- CRL gossip frequency and bandwidth budget at scale (>10 nodes)
- `applicable_law` value registry: who maintains the list of valid values?
- **[NEW]** `opaque_token` lifetime: 5 minutes default — operator configurable? Upper bound?
- **[NEW]** Transit cache TTL: 60s hard cap — should operators be able to reduce to 0 (no cache)?
- **[NEW]** Discovery opt-out enforcement: if a user sets `HIDDEN`, can the node operator override this? (Answer should be no — but enforcement mechanism needs specification.)
- **[NEW]** Discovery query rate limiting: per-origin-node, per-app, or per-user?

---

## 9. Consequences

**Positive:**
- Any messaging application can federate cross-regionally with one gateway node + SDK
- Regional operators remain legally independent — no shared infrastructure, no shared liability
- Trust revocation (ADR-010) closes the compromised-operator gap
- HA/scaling strategy (ADR-011) makes this production-deployable, not just POC
- Compliance profiles with regulation mapping lower operator configuration barrier
- Three-layer audit evidence trail is regulator-presentable
- `applicable_law` field creates explicit auditable declaration of operator's legal intent
- HTTPS fallback removes single point of failure for audit verification
- Metadata minimisation with honest limitations: operators know exactly what protection they get
- Data Transit Policy (§4.3) provides clear legal framing for cross-regional routing
- Federated User Discovery (ADR-012) enables cross-regional contact lookup without a central registry

**Negative / Trade-offs:**
- Cross-regional latency: +10–700 ms depending on profile
- Padding and dummy traffic increase bandwidth in strict/balanced profiles
- Revocation gossip adds background network traffic
- Shared state (dedup, DLQ, Merkle log) requires operational care
- No central enforcement: compliance is trust + audit + revocation, not control
- Bootstrap requires author to operate seed node until community adoption
- Discovery broadcast adds network load at scale — managed by hop_limit and DHT transition path

---

## 10. Operator Incentives

This section exists because a protocol with no clear answer to *"why would someone run a node?"* will not achieve adoption. The following is an honest assessment — not a sales pitch.

### 10.1 Why Run a Node

- **Compliance risk reduction** — verifiable, auditable mechanism to enforce data localisation; reduces legal exposure for the operator's own messaging infrastructure
- **Federation without shared infrastructure** — cross-border communication with corridor partners without trusting a third-party central server
- **Compliance infrastructure positioning** — operators can offer "compliant cross-border messaging" as a service to smaller organisations in their region
- **Governance participation** — T2+ nodes in good standing have a voice in protocol governance decisions

### 10.2 What Running a Node Costs

- **Hardware:** 2 vCPU, 4 GB RAM, 50 GB SSD. ~€20–50/month at typical providers in the target corridor
- **Operational:** policy config updates, annual certificate rotation, log export. ~1–2 h/month for a technical operator
- **Legal:** the operator is the accountable party under local law. This is a real cost that cannot be abstracted away

### 10.3 What Running a Node Does Not Require

- No token purchase, staking, or cryptocurrency
- No mandatory fees to the protocol
- No permission from the project author — anyone can run a node
- No KYC or identity verification of the operator by the protocol

### 10.4 Hosted Nodes — Onboarding Path

For operators and application developers who want to join the MRMI corridor without managing their own infrastructure, the project offers hosted nodes operated by the author.

**Target audience:**
- Startups integrating MRMI before investing in own infrastructure
- Applications testing federation before committing to a node
- Small operators in regions where no community node exists yet

**Registration model:**
```
App registers on hosted node → receives:
  app_id    = "com.myapp"
  api_key   = "mrmi_live_..."
  node_url  = "rs-hosted-01.mrmi.net:7777"
  sdk_config = { node_url, app_id, api_key }
```

**Accountability:**
The hosted node operator (project author) is the legally accountable party for the node under local law. Applications using hosted nodes accept this in the Terms of Service. This is an operational convenience — not a protocol feature. Large operators (e.g. Telegram-scale) are expected to run their own nodes.

**Scale path:**
Hosted nodes are a bootstrap mechanism. As the ecosystem matures, community nodes in each region reduce dependency on author-operated infrastructure.

> This model is defined here for clarity. Hosted node ToS, pricing, and SLA are operational concerns outside the scope of this ADR.

---

## 11. Acceptance Criteria — v0.1

The v0.1 milestone is considered complete when **all** of the following criteria are met on the seed corridor (RS ↔ RU, 2 nodes). Discovery criteria are deferred to v0.2.

### Delivery

| Criterion | Target |
|-----------|--------|
| Message delivery success rate | ≥ 99.5% over sustained 1,000 msg/hour for 24 hours |
| Dedup correctness | 0 duplicate deliveries during 24h test with intentional replay injection |
| DLQ behaviour | Messages failing 10 retries appear in DLQ within 5 min of final failure |
| Cross-region ACK latency (balanced profile) | p50 < 200 ms, p99 < 1,000 ms (excluding jitter) |

### Policy engine

| Criterion | Target |
|-----------|--------|
| Policy eval throughput | ≥ 100,000 decisions/second on reference hardware (2 vCPU) |
| Hot-reload | Policy config update applied within 5 seconds without node restart |
| Deny enforcement | 100% of deny-policy envelopes rejected; 0 false positives in allow direction |

### Audit & revocation

| Criterion | Target |
|-----------|--------|
| Merkle log integrity | 100% chain verification passes after 24h test run |
| DNS TXT publish | Root hash published within interval ± 10% (strict: 1h, balanced: 6h) |
| HTTPS fallback | `/.well-known/mrmi-audit` returns valid signed response within 500 ms |
| Revocation propagation | Blacklisted node rejected by all corridor peers within 60 seconds of quorum |

### Profiles

| Criterion | Target |
|-----------|--------|
| `strict` latency delta | p99 ≤ 700 ms additional vs. baseline |
| `balanced` latency delta | p99 ≤ 100 ms additional vs. baseline |
| `performance` overhead | Indistinguishable from baseline (< 5 ms delta) |

### Transit cache

| Criterion | Target |
|-----------|--------|
| Eviction correctness | Undelivered transit entries evicted within TTL ± 5 seconds |
| No disk persistence | Transit buffer confirmed in-memory only via process restart test |

### Management API (read-only)

| Criterion | Target |
|-----------|--------|
| `/api/v1/status` | Returns node health within 200 ms |
| `/api/v1/audit/latest` | Returns last 10 Merkle entries correctly |
| `/api/v1/dlq` | Returns current DLQ contents |
| `/api/v1/peers` | Returns list of known peer nodes |

### Operational

| Criterion | Target |
|-----------|--------|
| Single-node startup | Node ready to accept connections within 10 seconds of process start |
| .NET SDK roundtrip | send() → delivery confirmation ≤ 2s on balanced profile, cross-region |
| Documentation | Operator can deploy a node from README in < 2 hours (measured by first external contributor) |

---

## 12. Milestones by Version

### v0.1 Milestones — Core Protocol

| # | Task | Owner |
|---|------|-------|
| 1 | Define `.proto` schemas: `Envelope`, `PolicyResult`, `IdentityClaim`, `AuditEntry`, `ProfileConfig`, `BlacklistEntry`, `CRLEntry`, `ConnectRequest` — reserve fields for Discovery (v0.2) | Božidar Tadić |
| 2 | Go gateway: routing + policy engine + profiles + dedup (bbolt, configurable TTL) + Merkle log + DNS TXT + HTTPS fallback | Božidar Tadić |
| 3 | Go gateway: CRL store + revocation gossip + trust decay timer | Božidar Tadić |
| 4 | Go gateway: transit cache (in-memory, TTL-enforced, audit-logged) | Božidar Tadić |
| 5 | Go gateway: Management API read-only (status, audit, dlq, peers) | Božidar Tadić |
| 6 | .NET SDK v0.1: `MrmiClient` — send/receive, idempotency key, profile selection, blacklist API | Božidar Tadić |
| 7 | Deploy seed nodes: RS (author) + RU (volunteer). Publish DNS SRV + DNS TXT + HTTPS audit endpoint | Božidar Tadić + community |
| 8 | Hosted node setup: App registration flow, api_key issuance, SDK config generation | Božidar Tadić |
| 9 | Integration tests covering all v0.1 acceptance criteria (§11) | Božidar Tadić |
| 10 | GitHub repo: README, CONTRIBUTING.md, ADR directory, operator setup guide, `good first issue` labels | Božidar Tadić |
| 11 | CLI reference client (Go): send/receive text via SDK | *Open for contributors* |
| 12 | Java SDK v0.1 | *Open for contributors* |

### v0.2 Milestones — User Discovery

| # | Task | Owner |
|---|------|-------|
| 1 | Define `.proto` schemas: `DiscoveryRequest`, `DiscoveryResponse` | Božidar Tadić |
| 2 | Go gateway: DiscoveryRequest broadcast + hop_limit enforcement + response aggregation | Božidar Tadić |
| 3 | Go gateway: opaque_token generation + TTL expiry + post-handshake eviction | Božidar Tadić |
| 4 | App namespace isolation: `app_id` filtering (SAME_APP_ONLY / WHITELIST / OPEN) | Božidar Tadić |
| 5 | Auto-accept modes: MANUAL / AUTO_WHITELIST / AUTO_MUTUAL / AUTO_ALL | Božidar Tadić |
| 6 | Push notification webhook contract: node → App webhook on envelope arrival | Božidar Tadić |
| 7 | .NET SDK v0.2: discovery API, connect request, auto-accept config | Božidar Tadić |
| 8 | Management API write endpoints: peer register, DLQ replay, config reload | Božidar Tadić |
| 9 | Integration tests covering v0.2 discovery acceptance criteria | Božidar Tadić |
| 10 | Python SDK v0.2 | *Open for contributors* |

### v0.3 Milestones — Operator Infrastructure

| # | Task | Owner |
|---|------|-------|
| 1 | ADR-013: Storage interface + bbolt implementation (production-hardened) | Božidar Tadić |
| 2 | ADR-013: Redis adapter implementation — dedup index + DLQ + CRL cache | Božidar Tadić |
| 3 | Dynamic node discovery: gossip peer list, bootstrap from seed, dynamic join | Božidar Tadić |
| 4 | Management API: UI-ready endpoints, auth layer (api_key or JWT) | Božidar Tadić |
| 5 | Node dashboard UI: status, peers, audit log viewer, DLQ management, config editor | *Open for contributors* |
| 6 | Node settings UI: profile selector, allow/deny list editor, TOML export | *Open for contributors* |

---

## 13. Roadmap

| Version | Focus | Key deliverables |
|---------|-------|-----------------|
| **v0.1** | Core protocol | Routing, policy, audit, delivery, mTLS, revocation, node tiers (regional/alliance/global), hosted nodes, read-only API, seed corridor RS↔RU |
| **v0.2** | User discovery | Federated lookup, opaque token, app namespace isolation, auto-accept, push webhook, write API |
| **v0.3** | Operator infrastructure | Storage backends, cache layer, dynamic node discovery, dashboard UI, node settings UI, alliance/global node operator guide |
| **v1.0** | Scale & governance | DHT routing, EU alliance node, RU↔EU via RS neutral relay, governance model, operator certification, reputation system v1.0 |
| **post-v1.0** | Extensions | Video calls, WebRTC signaling extension module, additional SDK languages |

> **Principle:** Each version is independently deployable and useful. v0.1 alone gives a working compliant corridor. v0.2 adds discoverability. v0.3 makes it operationally mature. v1.0 makes it production-scale.

---

---

## ADR-013 — Storage & Cache *(v0.3)*

**Status: DEFERRED — decision recorded now to prevent rearchitecting in v0.3.**

### Problem

The node has four stateful components that require persistent or semi-persistent storage:

| Component | Access pattern | Persistence required |
|-----------|---------------|---------------------|
| Dedup index | Write-once, read-many, TTL eviction | Yes — survives restart |
| DLQ | Write, read, delete | Yes — survives restart |
| Merkle audit log | Append-only | Yes — permanent |
| CRL store | Read-heavy, rare writes | Yes — survives restart |
| Transit cache | Write, TTL evict | No — in-memory only |

### Storage interface pattern (Go)

```go
type NodeStore interface {
    // Dedup
    Deduped(key string, ttl time.Duration) (bool, error)

    // DLQ
    DLQPush(entry DLQEntry) error
    DLQList() ([]DLQEntry, error)
    DLQDelete(id string) error

    // Audit
    AuditAppend(entry AuditEntry) error
    AuditLatest(n int) ([]AuditEntry, error)

    // CRL
    CRLPut(entry CRLEntry) error
    CRLGet(nodeID string) (*CRLEntry, error)
    CRLList() ([]CRLEntry, error)
}
```

Two implementations shipped:

| Implementation | When | Config |
|---------------|------|--------|
| `BboltStore` | v0.1 default — single binary, zero deps | `backend = "bbolt"` |
| `RedisStore` | v0.3 — horizontal scaling, TTL native | `backend = "redis"` |

### TOML configuration

```toml
# v0.1 default
[storage]
backend  = "bbolt"
path     = "/var/lib/mrmi"

# v0.3 — switch to Redis (no other config changes required)
# [storage]
# backend   = "redis"
# redis_url = "redis://localhost:6379"
# key_prefix = "mrmi:"
```

> **Design principle:** Switching from bbolt to Redis requires changing one line in TOML. No code changes, no data migration for dedup/DLQ (ephemeral by nature). Merkle audit log migration tool provided separately.

---

## ADR-014 — Management API

**Status: PROPOSED — read-only in v0.1, write in v0.2, UI-ready in v0.3.**

### Rationale

If v0.1 ships with TOML-only config and no API, then v0.3 UI requires a full config layer rewrite. Defining the API contract now means the UI can be built directly on top of existing endpoints.

### v0.1 — Read-only endpoints

```
GET  /api/v1/status              → node health, uptime, version, region
GET  /api/v1/config              → current active config (sanitised — no private keys)
GET  /api/v1/audit/latest        → last N Merkle audit entries
GET  /api/v1/audit/verify        → trigger local chain verification, return result
GET  /api/v1/dlq                 → current DLQ contents
GET  /api/v1/peers               → list of known peer nodes + trust tier + last seen
GET  /api/v1/metrics             → Prometheus-compatible metrics endpoint
```

### v0.2 — Write endpoints

```
POST /api/v1/peers/register      → add new peer node
POST /api/v1/dlq/{id}/replay     → replay a DLQ entry
POST /api/v1/dlq/{id}/discard    → discard a DLQ entry
POST /api/v1/config/reload       → trigger hot-reload from TOML file
POST /api/v1/revoke/{node_id}    → publish blacklist entry (requires operator key signature)
```

### v0.3 — UI-ready additions

```
GET  /api/v1/config/schema       → JSON schema of valid config options (for UI form generation)
PUT  /api/v1/config              → write config via API (signs with operator key, triggers reload)
GET  /api/v1/apps                → list registered App IDs on this node
POST /api/v1/apps/register       → register new App (hosted node flow)
DELETE /api/v1/apps/{app_id}     → deregister App
```

### Auth model

```
v0.1:  no auth (localhost-only binding recommended)
v0.2:  api_key header — X-MRMI-Key: mrmi_op_...
v0.3:  api_key or JWT; scoped tokens (read-only vs. operator)
```

### Design principle

The Management API is an operator tool, not a user-facing API. It is never exposed to SDK clients or end users. Recommended deployment: bind to localhost or internal network interface only, reverse-proxy with auth for remote access.

---

## Appendix A — Full TOML Configuration Examples

These are complete, copy-paste-ready configurations. Replace values marked `# REPLACE` before deploying.

### A.1 Profile: `strict`

Recommended for: production nodes handling personal data under 152-ФЗ high category or GDPR Art.25.

```toml
[node]
node_id         = "ru-node-01"                  # REPLACE
region          = "RU"                          # REPLACE
node_scope      = "regional"                    # regional | alliance | global
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "RU-152FZ"                    # REPLACE
signed_by       = "ed25519:YOUR_PUBLIC_KEY"     # REPLACE

[profile]
name            = "strict"

[policy.outbound]
allow_to        = ["BY", "KZ", "AM", "RS"]     # REPLACE
deny_to         = ["US", "CN", "DE", "FR"]     # REPLACE
store_locally   = true

[policy.inbound]
min_trust_tier  = 0

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
export_to_operator  = true
dns_txt_publish     = true
dns_txt_interval_s  = 3600
https_well_known    = true

[transit]
cache_enabled   = true
cache_ttl_s     = 30                            # strict profile: 30s (not configurable above 30)
audit_transit   = true

[discovery]
enabled         = true
hop_limit       = 3

[network]
listen_addr     = "0.0.0.0:7777"
grpc_port       = 7777
metrics_port    = 9090
```

### A.2 Profile: `balanced`

Recommended for: standard production operation without high-sensitivity personal data.

```toml
[node]
node_id         = "rs-node-01"                  # REPLACE
region          = "RS"                          # REPLACE
node_scope      = "regional"                    # regional | alliance | global
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "RS-GDPR"                     # REPLACE
signed_by       = "ed25519:YOUR_PUBLIC_KEY"     # REPLACE

[profile]
name            = "balanced"

[policy.outbound]
allow_to        = ["RU", "BY", "KZ", "AM"]     # REPLACE
deny_to         = []
store_locally   = true

[policy.inbound]
min_trust_tier  = 0

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
export_to_operator  = false
dns_txt_publish     = true
dns_txt_interval_s  = 21600
https_well_known    = true

[transit]
cache_enabled   = true
cache_ttl_s     = 60                            # balanced profile default
audit_transit   = true

[discovery]
enabled         = true
hop_limit       = 3

[network]
listen_addr     = "0.0.0.0:7777"
grpc_port       = 7777
metrics_port    = 9090
```

### A.3 Profile: `performance`

Recommended for: internal dev/test nodes, CI/CD environments.
**Do not use for production nodes handling personal data.**

```toml
[node]
node_id         = "dev-node-01"                 # REPLACE
region          = "RS"                          # REPLACE
node_scope      = "global"                      # dev/test — no residency claims
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "NONE"
signed_by       = "ed25519:YOUR_PUBLIC_KEY"     # REPLACE

[profile]
name            = "performance"

[policy.outbound]
allow_to        = ["RU", "BY", "KZ", "AM", "RS"]
deny_to         = []
store_locally   = false

[policy.inbound]
min_trust_tier  = 0

[policy.audit]
log_all_decisions   = false
log_backend         = "local-merkle"
export_to_operator  = false
dns_txt_publish     = false
https_well_known    = false

[transit]
cache_enabled   = false
audit_transit   = false

[discovery]
enabled         = true
hop_limit       = 2

[network]
listen_addr     = "0.0.0.0:7777"
grpc_port       = 7777
metrics_port    = 9090
```

### A.4 Alliance Node example (BY/KZ/AM — EAEU corridor)

```toml
[node]
node_id         = "eaeu-node-01"                # REPLACE
node_scope      = "alliance"
regions         = ["BY", "KZ", "AM"]            # all regions served by this node
alliance_id     = "eaeu-corridor-01"            # reference to legal agreement
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "BY-PDPA"                     # law of the country where node is hosted
signed_by       = "ed25519:YOUR_PUBLIC_KEY"     # REPLACE

[profile]
name            = "balanced"

[policy.outbound]
allow_to        = ["RU", "RS", "BY", "KZ", "AM"]
deny_to         = []
store_locally   = true

[policy.routing]
allow_via       = ["regional", "alliance"]      # do not route via global nodes

[policy.inbound]
min_trust_tier  = 0

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
dns_txt_publish     = true
dns_txt_interval_s  = 21600
https_well_known    = true

[transit]
cache_enabled   = true
cache_ttl_s     = 60
audit_transit   = true

[discovery]
enabled         = true
hop_limit       = 3

[storage]
backend         = "bbolt"
path            = "/var/lib/mrmi"

[network]
listen_addr     = "0.0.0.0:7777"
grpc_port       = 7777
metrics_port    = 9090
```

---

## Appendix B — Operator Compliance Checklist

Step-by-step preparation for deploying an MRMI Gateway node in a regulated region.

### B.1 Legal preparation

- [ ] Identify the applicable legal framework for your region (e.g. 152-ФЗ, GDPR, KZ data localisation law)
- [ ] Confirm that your organisation is a registered legal entity in the region — you are the accountable party
- [ ] Consult your legal counsel to verify that the `strict` or `balanced` profile settings are appropriate for your data category
- [ ] Determine the data retention period required by your jurisdiction — set `dedup_ttl_h` accordingly
- [ ] Confirm that cross-border communication to your `allow_to` regions is legally permitted under your applicable law
- [ ] Prepare documentation of your node's purpose and data handling for potential regulatory inspection
- [ ] **[NEW]** Confirm with legal counsel that transit cache (§4.3) qualifies as "opaque data transport" under your applicable law, not "processing of personal data"

### B.2 Technical preparation

- [ ] Provision hardware meeting minimum spec: 2 vCPU, 4 GB RAM, 50 GB SSD
- [ ] Generate Ed25519 key pair: `mrmi keygen --output operator.key`
- [ ] **Declare `node_scope`** — regional, alliance, or global. For alliance nodes: confirm legal agreement exists and set `alliance_id`
- [ ] Register your node with the corridor directory (DNS SRV record or seed node registration)
- [ ] Configure `applicable_law` field in your policy config to match your legal framework
- [ ] Select and configure the appropriate profile (`strict` recommended for personal data)
- [ ] Set `allow_to` and `deny_to` lists — review with legal counsel
- [ ] Configure DNS TXT record for audit log root hash publication
- [ ] Enable HTTPS `/.well-known/mrmi-audit` endpoint and verify it returns a valid signed response
- [ ] Configure certificate rotation reminder — Ed25519 key must be rotated before expiry
- [ ] **[NEW]** Configure `[transit]` section: verify `cache_ttl_s` is within permitted bounds for your profile
- [ ] **[NEW]** Configure `[discovery]` section: set `hop_limit` and confirm discovery is enabled/disabled per your use case

### B.3 Operational readiness

- [ ] Test policy `DENY` enforcement: send a message to a denied region and confirm rejection + audit log entry
- [ ] Test policy `ALLOW` enforcement: send a message to an allowed region and confirm delivery + audit log entry
- [ ] Verify Merkle chain integrity: run `mrmi audit verify --local`
- [ ] Verify DNS TXT root hash matches local log: run `mrmi audit verify --dns`
- [ ] Verify HTTPS fallback: `curl https://your-node/.well-known/mrmi-audit`
- [ ] Simulate a revocation: publish a test blacklist entry and confirm propagation to peer node
- [ ] Run DLQ test: confirm failed messages appear in DLQ after 10 retries
- [ ] Document your node's `operator_id`, `region`, `applicable_law`, and policy version in your internal systems
- [ ] **[NEW]** Test transit cache eviction: confirm entries are removed after TTL with no persistence to disk
- [ ] **[NEW]** Test discovery: run a query against peer node, confirm opaque_token returned and expires correctly

### B.4 Ongoing operations

- [ ] Monitor Merkle log integrity daily: `mrmi audit verify --local`
- [ ] Monitor DNS TXT publish interval — alert if root hash is not updated within 2× the configured interval
- [ ] Review DLQ weekly — replay or discard stale messages
- [ ] Rotate Ed25519 key annually (or per your security policy)
- [ ] Update `policy_version` on every policy config change and re-sign
- [ ] Subscribe to MRMI Gateway security announcements (GitHub releases)

---

*This ADR is a living document. All decisions marked PROPOSED are open for community discussion.*

*MRMI Gateway Project · Open Source · v0.8 · 2025*
