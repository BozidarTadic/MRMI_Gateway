# MRMI Gateway — ADR Index

Architecture Decision Records for the MRMI Gateway protocol, one file per version.

Each file documents the decisions **introduced** in that version. The v0.8 file is the first full cumulative document; v0.5 is an earlier full snapshot.

---

| Version | Title | Status | Key decisions |
|---|---|---|---|
| [v0.1](MRMI_Gateway_ADR_v0_1.md) | Initial Protocol | Superseded | ADR-001–009, routing, delivery, identity tiers, Merkle log, compliance profiles, bootstrap |
| [v0.2](MRMI_Gateway_ADR_v0_2.md) | Delivery & Revocation | Superseded | ADR-007 refined, ADR-008 formalised, ADR-010 trust revocation, ADR-011 HA initiated |
| [v0.3](MRMI_Gateway_ADR_v0_3.md) | Metadata & Audit | Superseded | ADR-005 compliance mapping, Merkle audit log formalised, metadata minimisation honest limits |
| [v0.4](MRMI_Gateway_ADR_v0_4.md) | Bootstrap & Incentives | Superseded | Bootstrap strategy finalised, operator incentives (§10), ADR-011 HA completed |
| [v0.5](MRMI_Gateway_ADR_v0_5.md) | Acceptance Criteria | Superseded | §11 acceptance criteria, sequence diagrams, `applicable_law` field, HTTPS fallback, compliance checklist |
| [v0.6](MRMI_Gateway_ADR_v0_6.md) | Data Transit Policy | Superseded | §4.3 Data Transit Policy, ADR-012 Federated Discovery (proposed), opaque_token model |
| [v0.7](MRMI_Gateway_ADR_v0_7.md) | Versioned Roadmap | Superseded | §13 roadmap, hosted nodes (§10.4), ADR-013 Storage, ADR-014 Management API, milestones split |
| [v0.8](MRMI_Gateway_ADR_v0_8.md) | Node Tier Model | Accepted | Three-tier topology (Regional/Alliance/Global), `node_scope` field, ADR-006 expanded |
| [v0.9](MRMI_Gateway_ADR_v0_9.md) | Universal Federation Protocol | Proposed | ADR-015 Schema Registry, §1.3 Protocol Specification, ADR-005 `jurisdiction_isolation`, ADR-016 ISO 20022 |

---

## Full cumulative documents

- [v0.5](MRMI_Gateway_ADR_v0_5.md) — First full snapshot (also: [ADR_v0.5.md](ADR_v0.5.md))
- [v0.8](MRMI_Gateway_ADR_v0_8.md) — Current accepted baseline (full document)
- [v0.9](MRMI_Gateway_ADR_v0_9.md) — Latest proposed additions
- [MRMI_Gateway_ADR_v0_9.pdf](MRMI_Gateway_ADR_v0_9.pdf) — Original PDF source for v0.9
