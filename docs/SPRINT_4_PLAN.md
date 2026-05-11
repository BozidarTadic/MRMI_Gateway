# Sprint 4 Plan

> **COMPLETE** — commit `b7767cd` · all 4 tasks delivered · `go test ./...` green

## Context

Sprint 3 delivered the full trust enforcement layer: Ed25519 envelope signing, CRL revocation with gossip quorum, trust tier audit entries, trust decay, and dummy traffic. The `/.well-known/mrmi-audit` HTTP endpoint exists but returns an unsigned response. The policy engine must be restarted to pick up config changes. Operators have no CLI tooling for key management or audit verification. Nodes do not cross-check each other's audit roots.

Sprint 4 closes those gaps. By the end of Sprint 4: the audit endpoint is signed with the node's Ed25519 key; operators can verify audit roots locally, via DNS, or via HTTPS from a single CLI; the policy config hot-reloads within 5 seconds without a restart; and nodes gossip audit root hashes with peers so a compromised audit log cannot be silently hidden.

---

## Project Areas Reminder

| Area | Sprint 3 | Sprint 4 |
|---|---|---|
| Audit verification | DNS TXT (unsigned) ✅ | HTTPS well-known with Ed25519 signature, cross-node gossip |
| CLI tooling | `mrmi-gateway -version` ✅ | `mrmi keygen`, `mrmi audit verify` (local / DNS / HTTPS) |
| Policy engine | Tier enforcement, CRL ✅ | Hot-reload without restart |
| Cross-node trust | CRL gossip ✅ | Root hash gossip (opt-in) |
| Testing | 16 integration tests ✅ | Sprint 4 integration test + docs |

---

## Sprint 4 Goal

Operators can verify any node's audit trail without direct database access. Policy configs update live without downtime. Nodes cross-validate each other's audit chains automatically.

---

## Tasks

### 1. HTTPS `/.well-known/mrmi-audit` with Ed25519 signature (closes #16)

The endpoint exists and returns JSON. The `signature` field is a placeholder. This task adds a real Ed25519 signature so verifiers can confirm the response was produced by the node that holds the private key.

- HTTP handler returns `{ version, timestamp, root_hash, node_id, applicable_law, signature }`
- `signature` = Ed25519 sign of canonical JSON: keys sorted alphabetically, no whitespace, `signature` field excluded from the payload
- Enable per `https_well_known = true` in `[policy.audit]` TOML; return `404` when disabled
- Handler reads live `audit.Log.RootHash()` on every request — no caching
- Response within 500 ms (ADR §11 acceptance criterion)
- Tests: valid signed response; `identity.Verify` against node public key passes; disabled → 404; response latency < 500 ms

### 2. Cross-node root hash gossip (closes #17)

After each Merkle publish interval, send the current root hash to all peers. Receiving nodes store peer hashes and expose them via HTTP for operator inspection.

- gRPC method `ShareRootHash(RootHashMessage) RootHashAck` on `GatewayService`
- `RootHashMessage`: `node_id string`, `root_hash string`, `timestamp int64`, `signature []byte`
- Enable per `root_hash_gossip = true` in `[policy.audit]` TOML
- On publish interval: goroutine calls `ShareRootHash` on each peer (non-blocking, fire-and-forget)
- Receiving node: store latest `{ root_hash, timestamp }` per peer node ID in memory
- `GET /peers/audit` HTTP endpoint: returns JSON map of peer node IDs to their latest root hash + timestamp
- Tests: gossip fires; received hash stored; `/peers/audit` returns stored value; disabled config skips gossip

### 3. Policy hot-reload (closes #18)

Watch the TOML config file for writes and re-apply policy rules atomically within 5 seconds.

- `internal/hotreload/` package: `Watcher` with `Watch(ctx context.Context, path string, onChange func(config.Config))`
- Implementation: `fsnotify` for inotify/kqueue/ReadDirectoryChanges; debounce 500 ms to avoid double-fire on editor save
- On change: re-parse + re-validate; on success swap policy engine state via `sync/atomic.Pointer`; on failure log error and keep old config
- `policy_version` unchanged after reload → emit structured `slog.Warn`
- `app.Run`: start watcher goroutine when `--config` path is set (not in default/in-memory mode)
- Tests: write new TOML → engine uses new allow-list within 5 s; invalid TOML → old config preserved; `policy_version` unchanged → warning; watcher goroutine exits on context cancel

### 4. `mrmi` CLI — keygen + audit verify (closes #19)

Operator CLI per ADR Appendix B compliance checklist. Lives in `cmd/mrmi/` alongside the existing `cmd/mrmi-gateway/`.

**Commands:**

```
mrmi keygen --output <path>
    Generate Ed25519 key pair. Write private key (PEM) to <path>.
    Print public key (PEM) to stdout.

mrmi audit verify --local --config <path>
    Recompute Merkle root from local log file.
    Exit 0 on match, exit 1 on mismatch.

mrmi audit verify --dns --node <node_id>
    Fetch TXT record at _mrmi-audit.<node_id>.
    Parse v=1 ts=... root=sha256:... format.
    Print: PASS / FAIL + expected vs. actual root hash.

mrmi audit verify --https --url <url>
    GET <url> (defaults to /.well-known/mrmi-audit).
    Parse JSON response; verify Ed25519 signature.
    Print: PASS / FAIL + signature status + root hash match.
```

- Use `flag` package (no cobra dependency — keep binary small)
- Each verify command: exits 0 on PASS, exits 1 on FAIL; prints structured human-readable output
- `mrmi -version` prints `mrmi <App> (ADR <ADR>)` using `internal/version`
- Tests: `keygen` produces a valid Ed25519 pair that `identity.Verify` accepts; local verify passes on a fresh log; local verify fails after log entry is tampered

### 5. Sprint 4 integration tests and documentation (closes #20)

- Integration test: start a node with `https_well_known = true`; make HTTP request; verify Ed25519 signature in response
- Integration test: two-node corridor with `root_hash_gossip = true`; trigger gossip; assert `/peers/audit` on each node shows the other's hash
- Integration test: write a new TOML to a temp file; assert policy engine picks up new allow-list within 5 s
- Unit test: `mrmi keygen` + `mrmi audit verify --local` on a freshly created log
- Update `docs/LOCAL_TWO_NODE_GUIDE.md`: signed audit endpoint, `/peers/audit` inspection, hot-reload workflow, CLI reference
- Update `README.md`: tick Sprint 4 checkboxes, mark Sprint 4 complete

---

## Sprint 4 Definition of Done

- `go test ./...` passes
- `GET /.well-known/mrmi-audit` returns a valid Ed25519-signed JSON body when `https_well_known = true`
- `mrmi keygen` writes a valid key pair; all three `mrmi audit verify` modes exit 0 on a healthy node
- Policy config file changes apply within 5 seconds; invalid changes are rejected without disruption
- When `root_hash_gossip = true`, nodes gossip root hashes; received hashes visible at `/peers/audit`

---

## Explicitly Out of Sprint 4

- Storage persistence (bbolt / Redis) — Sprint 5 / v0.3
- Federated user discovery (DiscoveryRequest / DiscoveryResponse) — Sprint 5 / v0.2
- Node dashboard UI — Sprint 5 / v0.3
- mTLS CRL distribution point in certificate (requires CA infrastructure) — v0.3
- .NET SDK — Sprint 5 / v0.3
