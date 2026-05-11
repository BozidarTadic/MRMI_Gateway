# Sprint 3 Plan

> **COMPLETE** — commits `4d4c5e4`…`41c3c0c` · all 7 tasks delivered · `go test ./...` green

## Context

Sprint 2 delivered the RS→RU corridor: mTLS inter-node gRPC, tier-aware forwarding, retry/DLQ, jitter/padding, and DNS TXT publication. What it did not deliver is any trust enforcement beyond the basic allow/deny region check. An attacker can send any envelope from any identity with any trust tier and it will be forwarded if the region is permitted.

Sprint 3 closes those gaps. By the end of Sprint 3 every inter-node envelope carries an Ed25519 signature verified before policy evaluation, trust tiers are fully enforced and audited, per-sender ordering is tracked, revoked nodes are rejected at the mTLS handshake via gossip-propagated CRLs, stale cross-validations auto-decay trust, and dummy traffic prevents idle-period analysis.

---

## Project Areas Reminder

| Area | Sprint 2 | Sprint 3 |
|---|---|---|
| Core gateway | Peer forwarding, tier-aware routing ✅ | Sequence numbers, applicable_law completions |
| Policy and compliance | Jitter + padding ✅ | Trust tier audit, applicable_law DNS TXT + warn |
| Identity and trust | — | Ed25519 signing, trust tiers full enforcement, trust decay |
| Revocation | — | CRL store, gossip propagation |
| Privacy | DNS TXT publish ✅ | Dummy traffic generator |
| Testing | Forwarding, mTLS, retry, DLQ ✅ | Revocation, signing, trust tier, dummy traffic |

---

## Sprint 3 Goal

Every inter-node envelope must be signed by the sending node and verified by the receiver. Trust tier violations must appear in the audit log with reason and tier value. Revoked nodes must be rejected at the mTLS handshake within one gossip cycle. Idle corridors must emit dummy traffic at the profile-defined rate.

---

## Tasks

### 1. applicable_law completions (closes #8)

The field already parses in config, writes to audit entries, and appears in the HTTP well-known response. Two gaps remain.

- Add `applicable_law` to DNS TXT publisher output: `v=1 ts=<unix> root=<hash> node=<id> law=<applicable_law>`
- Startup warning in `app.Run` when `cfg.Node.ApplicableLaw == "NONE"` and `cfg.Profile.Name != "performance"`
- Tests: DNS TXT output contains `law=` field; warn triggers on balanced/strict profile with `NONE`

### 2. Trust tier audit fields (closes #11)

Trust tier enforcement already works in the policy engine. The audit entry is missing the tier value and the structured denial reason.

- Add `Reason` and `TrustTier` fields to `audit.Entry`
- Pass `reason` and `trustTier` through `audit.Log.Append`; update all call sites
- Policy engine: use constant `"TRUST_TIER_BELOW_MINIMUM"` as reason (was a freeform string)
- Tests: audit entry carries `trust_tier` and `reason` on DENY; reason is the constant string

### 3. Per-sender sequence number tracker (closes #10)

The `sequence_number` field already exists in `Envelope`. The tracker and receive-side validation are missing.

- `internal/session/` package: `Tracker` with `NextSeq(senderID string) uint64` and `Validate(senderID string, seq uint64) error`
- Tracker is per-node in-memory; sender ID is `sender_region` + `idempotency_key` prefix (stable within session)
- Sending node: gateway calls `tracker.NextSeq` before forwarding; sets `Envelope.SequenceNumber`
- Receiving node: adapter calls `tracker.Validate`; out-of-order logs a warning but does not reject (gap tolerance — full enforcement v1.0)
- Tests: in-order accepted; out-of-order triggers warning but returns no error; new sender ID starts at 1

### 4. Ed25519 envelope signing (closes #9)

- `internal/identity/` package:
  - `GenerateKey() (privKey, pubKey ed25519.PrivateKey/PublicKey, err error)`
  - `Sign(privKey ed25519.PrivateKey, env core.Envelope) []byte` — signs canonical JSON of envelope fields (all fields except `Signature` itself)
  - `Verify(pubKey ed25519.PublicKey, env core.Envelope, sig []byte) error`
- Add `Signature []byte` field to `core.Envelope` and `messages.go`
- Key loading in `app.Run`: load Ed25519 key from `[tls] signing_key` path if set; otherwise generate ephemeral key pair at startup with a log warning (`dev mode — ephemeral signing key`)
- Signing side: `forwardToPeer` closure in `app.go` signs envelope before gRPC call
- Verification side: gRPC adapter verifies signature before passing to `core.Gateway.SendEnvelope`; DENY with reason `INVALID_SIGNATURE` if verification fails
- Verification is skipped when `TLSConfig.Insecure = true` (backward-compatible with Sprint 2 tests)
- Tests: valid signature passes; tampered payload rejected; missing signature rejected on non-insecure config; insecure mode skips verification

### 5. CRL store and revocation gossip (closes #12)

