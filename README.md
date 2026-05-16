<p align="center">
  <img src=".github/assets/mrmi-gateway-logo.png" alt="MRMI Gateway logo" width="240">
</p>

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

## Current Status — v0.4

MRMI Gateway includes the core node runtime, policy engine, Merkle audit log, mTLS gRPC transport, REST management API, SDKs, embedded dashboard, persistence backends, federated discovery, transit cache, rate limiting, and local RS/RU demo corridor.

Roadmap planning and contributor work are tracked in GitHub Projects instead of this README.

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

**Start the full local demo on Windows:**

```powershell
powershell -ExecutionPolicy Bypass -File scripts\demo-start.ps1
```

This starts the RS gateway on `:8080`, the RU gateway on `:8081`, and the Blazor demo UI on `http://localhost:5294`.

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
docs/               — ADR and operator guides
```

## Contributing

See [CONTRIBUTING.md](docs/CONTRIBUTING.md). Open contributor work is tracked in GitHub Projects.

## License

[MIT](LICENSE) — Copyright (c) 2025 Božidar Tadić
