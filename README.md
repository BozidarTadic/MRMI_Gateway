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

Full architecture: [docs/ADR_v0.5.md](docs/ADR_v0.5.md)

## Key Properties

| Property | Mechanism |
|---|---|
| Cross-border policy enforcement | Signed TOML config with `applicable_law`, `allowed_regions`, `blocked_regions` |
| At-least-once delivery | Idempotency key + dedup index + ACK/retry |
| Verifiable audit | SHA-256 Merkle chain, root hash in DNS TXT + `/.well-known/mrmi-audit` |
| Identity trust | T0 (anonymous) → T3 (legal entity), revocable via CRL gossip |
| Traffic analysis resistance | Configurable timing jitter + payload padding per profile |
| Compliance profiles | `strict` / `balanced` / `performance` — maps to 152-ФЗ / GDPR / Kazakhstan |

## Current Status — v0.1 (Sprint 2 complete)

- [x] Protobuf contracts (`proto/mrmi/v1/contracts.proto`)
- [x] Config model — TOML parser, 3 profiles, `Config.Validate()`
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
- [ ] .NET SDK (Milestone 4)
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

Profile definitions (dedup TTL, jitter, padding, dummy traffic rates) live in `internal/config/presets.go`. Full TOML reference in [docs/ADR_v0.5.md — Appendix A](docs/ADR_v0.5.md#appendix-a--full-toml-configuration-examples).

## Repository Layout

```
cmd/mrmi-gateway/   — process entrypoint
internal/
  app/              — wiring: audit, policy, HTTP, gRPC, shutdown
  audit/            — Merkle chain log (SHA-256, Verify, RootHash)
  config/           — TOML parser, validation, profile presets
  dedup/            — idempotency key store with TTL + Purge
  integration/      — multi-node end-to-end tests
  policy/           — policy engine (region allow/deny, trust tier)
  server/           — HTTP endpoints (healthz, readyz, mrmi-audit)
  transport/grpc/   — gRPC server + client, JSON codec
proto/mrmi/v1/      — protobuf contracts
configs/            — operator TOML configs
docs/               — ADR, sprint plans, operator guides
```

## Contributing

See [CONTRIBUTING.md](docs/CONTRIBUTING.md). Items open for external contributors: CLI reference client (Milestone 8), Extension API (Milestone 9), Java SDK (Milestone 10).

## License

[MIT](LICENSE) — Copyright (c) 2025 Božidar Tadić
