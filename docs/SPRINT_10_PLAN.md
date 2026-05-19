# Sprint 10 Plan — v0.5

## Goal

Observability, operator ergonomics, and audit durability. Sprint 10 adds a Prometheus metrics endpoint, a richer `mrmi` CLI, and persistent audit storage so entries survive node restarts.

## Tasks

| # | Title | Status |
|---|---|---|
| [#59](https://github.com/BozidarTadic/MRMI_Gateway/issues/59) | Prometheus metrics endpoint (`metrics_addr`) | ✅ Done |
| [#61](https://github.com/BozidarTadic/MRMI_Gateway/issues/61) | `mrmi node` subcommands — status, peers, dlq, apps | ✅ Done |
| [#62](https://github.com/BozidarTadic/MRMI_Gateway/issues/62) | `mrmi token issue` subcommand | ✅ Done |
| [#60](https://github.com/BozidarTadic/MRMI_Gateway/issues/60) | Audit log persistence via NodeStore (bbolt / Redis) | ✅ Done |
| [#64](https://github.com/BozidarTadic/MRMI_Gateway/issues/64) | Sprint 10 acceptance tests + README v0.5 | ✅ Done |

## Design Notes

### Prometheus metrics (`#59`)

`internal/metrics/` exposes a `Registry` that counts envelope decisions (allow / deny / duplicate), DLQ depth, transit cache depth, rate-limit denials, and peer count. The gateway wires `SetOnAllow` / `SetOnDeny` / `SetOnDuplicate` callbacks to increment counters. The HTTP server mounts `/metrics` on the separate `metrics_addr` listener.

### CLI subcommands (`#61`, `#62`)

`cmd/mrmi/node.go` — `mrmi node status|peers|dlq|apps` — makes authenticated GET requests to the management API and formats output with `text/tabwriter`.

`cmd/mrmi/token.go` — `mrmi token issue --url --api-key [--scope] [--ttl]` — POSTs `{"scope", "ttl_minutes"}` to `/api/v1/token` and prints the JWT, scope, and RFC3339 expiry.

All subcommands follow the `cmdXxx(args []string) error` pattern and accept an `io.Writer` for testability.

### Audit persistence (`#60`)

`audit.Log.SetStore(store.NodeStore)` wires a persistent backend.

- `Append` updates the in-memory Merkle chain first (keeping `RootHash()` / `Verify()` authoritative), then calls `backend.AuditAppend` with a subset of fields.
- `Recent` prefers `backend.AuditLatest` (returning store-backed entries that survive restarts) and falls back to the in-memory slice when no backend is set.
- Both bbolt and Redis backends implement `AuditAppend` / `AuditLatest`. The in-memory path (no backend) is unchanged.

`app.Run` extracts the active `NodeStore` into a variable and calls `auditLog.SetStore(nodeStore)` immediately after constructing the log.

## Acceptance Tests (`test/acceptance/sprint10_test.go`)

| Test | What it covers |
|---|---|
| `TestAuditPersistence_SurvivesRestart` | bbolt-backed node — entries written in run 1 visible in run 2 |
| `TestCLINodeStatus_PrintsNodeID` | `mrmi node status` prints the configured node_id |
| `TestCLITokenIssue_PrintsJWT` | `mrmi token issue` prints a JWT with `ey` prefix |

Metrics acceptance tests live in `sprint10_metrics_test.go` (added in #59).
