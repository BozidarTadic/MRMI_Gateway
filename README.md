# MRMI Gateway

**Multi-Regional Multi-App Interlock** — open-source federation middleware for regulated cross-border messaging corridors.

MRMI Gateway sits between messaging applications and enforces legal-compliance constraints at the **transport layer** — not bolted on afterwards by operators. Think Apache Kafka for cross-border messaging infrastructure, with built-in policy enforcement, verifiable audit trails, and identity revocation.

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*

---

## Problem

Modern messaging apps operate across a fragmented regulatory landscape. Russia's **152-ФЗ**, the EU's **GDPR**, and Kazakhstan's data localisation laws impose strict requirements on where user data may reside and how it may cross borders. Existing federated protocols (Matrix, XMPP) treat cross-border data flow as an implementation concern — leaving operators to figure out compliance on their own.

MRMI Gateway makes compliance an architectural guarantee that operators cannot accidentally disable.

## Target Corridors (v0.1)

Primary: **RU · BY · KZ · AM · RS** — regions with similar data sovereignty concerns, existing commercial relationships, and no current cross-border protocol enforcement.

EU/US corridors: deferred to v1.0.

## Architecture

```
App A (RU)                                              App B (RS)
    │                                                       │
    ▼                                                       ▼
MRMI Node (RU) ──── gRPC/mTLS ────── MRMI Node (RS)
    │                                       │
    ├── Policy Engine (allow/deny by region + trust tier)
    ├── Merkle Audit Log (SHA-256 chained, DNS TXT published)
    ├── Identity Resolution (T0–T3 trust tiers)
    └── CRL + Blacklist Gossip (≥2 T2+ quorum for revocation)
```

Each node runs a Go binary. Nodes communicate over gRPC with mutual TLS. Every envelope is policy-checked, deduplicated via idempotency key, and appended to a Merkle audit log whose root hash is published to DNS TXT for independent verification.

Full architecture: [docs/MRMI_Gateway_ADR_v0_8.md](docs/MRMI_Gateway_ADR_v0_8.md)

## Key Properties

| Property | Mechanism |
|---|---|
| Cross-border policy enforcement | Signed TOML config with `applicable_law`, `allowed_regions`, `blocked_regions` |
| At-least-once delivery | Idempotency key + dedup index + ACK/retry |
| Verifiable audit | SHA-256 Merkle chain, root hash in DNS TXT + `/.well-known/mrmi-audit` |
| Identity trust | T0 (anonymous) → T3 (legal entity), revocable via CRL gossip |
| Traffic analysis resistance | Configurable timing jitter + payload padding per profile |
| Compliance profiles | `strict` / `balanced` / `performance` — maps to 152-ФЗ / GDPR / Kazakhstan |

## Current Status — v0.4 (Sprint 9 complete)

**Sprint 1 + 2 — complete**

- [x] Protobuf contracts (`proto/mrmi/v1/contracts.proto`)
- [x] Config model — TOML parser, 3 profiles, node tier (Regional/Alliance/Global), `Config.Validate()`
- [x] Policy engine — allow/deny by region + trust tier, all decisions audited
- [x] Merkle audit log — SHA-256 chained, `Verify()`, `RootHash()`
- [x] HTTP server — `/healthz`, `/readyz`, `/.well-known/mrmi-audit`
- [x] gRPC transport — server + client, `GatewayService` handler
- [x] App wiring + graceful shutdown
- [x] Dedup index — idempotency key store with TTL, duplicate decisions logged to audit
- [x] Local two-node RS/RU corridor — integration test + local configs
- [x] mTLS on all inter-node gRPC (`tlsutil` package, TLS 1.3 minimum)
- [x] Timing jitter + payload padding (applied on balanced/strict profiles before forwarding)
- [x] DNS TXT root hash publisher (stdout/file on configured interval)
- [x] Envelope forwarding between nodes (tier-preference routing: Regional → Alliance → Global → DLQ)
- [x] Retry with exponential backoff and dead-letter queue (`delivery` package)
- [x] Node tier model — `node_scope`, `alliance_id`, `node_region` in config and audit entries

**Sprint 3 — complete**

- [x] `applicable_law` in DNS TXT output + startup warning on unset production profile
- [x] Trust tier violation reason and tier value in audit entries
- [x] Per-sender sequence number tracker (`session` package)
- [x] Ed25519 envelope signing — sign on send, verify before policy evaluation (`identity` package)
- [x] CRL store and revocation gossip — ≥2 T2+ signatures required (`crl` package)
- [x] Trust decay timer — auto-reduce effective tier after 30 days without cross-validation
- [x] Dummy traffic generator — synthetic envelopes at profile-defined intervals (`dummy` package)

