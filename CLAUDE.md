# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./cmd/mrmi-gateway      # gateway binary
go build ./cmd/mrmi              # CLI tool

# Test
go test ./...                    # all tests
go test ./internal/core/...      # single package
go test -run TestName ./...      # single test

# Integration tests (require real network/FS)
go test -tags integration ./internal/integration/...

# Lint / vet
go vet ./...

# Regenerate protobuf (requires protoc + protoc-gen-go + protoc-gen-go-grpc on $PATH)
make proto
```

## Architecture

The codebase follows a strict layered architecture. The dependency flow is:

```
cmd/mrmi-gateway  →  internal/app  →  internal/core  →  internal/{domain}
                                   →  internal/transport/grpc  (adapter only)
                                   →  internal/server           (HTTP REST + SSE)
```

**`internal/app/app.go`** is the single wiring point. It constructs every component and starts the gRPC server and HTTP server. No business logic lives here — only construction, lifecycle, and goroutine management.

**`internal/core/`** owns the canonical domain types (`Envelope`, `Decision`, `NodeInfo`) and the `Gateway` use-case struct (`SendEnvelope`, `GetNodeInfo`). This package must never import proto, gRPC, or HTTP packages.

**`internal/transport/grpc/`** is a pure adapter: `adapter.go` translates proto↔core types; `server.go` and `client.go` manage gRPC lifecycle; `service.go` defines the `GatewayService` interface that `adapter.go` satisfies. The adapter receives a `GatewayService` interface — never imports `internal/core` directly.

**`internal/server/http.go`** serves the management REST API (JWT-authenticated) and embeds the dashboard SPA from `web/`. JWT is HMAC-SHA256; scopes are `read` and `operator`.

**`internal/policy/engine.go`** evaluates allow/deny decisions using the TOML config (`allowed_regions`, `blocked_regions`, `min_trust_tier`). Hot-reload is handled by `internal/hotreload/` calling `engine.Reload()`.

## Key cross-cutting flows

**Envelope send path:**  
`gRPC inbound → adapter → Gateway.SendEnvelope → policy.Engine.Evaluate → audit.Log.Append → delivery.Forwarder.Forward → gRPC outbound (with jitter + padding)`

**Delivery fallback:**  
`Forwarder` tries peers in tier-preference order (Regional → Alliance → Global). On failure it writes to `internal/transit/` (LRU+TTL, ≤60 s). A background ticker drains the transit cache into `delivery.DLQ`.

**Dynamic peer discovery (Sprint 7–8):**  
`BroadcastDiscovery` RPC fans out via `internal/discovery/Broadcaster` with hop-limited dedup. `internal/peerdiscovery/Registry` holds the live peer table, seeded from static config and updated via `ExchangePeers` gossip every `peer_gossip_interval_s`. Rate limiting on `BroadcastDiscovery`: 10 req/s, burst 20 per origin node (`internal/ratelimit/`).

**Federated discovery connect flow:**  
`BroadcastDiscovery` → `internal/discovery/` → fan-out to peers → `ConnectRequest` RPC → `internal/connect/Resolver` applies `AUTO_MUTUAL` / `AUTO_WHITELIST` / `AUTO_ALL` / `MANUAL` modes → `internal/token/` issues single-use SHA-256 opaque tokens.

**Persistent storage:**  
`internal/store/store.go` defines the `NodeStore` interface. Backends: `store/bbolt/` (embedded, single-node) and `store/redis/` (clustered). Default is in-memory. Selected via `[storage] backend =` in TOML.

## Config

TOML config is loaded by `internal/config/config.go`. Profiles (`strict` / `balanced` / `performance`) are presets in `config/presets.go` that set `TimingJitterMax`, `PaddingBucket`, `DedupTTL`, and `TransitCacheTTL`. Extend config by adding fields to the raw struct, mapping them in `apply()`, adding a `Validate()` case, and updating `configs/` examples. See `CODE_STANDARDS.md` § Config extension guide.

## Testing conventions

- Test servers always bind on `:0`; capture the address via `srv.Addr()`.  
- Integration tests use `//go:build integration` as the first line.  
- Acceptance tests live in `test/acceptance/` and wire components directly (no subprocess); see `startNodeV3` in `sprint8_test.go` as the canonical helper pattern.  
- Test certs are generated in-process via `internal/testcerts/`; never committed and never `openssl`.

## Proto

Source of truth is `proto/mrmi/v1/contracts.proto`. Generated files (`*.pb.go`, `*_grpc.pb.go`) are committed. To add a field: edit the proto → `make proto` → update consuming Go code. Never edit generated files by hand.

## Module name

The Go module is `MRMI_Gateway` (not a URL). Import paths are `MRMI_Gateway/internal/…`. Import `internal/transport/grpc` aliased as `grpctransport` everywhere.
