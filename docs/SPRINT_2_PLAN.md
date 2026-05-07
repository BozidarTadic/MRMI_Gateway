# Sprint 2 Plan

## Context

Sprint 1 delivered a working single-node gateway: policy evaluation, dedup, Merkle audit, gRPC and HTTP transport, and a passing two-node integration test. What it did not deliver is an actual corridor — the RS node evaluates policy but never forwards the envelope to RU. There is also no mTLS, no retry/DLQ, and the privacy profile fields (jitter, padding) are parsed but silently ignored.

Sprint 2 closes those gaps. By the end of Sprint 2 the RS→RU corridor should actually route messages end-to-end, under mTLS, with retry/DLQ, and with profile-driven jitter and padding applied.

---

## Project Areas Reminder

| Area | Sprint 1 | Sprint 2 |
|---|---|---|
| Core gateway | gRPC server, policy, dedup, audit, HTTP ✅ | Peer forwarding, peer config |
| Policy and compliance | Allow/deny by region and trust tier ✅ | Jitter + padding applied, DNS TXT publish |
| Delivery reliability | Idempotency + dedup ✅ | Retry backoff, DLQ |
| Security | — | mTLS on all inter-node gRPC |
| Audit | Merkle log, HTTPS well-known ✅ | DNS TXT publisher goroutine |
| Testing | Unit + integration ✅ | Forwarding, mTLS, retry, DLQ tests |
| SDKs | — | Sprint 3+ |

---

## Sprint 2 Goal

Make the corridor route messages. An RS node must receive an envelope, evaluate policy, apply jitter and padding, and forward it to the RU node over mTLS gRPC. If the peer is unreachable, the gateway must retry with exponential backoff and write to a DLQ after 10 failures. The DNS TXT publisher must emit audit root hashes on the configured interval.

---

## Tasks

### 1. Peer routing config

Add a `[peers]` section to the TOML config that maps region codes to gRPC addresses. The gateway uses this table to determine where to forward an ALLOW'd envelope.

- Parse `[peers]` section in `config.go` (map `string → string`)
- Add `PeerRoutes map[string]string` to `NetworkConfig`
- Validate that peer addresses are non-empty if present
- Add `[peers]` to the RS and RU local TOML files pointing at each other
- Add config tests for peer section parsing and validation

### 2. Envelope forwarding

Wire the gRPC client into the gateway send path. After an ALLOW decision, dial the peer node and forward the envelope. The response to the original caller reflects the peer's decision (ALLOW, DENY, or DUPLICATE), not just the local policy result.

- Add `forwardToPeer(ctx, envelope, peerAddr)` in `gateway.go`; dial lazily, reuse connection across calls
- On ALLOW: forward envelope to peer, return peer's `AuditRootHash` in the response
- On DENY or DUPLICATE: return immediately (no forward)
- If `PeerRoutes` has no entry for `RecipientRegion`, return ALLOW without forwarding (local-only mode — backward compatible with Sprint 1 tests)
- Two-phase audit: local ALLOW entry written before forwarding; peer ACK updates no additional local entry in Sprint 2 (simplification — full two-phase in Sprint 3)
- Integration test: send RS→RU envelope and assert that RU's audit log gains an entry

### 3. mTLS on inter-node gRPC

All inter-node connections must use mutual TLS per ADR-003. Add a `[tls]` config section for cert, key, and CA paths. Keep an `insecure` fallback for tests that do not set up certs.

- Add `[tls]` to TOML: `cert`, `key`, `ca`, `insecure` (bool, default false)
- Add `TLSConfig` struct to `config.go`; parse in `parseTOML`
- `internal/tlsutil/` package: `LoadServerTLS(cfg)` and `LoadClientTLS(cfg)` returning `*tls.Config`
- Apply server TLS in `NewServer` when `TLSConfig.Cert != ""`
- Apply client TLS in `Dial` when `TLSConfig.Cert != ""`; fall back to insecure when `TLSConfig.Insecure = true`
- Add `scripts/gen-test-certs.sh` — generates self-signed CA, server cert, and client cert for local corridor testing
- Integration test: full mTLS round-trip using test certs from `t.TempDir()`
- Update `LOCAL_TWO_NODE_GUIDE.md` with mTLS setup steps

### 4. Retry with exponential backoff and DLQ

Per ADR-007: failed forwards retry with exponential backoff (1 s → 2 s → 4 s → … → 5 min cap), and after 10 failures the envelope is written to a DLQ.