**Sprint 4 — complete**

- [x] HTTPS `/.well-known/mrmi-audit` with Ed25519 signature (`https_well_known = true`)
- [x] Cross-node root hash gossip — peers cross-verify audit chains (`root_hash_gossip = true`)
- [x] Policy hot-reload — config changes applied within 5 seconds without restart (`hotreload` package)
- [x] `mrmi` CLI — `keygen`, `audit verify --local/--dns/--https`

**Sprint 5 — complete**

- [x] Management REST API (`/api/v1/status`, `/api/v1/envelopes`, `/api/v1/audit/latest`, `/api/v1/dlq`, `/api/v1/crl`, `/api/v1/stream`)
- [x] .NET SDK `MrmiClient` — `Send`, `Receive`, DLQ/CRL management (`sdk/dotnet/MRMI.Gateway.Client/`)
- [x] Acceptance test suite (`test/acceptance/`) — 12 tests covering all REST endpoints + SSE stream
- [x] Seed Node Deployment Guide (`docs/SEED_NODE_GUIDE.md`)

**Sprint 6 — complete**

- [x] Push notification webhook — `internal/webhook/`, HMAC-SHA256, best-effort delivery
- [x] Management API write endpoints — peer register, discard/replay DLQ, config reload, revoke; API key auth
- [x] User discovery + connect protocol — `internal/registry/`, opaque tokens, auto-accept modes
- [x] .NET SDK v0.2 — `DiscoverAsync`, `ConnectAsync`, `AutoAcceptMode`
- [x] Python SDK v0.2 — `sdk/python/` (`mrmi-gateway-sdk`)
- [x] Blazor demo app — split-screen RS/RU corridor with live audit log (`demo/blazor/`)

**Sprint 7 — complete** (v0.2, federated discovery)

- [x] Federated discovery proto — `BroadcastDiscovery` + `ConnectRequest` RPCs in `contracts.proto`
- [x] Broadcast engine — hop-limited fan-out with dedup, 30 s staleness guard (`internal/discovery/`)
- [x] Opaque token lifecycle — SHA-256 single-use tokens, TTL-based purge (`internal/token/`)
- [x] App namespace isolation — `SAME_APP_ONLY` / `WHITELIST` / `OPEN` policy modes
- [x] ConnectRequest auto-accept — `MANUAL` / `AUTO_WHITELIST` / `AUTO_MUTUAL` / `AUTO_ALL`

**Sprint 8 — complete** (v0.3, persistence + dashboard)

- [x] `NodeStore` interface — pluggable persistence for dedup, DLQ, CRL, audit (`internal/store/`)
- [x] bbolt backend — embedded persistent key-value store (`internal/store/bbolt/`)
- [x] Redis backend — `go-redis/v9`, `SetNX` dedup, HSET for DLQ/CRL, RPUSH for audit (`internal/store/redis/`)
- [x] Dynamic peer discovery — `peerdiscovery.Registry`, gossip loop, bootstrap nodes, stale eviction
- [x] Management API v0.3 — JWT HMAC-SHA256 auth, `GET/PUT /api/v1/config`, app register/deregister endpoints
- [x] Embedded dashboard SPA — vanilla JS, dark theme, 5 pages: Status, Audit Log, DLQ, Settings, Apps (`web/`)
- [x] Sprint 8 integration tests — 14 new acceptance tests covering apps, dashboard, JWT, gossip, config PUT

**Sprint 9 — complete** (v0.4, production hardening)

- [x] Transit cache — in-memory LRU+TTL buffer (≤60 s, ≤1000 entries) before DLQ promotion (`internal/transit/`)
- [x] Discovery rate limiter — per-origin-node token bucket (10 req/s, burst 20) on `BroadcastDiscovery` RPC (`internal/ratelimit/`)
- [x] Alliance and global node operator guide — `docs/ALLIANCE_NODE_GUIDE.md`; example configs `configs/node.alliance.eaeu.toml`, `configs/node.global.relay.toml`
- [x] `POST /api/v1/token` — API-key-authenticated JWT issuance endpoint; scope `read`/`operator`, configurable TTL
- [x] Python SDK v0.3 — `jwt_token` option, `issue_token()`, `list_apps()`, `register_app()`, `delete_app()` (`sdk/python/`)
- [x] .NET SDK v0.3 — `JwtToken`/`ApiKey` options, `IssueTokenAsync()`, `ListAppsAsync()`, `RegisterAppAsync()`, `DeleteAppAsync()` (`sdk/dotnet/`)
- [x] Sprint 9 acceptance tests — 11 new tests: transit cache drain, rate limiter, JWT issuance flow, JWT→apps auth

