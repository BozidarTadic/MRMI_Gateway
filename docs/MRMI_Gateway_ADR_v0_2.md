# MRMI Gateway — Architecture Decision Record v0.2

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.3                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.2 — Delivery semantics, idempotency model, identity trust tiers, trust revocation |
| Supersedes | v0.1                                                                     |
| Covers App | v0.1 (target)                                                            |
| Created    | 2025                                                                     |

---

## What was decided in v0.2

Decisions introduced or refined between v0.1 and v0.2.

---

## ADR-007 — Delivery Semantics (refined)

**Dedup TTL clarified per profile:**
- `strict = 72h` — covers delayed delivery scenarios
- `balanced = 24h` — default for standard corridors
- `performance = 1h` — dev/test only

Configurable per-operator on top of profile preset.

---

## ADR-008 — Trust Tiers (formalised)

Trust tier model formalised with explicit definitions:

| Tier | Meaning | Mechanism |
|------|---------|-----------|
| T0 — Anonymous | Key exists, no claims | Default for new identities |
| T1 — Node-verified | Regional operator confirms key ownership | Challenge-response at registration |
| T2 — Claim-signed | Signed operator attribute claims | Operator signs claims |
| T3 — Cross-verified | Another T2 node co-signs | Web-of-Trust inter-corridor trust |

**Important:** Gateway does not verify real-world identity. Tier assignment is the node operator's legal responsibility.

---

## ADR-010 — Trust Revocation

**Mechanisms introduced:**

- **CRL (Certificate Revocation List):** each node maintains a signed CRL, propagated to peers via gossip on update
- **Node blacklist:** requires ≥2 independent T2+ nodes to corroborate before taking effect — prevents single-node censorship
- **Trust decay:** no cross-validation within 30 days → effective trust tier auto-reduced by one level
- **Key rotation:** operators must publish new Ed25519 key before expiry; failure → automatic T0 downgrade

**Anti-censorship guard:** A single node cannot blacklist another unilaterally. Minimum ≥2 independent T2+ signatures required.

---

## ADR-011 — Horizontal Scaling & High Availability (initiated)

Stateless/stateful component split defined:

| Component | Stateless? | Scaling Strategy |
|-----------|-----------|-----------------|
| Routing engine | ✅ Yes | Horizontal: multiple instances behind load balancer |
| Policy engine | ✅ Yes | Horizontal: config loaded at startup |
| Dedup index | ❌ No | Shared Redis / embedded KV |
| DLQ | ❌ No | Single writer per region; replicated for HA |
| Merkle audit log | ❌ No | Append-only; single writer |

v0.1 reference deployment: single process + embedded KV (bbolt). v0.2: Redis adapter; multiple routing instances.

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.1](MRMI_Gateway_ADR_v0_1.md) | **v0.2** (this file) | [v0.3](MRMI_Gateway_ADR_v0_3.md) |
