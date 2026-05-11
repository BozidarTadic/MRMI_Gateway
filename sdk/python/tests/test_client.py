"""Unit tests for MrmiClient using respx to mock HTTP calls."""
from __future__ import annotations

import base64
import json

import httpx
import pytest
import respx

from mrmi_gateway import (
    MrmiClient,
    MrmiClientOptions,
    RegisterAppRequest,
    AutoAcceptMode,
    SendEnvelopeRequest,
    DiscoveryQueryType,
)


BASE = "http://localhost:8080"


def make_client(api_key: str | None = None) -> MrmiClient:
    return MrmiClient(MrmiClientOptions(base_url=BASE, api_key=api_key))


# ── send ──────────────────────────────────────────────────────────────────────

@respx.mock
def test_send_returns_allow():
    respx.post(f"{BASE}/api/v1/envelopes").mock(return_value=httpx.Response(
        200,
        json={
            "decision": "ALLOW",
            "reason": "POLICY_ACCEPTED",
            "profile": "balanced",
            "node_id": "rs-node",
            "audit_root_hash": "sha256:abc",
            "peer_audit_root_hash": "",
        },
    ))
    with make_client() as client:
        result = client.send(SendEnvelopeRequest(
            idempotency_key="test-001",
            sender_region="RS",
            recipient_region="RU",
        ))
    assert result.decision == "ALLOW"
    assert result.is_allowed
    assert result.node_id == "rs-node"


@respx.mock
def test_send_raises_on_error():
    respx.post(f"{BASE}/api/v1/envelopes").mock(
        return_value=httpx.Response(400, text="idempotency_key required")
    )
    with make_client() as client:
        with pytest.raises(httpx.HTTPStatusError):
            client.send(SendEnvelopeRequest(
                idempotency_key="",
                sender_region="RS",
                recipient_region="RU",
            ))


@respx.mock
def test_send_serialises_payload():
    captured: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return httpx.Response(200, json={
            "decision": "ALLOW", "reason": "", "profile": "",
            "node_id": "", "audit_root_hash": "", "peer_audit_root_hash": "",
        })

    respx.post(f"{BASE}/api/v1/envelopes").mock(side_effect=capture)
    payload = b"\x01\x02\x03"
    with make_client() as client:
        client.send(SendEnvelopeRequest(
            idempotency_key="k1",
            sender_region="RS",
            recipient_region="RU",
            payload=payload,
        ))
    assert captured["body"]["payload"] == base64.b64encode(payload).decode()


# ── get_status ────────────────────────────────────────────────────────────────

@respx.mock
def test_get_status_parses_all_fields():
    respx.get(f"{BASE}/api/v1/status").mock(return_value=httpx.Response(
        200,
        json={
            "node_id": "rs-01",
            "region": "RS",
            "node_scope": "regional",
            "profile": "balanced",
            "applicable_law": "RS-GDPR",
            "app_version": "0.2.0",
            "adr_version": "0.8",
            "uptime_seconds": 42,
        },
    ))
    with make_client() as client:
        status = client.get_status()
    assert status.node_id == "rs-01"
    assert status.region == "RS"
    assert status.uptime_seconds == 42
    assert status.app_version == "0.2.0"


# ── get_audit_latest ──────────────────────────────────────────────────────────

@respx.mock
def test_get_audit_latest_returns_list():
    respx.get(url__regex=rf"{BASE}/api/v1/audit/latest.*").mock(return_value=httpx.Response(
        200,
        json=[{
            "seq": 1, "timestamp": 1000, "decision": "ALLOW",
            "reason": "POLICY_ACCEPTED", "trust_tier": 1,
            "sender_region": "RS", "recipient_region": "RU",
            "policy_version": "v1", "profile": "balanced", "entry_hash": "sha256:x",
        }],
    ))
    with make_client() as client:
        entries = client.get_audit_latest()
    assert len(entries) == 1
    assert entries[0].decision == "ALLOW"
    assert entries[0].sender_region == "RS"


# ── DLQ ───────────────────────────────────────────────────────────────────────

@respx.mock
def test_get_dlq_entries_returns_list():
    respx.get(f"{BASE}/api/v1/dlq").mock(return_value=httpx.Response(
        200,
        json=[{
            "index": 0, "peer_addr": "localhost:7777", "attempts": 3,
            "last_error": "dial timeout", "first_seen_unix": 1000,
            "last_attempt_unix": 2000, "envelope_id": "env-01",
            "sender_region": "RS", "recipient_region": "RU",
        }],
    ))
    with make_client() as client:
        entries = client.get_dlq_entries()
    assert len(entries) == 1
    assert entries[0].attempts == 3
    assert entries[0].last_error == "dial timeout"


