# MRMI Gateway Project Plan

> **Status: Complete** — All Sprint 1 tasks delivered. See Sprint 2 plan for next steps.

This document breaks the project into implementation areas and defines the first sprint plan for moving the v0.1 gateway toward a usable local two-node corridor.

## Project Areas

### 1. Core Gateway

- gRPC server and client
- Envelope send flow
- Routing between nodes
- Graceful shutdown

### 2. Policy and Compliance

- Region allow/deny rules
- Trust tier checks
- Compliance profiles: `strict`, `balanced`, `performance`
- Future policy hot-reload

### 3. Delivery Reliability

- Idempotency and deduplication
- Retry behavior
- Dead-letter queue for failed messages
- ACK handling

### 4. Security

- mTLS between nodes
- Certificate validation
- Revocation and CRL handling
- Blacklist gossip

### 5. Audit

- Merkle audit log
- `/.well-known/mrmi-audit`
- DNS TXT publisher
- Signed audit response

### 6. Testing and Operations

- Unit tests
- Integration tests
- Two-node RS/RU test environment
- Operator documentation

### 7. SDKs and Clients

- .NET SDK
- CLI reference client
- Later Java and Python SDKs

## Sprint 1 Plan

### Goal

Finish the minimum usable Go gateway path: receive an envelope, enforce policy, deduplicate by idempotency key, write audit evidence, and expose health/audit endpoints.

### Tasks

1. Confirm the current baseline.
   - Run all Go tests.
   - Verify the gateway starts from `configs/node.balanced.toml`.
   - Check `/healthz`, `/readyz`, and `/.well-known/mrmi-audit`.

2. Finish dedup integration.
   - Confirm the existing dedup index is correctly wired into gRPC `SendEnvelope`.
   - Add tests for duplicate gRPC sends using the same idempotency key.

3. Audit every decision.
   - Ensure allow, deny, and duplicate decisions are written to the audit log.
   - Confirm the send path returns the current audit root after each decision.

4. Add policy enforcement tests.
   - Test allowed region delivery.
   - Test denied region delivery.
   - Test minimum trust tier rejection.
   - Test duplicate envelope behavior.

5. Add a basic two-node local integration test.
   - Add RS and RU local configs with different ports.
   - Start both nodes locally.
   - Send an envelope from RS to RU.
   - Verify response, audit root, and no duplicate delivery on replay.

6. Update Sprint 1 documentation.
   - Add a short local two-node test guide.
   - Document current limitations: no mTLS, no DNS TXT publishing, and no DLQ yet.

## Sprint 1 Definition of Done

- `go test ./...` passes.
- Gateway starts locally.
- Health and audit endpoints work.
- gRPC `SendEnvelope` works.
- Duplicate idempotency key returns a duplicate response.
- Denied policy envelope is rejected.
- Audit root changes after policy decisions.
- Basic local RS/RU test is documented.

## Explicitly Out of Sprint 1

- Full mTLS
- CRL gossip
- DNS TXT publisher
- .NET SDK
- DLQ/retry system
- 24-hour performance acceptance tests

These items should move into Sprint 2 and later once the core send, audit, policy, and dedup path is stable.
