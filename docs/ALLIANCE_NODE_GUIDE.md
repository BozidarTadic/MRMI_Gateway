# Alliance and Global Node Operator Guide

This guide covers deploying an MRMI Gateway node with `node_scope = "alliance"` or `node_scope = "global"`.  
Regional operators who only need to peer with a single neighbouring country can follow the [Seed Node Guide](SEED_NODE_GUIDE.md) instead.

---

## Node scope overview

| Scope | Serves | Use case |
|---|---|---|
| `regional` | One country / legal zone | Typical operator node |
| `alliance` | A named group of countries | EAEU, ASEAN, SCO corridor hubs |
| `global` | Any region not covered by a regional or alliance peer | Last-resort relay |

Tier preference for envelope forwarding: **Regional → Alliance → Global**.  
A global node is contacted only when no regional or alliance peer can be reached.

---

## Prerequisites

Same as the [Seed Node Guide](SEED_NODE_GUIDE.md):

| Component | Minimum |
|---|---|
| Go | 1.21 |
| Linux | Ubuntu 22.04 / Debian 12 |
| Open ports | 8080 (HTTP), 7777 (gRPC) |
| TLS certificate | Signed by your PKI CA |

---

## Alliance node

### 1. Config

```toml
[node]
node_id        = "eaeu-hub-01"
node_scope     = "alliance"
regions        = ["BY", "KZ", "AM", "KG", "TJ"]  # regions this node covers
alliance_id    = "eaeu-2014"                       # legal agreement reference
operator_id    = "eaeu-clearinghouse"
policy_version = "0.4.0"
applicable_law = "EAEU-PDL"
signed_by      = "ed25519:YOUR_PUBLIC_KEY_HEX"

[profile]
name = "balanced"

[policy.outbound]
allow_to      = ["RS", "RU", "CN", "IR"]   # cross-alliance corridors
deny_to       = []
store_locally = true

[policy.inbound]
min_trust_tier = 1

[policy.routing]
allow_via = ["regional", "alliance"]   # never forward via a global node

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
dns_txt_publish     = true
dns_txt_interval_s  = 3600
https_well_known    = true

[network]
grpc_listen_addr = "0.0.0.0:7777"
http_listen_addr = "0.0.0.0:8080"

[network.peers]
  [network.peers.RS]
  addr       = "rs-seed.example.com:7777"
  node_scope = "regional"

  [network.peers.RU]
  addr       = "ru-seed.example.com:7777"
  node_scope = "regional"

[tls]
cert     = "/etc/mrmi/node.crt"
key      = "/etc/mrmi/node.key"
ca       = "/etc/mrmi/ca.crt"
insecure = false

[storage]
backend = "bbolt"
path    = "/var/lib/mrmi"
```

**Required fields** for an alliance node:

| Field | Purpose |
|---|---|
| `node_scope = "alliance"` | Signals tier in peer selection |
| `regions` | List of country codes this hub covers; peers use this to route to you |
| `alliance_id` | Human-readable reference to the legal agreement (auditors check this) |

### 2. Register this node as an alliance peer on regional nodes

On each regional node within the alliance, add a peer entry referencing this hub:

```toml
[network.peers]
  [network.peers.eaeu-hub-01]
  addr       = "eaeu-hub-01.example.com:7777"
  node_scope = "alliance"
  regions    = ["BY", "KZ", "AM", "KG", "TJ"]
```

The forwarder will use this hub as a fallback when no direct regional peer is available for a destination within `regions`.

---

## Global node

A global node has no `regions` constraint — it accepts envelopes destined for any region and forwards them onward.  
**Important**: global nodes make no data-residency guarantees. Configure `disclaimer` and communicate this clearly to connected operators.

### 1. Config

```toml
[node]
node_id        = "global-relay-01"
node_scope     = "global"
region         = ""                         # no single region
disclaimer     = "no-data-residency-claims"
operator_id    = "global-ops"
policy_version = "0.4.0"
applicable_law = "NONE"
signed_by      = "ed25519:YOUR_PUBLIC_KEY_HEX"

[profile]
name = "performance"   # global relays prioritise throughput; no jitter or padding

[policy.outbound]
allow_to      = []   # empty = allow all; set explicit deny_to for embargoed regions
deny_to       = []
store_locally = false

[policy.inbound]
min_trust_tier = 0

[policy.audit]
log_all_decisions   = true
log_backend         = "local-merkle"
dns_txt_publish     = false   # global nodes typically don't publish DNS TXT records
https_well_known    = true

[network]
grpc_listen_addr = "0.0.0.0:7777"
http_listen_addr = "0.0.0.0:8080"

[tls]
cert     = "/etc/mrmi/node.crt"
key      = "/etc/mrmi/node.key"
ca       = "/etc/mrmi/ca.crt"
insecure = false

[storage]
backend = "bbolt"
path    = "/var/lib/mrmi"
```

**Required fields** for a global node:

| Field | Purpose |
|---|---|
| `node_scope = "global"` | Marks node as last-resort relay |
| `disclaimer` | Declares the data-residency posture to connecting peers |
| `applicable_law = "NONE"` | Explicitly acknowledges no jurisdiction-specific law applies |

### 2. Register this node as a global peer on regional/alliance nodes

On any node that wants global fallback coverage:

```toml
[network.peers]
  [network.peers.global-relay-01]
  addr       = "global-relay-01.example.com:7777"
  node_scope = "global"
```

To restrict forwarding so envelopes never traverse a global node, set `allow_via` on the sending node:

```toml
[policy.routing]
allow_via = ["regional", "alliance"]
```

---

## Transit cache interaction

Both alliance and global nodes benefit from the transit cache (enabled by default on `balanced` and `strict` profiles).  
When a peer is temporarily unreachable, envelopes are held in memory for up to 60 s before DLQ promotion.  
On global nodes running `performance` profile the transit cache is disabled (`transit_cache_ttl_s = 0`).

To enable it explicitly on `performance`-profile global nodes:

```toml
[profile]
name                = "performance"
transit_cache_ttl_s = 30
```

---

## Discovery rate limiting

All nodes enforce a per-origin-node token bucket on `BroadcastDiscovery` RPCs: **10 req/s, burst 20**.  
Alliance and global nodes receive traffic from many regional peers simultaneously; they apply the same limit per `OriginNodeID`.  
Callers that exceed the limit receive `RESOURCE_EXHAUSTED` and should back off exponentially.

---

## Monitoring

All nodes expose Prometheus metrics on `:9090` (configurable via `metrics_addr`).  
Recommended alerts for alliance/global nodes:

| Metric | Alert condition |
|---|---|
| `mrmi_dlq_size` | > 100 entries for > 5 minutes |
| `mrmi_transit_cache_len` | > 500 entries for > 1 minute |
| `mrmi_grpc_rpc_duration_seconds{method="BroadcastDiscovery"}` | p99 > 1 s |
| `mrmi_audit_chain_length` | Stops growing for > 10 minutes under load |

---

## Example topology

```
  RS-node-01 (regional)
       │
       ├── direct peer ──► RU-node-01 (regional)
       │
       └── alliance fallback ──► EAEU-hub-01 (alliance: BY, KZ, AM, KG, TJ)
                                      │
                                      └── global fallback ──► global-relay-01
```
