# MRMI Gateway Python SDK

Official Python SDK for the [MRMI Gateway](https://github.com/BozidarTadic/MRMI_Gateway) — multi-regional messaging and financial corridor client.

**Version:** 0.4.0 · **Python:** ≥ 3.9 · **License:** MIT

---

## Installation

```bash
pip install mrmi-gateway-sdk
```

Or from source:

```bash
cd sdk/python
pip install -e ".[dev]"
```

---

## Quick start

```python
from mrmi_gateway import MrmiClient, MrmiClientOptions, SendEnvelopeRequest, SchemaType

with MrmiClient(MrmiClientOptions(
    base_url="https://your-node:8080",
    api_key="your-api-key",
)) as client:
    resp = client.send(SendEnvelopeRequest(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
        payload=b"hello world",
    ))
    print(resp.decision, resp.audit_root_hash)
```

---

## Schema types

The `schema_type` field declares the domain of the envelope payload. The gateway applies domain-specific policy (dedup TTL, audit retention, jurisdiction isolation) based on this value.

| Constant | Wire value | Description |
|---|---|---|
| `SchemaType.MESSAGING` | `"messaging"` | General cross-border messaging (default) |
| `SchemaType.ISO20022` | `"iso20022"` | ISO 20022 financial messages (ADR-016) |
| `SchemaType.HL7FHIR` | `"hl7fhir"` | HL7 FHIR health data |
| `SchemaType.EDIFACT` | `"edifact"` | UN/EDIFACT logistics |
| `SchemaType.custom("id")` | `"custom:id"` | Custom domain type |

```python
from mrmi_gateway import SchemaType, SendEnvelopeRequest

# Explicit schema type
req = SendEnvelopeRequest(
    idempotency_key="k1",
    sender_region="RS",
    recipient_region="RU",
    schema_type=SchemaType.ISO20022,
    schema_version="2019",
)

# Custom type
req = SendEnvelopeRequest(
    idempotency_key="k2",
    sender_region="RS",
    recipient_region="RU",
    schema_type=SchemaType.custom("my-domain"),
    schema_version="1.0.0",
)
```

---

## ISO 20022 financial messages

Use `Iso20022EnvelopeBuilder` to construct ISO 20022 envelopes with the correct defaults. It sets `schema_type="iso20022"`, `schema_version="1.0.0"`, and optionally a BIC routing hint.

The gateway automatically applies ADR-016 policy for `iso20022` envelopes:
- 72-hour dedup TTL (regardless of profile)
- `retain_long=true` audit flag (≥ 7-year retention)
- Settlement finality audit entry after successful delivery (when configured)
- Cutoff window enforcement per corridor

```python
from mrmi_gateway import MrmiClient, MrmiClientOptions
from mrmi_gateway.adapters import Iso20022EnvelopeBuilder

with MrmiClient(MrmiClientOptions(base_url="https://node:8080", api_key="key")) as client:
    req = Iso20022EnvelopeBuilder(
        idempotency_key="MSGID-20260602-001",
        sender_region="RS",
        recipient_region="RU",
        payload=xml_bytes,
        schema_version="2019",     # ISO 20022 message version
        routing_hint="BANKRS",     # Optional: BIC prefix for path optimisation
    ).build()

    resp = client.send(req)
    if resp.is_allowed:
        print("Delivered:", resp.peer_audit_root_hash)
```

### BIC routing hint

The `routing_hint` field accepts the first 6 characters of a BIC code. It is used by the gateway to prefer a matching peer when routing, but is never used for identity verification. Omitting it is the common case — the gateway will route by region as normal.

---

## Receiving envelopes (SSE)

```python
with MrmiClient(MrmiClientOptions(base_url="https://node:8080", api_key="key")) as client:
    for envelope in client.receive():
        print(f"From {envelope.sender_region}: {envelope.payload}")
```

---

## Authentication

**API key:**
```python
MrmiClientOptions(base_url="...", api_key="your-api-key")
```

**JWT bearer token:**
```python
MrmiClientOptions(base_url="...", jwt_token="your.jwt.token")
```

JWT takes precedence over API key when both are provided. Issue short-lived JWT tokens programmatically:

```python
issued = client.issue_token(scope="operator", ttl_minutes=30)
# use issued.token as jwt_token in subsequent clients
```

---

## Node status and audit

```python
status = client.get_status()
print(status.node_id, status.profile, status.uptime_seconds)

entries = client.get_audit_latest(count=50)
for e in entries:
    print(e.seq, e.decision, e.sender_region, "→", e.recipient_region)
```

---

## Dead-letter queue (DLQ)

```python
entries = client.get_dlq_entries()
for e in entries:
    print(f"[{e.index}] {e.envelope_id} — {e.reason or 'exhausted'}")
    if e.next_open_unix:
        print(f"  Next window opens at unix ms: {e.next_open_unix}")

# Replay an entry
result = client.replay_dlq_entry(index=0)
print(result.decision)

# Discard an entry
client.remove_dlq_entry(index=0)
```

ISO 20022 envelopes rejected because they arrived outside their processing window are queued with `reason="outside_cutoff_window"` and `next_open_unix` set to the millisecond timestamp of the next window opening.

---

## Discovery and connect

```python
results = client.discover("Marko Petrović")
for r in results:
    conn = client.connect(r.opaque_token, requester_id="my-user", requester_region="RU")
    if conn.is_accepted:
        print("Session:", conn.session_id)
```

---

## Running tests

```bash
cd sdk/python
pip install -e ".[dev]"
pytest tests/ -v
```
