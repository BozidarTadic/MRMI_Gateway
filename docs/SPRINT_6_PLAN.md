# Sprint 6 Plan

## Context

Sprint 5 delivered the management REST API, .NET SDK, acceptance tests, and seed node guide. The gateway is now fully operable via HTTP from any language. Sprint 6 adds the v0.2 federation layer: apps can receive push notifications via webhook; operators manage nodes through write-capable authenticated endpoints; applications discover users across corridors and establish connections with configurable auto-accept policies; and a Blazor demo app brings the whole corridor to life.

---

## Sprint 6 Goal

A .NET application using `MRMI.Gateway.Client` v0.2 can discover users on a remote node, send a connect request, and observe the full RS→RU envelope flow in a live Blazor demo. Operators authenticate to write-capable management endpoints. Apps receive webhook notifications for every ALLOW'd envelope.

---

## Tasks

### 1. Push notification webhook (closes #30)

`internal/webhook/` package.

- `[apps.<app_id>]` TOML section: `webhook_url`, `webhook_secret`, `webhook_timeout_s`
- `Notifier.Notify(ctx, appID, env)` — POST JSON `{node_id, app_id, idempotency_key, sender_region, recipient_region, timestamp}`; sign body with HMAC-SHA256; `X-MRMI-Signature: sha256=<hex>`; retry once on 5xx; best-effort (failure does not block gateway)
- Wire after ALLOW decision in `core.Gateway.SendEnvelope`
- Tests: payload correct; HMAC verifiable; not called on DENY; timeout respected

### 2. Management API write endpoints + API key auth (closes #31)

- `[api]` TOML section: `api_key` (empty = unauthenticated, localhost only)
- Auth middleware: reject `X-MRMI-Key` mismatch with 401
- New endpoints:
  - `POST /api/v1/peers/register` — `{node_id, addr, node_scope, region}` → adds to runtime peer list
  - `POST /api/v1/dlq/{id}/discard` — remove DLQ entry by index
  - `POST /api/v1/config/reload` — re-read TOML + apply via `engine.Reload`
  - `POST /api/v1/revoke/{node_id}` — body: `{reason, signature_b64}`; adds to CRL store

### 3. User discovery + connect protocol (closes #33 prerequisite)

- `[apps.<app_id>.users]` TOML: `"user-id" = { display_hint = "Name", region = "RS" }`
- `internal/registry/` package: `DiscoveryResult`, `ConnectResult`, `Registry.Discover`, `Registry.Connect`
- Opaque tokens (UUID, 5-min TTL) returned on discovery; consumed on connect
- `GET /api/v1/discover?q=<query>&type=display_hint|app_id` — returns matching registered users
- `POST /api/v1/connect` — `{opaque_token, requester_id, requester_region}` → `{status, session_id}`
- Auto-accept modes from `apps.<id>.auto_accept`: `manual | auto_whitelist | auto_mutual | auto_all`

### 4. .NET SDK v0.2 (closes #33)

- `MrmiClient.DiscoverAsync(query, queryType, ct)` → `IReadOnlyList<DiscoveryResult>`
- `MrmiClient.ConnectAsync(opaqueToken, requesterId, requesterRegion, ct)` → `ConnectResult`
- `AutoAcceptMode` enum: `Manual, AutoWhitelist, AutoMutual, AutoAll`
- SDK version bump to `0.2.0`; XML doc comments; `ConnectOptions` record
- xUnit tests with mock HTTP

### 5. Python SDK v0.2 (closes #34)

`sdk/python/mrmi_gateway/` — pure Python 3.11+, no external deps beyond `httpx`.

- `MrmiClient(base_url, api_key=None)` — `send`, `receive` (SSE generator), `discover`, `connect`, `get_status`
- `pyproject.toml` + `pytest` tests
- Type annotations throughout

### 6. Blazor demo app (closes #42)

`demo/blazor/MRMI.Demo.Blazor/` — Blazor Server, .NET 10.

- Split-screen layout: RS sender panel (left) · RU receiver inbox (right)
- Bottom corridor log panel — polls `GET /api/v1/audit/latest` on both nodes every 2 s
- RS panel: Marko Petrović dummy user, message input, `MrmiClient.SendAsync`
- RU panel: Иван Иванов dummy user, live inbox via `IHostedService` + `StreamAsync`
- Configurable node URLs in `appsettings.json`

### 7. v0.2 integration tests + documentation (closes #32)

- Acceptance test: webhook HMAC delivery on ALLOW
- Acceptance test: `POST /api/v1/config/reload` applies new policy
- Acceptance test: `POST /api/v1/peers/register` appears in runtime peer list
- Acceptance test: discovery round-trip — register user → discover → connect → ACCEPTED
- Update `docs/LOCAL_TWO_NODE_GUIDE.md`: webhook config, auto-accept, discovery, write API
- Update `README.md`: Sprint 6 complete

---

## Definition of Done

- `go test ./...` passes
- `dotnet test sdk/dotnet/` passes
- `pytest sdk/python/` passes (no live node required)
- Blazor app renders split-screen layout and compiles cleanly
- All 6 issues closed
