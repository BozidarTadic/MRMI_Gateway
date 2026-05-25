# MRMI Gateway — Architecture Decision Record v0.7

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.8                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.7 — Versioned roadmap, Discovery → v0.2, Storage/Cache/UI → v0.3, hosted node model, ADR-014 Management API, ADR-013 Storage interface, milestones split by version |
| Supersedes | v0.6                                                                     |
| Covers App | v0.1 (target) · v0.2–v1.0 (roadmap)                                     |
| Created    | 2025                                                                     |

---

## What was decided in v0.7

---

## §13 — Versioned Roadmap (introduced)

Scope split across version milestones formalised:

| Version | Focus | Key deliverables |
|---------|-------|-----------------|
| **v0.1** | Core protocol | Routing, policy, audit, delivery, mTLS, revocation, read-only API, seed corridor RS↔RU |
| **v0.2** | User discovery | Federated lookup, opaque token, namespace isolation, auto-accept, push webhook, write API |
| **v0.3** | Operator infrastructure | Storage backends, cache layer, dynamic node discovery, dashboard UI |
| **v1.0** | Scale & governance | DHT routing, EU/US corridor, governance model, operator certification |

> **Principle:** Each version is independently deployable and useful.

Discovery formally moved from v0.1 → v0.2. Storage / Cache / Node Discovery / Dashboard UI moved to v0.3.

---

## §10.4 — Hosted Nodes (introduced)

For operators who want to join the corridor without managing their own infrastructure:

```
App registers on hosted node → receives:
  app_id    = "com.myapp"
  api_key   = "mrmi_live_..."
  node_url  = "rs-hosted-01.mrmi.net:7777"
```

**Target audience:** Startups testing federation, applications before committing to own node, small operators in uncovered regions.

Hosted nodes are a bootstrap mechanism — as the ecosystem matures, community nodes reduce dependency on author-operated infrastructure.

---

## ADR-013 — Storage & Cache (DEFERRED, v0.3)

Storage interface defined now to prevent v0.3 rearchitecting:

```go
type NodeStore interface {
    Deduped(key string, ttl time.Duration) (bool, error)
    DLQPush(entry DLQEntry) error
    DLQList() ([]DLQEntry, error)
    DLQDelete(id string) error
    AuditAppend(entry AuditEntry) error
    AuditLatest(n int) ([]AuditEntry, error)
    CRLPut(entry CRLEntry) error
    CRLGet(nodeID string) (*CRLEntry, error)
    CRLList() ([]CRLEntry, error)
}
```

Two implementations planned:
- `BboltStore` — v0.1 default, single binary, zero deps
- `RedisStore` — v0.3, horizontal scaling, native TTL

Switching backend requires **one TOML line change** only — no code changes, no data migration for dedup/DLQ.

---

## ADR-014 — Management API (introduced)

Read-only in v0.1, write in v0.2, UI-ready in v0.3.

### v0.1 Read-only endpoints

```
GET  /api/v1/status
GET  /api/v1/config
GET  /api/v1/audit/latest
GET  /api/v1/audit/verify
GET  /api/v1/dlq
GET  /api/v1/peers
GET  /api/v1/metrics
```

### v0.2 Write endpoints

```
POST /api/v1/peers/register
POST /api/v1/dlq/{id}/replay
POST /api/v1/dlq/{id}/discard
POST /api/v1/config/reload
POST /api/v1/revoke/{node_id}
```

### Auth model

```
v0.1: no auth (localhost-only binding recommended)
v0.2: api_key header — X-MRMI-Key: mrmi_op_...
v0.3: api_key or JWT; scoped tokens (read-only vs. operator)
```

**Design principle:** Management API is an operator tool, never exposed to SDK clients or end users.

---

## Milestones Split by Version (introduced)

Per-version milestone tables introduced (v0.1, v0.2, v0.3). Previously all milestones were in a single flat list. See [v0.8 ADR §12](MRMI_Gateway_ADR_v0_8.md#12-milestones-by-version) for the current milestone state.

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.6](MRMI_Gateway_ADR_v0_6.md) | **v0.7** (this file) | [v0.8](MRMI_Gateway_ADR_v0_8.md) |