**Future**

- [ ] DHT-based peer discovery (v1.0)
- [ ] CLI reference client (open for contributors)
- [ ] Java SDK (open for contributors)

## Quick Start

**Prerequisites:** Go 1.21+, `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc`

```bash
git clone https://github.com/tadicbb/mrmi-gateway
cd mrmi-gateway
go run ./cmd/mrmi-gateway -config configs/node.balanced.toml
```

This starts the node on `:8080` (HTTP) and `:7777` (gRPC) with the balanced compliance profile.

**Verify the node is up:**

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/.well-known/mrmi-audit
```

**Run the full test suite:**

```bash
go test ./...
```

**Local two-node corridor (RS + RU):** see [docs/LOCAL_TWO_NODE_GUIDE.md](docs/LOCAL_TWO_NODE_GUIDE.md).

## Configuration

Nodes are configured via signed TOML files. Three compliance profiles are available (`strict`, `balanced`, `performance`); the shipped configs use `balanced`:

| File | Purpose |
|---|---|
| `configs/node.balanced.toml` | Single RS node — default starting point |
| `configs/node.rs.local.toml` | RS node for local two-node corridor testing |
| `configs/node.ru.local.toml` | RU node for local two-node corridor testing |

Profile definitions (dedup TTL, jitter, padding, dummy traffic rates) live in `internal/config/presets.go`. Full TOML reference in [docs/MRMI_Gateway_ADR_v0_8.md — Appendix A](docs/MRMI_Gateway_ADR_v0_8.md#appendix-a--full-toml-configuration-examples).

## Repository Layout

```
cmd/mrmi-gateway/   — node process entrypoint
cmd/mrmi/           — operator CLI (keygen, audit verify)
internal/
  app/              — wiring: audit, policy, HTTP, gRPC, inbox, shutdown
  audit/            — Merkle chain log (SHA-256, Verify, RootHash, Recent)
  config/           — TOML parser, validation, profile presets
  core/             — domain types: Gateway, Envelope, SendRequest/Response
  crl/              — Certificate Revocation List store (≥2 sig quorum)
  dedup/            — idempotency key store with TTL + Purge
  delivery/         — Forwarder, retry backoff, DLQ
  dnstxt/           — DNS TXT root hash publisher
  dummy/            — synthetic dummy traffic generator
  identity/         — Ed25519 key generation, envelope sign/verify
  inbox/            — fan-out broadcaster for SSE stream subscribers
  integration/      — multi-node end-to-end tests
  registry/         — user discovery, opaque tokens, connect/auto-accept
  webhook/          — HMAC-SHA256 push notifications to app webhooks
  policy/           — policy engine (region allow/deny, trust tier, CRL)
  server/           — HTTP endpoints (healthz, readyz, management API, SSE)
  session/          — per-sender sequence number tracker
  testcerts/        — in-process self-signed cert generation (tests only)
  tlsutil/          — LoadServerTLS / LoadClientTLS
  transport/grpc/   — gRPC server + client, JSON codec
  hotreload/        — config file watcher; atomic policy hot-reload
  peercache/        — in-memory store for peer audit root hashes
  trustdecay/       — effective tier decay after 30d without cross-validation
  version/          — single source of truth for App + ADR version strings
proto/mrmi/v1/      — protobuf contracts
configs/            — operator TOML configs
sdk/dotnet/         — .NET 10 SDK (MRMI.Gateway.Client NuGet package)
sdk/python/         — Python SDK (mrmi-gateway-sdk, PyPI)
demo/blazor/        — Blazor Server demo: split-screen RS/RU corridor
test/acceptance/    — end-to-end REST API acceptance tests
docs/               — ADR, sprint plans, operator guides
```

## Contributing

See [CONTRIBUTING.md](docs/CONTRIBUTING.md). Items open for external contributors: CLI reference client (Milestone 8), Extension API (Milestone 9), Java SDK (Milestone 10).

## License

[MIT](LICENSE) — Copyright (c) 2025 Božidar Tadić
