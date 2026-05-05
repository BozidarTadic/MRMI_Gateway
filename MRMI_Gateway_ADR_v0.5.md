# MRMI Gateway — Architecture Decision Record

**Multi-Regional Multi-App Interlock**

---

| Field        | Value                                                                                     |
|--------------|-------------------------------------------------------------------------------------------|
| ADR ID       | MRMI-ADR-001                                                                              |
| Status       | PROPOSED                                                                                  |
| Author       | Božidar Tadić                                                                             |
| Version      | 0.5 — Acceptance criteria, sequence diagrams, full TOML examples, `applicable_law` field, gossip load note, HTTPS fallback for DNS TXT, operator compliance checklist |
| Supersedes   | v0.4                                                                                      |
| Created      | 2025                                                                                      |

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*

---

## Table of Contents

- [0. Changelog](#0-changelog)
- [1. Context](#1-context)
- [2. Architecture Decision Records](#2-architecture-decision-records)
- [3. Architecture Overview](#3-architecture-overview)
- [4. Data & Metadata Privacy](#4-data--metadata-privacy)
- [5. Explicit Non-Goals](#5-explicit-non-goals)
- [6. Risks & Mitigations](#6-risks--mitigations)
- [7. Bootstrap Strategy](#7-bootstrap-strategy)
- [8. Open Questions](#8-open-questions)
- [9. Consequences](#9-consequences)
- [10. Operator Incentives](#10-operator-incentives)
- [11. Acceptance Criteria — v0.1](#11-acceptance-criteria--v01)
- [12. Next Steps — v0.1 Milestones](#12-next-steps--v01-milestones)
- [Appendix A — Full TOML Configuration Examples](#appendix-a--full-toml-configuration-examples)
- [Appendix B — Operator Compliance Checklist](#appendix-b--operator-compliance-checklist)

---

## 0. Changelog

### v0.4 → v0.5

| Change | Detail |
|--------|--------|
| §11 — Acceptance criteria added | Concrete, measurable success criteria for v0.1 milestone |
| Sequence diagrams added | Delivery flow, revocation flow, audit verification — as ASCII sequence diagrams |
| Appendix A — Full TOML configs | Complete, copy-paste-ready configurations for each profile |
| `applicable_law` field in policy config | Operator explicitly declares applicable legal framework in signed config |
| Open Questions — gossip load | Added: CRL gossip bandwidth at 50+ nodes — relevant post-Phase 1 |
| ADR-005 — HTTPS fallback | `/.well-known/mrmi-audit` as fallback to DNS TXT for root hash publication |
| Appendix B — Compliance checklist | Step-by-step operator onboarding: legal and technical preparation |

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

**In scope:**
Federation gateway, regional node (Go), policy engine with compliance profiles, Merkle audit log + DNS TXT + HTTPS fallback verification, signed policy configs with `applicable_law`, identity resolution (T0–T3) + revocation (ADR-010), at-least-once delivery, horizontal scaling / HA strategy (ADR-011), SDK interfaces (.NET v0.1; Java/Python v0.2), CLI reference client.

**Out of scope:**
Full messenger UI, media/CDN, end-user authentication, push notifications, EU/US corridor (v1.0), billing, blockchain consensus, WebRTC signaling (extension module — see §8), full traffic anonymity (Tor-level).

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
- **HTTPS fallback (new in v0.5):** `GET /.well-known/mrmi-audit` — JSON endpoint exposing `{ version, timestamp, root_hash, node_id, signature }`. Used when DNS TXT is unavailable or potentially compromised
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

Default model: one node per region. Multiple applications share a node via SDK with isolated `app_id` queues.

```
┌──────────────────────────────────┬───────────────────────────────┐
│  REGION: RU                      │  REGION: RS                   │
│  ┌─────────┐  ┌─────────┐        │  ┌─────────┐                 │
│  │  App A  │  │  App B  │        │  │  App C  │                 │
│  └────┬────┘  └────┬────┘        │  └────┬────┘                 │
│       └─────┬──────┘             │       │                      │
│       ┌─────▼──────┐             │  ┌────▼──────┐               │
│       │  MRMI Node  │◄──gRPC/mTLS──►│  MRMI Node│               │
│       │     RU      │             │  │     RS    │               │
│       └─────────────┘             │  └───────────┘               │
│  152-ФЗ compliant                │  GDPR compliant              │
└──────────────────────────────────┴───────────────────────────────┘
```

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

## 3. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                 MRMI GATEWAY — MESSAGE FLOW (v0.5)                   │
└──────────────────────────────────────────────────────────────────────┘

App A (RU)                                               App B (RS)
    │ SDK                                             SDK │
    ▼                                                     ▼
┌────────────────────────────────┐     ┌────────────────────────────────┐
│  MRMI Node(s) — RU             │     │  MRMI Node(s) — RS             │
│                                │     │                                │
│  1. Validate idempotency key   │     │  7. Dedup check (TTL/profile)  │
│     (dedup TTL per profile)    │     │  8. Policy engine              │
│  2. Check CRL (revocation)     │─────►  9. Check CRL                  │
│  3. Policy engine + profile    │mTLS │  10. Deliver to App B SDK      │
│  4. Apply padding + jitter     │     │  11. Write Merkle audit entry  │
│  5. Sign + forward envelope    │     │  12. ACK to RU node            │
│  6. Write Merkle audit entry   │     │                                │
└────────────────────────────────┘     └────────────────────────────────┘
     │                                                      │
Merkle → DNS TXT + HTTPS (/.well-known/mrmi-audit)    Merkle → DNS TXT + HTTPS
CRL gossip ◄──────────────────────────────────────── CRL gossip
152-ФЗ compliant                                      GDPR compliant
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

---

## 6. Risks & Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Regulatory landscape changes | 🔴 HIGH | Policy engine: declarative, versioned, hot-reloadable. Profile change requires no restart. |
| Operator non-compliance / fake identity | 🔴 HIGH | mTLS + signed policy config + Merkle log + CRL gossip. Revocation (ADR-010) closes v0.3 gap. |
| Compromised operator node | 🔴 HIGH | Blacklist requires ≥2 T2+ corroborations. Trust decay auto-reduces tier after 30 days without cross-validation. |
| Network fragmentation | 🔴 HIGH | Stateless routing instances can relocate. Fallback routing via neutral-region relay. CRL gossip continues via any reachable peer. |
| Adoption paradox / cold-start | 🔴 HIGH | Bootstrap: RS+RU seed nodes. Operator incentives §10. Acceptance criteria §11 define first milestone concretely. |
| DHT partition resilience (Phase 2) | 🟡 MED | Quorum for DHT writes. Fallback to last-known DNS seed. HA topology (ADR-011) reduces single-node risk. |
| Metadata leakage — graph analysis | 🟡 MED | §4.2 honest limits documented. Partial mitigation via padding/jitter/dummy traffic. Full anonymity explicitly out of scope. |
| Scaling bottleneck | 🟡 MED | ADR-011: stateless routing + shared state split. v0.1 single-node; HA documented for operators. |
| CRL gossip load at 50+ nodes | 🟡 MED | Not relevant for v0.1 (2 nodes). Bandwidth budget and update frequency to be evaluated before Phase 2 transition. Added to Open Questions. |
| Protocol ossification | 🟡 MED | Protobuf deprecation + reserved IDs. Breaking changes require major version bump + migration guide. |
| Economic sustainability | 🟡 MED | §10 honest operator incentive model. No token economics. |
| Legal liability misattribution | 🟡 MED | Three-layer evidence trail. Documentation explicitly disclaims certification. `applicable_law` field in policy config. |
| Performance overhead from privacy features | 🟢 LOW | `performance` profile disables all overhead. Configurable per operator. |

---

## 7. Bootstrap Strategy

### Phase 1 — Seed Corridor (v0.1)
- Author: seed node **RS** (Belgrade)
- Community volunteer: seed node **RU**
- Discovery: DNS SRV — `_mrmi._tcp.rs.mrmi.net`
- Two nodes demonstrate end-to-end protocol operation

### Phase 2 — Corridor Expansion (v0.2)
- Open operator registration: BY, KZ, AM
- Operator requirements published: hardware spec, uptime SLA, legal entity
- Hybrid DNS + DHT discovery; DHT partition resilience tested before transition
- CRL gossip bandwidth evaluation before expansion beyond 10 nodes

### Phase 3 — Community Governance (v1.0)
- Protocol governance → foundation or community committee
- EU/US corridor design — separate legal analysis required
- Operator certification programme (optional)

---

## 8. Open Questions

> **RESOLVED — WebRTC Signaling:** NOT part of core. Signaling metadata is more sensitive than routing metadata. Session state is architecturally distinct from stateless routing. Resolution: separate extension module with a defined Extension API (event stream contract). See Milestone #9.

Open for community discussion before v0.2:

- Certificate issuance for new operators: self-signed + web-of-trust, or dedicated MRMI CA?
- Read receipts / typing indicators: cross-regional? (metadata leakage vs. UX)
- DHT transition threshold: at what node count?
- Governance model post-v1.0: foundation, committee, operator vote?
- Extension API contract: event stream interface for signaling and other extensions
- Reputation scoring algorithm: signals, weights, v1.0 full design
- CRL gossip frequency and bandwidth budget at scale (>10 nodes)
- `applicable_law` value registry: who maintains the list of valid values?

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

**Negative / Trade-offs:**
- Cross-regional latency: +10–700 ms depending on profile
- Padding and dummy traffic increase bandwidth in strict/balanced profiles
- Revocation gossip adds background network traffic
- Shared state (dedup, DLQ, Merkle log) requires operational care
- No central enforcement: compliance is trust + audit + revocation, not control
- Bootstrap requires author to operate seed node until community adoption

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

> **Honest position:** The MRMI Gateway protocol is free infrastructure. The incentive to run a node is operational benefit, not financial reward. If an organisation has no cross-regional messaging need and no compliance obligation, there is no reason for them to run a node. The protocol is designed for organisations that do.

---

## 11. Acceptance Criteria — v0.1

The v0.1 milestone is considered complete when **all** of the following criteria are met on the seed corridor (RS ↔ RU, 2 nodes):

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

### Operational

| Criterion | Target |
|-----------|--------|
| Single-node startup | Node ready to accept connections within 10 seconds of process start |
| .NET SDK roundtrip | send() → delivery confirmation ≤ 2s on balanced profile, cross-region |
| Documentation | Operator can deploy a node from README in < 2 hours (measured by first external contributor) |

---

## 12. Next Steps — v0.1 Milestones

| # | Task | Owner |
|---|------|-------|
| 1 | Define `.proto` schemas: `Envelope`, `PolicyResult`, `IdentityClaim`, `AuditEntry`, `ProfileConfig`, `BlacklistEntry`, `CRLEntry` | Božidar Tadić |
| 2 | Go gateway: routing + policy engine + profiles + dedup (configurable TTL) + Merkle log + DNS TXT + HTTPS fallback | Božidar Tadić |
| 3 | Go gateway: CRL store + revocation gossip + trust decay timer | Božidar Tadić |
| 4 | .NET SDK: `MrmiClient` — send/receive, idempotency key, profile selection, blacklist API | Božidar Tadić |
| 5 | Deploy seed nodes: RS (author) + RU (volunteer). Publish DNS SRV + first DNS TXT + HTTPS audit endpoint | Božidar Tadić + community |
| 6 | Integration tests covering all acceptance criteria in §11 | Božidar Tadić |
| 7 | GitHub repo: README, CONTRIBUTING.md, ADR directory, operator setup guide, `good first issue` labels | Božidar Tadić |
| 8 | CLI reference client (Go): send/receive text via SDK | *Open for contributors* |
| 9 | Extension API design: event stream contract for signaling and other extensions | *Open for contributors* |
| 10 | Java SDK | *Open for contributors* |

---

## Appendix A — Full TOML Configuration Examples

These are complete, copy-paste-ready configurations. Replace values marked `# REPLACE` before deploying.

### A.1 Profile: `strict`

Recommended for: production nodes handling personal data under 152-ФЗ high category or GDPR Art.25.

```toml
[node]
node_id         = "ru-node-01"                  # REPLACE
region          = "RU"                          # REPLACE
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "RU-152FZ"                    # REPLACE — see applicable_law registry
signed_by       = "ed25519:YOUR_PUBLIC_KEY"     # REPLACE

[profile]
name            = "strict"

# Profile presets — override only if you have a specific reason
# [profile_override]
# timing_jitter_max_ms = 300

[policy.outbound]
allow_to        = ["BY", "KZ", "AM", "RS"]     # REPLACE — list permitted destination regions
deny_to         = ["US", "CN", "DE", "FR"]     # REPLACE — list denied destination regions
store_locally   = true

[policy.inbound]
min_trust_tier  = 0                             # 0 = accept T0+; raise to require verified senders

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
export_to_operator  = true
dns_txt_publish     = true
dns_txt_interval_s  = 3600                      # 1 hour
https_well_known    = true                      # enable /.well-known/mrmi-audit

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
dns_txt_interval_s  = 21600                     # 6 hours
https_well_known    = true

[network]
listen_addr     = "0.0.0.0:7777"
grpc_port       = 7777
metrics_port    = 9090
```

### A.3 Profile: `performance`

Recommended for: internal dev/test nodes, CI/CD environments, non-regulated internal tooling.
**Do not use for production nodes handling personal data.**

```toml
[node]
node_id         = "dev-node-01"                 # REPLACE
region          = "RS"                          # REPLACE
operator_id     = "your-org-id"                 # REPLACE
policy_version  = "1.0.0"
applicable_law  = "NONE"                        # internal/test only
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
log_all_decisions   = false                     # deny-only per performance profile
log_backend         = "local-merkle"
export_to_operator  = false
dns_txt_publish     = false
https_well_known    = false

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

### B.2 Technical preparation

- [ ] Provision hardware meeting minimum spec: 2 vCPU, 4 GB RAM, 50 GB SSD
- [ ] Generate Ed25519 key pair: `mrmi keygen --output operator.key`
- [ ] Register your node with the corridor directory (DNS SRV record or seed node registration)
- [ ] Configure `applicable_law` field in your policy config to match your legal framework
- [ ] Select and configure the appropriate profile (`strict` recommended for personal data)
- [ ] Set `allow_to` and `deny_to` lists — review with legal counsel
- [ ] Configure DNS TXT record for audit log root hash publication
- [ ] Enable HTTPS `/.well-known/mrmi-audit` endpoint and verify it returns a valid signed response
- [ ] Configure certificate rotation reminder — Ed25519 key must be rotated before expiry

### B.3 Operational readiness

- [ ] Test policy `DENY` enforcement: send a message to a denied region and confirm rejection + audit log entry
- [ ] Test policy `ALLOW` enforcement: send a message to an allowed region and confirm delivery + audit log entry
- [ ] Verify Merkle chain integrity: run `mrmi audit verify --local`
- [ ] Verify DNS TXT root hash matches local log: run `mrmi audit verify --dns`
- [ ] Verify HTTPS fallback: `curl https://your-node/.well-known/mrmi-audit`
- [ ] Simulate a revocation: publish a test blacklist entry and confirm propagation to peer node
- [ ] Run DLQ test: confirm failed messages appear in DLQ after 10 retries
- [ ] Document your node's `operator_id`, `region`, `applicable_law`, and policy version in your internal systems

### B.4 Ongoing operations

- [ ] Monitor Merkle log integrity daily: `mrmi audit verify --local`
- [ ] Monitor DNS TXT publish interval — alert if root hash is not updated within 2× the configured interval
- [ ] Review DLQ weekly — replay or discard stale messages
- [ ] Rotate Ed25519 key annually (or per your security policy)
- [ ] Update `policy_version` on every policy config change and re-sign
- [ ] Subscribe to MRMI Gateway security announcements (GitHub releases)

---

*This ADR is a living document. All decisions marked PROPOSED are open for community discussion.*

*MRMI Gateway Project · Open Source · v0.5 · 2025*
