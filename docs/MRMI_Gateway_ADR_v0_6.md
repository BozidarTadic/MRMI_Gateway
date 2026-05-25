# MRMI Gateway — Architecture Decision Record v0.6

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.7                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.6 — Data Transit Policy (§4.3), Federated User Discovery (ADR-012), transit cache conditions, opaque_token model |
| Supersedes | v0.5                                                                     |
| Covers App | v0.1 (target) · v0.2 (discovery)                                        |
| Created    | 2025                                                                     |

---

## What was decided in v0.6

---

## §4.3 — Data Transit Policy (NEW)

### Core distinction

| Concept | Definition | MRMI position |
|---------|-----------|---------------|
| **Data-in-transit** | Data actively moving through a node en route to destination | **Permitted** through any corridor node |
| **Data-at-rest** | Data stored persistently in a node's jurisdiction | **Prohibited** outside the user's origin region |
| **Transit cache** | Short-lived encrypted buffer held by a routing node | **Permitted** under strict conditions |

**Analogy:** A letter passing through a foreign postal hub is transit — legally and architecturally acceptable. A postal hub retaining a copy of the letter is storage — a violation of data sovereignty.

### Transit cache — permitted conditions (ALL must be met)

1. **TTL is strictly enforced** — maximum 60 seconds; entry deleted after TTL with no recovery
2. **Payload is opaque to the transit node** — E2E encrypted blob; node cannot read it
3. **Audit trail records transit** — logs `{ origin_region, destination_region, transit_node_id, timestamp_in, timestamp_out_or_evicted }`
4. **No persistence beyond TTL** — in-memory only; not written to disk; not replicated
5. **Transit node cannot decrypt** — enforced by application-layer E2E encryption

### TOML configuration

```toml
[transit]
cache_enabled  = true
cache_ttl_s    = 60    # max: 60; operator may reduce, not increase
audit_transit  = true
```

`strict` profile enforces `cache_ttl_s = 30` and `audit_transit = true` with no operator override above these values.

### Legal framing

MRMI Gateway's transit model is designed to qualify as **"transmission of opaque encrypted data"** rather than **"processing of personal data"** — because the transit node never accesses plaintext and holds no data after TTL expiry.

> **⚠️ This framing is architectural guidance, not legal certification.** Operators must verify with legal counsel.

---

## ADR-012 — Federated User Discovery (PROPOSED, v0.2)

**Status:** PROPOSED — scheduled for v0.2. Proto fields reserved in v0.1 schemas.

### Problem

A user knows a contact's identifier but does not know which region or application they're registered in. Cross-regional lookup is needed without a centralised identity registry.

### Design decision

**Discovery is app-driven, not node-driven.** Nodes broadcast the request; each connected application decides independently whether and how to respond.

### Discovery flow

```
User → SDK → Local Node
             ↓
    Broadcast DiscoveryRequest to peer nodes (hop_limit enforced)
             ↓
    Each peer node forwards to its connected Apps
             ↓
    Each App evaluates its own discovery policy → responds or SILENT
             ↓
    Origin node aggregates responses → list returned to user
             ↓
    User selects → SDK sends ConnectRequest using opaque_token
             ↓
    Remote app resolves token → real user_id (never exposed)
             ↓
    Connection established. Token expires immediately.
```

### Privacy guarantee

- **Node layer never sees plaintext identifier** — only SHA-256 hash reaches peer nodes
- **`opaque_token`** maps to real `user_id` only inside the App's own database
- Token exposed only to requesting user; expires after handshake or 5-minute TTL

### User visibility levels

| Level | Behaviour |
|-------|-----------|
| `DISCOVERABLE` | Visible to any node — app responds to all requests |
| `FRIENDS_ONLY` | Responds only if sender is in contact list |
| `HIDDEN` | Always SILENT — not discoverable |

### Auto-accept configuration (app-level)

| Mode | Behaviour |
|------|-----------|
| `MANUAL` | User must confirm every incoming connection (default) |
| `AUTO_WHITELIST` | Auto-accept if origin node is in operator's trusted list |
| `AUTO_MUTUAL` | Auto-accept if both parties initiated discovery within TTL |
| `AUTO_ALL` | Auto-accept all — for public bots/services |

### Scale path

| Phase | Mechanism |
|-------|-----------|
| v0.1 seed corridor | Broadcast, `hop_limit = 3` |
| v0.2 expansion | Broadcast, configurable `hop_limit` |
| v0.3+ DHT | Hash-based DHT; broadcast deprecated |

> `query_hash` field in `DiscoveryRequest` is designed as a direct DHT key for the future transition — no protocol change required when switching.

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.5](MRMI_Gateway_ADR_v0_5.md) | **v0.6** (this file) | [v0.7](MRMI_Gateway_ADR_v0_7.md) |
