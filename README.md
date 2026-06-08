<p style="text-align: center;">
  <img src=".github/assets/mrmi-gateway-logo.png" alt="MRMI Gateway logo" style="width: 240px; max-width: 100%; height: auto;">
</p>

# MRMI Gateway

[![CI](https://github.com/BozidarTadic/MRMI_Gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/BozidarTadic/MRMI_Gateway/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](go.mod)

**Multi-Regional Multi-App Interlock** — open-source federation middleware that enforces legal-compliance constraints at the **transport layer** for regulated cross-border data corridors.

MRMI Gateway sits between applications and handles cross-border routing, policy enforcement, verifiable audit trails, and identity revocation — without ever touching the payload. Think Apache Kafka for cross-border regulated infrastructure: universal by design, with messaging as the primary and current focus.

> **v0.9 goal:** make MRMI universally adaptable — not only for messaging, but for any regulated domain (financial, healthcare, logistics). Messaging remains the production target and default domain.

> *"Legal compliance is not a deployment concern — it is an architectural constraint enforced at the transport layer."*

---

## Problem

Modern messaging apps operate across a fragmented regulatory landscape. Russia's **152-ФЗ**, the EU's **GDPR**, and Kazakhstan's data localisation laws impose strict requirements on where user data may reside and how it may cross borders. Existing federated protocols (Matrix, XMPP) treat cross-border data flow as an implementation concern — leaving operators to figure out compliance on their own.

MRMI Gateway makes compliance an architectural guarantee that operators cannot accidentally disable.

## Target Corridors (v0.1)

Primary: **RU · BY · KZ · AM · RS** — regions with similar data sovereignty concerns, existing commercial relationships, and no current cross-border protocol enforcement.

EU/US corridors: deferred to v1.0.

## Architecture

Starting with v0.9, MRMI is formally a **protocol specification**, not only a gateway implementation. The Go binary is the reference implementation — any language may implement a compatible node.

```
┌──────────────────────────────────────────────────────────┐
│  APPLICATION LAYER                                       │
│  Business logic, domain semantics, user-facing features  │
│  (Messaging app, Financial system, EHR, Logistics)       │
└────────────────────────┬─────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────┐
│  SCHEMA LAYER  (new in v0.9)                             │
│  Schema Registry — domain adapter registration & routing │
│  Adapters: messaging | iso20022 | hl7fhir | edifact      │
│  Gateway validates envelope; payload stays opaque        │
└────────────────────────┬─────────────────────────────────┘
                         │
┌────────────────────────▼─────────────────────────────────┐
│  TRANSPORT LAYER                                         │
│  Routing · Delivery · Policy · Audit · mTLS · Revocation │
│  Does NOT open payload — envelope only                   │
└──────────────────────────────────────────────────────────┘

App A (RU) ── SDK ── MRMI Node (RU) ══ gRPC/mTLS ══ MRMI Node (RS) ── SDK ── App B (RS)
```

Each node runs a Go binary. Nodes communicate over gRPC with mutual TLS. Every envelope is policy-checked, deduplicated via idempotency key, and appended to a Merkle audit log whose root hash is published to DNS TXT for independent verification.

Full architecture: [docs/MRMI_Gateway_ADR_v0_9.md](docs/MRMI_Gateway_ADR_v0_9.md) · [Protocol Specification](docs/PROTOCOL_SPEC.md)

## Key Properties

| Property | Mechanism |
|---|---|
| Cross-border policy enforcement | Signed TOML config with `applicable_law`, `allowed_regions`, `blocked_regions` |
| At-least-once delivery | Idempotency key + dedup index + ACK/retry |
| Verifiable audit | SHA-256 Merkle chain, root hash in DNS TXT + `/.well-known/mrmi-audit` |
| Identity trust | T0 (anonymous) → T3 (legal entity), revocable via CRL gossip |
| Traffic analysis resistance | Configurable timing jitter + payload padding per profile |
| Compliance profiles | `strict` / `balanced` / `performance` — maps to 152-ФЗ / GDPR / Kazakhstan |
| Schema Registry | `schema_type` + `schema_version` envelope fields; built-in adapters: `messaging`, `iso20022`, `hl7fhir`, `edifact`, `custom:*` |
| Jurisdiction isolation | Per-schema-type tier and jurisdiction restrictions independent of sender/recipient region |
| Financial corridors | ISO 20022 adapter with settlement finality, cutoff windows, BIC routing hint, 72h dedup TTL |
| Protocol independence | Any language may implement a compatible MRMI node — envelope contract is implementation-independent |

## Current Status — ADR v0.9 (v0.1.0-dev)

The core node runtime is complete and functional: policy engine, Merkle audit log, mTLS gRPC transport, REST management API, embedded dashboard, persistence backends (bbolt / Redis), federated peer discovery, transit cache, rate limiting, Prometheus metrics, and a local RS/RU demo corridor.

**v0.9 adds (in progress):** Schema Registry (ADR-015), `schema_type`/`schema_version` envelope fields, `jurisdiction_isolation` policy extension, messaging domain adapter, Protocol Specification document, and `.NET SDK` SchemaType enum. ISO 20022 financial adapter is scoped to v0.2.

Roadmap and contributor work are tracked in [GitHub Projects](https://github.com/BozidarTadic/MRMI_Gateway/projects).

---

## Quick Start

**Prerequisites:** Go 1.25+

```bash
git clone https://github.com/BozidarTadic/MRMI_Gateway
cd MRMI_Gateway
go run ./cmd/mrmi-gateway -config configs/node.balanced.toml
```

This starts the node on `:8080` (HTTP management API) and `:7777` (gRPC) with the balanced compliance profile.

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

**Start the same demo with .NET Aspire as the orchestrator:**

```powershell
dotnet run --project demo\aspire\MRMI.Demo.AppHost\MRMI.Demo.AppHost.csproj
```

Open the Aspire dashboard URL printed by the AppHost, then open `mrmi-demo-ui`. Details: [demo/aspire/README.md](demo/aspire/README.md).

---

## Configuration

Nodes are configured via TOML files. Three compliance profiles are available; the shipped configs use `balanced`:

| File | Purpose | TLS | Notes |
|---|---|---|---|
| `configs/node.balanced.toml` | Single RS node — starting point | plaintext | No peers; standalone |
| `configs/node.rs.local.toml` | RS node for local two-node corridor | `insecure = true` | Demo only |
| `configs/node.ru.local.toml` | RU node for local two-node corridor | `insecure = true` | Demo only |
| `configs/node.global.relay.toml` | Global relay template | mTLS (cert paths) | Production example |
| `configs/node.alliance.eaeu.toml` | EAEU alliance hub template | mTLS (cert paths) | Production example |

Profile definitions (dedup TTL, jitter, padding, dummy traffic rates) live in `internal/config/presets.go`. Full TOML reference in [docs/MRMI_Gateway_ADR_v0_9.md](docs/MRMI_Gateway_ADR_v0_9.md).

### Production Readiness Checklist

Before deploying a node in a production corridor, verify the following:

| Setting | Demo default | Production requirement |
|---|---|---|
| `[tls] insecure` | `true` | `false`; set `cert`, `key`, `ca` |
| `[node] signed_by` | `ed25519:REPLACE_ME` | Real public key; run `mrmi keygen --output <path>` |
| `[node] operator_id` | `example-operator` | Your organisation identifier |
| `[node] policy_version` | `0.1.0` | Increment on every policy change |
| `[api] api_key` | `demo-key` or unset | Secret key for management API write access |
| `[storage] backend` | in-memory (unset) | `bbolt` (single-node) or `redis` (clustered) |
| `[policy.audit] dns_txt_publish` | `true` | Requires a real DNS provider integration |
| `[network] metrics_addr` | `0.0.0.0:9090` | Restrict to internal network |

---

## Repository Layout

```
cmd/mrmi-gateway/   — node process entrypoint
cmd/mrmi/           — operator CLI (keygen, audit verify, node status)
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
  policy/           — policy engine (region allow/deny, trust tier, CRL, jurisdiction_isolation)
  schema/           — Schema Registry: adapter registration, schema_type routing, jurisdiction isolation
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
configs/            — operator TOML configs (demo and production templates)
sdk/dotnet/         — .NET 10 SDK (MRMI.Gateway.Client NuGet package)
sdk/python/         — Python SDK (mrmi-gateway-sdk, PyPI)
demo/blazor/        — Blazor Server demo: split-screen RS/RU corridor
test/acceptance/    — end-to-end REST API acceptance tests
docs/               — ADR v0.1–v0.9 (per-version files + index) + operator guides
```

## Contributing

See [CONTRIBUTING.md](docs/CONTRIBUTING.md). Open contributor work is tracked in [GitHub Projects](https://github.com/BozidarTadic/MRMI_Gateway/projects).

## License

[MIT](LICENSE) — Copyright (c) 2026 Božidar Tadić
