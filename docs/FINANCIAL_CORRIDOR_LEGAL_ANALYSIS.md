# MRMI Gateway — Financial Corridor Legal Analysis
## ISO 20022 Adapter: Jurisdiction Classification for RU, RS, BY

**Document status:** Pre-launch preliminary analysis — v0.2 gate  
**Prepared by:** Božidar Tadić (architecture) — requires qualified legal counsel review before v0.2 launch  
**Scope:** Corridors RS→RU, RU→RS, RS→BY, BY→RS  
**Reference:** ADR-016, `docs/MRMI_Gateway_ADR_v0_9.md §ADR-016`

> **Disclaimer.** This document is a technical and regulatory framing prepared by the engineering team based on published law. It is not a legal opinion. No corridor should be activated in production without a formal opinion from qualified counsel in each jurisdiction.

---

## 1. Architecture Characterisation

Before analysing each jurisdiction the operative facts must be fixed, because the legal classification turns entirely on what MRMI does and does not do.

| Fact | MRMI behaviour |
|---|---|
| Funds held | **No.** MRMI holds no fiat currency, no e-money, no crypto. |
| Accounts maintained | **No.** MRMI maintains no account numbers, balances, or ledgers. |
| Settlement execution | **No.** MRMI routes a message; settlement is executed exclusively within the counterparty core-banking or payment system. |
| Payment initiation | **No.** MRMI does not create, authorise, or submit payment instructions. It transports instructions already authorised by the originating system. |
| Value transformation | **No.** Payloads are treated as opaque byte sequences by the transport layer. MRMI applies no FX conversion, no netting, no clearing. |
| Counterparty access | **No.** MRMI does not have access to sender or recipient accounts. |
| Message format awareness | **Partial.** The `iso20022` adapter identifies the schema type for policy purposes (dedup TTL, cutoff windows, audit retention). Payload content is never inspected. |

The safe-harbour argument in all three jurisdictions rests on the same foundation: **MRMI is a sovereignty-preserving message transport, structurally analogous to a SWIFT service bureau or a licensed clearing network's technical gateway**. It routes signed, encrypted envelopes between nodes operated by licensed financial institutions. The payment system is operated by the institutions at each end; MRMI is the pipe between them.

---

## 2. Russia (RU Corridor)

### 2.1 Governing Law

| Instrument | Relevance |
|---|---|
| Federal Law No. 161-FZ "On the National Payment System" (2011, as amended) | Defines payment system operators (PSO), payment infrastructure service operators (PISO), payment service providers. |
| Bank of Russia Regulation 383-P | Transfer of funds — rules applicable to payment service providers. |
| Bank of Russia Regulation 719-P / 747-P | Reporting and operational requirements for payment infrastructure. |
| Federal Law No. 152-FZ "On Personal Data" | Data localisation requirements for personal data of Russian citizens. |
| Federal Law No. 149-FZ "On Information, Information Technologies and Information Security" | Cross-border data transmission framework. |

### 2.2 Classification Analysis

**Is MRMI a payment system operator (PSO) under 161-FZ?**

A PSO under Article 15 of 161-FZ is an organisation that determines the rules of a payment system and bears responsibility for its operation. MRMI Gateway's protocol is open and each operator independently determines how their node participates. There is no central rules-setting authority, no central counterparty, and no central settlement mechanism. Each corridor is a bilateral agreement between two operators. This falls outside the statutory definition of a PSO.

**Is MRMI a payment infrastructure service operator (PISO) under 161-FZ?**

A PISO provides operational, clearing, or settlement services within a payment system. MRMI provides none of these: it does not clear positions, maintain exposure, or execute settlement. It routes cryptographically signed message envelopes. The closest analogy is a licensed technology provider to a payment system — a category that does not require PISO registration under 161-FZ.

**Data localisation risk (152-FZ)**

