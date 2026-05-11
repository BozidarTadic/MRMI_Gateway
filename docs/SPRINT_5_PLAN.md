# Sprint 5 Plan

> **COMPLETE** — all 4 tasks delivered · `go test ./...` green · `dotnet test` green (10/10)

## Context

Sprint 4 delivered signed audit endpoints, cross-node root hash gossip, policy hot-reload, and the `mrmi` CLI. Operators can now verify audit chains without database access, and policy configs update live without downtime.

Sprint 5 closes the last gap between the core gateway and external application developers. Go applications already communicate over gRPC, but non-Go clients have no first-class SDK. The management REST API exists only as raw HTTP endpoints with no typed client. The node has no documented path from a developer laptop to a production seed node.

Sprint 5 delivers: a management REST API with envelope send, audit, DLQ, CRL, and SSE stream endpoints; a .NET 10 SDK (`MRMI.Gateway.Client`) that wraps those endpoints; a 24-hour acceptance test suite; and a Seed Node Deployment Guide.

---

## Project Areas Reminder

| Area | Sprint 4 | Sprint 5 |
|---|---|---|
| REST API | `/healthz`, `/readyz`, `/.well-known/mrmi-audit`, `/peers/audit` | `/api/v1/status`, `/api/v1/audit/latest`, `/api/v1/envelopes`, `/api/v1/dlq`, `/api/v1/crl`, `/api/v1/stream` (SSE) |
| .NET SDK | — | `MrmiClient.Send`, `MrmiClient.Receive`, DLQ/CRL management |
| Acceptance tests | 5 Sprint 4 integration tests | 24-hour acceptance suite (`test/acceptance/`) |
| Deployment docs | LOCAL_TWO_NODE_GUIDE ✅ | Seed Node Deployment Guide |

---

## Sprint 5 Goal

A .NET 10 application can send and receive envelopes through an MRMI Gateway node using `MRMI.Gateway.Client`. Operators can deploy a seed node by following `docs/SEED_NODE_GUIDE.md`.

---

## Tasks

### 1. Management REST API (closes #21 prerequisite)

Add typed management endpoints to `internal/server/http.go`. All endpoints return `application/json`.

**New endpoints:**

- `GET /api/v1/status` — node ID, region, profile, applicable law, app version, ADR version, uptime seconds
- `GET /api/v1/audit/latest` — last 20 audit log entries (newest-first)
- `POST /api/v1/envelopes` — receive an envelope from an SDK client; run through policy, audit, and forward if configured
- `GET /api/v1/dlq` — list dead-letter queue entries (index, peer addr, attempts, last error, envelope ID, regions)
- `DELETE /api/v1/dlq/{index}` — remove DLQ entry at given index
- `POST /api/v1/dlq/{index}/replay` — re-submit DLQ entry at given index through the gateway
- `GET /api/v1/crl` — list CRL entries (node ID, reason, sig count, revoked_at, is_effective)
- `POST /api/v1/crl` — submit a revocation signature `{ node_id, reason, signature_b64 }`
- `GET /api/v1/stream` — Server-Sent Events; emits `data: <json>` for each ALLOW decision received

**`NewHTTPServer` takes a `ServerDeps` struct** so new deps can be added without changing all call sites.

**`internal/inbox/` package** — fan-out broadcaster for SSE subscribers; `gateway.SetOnAllow` callback wires it.

### 2. .NET SDK — send/receive (closes #21)

`sdk/dotnet/MRMI.Gateway.Client/` — .NET 10 class library.

**`MrmiClient`:**
- `Send(SendEnvelopeRequest request, CancellationToken ct)` → `SendEnvelopeResponse`
- `Receive(Func<ReceivedEnvelope, Task> handler, CancellationToken ct)` — connects to `/api/v1/stream`, calls handler for each SSE event
- `GetStatus(CancellationToken ct)` → `NodeStatusResponse`
- `GetAuditLatest(CancellationToken ct)` → `IReadOnlyList<AuditEntry>`

**`MrmiProfile` enum:** `Strict`, `Balanced`, `Performance`

**`MrmiClientOptions`:** `BaseUrl`, `Timeout`, `HttpClient` (injectable for tests)

**`MRMI.Gateway.Client.Tests/`** — xUnit tests with mock HTTP responses.

### 3. .NET SDK — blacklist and revocation (closes #22)

Extend `MrmiClient` with:
- `GetDLQEntries(CancellationToken ct)` → `IReadOnlyList<DlqEntry>`
- `RemoveDLQEntry(int index, CancellationToken ct)`
- `ReplayDLQEntry(int index, CancellationToken ct)` → `ReplayResult`
- `GetCRLEntries(CancellationToken ct)` → `IReadOnlyList<CrlEntry>`
- `PublishRevocationSignature(string nodeId, string reason, byte[] signature, CancellationToken ct)`

### 4. Acceptance tests + seed node guide (closes #23)

- `test/acceptance/acceptance_test.go` — starts a real gateway node in-process; tests all REST endpoints end-to-end; verifies SSE stream delivers events; verifies DLQ replay; verifies CRL submission
- `docs/SEED_NODE_GUIDE.md` — step-by-step: OS prerequisites, TLS cert generation, TOML config for a regional node, systemd unit file, DNS TXT verification, health check procedure
- `README.md` — tick Sprint 5 checkboxes, mark Sprint 5 complete

---

## Sprint 5 Definition of Done

- `go test ./...` passes
- `dotnet test sdk/dotnet/MRMI.Gateway.Client.Tests/` passes
- All 9 management REST endpoints respond correctly
- SSE stream delivers events to a connected client within 1 second of an ALLOW decision
- .NET `MrmiClient.Send` + `Receive` round-trip works against a live gateway node
- `docs/SEED_NODE_GUIDE.md` exists and covers TLS, TOML, systemd, DNS verification

---

## Explicitly Out of Sprint 5

- Persistent storage (bbolt / Redis) — v0.3
- Federated user discovery — v0.2
- Node dashboard UI — v0.3
- Java SDK — open for contributors
- mTLS CRL distribution point in certificate — v0.3