@respx.mock
def test_remove_dlq_entry_succeeds():
    respx.delete(f"{BASE}/api/v1/dlq/0").mock(
        return_value=httpx.Response(204)
    )
    with make_client() as client:
        client.remove_dlq_entry(0)  # must not raise


@respx.mock
def test_replay_dlq_entry_returns_result():
    respx.post(f"{BASE}/api/v1/dlq/0/replay").mock(return_value=httpx.Response(
        200, json={"decision": "ALLOW", "reason": "POLICY_ACCEPTED"},
    ))
    with make_client() as client:
        result = client.replay_dlq_entry(0)
    assert result.decision == "ALLOW"
    assert result.is_allowed


# ── CRL ───────────────────────────────────────────────────────────────────────

@respx.mock
def test_get_crl_entries_returns_list():
    respx.get(f"{BASE}/api/v1/crl").mock(return_value=httpx.Response(
        200,
        json=[{
            "node_id": "bad-node", "reason": "compromised",
            "sig_count": 2, "is_effective": True, "revoked_at_unix": 9999,
        }],
    ))
    with make_client() as client:
        entries = client.get_crl_entries()
    assert len(entries) == 1
    assert entries[0].is_effective
    assert entries[0].node_id == "bad-node"


@respx.mock
def test_publish_revocation_signature_posts_correct_payload():
    captured: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return httpx.Response(200)

    respx.post(f"{BASE}/api/v1/crl").mock(side_effect=capture)
    sig = bytes([1, 2, 3])
    with make_client() as client:
        client.publish_revocation_signature("n1", "key compromise", sig)

    assert captured["body"]["node_id"] == "n1"
    assert captured["body"]["reason"] == "key compromise"
    assert captured["body"]["signature_b64"] == base64.b64encode(sig).decode()


# ── discover ──────────────────────────────────────────────────────────────────

@respx.mock
def test_discover_returns_results():
    respx.get(url__regex=rf"{BASE}/api/v1/discover.*").mock(return_value=httpx.Response(
        200,
        json=[{
            "node_id": "rs-01", "app_id": "rs-app", "user_id": "user-marko",
            "display_hint": "Marko Petrović", "region": "RS",
            "opaque_token": "tok-abc", "token_expires": 9999999,
        }],
    ))
    with make_client() as client:
        results = client.discover("marko")
    assert len(results) == 1
    assert results[0].user_id == "user-marko"
    assert results[0].opaque_token == "tok-abc"


@respx.mock
def test_discover_app_id_sends_correct_type():
    captured_url: list[str] = []

    def capture(request: httpx.Request) -> httpx.Response:
        captured_url.append(str(request.url))
        return httpx.Response(200, json=[])

    respx.get(url__regex=rf"{BASE}/api/v1/discover.*").mock(side_effect=capture)
    with make_client() as client:
        client.discover("rs-app", DiscoveryQueryType.APP_ID)

    assert "type=app_id" in captured_url[0]


@respx.mock
def test_discover_empty_returns_empty_list():
    respx.get(url__regex=rf"{BASE}/api/v1/discover.*").mock(
        return_value=httpx.Response(200, json=[])
    )
    with make_client() as client:
        results = client.discover("nobody")
    assert results == []


# ── connect ───────────────────────────────────────────────────────────────────

@respx.mock
def test_connect_returns_accepted():
    respx.post(f"{BASE}/api/v1/connect").mock(return_value=httpx.Response(
        200, json={"status": "ACCEPTED", "session_id": "sess-001", "expires_at": 9999999},
    ))
    with make_client() as client:
        result = client.connect("tok-abc", "ru-user", "RU")
    assert result.status == "ACCEPTED"
    assert result.is_accepted
    assert result.session_id == "sess-001"


@respx.mock
def test_connect_returns_pending():
    respx.post(f"{BASE}/api/v1/connect").mock(return_value=httpx.Response(
        200, json={"status": "PENDING"},
    ))
    with make_client() as client:
        result = client.connect("tok-xyz", "ru-user", "US")
    assert result.status == "PENDING"
    assert not result.is_accepted


# ── api key header ────────────────────────────────────────────────────────────