- `internal/dlq/` package: thread-safe in-memory DLQ (`Entry` with envelope, peer address, attempt count, last error, first-seen and last-attempt timestamps); `Append`, `Entries`, `Remove` methods
- `internal/retry/` package: `SendWithRetry(ctx, send func() error, policy RetryPolicy) error` — exponential backoff, respects context cancellation
- Default retry policy: base 1 s, multiplier 2, cap 5 min, max attempts 10
- After `MaxAttempts` failures: write to DLQ, return a `QUEUED` decision to caller with reason
- Add `RetryPolicy` to `ProfileConfig` (parsed from `[profile_override]` or defaulted by profile preset)
- gRPC method `GetDLQEntries()` on `GatewayService` — returns current DLQ snapshot
- HTTP endpoint `GET /dlq` — JSON array of DLQ entries (operator inspection)
- Tests: forward failure → retry count verified, forward failure × 10 → DLQ entry confirmed, DLQ exposed via both APIs

### 5. Jitter and padding

Apply the profile-defined privacy controls in `SendEnvelope` before forwarding.

- **Jitter**: before calling `forwardToPeer`, sleep a random duration in `[0, TimingJitterMax]`; skip when `TimingJitterMax = 0` (performance profile)
- **Padding**: round `len(Envelope.Payload)` up to the nearest `PaddingBucket` boundary; fill with zero bytes; set `Envelope.PaddedTo`; skip when `PaddingBucket = 0` (performance profile)
- Tests: send envelope with balanced profile, assert `PaddedTo` is a multiple of 4096 and `len(payload) == PaddedTo`; assert strict profile pads to 16384 boundary; assert performance profile leaves payload unmodified

### 6. DNS TXT publisher

Start a goroutine in `app.Run` that periodically formats the DNS TXT record value and writes it to a configurable output so the operator can push it to their DNS provider.

- `internal/dnstxt/` package: `Publisher` with `Run(ctx)` goroutine; publishes `v=1 ts=<unix> root=<root_hash> node=<node_id>` to a configurable sink
- Sink options (set by config or flag): `stdout` (default), file path
- Triggered by `AuditPolicy.DNSTXTPublish = true`; interval from `AuditPolicy.DNSTXTInterval`
- Log a warning on startup when `dns_txt_publish = true` but no DNS provider is configured (operator action required)
- Tests: mock clock triggers publish; assert output matches expected TXT format; assert goroutine stops cleanly on context cancel

### 7. Sprint 2 integration test and documentation

- End-to-end corridor test: start RS and RU nodes in-process (with forwarding and mTLS using test certs), send envelope from test client to RS, assert RU audit log has an ALLOW entry
- DLQ test: start RS node pointing at an unreachable RU address, send 10+ envelopes, assert DLQ grows and retries are attempted
- Update `docs/LOCAL_TWO_NODE_GUIDE.md`: mTLS cert setup, peer config, DLQ inspection
- Update `README.md`: tick mTLS, forwarding, jitter/padding, DNS TXT checkboxes

---

## Sprint 2 Definition of Done

- `go test ./...` passes
- RS node forwards an ALLOW'd envelope to RU node over mTLS gRPC
- RU node's audit log gains an entry when an envelope is forwarded to it
- Forward failure triggers exponential retry; 10th failure writes to DLQ
- DLQ is inspectable via `GET /dlq` and `GetDLQEntries()` gRPC
- Envelope payload is padded to `PaddingBucket` boundary on balanced and strict profiles
- A timing jitter delay is applied before forwarding on balanced and strict profiles
- DNS TXT record value is emitted to stdout or file on the configured interval when `dns_txt_publish = true`
- All three local TOML configs (`balanced`, `rs.local`, `ru.local`) demonstrate correct peer and TLS config

---

## Explicitly Out of Sprint 2

- CRL store and revocation gossip (Milestone 3 — Sprint 3)
- Trust decay timer (Sprint 3)
- Dummy traffic generator (Sprint 3)
- Ed25519 signature on `/.well-known/mrmi-audit` response (Sprint 3)
- Policy hot-reload within 5 seconds (Sprint 3)
- 24-hour acceptance test run (§11 — Sprint 4 or post-seed-node deployment)
- .NET SDK (Milestone 4 — separate work stream)
- Actual DNS provider integration (operator responsibility; Sprint 2 only emits the TXT value)
