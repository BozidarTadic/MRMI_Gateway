# Local Two-Node Test Guide

This guide explains how to run an RS + RU corridor locally, verify both nodes are healthy, and exercise the full send / audit / dedup / forwarding path.

## Prerequisites

- Go 1.21 or later
- `curl` (for HTTP endpoint checks)
- OpenSSL or `scripts/gen-test-certs.sh` (for mTLS — optional for local dev with `insecure = true`)

No additional tools are required. The gRPC layer is tested through the Go test suite (see [Automated integration test](#automated-integration-test) below).

## mTLS certificate setup

Inter-node gRPC uses mutual TLS per ADR-003. For local development the shipped configs default to `insecure = true`, which bypasses certificate verification. For a production-like setup, generate self-signed certs:

```bash
bash scripts/gen-test-certs.sh   # writes certs/ to the repo root
```

Then update both TOML configs to reference the generated paths:

```toml
[tls]
cert = "certs/node.crt"
key  = "certs/node.key"
ca   = "certs/ca.crt"
# insecure = false  (default; omit or set explicitly)
```

Integration tests use in-process cert generation via `internal/testcerts` so they do not require any external cert files.

## Peer config

Each node declares its peer in `[peers.<REGION>]`. The RS local config already points at RU:

```toml
# configs/node.rs.local.toml
[peers.RU]
addr       = "localhost:7778"
node_scope = "regional"
```

Add the symmetric entry to `configs/node.ru.local.toml` for bidirectional forwarding:

```toml
[peers.RS]
addr       = "localhost:7777"
node_scope = "regional"
```

When a peer entry exists for the envelope's `recipient_region`, the node forwards the ALLOW'd envelope to that peer. When no entry exists, the node returns ALLOW without forwarding (backward-compatible single-node mode).

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

## DLQ inspection

When forwarding to a peer fails after all retries, the envelope moves to the dead-letter queue. Inspect via the gRPC `GetDLQEntries` call or the HTTP endpoint:

```bash
curl http://localhost:8080/dlq   # → JSON array of DLQ entries
```

Each entry shows the envelope, target peer address, number of attempts, and the last error. Entries remain in memory until the process restarts; persistence is a Sprint 3 milestone.

## DNS TXT publishing

When `dns_txt_publish = true` in the audit config, the node periodically emits its audit root hash to stdout (no external DNS provider required in dev mode):

```
v=1 ts=1746700000 root=sha256:abc123... node=rs-node-01
```

This value is intended to be published as a DNS TXT record at `_mrmi-audit.<node_id>` for independent third-party verification.

## Automated integration test

The `internal/integration` package spins up nodes in-process on random ports and exercises the full stack:

```bash
go test ./internal/integration/... -v
```

Expected output:

```
=== RUN   TestTwoNodeLocalCorridor
--- PASS: TestTwoNodeLocalCorridor
=== RUN   TestTwoNodeAuditRootsAreIndependent
--- PASS: TestTwoNodeAuditRootsAreIndependent
=== RUN   TestMTLS_RoundTrip
--- PASS: TestMTLS_RoundTrip
=== RUN   TestMTLS_InsecureClientRejected
--- PASS: TestMTLS_InsecureClientRejected
=== RUN   TestTwoNodeForwardingCorridor
--- PASS: TestTwoNodeForwardingCorridor
=== RUN   TestDLQAfterExhaustedRetries
--- PASS: TestDLQAfterExhaustedRetries
```

`TestTwoNodeLocalCorridor` verifies:

1. RS node accepts an RS→RU envelope and returns `ALLOW` with a non-zero audit root hash.
2. Replaying the same `idempotency_key` to RS returns `DUPLICATE` and advances the audit root.
3. The same `idempotency_key` sent to the **RU** node returns `ALLOW` — each node has an independent dedup store.
4. A route not present in RU's allow-list (→US) returns `DENY`.

`TestTwoNodeAuditRootsAreIndependent` confirms the two Merkle chains never share a root hash, and each chain passes `Verify()`.

`TestMTLS_RoundTrip` starts a node with self-signed mTLS certs and verifies a full round-trip with a mutually authenticated client.

`TestMTLS_InsecureClientRejected` confirms that a client presenting no certificate is rejected by an mTLS server.

`TestTwoNodeForwardingCorridor` wires RS→RU forwarding in-process:

1. RS receives an RS→RU envelope, evaluates ALLOW locally.
2. RS forwards the envelope to the RU node via gRPC.
3. RU's audit log gains one ALLOW entry.
4. RS response includes `peer_audit_root_hash` from RU.

`TestDLQAfterExhaustedRetries` confirms that forwarding failures write to the DLQ:

1. RS is configured with an unreachable peer address.
2. Three envelopes are sent; each fails immediately (1 attempt, no backoff).
3. The DLQ has ≥ 3 entries; the local RS response still returns `ALLOW`.

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
