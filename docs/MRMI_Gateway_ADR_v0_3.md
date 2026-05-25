# MRMI Gateway — Architecture Decision Record v0.3

**Multi-Regional Multi-App Interlock**

---

| Field      | Value                                                                    |
|------------|--------------------------------------------------------------------------|
| ADR ID     | MRMI-ADR-001                                                             |
| Status     | SUPERSEDED by v0.4                                                       |
| Author     | Božidar Tadić                                                            |
| Version    | 0.3 — Metadata minimisation, Merkle audit log formalised, signed policy configs, configuration profiles with compliance mapping |
| Supersedes | v0.2                                                                     |
| Covers App | v0.1 (target)                                                            |
| Created    | 2025                                                                     |

---

## What was decided in v0.3

---

## ADR-005 — Policy Engine: Compliance Mapping Formalised

Configuration profiles explicitly mapped to regulatory frameworks:

| Profile | Compliance mapping |
|---------|--------------------|
| `strict` | GDPR Art.25 / 152-ФЗ high category |
| `balanced` | Standard operation |
| `performance` | Internal dev/test only |

> **Note:** Compliance mapping is indicative guidance, not legal certification. Operators must verify with legal counsel.

**Signed policy configs introduced:** Policies include an explicit `applicable_law` declaration (pre-formalised — formalised with `applicable_law` field in v0.5).

---

## ADR-005 — Merkle Audit Log (formalised)

Audit log entry structure formalised:

```
AuditEntry {
  seq:              uint64   // monotonic
  timestamp:        unix_ms
  decision:         ALLOW | DENY
  sender_region:    string
  recipient_region: string
  policy_version:   string
  profile:          strict | balanced | performance
  prev_hash:        sha256   // chain integrity
  entry_hash:       sha256   // this entry
}
```

**Three-layer evidence trail defined:**
1. Signed policy config — what rules were in effect
2. Merkle log — what decisions were made
3. DNS TXT root hash — externally verifiable proof log was not tampered with

---

## §4.2 — Metadata Minimisation (Honest Scope)

Even with encrypted payload, metadata constitutes sensitive data under GDPR. Three transport-level mitigations applied per profile:

- **Payload padding** — envelopes padded to fixed size bucket
- **Timing jitter** — randomised delay before forwarding
- **Dummy traffic** — synthetic envelopes when idle

**Known limits explicitly documented:**
1. Long-term graph analysis can reveal organisational relationships
2. Traffic volume correlation remains partially detectable
3. Timing correlation: jitter provides probabilistic, not absolute, protection

> MRMI Gateway is **not** a replacement for Tor. It provides metadata protection appropriate for **regulated business communication**.

---

## Navigation

| Previous | Current | Next |
|---|---|---|
| [v0.2](MRMI_Gateway_ADR_v0_2.md) | **v0.3** (this file) | [v0.4](MRMI_Gateway_ADR_v0_4.md) |
