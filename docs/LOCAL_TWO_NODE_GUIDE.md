# Local Two-Node Test Guide

This guide explains how to run an RS + RU corridor locally, verify both nodes are healthy, and exercise the full send / audit / dedup path.

## Prerequisites

- Go 1.21 or later
- `curl` (for HTTP endpoint checks)

No additional tools are required. The gRPC layer is tested through the Go test suite (see [Automated integration test](#automated-integration-test) below). A CLI client is planned for Sprint 2.

## Start both nodes

Open two terminals from the repository root.

**Terminal 1 — RS node (gRPC :7777, HTTP :8080)**

```bash
go run ./cmd/mrmi-gateway -config configs/node.rs.local.toml
```

**Terminal 2 — RU node (gRPC :7778, HTTP :8081)**

```bash
go run ./cmd/mrmi-gateway -config configs/node.ru.local.toml
```

Both nodes start with the `balanced` compliance profile. Startup is immediate; no external dependencies are required.

## Verify nodes are healthy

```bash
# RS node
curl http://localhost:8080/healthz          # → ok
curl http://localhost:8080/readyz           # → ready
curl http://localhost:8080/.well-known/mrmi-audit   # → JSON with root_hash

# RU node
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
curl http://localhost:8081/.well-known/mrmi-audit
```

The audit endpoint returns a JSON object:

```json
{
  "version": 1,
  "timestamp": 1746700000,
  "root_hash": "sha256:0000...0000",
  "node_id": "rs-node-01",
  "applicable_law": "RS-GDPR",
  "signature": "ed25519:REPLACE_ME"
}
```

`root_hash` starts as all-zeros (empty log). It changes after the first gRPC envelope is processed.

## Automated integration test

The `internal/integration` package spins up both nodes in-process on random ports and exercises the full send / dedup / audit path:

```bash
go test ./internal/integration/... -v
```

Expected output:

```
=== RUN   TestTwoNodeLocalCorridor
--- PASS: TestTwoNodeLocalCorridor
=== RUN   TestTwoNodeAuditRootsAreIndependent
--- PASS: TestTwoNodeAuditRootsAreIndependent
```

`TestTwoNodeLocalCorridor` verifies:

1. RS node accepts an RS→RU envelope and returns `ALLOW` with a non-zero audit root hash.
2. Replaying the same `idempotency_key` to RS returns `DUPLICATE` and advances the audit root (the duplicate is logged).
3. The same `idempotency_key` sent to the **RU** node returns `ALLOW` — each node maintains an independent dedup store.
4. A route not present in RU's allow-list (→US) returns `DENY`.

`TestTwoNodeAuditRootsAreIndependent` confirms that the two Merkle chains never share a root hash, and that each chain passes `Verify()`.

## Run the full test suite

```bash
go test ./...
```

All packages must pass before any changes are merged.

## Node config reference

| File | Region | gRPC | HTTP | Allows |
|---|---|---|---|---|
| `configs/node.rs.local.toml` | RS | `:7777` | `:8080` | RU, BY, KZ, AM |
| `configs/node.ru.local.toml` | RU | `:7778` | `:8081` | RS, BY, KZ, AM |
| `configs/node.balanced.toml` | RS | `:7777` | `:8080` | RU, BY, KZ, AM |

`node.balanced.toml` and `node.rs.local.toml` are equivalent. The `.local.toml` variants are the canonical configs for the two-node corridor scenario.

## Current limitations (Sprint 1)

The following features are out of scope for Sprint 1 and will be addressed in Sprint 2 and later.

### No mTLS between nodes

Inter-node gRPC currently uses an **insecure** connection (`grpc.WithInsecure()`). ADR-003 requires mutual TLS for all inter-node traffic. Until mTLS is implemented, do not expose node gRPC ports on a network boundary.

The `signed_by = "ed25519:REPLACE_ME"` value in the config files is a placeholder. Real deployments require a generated Ed25519 key pair and a valid certificate chain. The `/.well-known/mrmi-audit` endpoint returns `signed_by` as a plain string — the response is not yet cryptographically signed.

### No DNS TXT publishing

The `dns_txt_publish = true` flag is parsed and stored in config but the publisher goroutine does not exist yet. Audit root hashes are only accessible via `/.well-known/mrmi-audit`. Independent DNS-based verification is not available in this release.

### No dead-letter queue or retry

Failed envelope delivery has no retry path. If a node is unreachable, the sender receives a gRPC transport error. There is no DLQ, no backoff retry, and no persistence of undelivered envelopes. At-least-once delivery is enforced only by the idempotency key dedup on the receiving side — the sending side provides no retry guarantee.

### Timing jitter and payload padding not applied

`ProfileConfig.TimingJitterMax` and `ProfileConfig.PaddingBucket` are parsed from config but are not applied in `SendEnvelope`. Traffic analysis resistance is not active in this release.

### No forwarding between nodes

The gateway evaluates routing policy and returns a decision, but does not forward envelopes from RS to RU automatically. Actual inter-node forwarding is a Sprint 2 milestone.
