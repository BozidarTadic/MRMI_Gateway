# MRMI Gateway — Architecture Decision Record v0.5

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.6                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.5 — Acceptance criteria, sequence diagrams, full TOML examples, `applicable_law` field, HTTPS fallback for DNS TXT, operator compliance checklist |
| Supersedes | v0.4                                                                     |
| Covers App | v0.1 (target)                                                            |
| Created    | 2025                                                                     |

---

## What was decided in v0.5

---

## §11 — Acceptance Criteria (introduced)

Concrete, measurable success criteria for the v0.1 milestone. Key targets:

**Delivery**
- Message delivery success rate ≥ 99.5% over 1,000 msg/hour for 24 hours
- Zero duplicate deliveries during 24h test with intentional replay injection
- DLQ population within 5 min of 10th retry failure

**Policy engine**
- ≥ 100,000 decisions/second on 2 vCPU reference hardware
- Hot-reload applied within 5 seconds without restart
- 100% of deny-policy envelopes rejected; 0 false positives

**Audit & revocation**
- 100% Merkle chain verification passes after 24h test run
- Blacklisted node rejected by all corridor peers within 60 seconds of quorum

**Management API**
- `/api/v1/status` returns within 200 ms
- `/api/v1/audit/latest` returns last 10 Merkle entries

---

## `applicable_law` field (formalised)

Operators now explicitly declare applicable legal framework in signed TOML config:

```toml
[node]
applicable_law = "RU-152FZ"   # or "RS-GDPR", "BY-PDPA", etc.
```

This field appears in every audit entry — creating an auditable declaration of the operator's legal intent.

---

## ADR-005 — HTTPS Fallback for DNS TXT (introduced)

`GET /.well-known/mrmi-audit` — JSON endpoint exposing:
```json
{
  "version": 1,
  "timestamp": 1719000000,
  "root_hash": "sha256:AbCd1234...",
  "node_id": "rs.mrmi.net",
  "applicable_law": "RS-GDPR",
  "signature": "ed25519:XyZ..."
}
```

Used when DNS TXT is unavailable or potentially compromised. Removes single point of failure for audit verification.

---

## Appendix A — Full TOML Examples (introduced)

Complete, copy-paste-ready configurations added for all three profiles (`strict`, `balanced`, `performance`) and the alliance node case. See [v0.8 ADR Appendix A](MRMI_Gateway_ADR_v0_8.md#appendix-a--full-toml-configuration-examples) for the current versions.

---

## Appendix B — Operator Compliance Checklist (introduced)

Step-by-step checklist covering:
- Legal preparation (identify applicable law, confirm legal entity, consult counsel)
- Technical preparation (hardware, key generation, DNS, cert rotation)
- Operational readiness (policy DENY/ALLOW tests, Merkle verification, DLQ test)
- Ongoing operations (daily log monitoring, annual key rotation)

See [v0.8 ADR Appendix B](MRMI_Gateway_ADR_v0_8.md#appendix-b--operator-compliance-checklist) for the current checklist.

---

## Sequence Diagrams (introduced)

ASCII sequence diagrams added for:
1. Message delivery flow (App A → Node RU → Node RS → App B)
2. Revocation flow (blacklist publish → quorum → CRL update → peer rejection)
3. Audit verification flow (Auditor → DNS/HTTPS → Merkle log)

See [v0.8 ADR](MRMI_Gateway_ADR_v0_8.md) for current diagram versions.

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.4](MRMI_Gateway_ADR_v0_4.md) | **v0.5** (this file) | [v0.6](MRMI_Gateway_ADR_v0_6.md) |

*Also available as the original full-document snapshot: [ADR_v0.5.md](ADR_v0.5.md)*
