# MRMI Gateway — Architecture Decision Record v0.4

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.5                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.4 — Bootstrap strategy, operator incentives, ADR-011 HA split completed |
| Supersedes | v0.3                                                                     |
| Covers App | v0.1 (target)                                                            |
| Created    | 2025                                                                     |

---

## What was decided in v0.4

---

## §7 — Bootstrap Strategy (finalised)

### Phase 1 — Seed Corridor (v0.1)
- Author: seed node **RS** (Belgrade) — scope: global (neutral bootstrap node)
- Community volunteer: seed node **RU** — scope: regional
- Discovery: DNS SRV — `_mrmi._tcp.rs.mrmi.net`
- Two nodes demonstrate end-to-end protocol operation

### Phase 2 — Corridor Expansion (v0.2)
- Open operator registration: BY, KZ, AM
- BY/KZ/AM may join as individual regional nodes or form an alliance node (EAEU-compatible)
- RS node transitions from global → regional as corridor matures

### Phase 3 — Community Governance (v1.0)
- Protocol governance → foundation or community committee
- EU alliance node design begins
- RU↔EU corridor via RS as neutral global relay — separate legal analysis required

---

## §10 — Operator Incentives (introduced)

### Why Run a Node

- **Compliance risk reduction** — verifiable, auditable data localisation enforcement
- **Federation without shared infrastructure** — cross-border communication without trusting a third-party server
- **Compliance infrastructure positioning** — offer "compliant cross-border messaging" as a service
- **Governance participation** — T2+ nodes in good standing have a voice in protocol governance

### Cost of Running a Node

- **Hardware:** 2 vCPU, 4 GB RAM, 50 GB SSD — ~€20–50/month
- **Operational:** ~1–2 h/month for policy config, certificate rotation, log export
- **Legal:** operator is the accountable party under local law

### What It Does NOT Require

- No token purchase, staking, or cryptocurrency
- No mandatory fees to the protocol
- No permission from the project author
- No KYC or identity verification by the protocol

---

## ADR-011 — Horizontal Scaling & High Availability (completed)

HA topology finalised:

```
Load Balancer (gRPC-aware)
     │              │
MRMI-1          MRMI-2         ...
(routing)       (routing)
     └──────────────┘
        Shared State
        (Redis/KV)
        Dedup index
        DLQ
           │
      Merkle Log
      (append-only)
```

v0.1 reference deployment: single process + embedded KV (bbolt).

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.3](MRMI_Gateway_ADR_v0_3.md) | **v0.4** (this file) | [v0.5](MRMI_Gateway_ADR_v0_5.md) |