If ISO 20022 messages routed via MRMI contain personal data of Russian citizens (e.g. payer/payee names in `pmt.Cdtr.Nm`), 152-FZ applies. The MRMI node serving the RU leg of a corridor must be physically located in Russia to comply with the primary-copy localisation requirement. Operators must implement payload-level data residency controls — MRMI's `NodeScope=regional` and `jurisdiction_isolation` policy provide the enforcement mechanism, but the operator is responsible for ensuring payload data never transits a non-RU node.

**SPFS considerations**

The Russian Financial Messaging System (SPFS) is the Bank of Russia-operated alternative to SWIFT for domestic and some cross-border transactions. MRMI does not compete with SPFS; it is a transport layer above or alongside SPFS connectivity. Operators using MRMI to route ISO 20022 messages to Russian counterparties should confirm with Bank of Russia that their specific deployment model does not require SPFS participation.

### 2.3 Required Operator Disclosures (RU)

1. Terms of Service must explicitly state that MRMI does not provide payment services, does not hold funds, and is not a PSO or PISO.
2. If payload data contains personal data of Russian citizens, the operator must confirm data is processed and stored exclusively on Russian-territory infrastructure.
3. Operators should maintain a legal opinion confirming their specific deployment does not trigger 161-FZ registration requirements.
4. If the corridor carries messages that may be interpreted as cross-border fund transfers, the operator must hold, or confirm its counterparty holds, the relevant FX or remittance licences.

### 2.4 Go/No-Go Recommendation

**CONDITIONAL GO**

The RU corridor does not require a PSO or PISO licence for MRMI as a transport layer, provided:
- The RU node is physically located in Russia (152-FZ compliance).
- Operators confirm no personal data is routed via non-RU nodes.
- A qualified Russian financial law practitioner confirms the deployment model does not trigger 161-FZ obligations.
- SPFS participation requirements are clarified with Bank of Russia if the use case intersects with domestic payment instructions.

The engineering architecture (regional node scope, `jurisdiction_isolation`, `retain_long` audit) already supports the data sovereignty requirements. The legal gate is an operator obligation, not an architecture gap.

---

## 3. Serbia (RS Corridor)

### 3.1 Governing Law

| Instrument | Relevance |
|---|---|
| Law on Payment Services ("Zakon o platnim uslugama", 2018, amended 2023) | Serbia's PSD2-aligned framework. Defines payment service providers, payment systems, and technical service providers. |
| Law on the National Payment Card Scheme | Separate instrument for card schemes; not applicable to ISO 20022 message routing. |
| National Bank of Serbia (NBS) Decision on Payment System Operations | Operational requirements for payment systems. |
| Law on Electronic Document, Electronic Identification and Trust Services | Relevant to digital signature requirements on financial messages. |

### 3.2 Classification Analysis

**Is MRMI a payment service provider under Serbian law?**

Serbia's Payment Services Law follows PSD2 closely. Under Article 3 of the Law, payment services include: credit transfers, direct debits, payment card services, money remittance, and payment initiation services. Technical service providers — those providing "technical services that support the provision of payment services, including processing, storage, and security" — are explicitly excluded from the payment service provider definition (Article 4, paragraph 2, item 6, analogous to PSD2 Recital 19 and Article 3(j)).

MRMI is a technical service provider by this definition. It provides cryptographic message transport, policy-enforced routing, and tamper-evident audit logging — all technical services supporting payment systems operated by licensed institutions. MRMI does not have access to funds, does not process payment orders in the regulatory sense, and does not hold any payment accounts.

**Is MRMI a payment system under Serbian law?**

A payment system under the Law is a formal system with standard rules and procedures for processing payment transactions between participants, typically with central settlement. MRMI has no central settlement, no netting, and no formal membership structure. It is a bilateral message routing protocol. It does not meet the definition of a payment system.

**ISO 20022 context**

The National Bank of Serbia operates an Instant Payment System (IPS) based on ISO 20022. Serbian financial institutions are therefore familiar with the standard, and the regulatory environment is more mature in interpreting ISO 20022 infrastructure roles. MRMI's positioning as infrastructure above the IPS layer (routing messages between institutions that then process them through IPS or bilateral settlement) is well-precedented.

