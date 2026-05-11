"""MRMI Gateway Python client."""
from __future__ import annotations

import base64
import json
from collections.abc import Generator, Iterator
from dataclasses import dataclass
from typing import Callable, Optional

import httpx

from .models import (
    AppInfo,
    AuditEntry,
    AutoAcceptMode,
    ConnectResult,
    CrlEntry,
    DiscoveryQueryType,
    DiscoveryResult,
    DlqEntry,
    IssuedToken,
    NodeStatusResponse,
    ReceivedEnvelope,
    RegisterAppRequest,
    ReplayResult,
    SendEnvelopeRequest,
    SendEnvelopeResponse,
)


@dataclass
class MrmiClientOptions:
    base_url: str
    api_key: Optional[str] = None
    jwt_token: Optional[str] = None  # Bearer JWT; takes precedence over api_key
    timeout: float = 10.0


class MrmiClient:
    """Thread-safe HTTP client for the MRMI Gateway management REST API."""

    def __init__(self, options: MrmiClientOptions) -> None:
        self._base = options.base_url.rstrip("/")
        self._timeout = options.timeout
        headers: dict[str, str] = {}
        if options.jwt_token:
            headers["Authorization"] = f"Bearer {options.jwt_token}"
        elif options.api_key:
            headers["X-MRMI-Key"] = options.api_key
        self._http = httpx.Client(headers=headers, timeout=options.timeout)

    # ── Envelope send ─────────────────────────────────────────────────────────

    def send(self, request: SendEnvelopeRequest) -> SendEnvelopeResponse:
        """Send an envelope through the gateway."""
        resp = self._http.post(f"{self._base}/api/v1/envelopes", json=request.to_dict())
        resp.raise_for_status()
        return SendEnvelopeResponse.from_dict(resp.json())

    # ── SSE receive ───────────────────────────────────────────────────────────

    def receive(self) -> Generator[ReceivedEnvelope, None, None]:
        """
        Connect to the gateway SSE stream and yield ReceivedEnvelope objects.
        Blocks until the connection is closed or the caller breaks out.

        Usage::

            for envelope in client.receive():
                print(envelope.sender_region, envelope.idempotency_key)
        """
        with httpx.stream(
            "GET",
            f"{self._base}/api/v1/stream",
            headers={"Accept": "text/event-stream"},
            timeout=None,
        ) as response:
            response.raise_for_status()
            for line in response.iter_lines():
                if line.startswith("data: "):
                    data = line[len("data: "):]
                    try:
                        yield ReceivedEnvelope.from_dict(json.loads(data))
                    except (json.JSONDecodeError, KeyError):
                        continue

    # ── Status ────────────────────────────────────────────────────────────────

    def get_status(self) -> NodeStatusResponse:
        resp = self._http.get(f"{self._base}/api/v1/status")
        resp.raise_for_status()
        return NodeStatusResponse.from_dict(resp.json())

    # ── Audit ─────────────────────────────────────────────────────────────────

    def get_audit_latest(self, count: int = 20) -> list[AuditEntry]:
        resp = self._http.get(f"{self._base}/api/v1/audit/latest?n={count}")
        resp.raise_for_status()
        return [AuditEntry.from_dict(e) for e in resp.json()]

    # ── DLQ ───────────────────────────────────────────────────────────────────

    def get_dlq_entries(self) -> list[DlqEntry]:
        resp = self._http.get(f"{self._base}/api/v1/dlq")
        resp.raise_for_status()
        return [DlqEntry.from_dict(e) for e in resp.json()]

    def remove_dlq_entry(self, index: int) -> None:
        resp = self._http.delete(f"{self._base}/api/v1/dlq/{index}")
        resp.raise_for_status()

    def replay_dlq_entry(self, index: int) -> ReplayResult:
        resp = self._http.post(f"{self._base}/api/v1/dlq/{index}/replay")
        resp.raise_for_status()
        return ReplayResult.from_dict(resp.json())

    # ── CRL ───────────────────────────────────────────────────────────────────

    def get_crl_entries(self) -> list[CrlEntry]:
        resp = self._http.get(f"{self._base}/api/v1/crl")
        resp.raise_for_status()
        return [CrlEntry.from_dict(e) for e in resp.json()]

    def publish_revocation_signature(
        self, node_id: str, reason: str, signature: bytes
    ) -> None:
        resp = self._http.post(
            f"{self._base}/api/v1/crl",
            json={
                "node_id": node_id,
                "reason": reason,
                "signature_b64": base64.b64encode(signature).decode(),
            },
        )
        resp.raise_for_status()

    # ── Discovery / Connect (v0.2) ─────────────────────────────────────────────

    def discover(
        self,
        query: str,
        query_type: DiscoveryQueryType = DiscoveryQueryType.DISPLAY_HINT,
    ) -> list[DiscoveryResult]:
        """
        Discover users registered on this gateway node.

        :param query: Display name substring (display_hint) or exact app ID (app_id).
        :param query_type: How to interpret *query*.
        """
        resp = self._http.get(
            f"{self._base}/api/v1/discover",
            params={"q": query, "type": query_type.value},
        )
        resp.raise_for_status()
        return [DiscoveryResult.from_dict(r) for r in resp.json()]

    def connect(
        self,
        opaque_token: str,
        requester_id: str,
        requester_region: str,
    ) -> ConnectResult:
        """
        Send a connect request using the opaque token from :meth:`discover`.
        The token is single-use and expires in 5 minutes.
        """
        resp = self._http.post(
            f"{self._base}/api/v1/connect",
            json={
                "opaque_token": opaque_token,
                "requester_id": requester_id,
                "requester_region": requester_region,
            },
        )
        resp.raise_for_status()
        return ConnectResult.from_dict(resp.json())

    # ── JWT token issuance (v0.3) ──────────────────────────────────────────────

    def issue_token(self, scope: str = "read", ttl_minutes: int = 60) -> IssuedToken:
        """
        Issue a short-lived JWT bearer token.  Requires operator API key auth.

        :param scope: ``"read"`` or ``"operator"``.
        :param ttl_minutes: Token validity in minutes (1–1440; default 60).
        """
        resp = self._http.post(
            f"{self._base}/api/v1/token",
            json={"scope": scope, "ttl_minutes": ttl_minutes},
        )
        resp.raise_for_status()
        return IssuedToken.from_dict(resp.json())

    # ── App management (v0.3) ──────────────────────────────────────────────────

    def list_apps(self) -> list[AppInfo]:
        """Return all apps registered on this node."""
        resp = self._http.get(f"{self._base}/api/v1/apps")
        resp.raise_for_status()
        return [AppInfo.from_dict(a) for a in resp.json()]

    def register_app(self, request: RegisterAppRequest) -> AppInfo:
        """Register a new application.  Requires operator scope."""
        resp = self._http.post(
            f"{self._base}/api/v1/apps/register",
            json=request.to_dict(),
        )
        resp.raise_for_status()
        return AppInfo.from_dict(resp.json())

    def delete_app(self, app_id: str) -> None:
        """Delete a registered application.  Requires operator scope."""
        resp = self._http.delete(f"{self._base}/api/v1/apps/{app_id}")
        resp.raise_for_status()

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> "MrmiClient":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()
