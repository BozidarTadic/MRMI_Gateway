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
v=1 ts=1746700000 root=sha256:abc123... node=rs-node-01 law=RS-GDPR
```

This value is intended to be published as a DNS TXT record at `_mrmi-audit.<node_id>` for independent third-party verification. The `law=` field carries the node's declared `applicable_law` so auditors can identify the legal framework without a separate lookup.

## Automated integration test

The `internal/integration` package spins up nodes in-process on random ports and exercises the full stack:

```bash
go test ./internal/integration/... -v
```

Expected output (all 16 tests):

```
=== RUN   TestMTLS_RoundTrip
--- PASS: TestMTLS_RoundTrip
=== RUN   TestMTLS_InsecureClientRejected
--- PASS: TestMTLS_InsecureClientRejected
=== RUN   TestSigning_ValidSignatureAllowed
--- PASS: TestSigning_ValidSignatureAllowed
=== RUN   TestSigning_TamperedPayloadRejected
--- PASS: TestSigning_TamperedPayloadRejected
=== RUN   TestSigning_MissingSignatureRejected
--- PASS: TestSigning_MissingSignatureRejected
=== RUN   TestTrustTier_BelowMinimum_AuditEntry
--- PASS: TestTrustTier_BelowMinimum_AuditEntry
=== RUN   TestTrustTier_AtMinimum_Allowed
--- PASS: TestTrustTier_AtMinimum_Allowed
=== RUN   TestCRL_RevokedNodeDenied
--- PASS: TestCRL_RevokedNodeDenied
=== RUN   TestCRL_SingleSigNotRevoked
--- PASS: TestCRL_SingleSigNotRevoked
=== RUN   TestCRL_Merge_PropagatesRevocation
--- PASS: TestCRL_Merge_PropagatesRevocation
=== RUN   TestDummyTraffic_AuditEntry
--- PASS: TestDummyTraffic_AuditEntry
=== RUN   TestDummyTraffic_NotForwarded
--- PASS: TestDummyTraffic_NotForwarded
=== RUN   TestTwoNodeLocalCorridor
--- PASS: TestTwoNodeLocalCorridor
=== RUN   TestTwoNodeAuditRootsAreIndependent
--- PASS: TestTwoNodeAuditRootsAreIndependent
=== RUN   TestTwoNodeForwardingCorridor
--- PASS: TestTwoNodeForwardingCorridor
=== RUN   TestDLQAfterExhaustedRetries
--- PASS: TestDLQAfterExhaustedRetries
```

**Sprint 2 tests** (`two_node_test.go`):

- `TestTwoNodeLocalCorridor` — ALLOW, DUPLICATE, independent dedup, DENY for out-of-policy region.
- `TestTwoNodeAuditRootsAreIndependent` — two nodes produce separate Merkle chains.
- `TestMTLS_RoundTrip` — mTLS with self-signed certs; mutual authentication passes.
- `TestMTLS_InsecureClientRejected` — client with no cert is rejected by mTLS server.
- `TestTwoNodeForwardingCorridor` — RS receives ALLOW, forwards to RU, RU's audit log gains one entry, RS response carries `peer_audit_root_hash`.
- `TestDLQAfterExhaustedRetries` — unreachable peer; 3 envelopes → DLQ ≥3, local decision still ALLOW.

**Sprint 3 tests** (`sprint3_test.go`):

- `TestSigning_ValidSignatureAllowed` — Ed25519-signed envelope passes verification and is ALLOW'd.
- `TestSigning_TamperedPayloadRejected` — payload modified after signing → DENY / INVALID_SIGNATURE.
- `TestSigning_MissingSignatureRejected` — no signature sent → DENY / INVALID_SIGNATURE.
- `TestTrustTier_BelowMinimum_AuditEntry` — T0 envelope to node with `min_trust_tier=1` → DENY, audit entry has `reason=TRUST_TIER_BELOW_MINIMUM` and `trust_tier=0`.
- `TestTrustTier_AtMinimum_Allowed` — envelope at exactly `min_trust_tier` is ALLOW'd.
- `TestCRL_RevokedNodeDenied` — 2-signature CRL entry → DENY / NODE_REVOKED.
- `TestCRL_SingleSigNotRevoked` — 1-signature CRL entry does not revoke the node.
- `TestCRL_Merge_PropagatesRevocation` — gossip merge of 2-sig entry from peer → NODE_REVOKED on local node.
- `TestDummyTraffic_AuditEntry` — IsDummy=true bypasses `min_trust_tier=3`, logged as ALLOW/DUMMY.
- `TestDummyTraffic_NotForwarded` — dummy envelope on RS is not forwarded to RU peer; RS has ALLOW/DUMMY, RU has 0 entries.

## Sprint 3 features

### Ed25519 envelope signing

Every node signs outgoing envelopes with an Ed25519 private key. Receiving nodes configured with `NewAdapterWithVerify` reject envelopes whose signature does not match.

**Dev mode (default):** an ephemeral key is generated at startup and a warning is logged:

```
[identity] using ephemeral Ed25519 signing key — set signing_key in [tls] for a persistent key
```

Ephemeral keys are sufficient for local testing. Each process restart generates a new key, so two nodes in the same dev corridor accept each other's envelopes by default (no verification enforced at the transport layer unless you call `NewAdapterWithVerify`).

**Production mode:** place a persistent Ed25519 key at a path known to both nodes. A `mrmi keygen` CLI is planned for Sprint 4. Until then, generate a key with:

```bash
openssl genpkey -algorithm ed25519 -out certs/node.ed25519.key
openssl pkey -in certs/node.ed25519.key -pubout -out certs/node.ed25519.pub
```

### Trust tier configuration

Set a minimum trust tier for inbound envelopes in `[policy.inbound]`:

```toml
[policy.inbound]
min_trust_tier = 1   # reject T0 (anonymous) senders
```

Valid tiers: `0` (anonymous), `1` (registered), `2` (verified), `3` (legal entity). The default is `0` (accept all).

Envelopes that fall below the minimum produce a `DENY` audit entry with `reason = "TRUST_TIER_BELOW_MINIMUM"` and the sender's tier value. The value is visible in the audit log's `trust_tier` field.

To test a tier violation locally:

```bash
# Start RU with min_trust_tier = 1 (edit node.ru.local.toml or use a test config)
# Send a T0 envelope from RS and observe the DENY decision on the RU audit log
curl http://localhost:8081/.well-known/mrmi-audit   # root_hash advances after DENY
```

### CRL and node revocation

The CRL store holds revocation entries for individual nodes. An entry is **effective** only when it carries ≥2 independent signatures (quorum). Single-signature entries are accepted but do not revoke the node.

**Revoke a node (programmatically in tests):**

```go
store := crl.New()
store.Revoke("ru-node-01", "compromised key", []byte("sig-alpha"))
store.Revoke("ru-node-01", "compromised key", []byte("sig-beta"))
// store.IsRevoked("ru-node-01") == true
```

**Gossip merge:** CRL entries propagate between nodes via `store.Merge(peer.Entries())`. Once a merged store reaches quorum (≥2 sigs), the node is treated as revoked.

Revoked envelopes are denied with `reason = "NODE_REVOKED"` before any region-policy evaluation.

### Trust decay timer

The `trustdecay` package reduces a peer's effective tier by 1 if no cross-validation has been recorded within the decay window (default 30 days).

The timer runs automatically in the background. In production, call `timer.RecordValidation(peerID)` whenever a successful mutual authentication completes. During local dev the decay window is far enough in the future that it has no effect.

### Dummy traffic generator

Enable dummy traffic with any profile that has a non-zero `DummyTrafficRate`:

| Profile | Rate |
|---|---|
| `strict` | 1 envelope / 5 s / peer |
| `balanced` | 1 envelope / 60 s / peer |
| `performance` | disabled |

Dummy envelopes are indistinguishable from real traffic on the wire (same padding, same timing jitter). On the receiving node they are logged as `ALLOW/DUMMY` in the audit log and are **not** forwarded further.

To observe dummy traffic in the two-node setup:

1. Start both nodes with the `strict` profile.
2. Wait 5–10 seconds.
3. Check the RU audit log — it will contain `ALLOW/DUMMY` entries from RS's generator.

```bash
curl http://localhost:8081/.well-known/mrmi-audit   # root_hash advances as dummy envelopes arrive
```

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