### 3.3 Required Operator Disclosures (RS)

1. Terms of Service must state that MRMI is a technical service provider and does not provide payment services as defined under the Law on Payment Services.
2. Operators routing ISO 20022 messages through a Serbian node should be prepared to demonstrate to NBS upon request that their node performs no settlement, clearing, or payment initiation.
3. Digital signature requirements: ISO 20022 messages signed by MRMI's Ed25519 node identity are transport signatures, not qualified electronic signatures under Serbian e-signature law. Operators must ensure the business-level message authentication requirements of their counterparties are met separately.
4. Audit logs (`retain_long=true`, ≥7 year retention) align with Serbian archiving requirements for financial records.

### 3.4 Go/No-Go Recommendation

**GO**

The RS corridor can proceed. Serbian law (PSD2-aligned) explicitly excludes technical service providers from payment service provider licensing requirements. The safe-harbour argument is well-grounded in the text of the Law on Payment Services and in EU regulatory precedent that Serbia has adopted.

Recommended pre-launch step: obtain a confirmatory legal opinion from a Serbian financial law practitioner and notify NBS of the intended deployment model as a courtesy — NBS has a track record of proactive engagement with fintech infrastructure providers.

---

## 4. Belarus (BY Corridor)

### 4.1 Governing Law

| Instrument | Relevance |
|---|---|
| Law of the Republic of Belarus No. 236-Z "On Payment Systems and Payment Services" (2014, as amended) | Defines payment systems, payment service operators, and payment service providers. |
| National Bank of the Republic of Belarus (NBRB) Resolution No. 201 | Licensing requirements for payment service operators. |
| Law No. 99-Z "On Currency Regulation and Currency Control" | Cross-border payment restrictions and foreign currency transaction requirements. |
| Resolution of the Council of Ministers No. 851 | Data processing and cross-border data transfer requirements. |
| Decree No. 8 "On the Development of the Digital Economy" (Hi-Tech Park regulation) | Relevant for technology companies operating in BY; may provide a beneficial regulatory environment. |

### 4.2 Classification Analysis

**Is MRMI a payment system operator under 236-Z?**

Under Article 2 of Law 236-Z, a payment system operator is an entity that "determines the rules of the payment system, ensures their implementation, and bears responsibility for its operation." The analysis for BY parallels RU: MRMI has no central rules-setter, no settlement function, and no central counterparty. Each deployment is a bilateral operator agreement. The statutory definition is not met.

However, Belarusian law is more prescriptive and less well-interpreted in the context of cross-border fintech infrastructure than either Russian or Serbian law. The NBRB has historically required formal consultation for novel payment infrastructure models. MRMI's architecture — decentralised, bilateral, settlement-free — has no clear domestic precedent in Belarus.

**Currency regulation risk (99-Z)**

Unlike Russia and Serbia, Belarus maintains tighter currency regulation under Law 99-Z. Cross-border transmission of payment instructions (even without settlement) between Belarusian and foreign entities may require currency control compliance steps by the licensed banking partners involved. MRMI as transport layer does not itself perform currency transactions, but operators must verify their use case does not create compliance obligations under 99-Z for their customers.

**Sanctions exposure**

The BY corridor carries the highest geopolitical risk of the three jurisdictions. Operators must conduct a full OFAC, EU, and SECO sanctions screening before activating the BY corridor. MRMI's architecture is sanctions-neutral (it does not screen payloads), which means sanctions compliance is entirely the operator's responsibility. This must be stated clearly in operator agreements. **This is not an architecture gap — it is an operator obligation that cannot be delegated to MRMI.**

**HTP (Hi-Tech Park) angle**

Technology companies registered in Belarus's Hi-Tech Park (HTP) benefit from preferential regulation under Decree No. 8. If the operator is an HTP resident, the interpretation of MRMI as a "software technology" rather than a "payment service" is better supported. Operators should explore HTP registration if they are not already residents.