@respx.mock
def test_api_key_sent_as_header():
    captured_headers: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured_headers.update(dict(request.headers))
        return httpx.Response(200, json={
            "node_id": "", "region": "", "node_scope": "",
            "profile": "", "applicable_law": "",
            "app_version": "", "adr_version": "", "uptime_seconds": 0,
        })

    respx.get(f"{BASE}/api/v1/status").mock(side_effect=capture)
    with make_client(api_key="secret-key") as client:
        client.get_status()

    assert captured_headers.get("x-mrmi-key") == "secret-key"


# ── JWT auth header (v0.3) ────────────────────────────────────────────────────

@respx.mock
def test_jwt_token_sent_as_bearer_header():
    captured_headers: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured_headers.update(dict(request.headers))
        return httpx.Response(200, json={
            "node_id": "", "region": "", "node_scope": "",
            "profile": "", "applicable_law": "",
            "app_version": "", "adr_version": "", "uptime_seconds": 0,
        })

    respx.get(f"{BASE}/api/v1/status").mock(side_effect=capture)
    opts = MrmiClientOptions(base_url=BASE, jwt_token="my.jwt.token")
    with MrmiClient(opts) as client:
        client.get_status()

    assert captured_headers.get("authorization") == "Bearer my.jwt.token"


@respx.mock
def test_jwt_takes_precedence_over_api_key():
    captured_headers: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured_headers.update(dict(request.headers))
        return httpx.Response(200, json={
            "node_id": "", "region": "", "node_scope": "",
            "profile": "", "applicable_law": "",
            "app_version": "", "adr_version": "", "uptime_seconds": 0,
        })

    respx.get(f"{BASE}/api/v1/status").mock(side_effect=capture)
    opts = MrmiClientOptions(base_url=BASE, api_key="k", jwt_token="my.jwt.token")
    with MrmiClient(opts) as client:
        client.get_status()

    assert captured_headers.get("authorization") == "Bearer my.jwt.token"
    assert "x-mrmi-key" not in captured_headers


# ── issue_token (v0.3) ────────────────────────────────────────────────────────

@respx.mock
def test_issue_token_returns_issued_token():
    respx.post(f"{BASE}/api/v1/token").mock(return_value=httpx.Response(
        200, json={"token": "signed.jwt.here", "scope": "operator", "expires_at": 9999999},
    ))
    with make_client(api_key="secret") as client:
        issued = client.issue_token(scope="operator", ttl_minutes=30)
    assert issued.token == "signed.jwt.here"
    assert issued.scope == "operator"
    assert issued.expires_at == 9999999


@respx.mock
def test_issue_token_raises_on_error():
    respx.post(f"{BASE}/api/v1/token").mock(
        return_value=httpx.Response(503, text="JWT not configured")
    )
    with make_client(api_key="secret") as client:
        with pytest.raises(httpx.HTTPStatusError):
            client.issue_token()


# ── app management (v0.3) ─────────────────────────────────────────────────────

@respx.mock
def test_list_apps_returns_list():
    respx.get(f"{BASE}/api/v1/apps").mock(return_value=httpx.Response(
        200,
        json=[{"app_id": "my-app", "webhook_url": "https://example.com/hook", "auto_accept": "manual"}],
    ))
    with make_client(api_key="secret") as client:
        apps = client.list_apps()
    assert len(apps) == 1
    assert apps[0].app_id == "my-app"
    assert apps[0].webhook_url == "https://example.com/hook"


@respx.mock
def test_register_app_returns_app_info():
    captured: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return httpx.Response(200, json={
            "app_id": "new-app", "webhook_url": "", "auto_accept": "manual",
        })

    respx.post(f"{BASE}/api/v1/apps/register").mock(side_effect=capture)
    req = RegisterAppRequest(app_id="new-app", auto_accept=AutoAcceptMode.AUTO_ALL)
    with make_client(api_key="secret") as client:
        app = client.register_app(req)

    assert app.app_id == "new-app"
    assert captured["body"]["app_id"] == "new-app"
    assert captured["body"]["auto_accept"] == "auto_all"


@respx.mock
def test_delete_app_succeeds():
    respx.delete(f"{BASE}/api/v1/apps/my-app").mock(
        return_value=httpx.Response(204)
    )
    with make_client(api_key="secret") as client:
        client.delete_app("my-app")  # must not raise


@respx.mock
def test_delete_app_raises_on_not_found():
    respx.delete(f"{BASE}/api/v1/apps/missing").mock(
        return_value=httpx.Response(404, text="app not found")
    )
    with make_client(api_key="secret") as client:
        with pytest.raises(httpx.HTTPStatusError):
            client.delete_app("missing")
