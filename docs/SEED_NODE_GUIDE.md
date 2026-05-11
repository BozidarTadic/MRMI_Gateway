# Seed Node Deployment Guide

This guide walks through deploying an MRMI Gateway seed node — a publicly reachable Regional node that other operators can federate with.

---

## Prerequisites

| Component | Minimum version |
|---|---|
| Go | 1.21 |
| Linux (recommended) | Ubuntu 22.04 / Debian 12 |
| Open ports | 8080 (HTTP), 7777 (gRPC) |
| DNS | A record for your node's FQDN |

---

## 1. Build the binary

```bash
git clone https://github.com/tadicbb/mrmi-gateway
cd mrmi-gateway
go build -o /usr/local/bin/mrmi-gateway ./cmd/mrmi-gateway
go build -o /usr/local/bin/mrmi           ./cmd/mrmi
```

Verify:

```bash
mrmi-gateway -version
mrmi -version
```

---

## 2. Generate Ed25519 signing keys

Each node needs a persistent Ed25519 signing key. The private key signs envelopes forwarded to peers; the public key is shared out-of-band for peer verification.

```bash
# Create key directory
mkdir -p /etc/mrmi

# Generate key pair — private key written to file, public key printed to stdout
mrmi keygen --output /etc/mrmi/node.key > /etc/mrmi/node.pub

chmod 600 /etc/mrmi/node.key
chmod 644 /etc/mrmi/node.pub
```

---

## 3. TLS certificates

Inter-node gRPC uses mTLS (TLS 1.3 minimum). For a seed node you need:

- `server.crt` + `server.key` — the node's certificate and private key
- `ca.crt` — the CA certificate peers use to verify your node

**Self-signed (development only):**

```bash
# Generate CA
openssl req -x509 -newkey ed25519 -keyout /etc/mrmi/ca.key -out /etc/mrmi/ca.crt \
  -days 3650 -nodes -subj "/CN=mrmi-ca"

# Generate node cert signed by CA
openssl req -newkey ed25519 -keyout /etc/mrmi/tls.key -out /etc/mrmi/tls.csr \
  -nodes -subj "/CN=rs.example.com"
openssl x509 -req -in /etc/mrmi/tls.csr -CA /etc/mrmi/ca.crt -CAkey /etc/mrmi/ca.key \
  -CAcreateserial -out /etc/mrmi/tls.crt -days 365
```

**Production:** Use certificates from your CA of choice (Let's Encrypt, internal PKI). The node validates peer certificates against `tls.ca_cert`.

---

## 4. Write the node config

Create `/etc/mrmi/node.toml`:

```toml
[node]
node_id        = "rs-seed-01"
node_scope     = "regional"
region         = "RS"
operator_id    = "your-org"
policy_version = "v1"
applicable_law = "RS-GDPR"
# Ed25519 public key of this node, used by peers to verify signatures.
# Generate with: mrmi keygen --output /etc/mrmi/node.key > /etc/mrmi/node.pub
signed_by      = "ed25519:YOUR_PUBLIC_KEY_BASE64_HERE"

[network]
http_listen_addr = ":8080"
grpc_listen_addr = ":7777"
shutdown_timeout_s = 30

# Add peer nodes here. Each peer must have this node listed as a peer too.
# [network.peers]
# [network.peers.ru-seed-01]
# addr      = "ru.example.com:7777"
# tier      = "regional"
# region    = "RU"

[tls]
cert     = "/etc/mrmi/tls.crt"
key      = "/etc/mrmi/tls.key"
ca_cert  = "/etc/mrmi/ca.crt"
insecure = false

[policy]
profile = "balanced"

[policy.outbound]
allow_to = ["RU", "BY", "KZ", "AM"]

[policy.audit]
dns_txt_publish   = true
dns_txt_interval_s = 300
https_well_known  = true
root_hash_gossip  = true
```

Adjust `region`, `allow_to`, and peer list for your corridor.

---

## 5. Validate the config

```bash
mrmi-gateway -config /etc/mrmi/node.toml -validate
```

If the binary does not have `-validate`, do a dry-run and kill it after readyz passes:

```bash
mrmi-gateway -config /etc/mrmi/node.toml &
sleep 2 && curl -sf http://localhost:8080/readyz && kill %1
```

---

## 6. systemd unit file

Create `/etc/systemd/system/mrmi-gateway.service`:

```ini
[Unit]
Description=MRMI Gateway node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=mrmi
Group=mrmi
ExecStart=/usr/local/bin/mrmi-gateway -config /etc/mrmi/node.toml
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/mrmi

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
useradd -r -s /sbin/nologin mrmi
chown -R mrmi:mrmi /etc/mrmi
systemctl daemon-reload
systemctl enable mrmi-gateway
systemctl start  mrmi-gateway
systemctl status mrmi-gateway
```

---

## 7. Firewall

Allow the two node ports:

```bash
# ufw
ufw allow 8080/tcp comment "MRMI HTTP"
ufw allow 7777/tcp comment "MRMI gRPC"

# firewalld
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --permanent --add-port=7777/tcp
firewall-cmd --reload
```

---

## 8. DNS TXT verification

After the first `dns_txt_interval_s` seconds, the node emits the audit root hash to stdout (and optionally to your DNS provider). Publish the emitted record at `_mrmi-audit.<node_id>.<your-domain>`.

Expected format:

```
v=1 ts=<unix> root=sha256:<hex> node=<node_id> law=<applicable_law>
```

Verify from any host:

```bash
mrmi audit verify --dns --node rs-seed-01.example.com
```

---

## 9. HTTPS audit endpoint verification

With `https_well_known = true`, the node serves a signed audit response:

```bash
curl -s https://rs.example.com:8080/.well-known/mrmi-audit | jq .
```

Verify the Ed25519 signature:

```bash
mrmi audit verify --https --url https://rs.example.com:8080/.well-known/mrmi-audit \
                  --pubkey /etc/mrmi/node.pub
```

---

## 10. Health check procedure

| Check | Command | Expected |
|---|---|---|
| HTTP liveness | `curl http://localhost:8080/healthz` | `ok` |
| HTTP readiness | `curl http://localhost:8080/readyz` | `ready` |
| Node status | `curl http://localhost:8080/api/v1/status \| jq .` | JSON with `node_id` |
| Audit root | `curl http://localhost:8080/.well-known/mrmi-audit \| jq .root_hash` | `sha256:...` |
| Peer audit roots | `curl http://localhost:8080/peers/audit \| jq .` | JSON map of peer hashes |
| DLQ | `curl http://localhost:8080/api/v1/dlq \| jq length` | 0 on a healthy node |

---

## 11. Monitoring recommendations

- **Uptime**: scrape `/healthz` every 30 seconds; alert if non-200 for 2 consecutive checks.
- **DLQ depth**: `GET /api/v1/dlq` — alert when count > 0 for more than 5 minutes.
- **Audit root drift**: compare `root_hash` from `/.well-known/mrmi-audit` against DNS TXT record at each `dns_txt_interval_s` tick.
- **Peer gossip**: `GET /peers/audit` — alert if any peer hash is more than 2 × `dns_txt_interval_s` seconds old.

---

## Upgrading

```bash
# Build new binary
go build -o /tmp/mrmi-gateway-new ./cmd/mrmi-gateway

# Atomic swap (systemd will restart on failure)
mv /tmp/mrmi-gateway-new /usr/local/bin/mrmi-gateway
systemctl restart mrmi-gateway
```

Hot-reload applies to policy config changes only — a binary upgrade requires a restart (typically < 1 second with the balanced profile).