### 4.3 Required Operator Disclosures (BY)

1. Terms of Service must disclaim payment service provision and explicitly assign sanctions compliance obligations to the operator/customer.
2. Operators must obtain NBRB consultation or formal legal opinion before routing ISO 20022 messages through a BY node.
3. A full sanctions screening programme must be in place and documented before the BY corridor is activated.
4. If the operator is not an HTP resident, explore whether HTP registration is viable — it strengthens the technical-service-provider classification.
5. Audit log retention (`retain_long=true`) must be maintained on BY-territory infrastructure to meet local data processing requirements.

### 4.4 Go/No-Go Recommendation

**CONDITIONAL GO — higher gate than RU**

The BY corridor should not be activated without:
1. A formal legal opinion from a Belarusian financial law practitioner confirming MRMI does not trigger payment system operator registration under 236-Z.
2. Full OFAC/EU/SECO sanctions compliance programme documented and auditable.
3. NBRB proactive disclosure of the deployment model.
4. Confirmation from any Belarusian banking partners that their use of MRMI does not create currency regulation obligations under 99-Z.

The engineering architecture supports BY deployment (regional node scope, jurisdiction isolation, full Merkle audit). The legal gate is higher here than for RU and RS due to the less-mature regulatory interpretation environment and sanctions exposure.

---

## 5. Cross-Cutting Safe-Harbour Arguments

These arguments apply across all three jurisdictions and should be included in any legal opinion request:

| Argument | Basis |
|---|---|
| No funds held | MRMI operates at the message layer only. No fiat, e-money, or crypto at any point. |
| No accounts | No account numbers, balances, or ledgers maintained anywhere in the MRMI architecture. |
| No settlement | Settlement is executed exclusively within the counterparty systems. MRMI provides the transport, not the rails. |
| No payment initiation | Messages are already authorised by the originating system before entering MRMI. MRMI does not generate or authorise payment instructions. |
| No value transformation | No FX conversion, netting, or clearing occurs at any MRMI node. |
| Bilateral architecture | No central intermediary, no central rules-setter, no central counterparty. Each corridor is an independent bilateral agreement between two licensed operators. |
| Operator sovereignty | Each operator controls their own node. MRMI provides the protocol, not a centralised service. |
| Tamper-evident audit | Merkle chain audit log with SHA-256 hashing provides regulator-accessible evidence of all routing decisions. |
| Ed25519 node identity | All messages carry cryptographic proof of routing path. No anonymised routing that could mask regulatory evasion. |

---

## 6. Compliance Note Update (ADR-016)

The existing ADR-016 compliance note is reproduced here for reference:

> *The `iso20022` adapter provides technical infrastructure for financial message routing. It does not constitute a payment system, a banking licence, or a money transmission service. Operators must obtain all applicable regulatory approvals. Legal accountability rests entirely with the operator.*

**Recommended addition** — include the following sentence to reference this analysis:

> *A jurisdiction-by-jurisdiction legal framing for the RU, RS, and BY corridors is maintained in `docs/FINANCIAL_CORRIDOR_LEGAL_ANALYSIS.md`. Go/no-go recommendations: RS corridor — GO; RU corridor — CONDITIONAL GO; BY corridor — CONDITIONAL GO (higher gate).*

---

## 7. Summary — Go/No-Go Matrix

| Corridor | Recommendation | Gate condition |
|---|---|---|
| **RS (Serbia)** | **GO** | Confirmatory legal opinion recommended; no licence required. |
| **RU (Russia)** | **CONDITIONAL GO** | Legal opinion + data localisation confirmation + SPFS clarification required. |
| **BY (Belarus)** | **CONDITIONAL GO** | Legal opinion + NBRB disclosure + sanctions programme + currency regulation check required. |

All three corridors may proceed to technical testing. **No corridor should be activated in production until the stated gate conditions are met and documented.**

---

*Last updated: 2026-06-02. Review before any v0.2 production activation.*
