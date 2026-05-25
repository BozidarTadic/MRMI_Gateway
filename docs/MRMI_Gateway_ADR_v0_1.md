# MRMI Gateway — Architecture Decision Record v0.1

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.2                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.1 — Initial protocol decisions: core ADRs, routing, delivery, identity, audit, compliance profiles, bootstrap |
| Supersedes | —                                                                        |
| Covers App | v0.1 (target)                                                            |
| Created    | 2025                                                                     |

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*

---

## What was decided in v0.1

This is the founding set of architectural decisions for MRMI Gateway. Every ADR from this version forward builds on these.

---

## ADR-001 — Standalone Service

Gateway is an independent binary. Applications integrate via SDK or direct gRPC.

**Decision:** Standalone service — not embedded library, not Kubernetes sidecar.

**Rationale:** Full lifecycle independence. Any language, any platform, own release cycle. One additional process is an acceptable trade-off.

---

## ADR-002 — Core Language: Go

Gateway core in Go. SDKs published separately (ADR-009).

**Decision:** Go — single binary, native gRPC, proven in proxy/gateway space (Envoy, Caddy), low contributor barrier.

---

## ADR-003 — Inter-Node Protocol: gRPC + mTLS

All cross-region node-to-node communication uses gRPC with mutual TLS. Protobuf defines all contracts.

**Key properties:**
- mTLS: only registered, certificate-holding nodes may communicate
- Revoked node certificates propagated via CRL gossip
- Protobuf versioning: field deprecation + reserved IDs from v0.1
- REST: health, metrics, operator tooling only

**Delivery semantics:** At-least-once. UUID v7 idempotency key; dedup TTL per profile.

**Retry / backoff:** Exponential 1s → 2s → 4s → … → max 5 min. DLQ per node after 10 failed attempts.

**Ordering:** Per-sender ordering preserved within a session (sequence number in envelope header). Cross-sender ordering not guaranteed.

---

## ADR-004 — Identity Model

Federated addressing: `user@region-domain` (e.g. `boza@rs.mrmi.net`).

Directory stores **only** minimum cryptographic identity — no PII federated:
- Ed25519 public key
- Routing endpoint (gRPC address of regional gateway node)
- Region identifier
- Key expiry timestamp

**Trust tiers (ADR-008):**

| Tier | Meaning |
|------|---------|
| T0 — Anonymous | Key exists, no claims |
| T1 — Node-verified | Regional operator confirms key ownership |
| T2 — Claim-signed | Signed operator attribute claims |
| T3 — Cross-verified | Another T2 node co-signs |

> Gateway does not verify real-world identity. Tier assignment is the regional node operator's responsibility.

---

## ADR-005 — Policy Engine + Configuration Profiles

Each node runs a local Policy Engine — no network calls, no central policy server. Policies are TOML-declared, signed by the node operator's private key.

**Configuration profiles:**

| Parameter | `strict` | `balanced` | `performance` |
|-----------|----------|------------|---------------|
| Payload padding | 16 KB | 4 KB | Disabled |
| Timing jitter | 0–500 ms | 0–100 ms | Disabled |
| Dummy traffic | 1 msg/5s/peer | 1 msg/60s/peer | Disabled |
| Dedup TTL | 72 h | 24 h | 1 h |
| Compliance mapping | 152-ФЗ high / GDPR Art.25 | Standard | Dev/test only |

**Merkle audit log:**
- Every policy decision is an immutable SHA-256 chained log entry
- Root hash published to DNS TXT at configurable interval
- Cross-node hash gossip for cross-verifiable audit trail

---

## ADR-006 — Deployment Topology

Initial model: one regional node per country. Single node serves one jurisdiction.

```
REGION: RU                   REGION: RS
  MRMI Node ◄── gRPC/mTLS ──► MRMI Node
  152-ФЗ                       GDPR
```

> **Expanded to three-tier model in v0.8 (Regional / Alliance / Global).**

---

## ADR-007 — Delivery Semantics

- At-least-once across regional boundaries
- Idempotency: UUID v7 per envelope; TTL per profile
- Per-sender ordering within session; no global ordering
- Retry: exponential backoff → DLQ after 10 failures
- Receiving node ACKs envelope receipt; application-level ACK out of scope

---

## ADR-009 — SDK-First Strategy

| SDK | Target | Release |
|-----|--------|---------|
| Go | Gateway core companion | v0.1 — ships with core |
| .NET / C# | Primary author stack; enterprise reference | v0.1 |
| Java / Kotlin | JVM enterprise | v0.2 |
| Python | Scripting / ML pipeline | v0.2 |

---

## 1. Context — Initial Problem Statement

Modern messaging apps operate across a fragmented regulatory landscape. Russia's **152-ФЗ**, the EU's **GDPR**, and Kazakhstan's data localisation laws impose strict requirements on where user data may reside and how it may cross borders.

Existing federated protocols (Matrix, XMPP) treat cross-border compliance as an implementation concern — leaving operators to figure it out on their own and risk misconfiguring it.

**Core insight:** Legal compliance is not a deployment concern. It must be enforced at the transport layer — not bolted on afterwards.

### Target Use-Case — v0.1

Primary corridor: **RU · BY · KZ · AM · RS** — regions with similar data sovereignty concerns and no current cross-border protocol enforcement. EU/US corridors deferred to v1.0.

### v0.1 Scope

Federation gateway core, regional node (Go), policy engine with compliance profiles, Merkle audit log + DNS TXT, signed policy configs, identity resolution (T0–T3) + revocation (ADR-010), at-least-once delivery + DLQ + retry, .NET SDK v0.1, Go CLI client.

---

## 7. Bootstrap Strategy

### Phase 1 — Seed Corridor (v0.1)
- Author: seed node **RS** (Belgrade) — scope: global (neutral bootstrap node)
- Community volunteer: seed node **RU** — scope: regional
- Discovery: DNS SRV — `_mrmi._tcp.rs.mrmi.net`
- Two nodes demonstrate end-to-end protocol operation

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| — | **v0.1** (this file) | [v0.2](MRMI_Gateway_ADR_v0_2.md) |

*Full cumulative ADR state first documented in [v0.5](MRMI_Gateway_ADR_v0_5.md).*