- `internal/crl/` package:
  - `Entry` struct: `NodeID`, `Reason`, `RevokedAt time.Time`, `Signatures [][]byte`
  - `Store` with `Revoke(nodeID, reason string, sig []byte)`, `IsRevoked(nodeID string) bool`, `Entries() []Entry`, `Merge([]Entry)`
  - Blacklist rule: entry is effective only when `len(Signatures) >= 2`
- gRPC message types `ShareCRLRequest` / `ShareCRLResponse` in `messages.go`
- `ShareCRL` gRPC method on `GatewayService` — receives peer CRL snapshot, calls `store.Merge`, returns ack
- On merge: if a newly effective entry (≥2 sigs) matches a connected peer, close that peer's connection
- Policy engine: check `crlStore.IsRevoked(cfg.Node.NodeID)` on startup; check sender on receive (deny with reason `NODE_REVOKED`)
- `app.Run`: pass CRL store to gateway and adapter; start gossip goroutine that calls `ShareCRL` on all peers every `cfg.Network.CRLGossipInterval` (default 60s)
- Tests: single-sig entry does not revoke; two-sig entry revokes; gossip propagates entry to peer; revoked node DENY in policy engine

### 6. Trust decay timer (closes #13)

- `internal/trustdecay/` package:
  - `Timer` with `RecordValidation(peerID string)`, `EffectiveTier(peerID string, announcedTier uint32) uint32`, `Run(ctx context.Context)`
  - Decay window configurable, default 30 days
  - On decay trigger: emit structured `slog.Warn`; reduce effective tier by 1 (floor T0); do not modify CRL
- Wire into policy engine: `engine.Evaluate` receives `effectiveTier` from `Timer.EffectiveTier` rather than raw `TrustTier`
- `app.Run`: construct `Timer`, start `Run` goroutine
- Tests: old timestamp decays tier by 1; fresh timestamp preserves tier; goroutine exits on cancel

### 7. Dummy traffic generator (closes #14)

- `internal/dummy/` package:
  - `Generator` with `Run(ctx context.Context, peers []config.PeerConfig, send func(core.Envelope))`
  - Interval per profile: `strict` = 5s, `balanced` = 60s, `performance` = disabled (Generator.Run returns immediately)
  - Dummy envelope: zero payload, padded to profile bucket, `IsDummy = true`, unique `IdempotencyKey` per tick, `SenderRegion = RecipientRegion` (loopback — no routing)
- Add `IsDummy bool` field to `core.Envelope` and `messages.go`
- Receiving node: adapter detects `IsDummy = true`; skips policy evaluation; appends audit entry with decision `ALLOW/DUMMY`; does not forward
- `app.Run`: construct Generator after forwarder is wired; start `Run` goroutine
- Tests: strict profile generates at ≤5s interval; performance profile generates nothing; dummy flag prevents policy eval and forwarding

### 8. Sprint 3 integration tests and documentation (closes #15)

- E2E signing test: two-node corridor, tamper envelope payload after signing, assert RU rejects with `INVALID_SIGNATURE` audit entry
- E2E trust tier test: send T0 envelope to node with `min_trust_tier=1`; assert DENY audit entry with `reason = "TRUST_TIER_BELOW_MINIMUM"` and `trust_tier = 0`
- E2E CRL test: start RS+RU, add two-sig CRL entry for RS node ID, assert next envelope from RS is denied with `NODE_REVOKED`
- E2E dummy traffic test: balanced profile two-node corridor, assert audit log gains `ALLOW/DUMMY` entries within 70s
- Update `docs/LOCAL_TWO_NODE_GUIDE.md`: Ed25519 key setup, CRL test steps, trust tier config, dummy traffic toggle
- Update `README.md`: tick signing, CRL, trust decay, dummy traffic checkboxes

---

## Sprint 3 Definition of Done

- `go test ./...` passes
- Every inter-node envelope carries a valid Ed25519 signature; invalid signatures are rejected before policy evaluation
- Audit entries carry `reason`, `trust_tier`, and `applicable_law`; DNS TXT output includes `law=` field
- Startup warns when `applicable_law = "NONE"` on a non-performance profile
- CRL with ≥2 signatures revokes a node; a single-sig entry has no effect
- Revoked node envelopes receive `DENY` with reason `NODE_REVOKED`
- Peers with no cross-validation in 30 days have effective trust tier auto-reduced
- Dummy traffic is generated at profile-defined intervals; dummy envelopes do not reach policy evaluation and are logged as `ALLOW/DUMMY`

---

## Explicitly Out of Sprint 3

- `mrmi keygen` CLI command (Sprint 4)
- Policy hot-reload (Sprint 4)
- HTTPS `/.well-known/mrmi-audit` Ed25519 signature (Sprint 4)
- Cross-node root hash gossip (Sprint 4)
- mTLS certificate rejection of revoked nodes at handshake level (requires CRL distribution point in cert — deferred to Sprint 4; Sprint 3 rejects at policy layer)
- .NET SDK (Sprint 5)
- Actual DNS provider integration (operator responsibility)
