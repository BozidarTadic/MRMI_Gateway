"""Data models for the MRMI Gateway SDK."""
from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class DiscoveryQueryType(str, Enum):
    DISPLAY_HINT = "display_hint"
    APP_ID = "app_id"


class AutoAcceptMode(str, Enum):
    MANUAL = "manual"
    AUTO_WHITELIST = "auto_whitelist"
    AUTO_MUTUAL = "auto_mutual"
    AUTO_ALL = "auto_all"


@dataclass
class SendEnvelopeRequest:
    idempotency_key: str
    sender_region: str
    recipient_region: str
    trust_tier: int = 0
    payload: Optional[bytes] = None
    sender_identity: Optional[bytes] = None

    def to_dict(self) -> dict:
        import base64
        d: dict = {
            "idempotency_key": self.idempotency_key,
            "sender_region": self.sender_region,
            "recipient_region": self.recipient_region,
            "trust_tier": self.trust_tier,
        }
        if self.payload is not None:
            d["payload"] = base64.b64encode(self.payload).decode()
        if self.sender_identity is not None:
            d["sender_identity"] = base64.b64encode(self.sender_identity).decode()
        return d


@dataclass
class SendEnvelopeResponse:
    decision: str
    reason: str
    profile: str
    node_id: str
    audit_root_hash: str
    peer_audit_root_hash: str

    @property
    def is_allowed(self) -> bool:
        return self.decision == "ALLOW"

    @classmethod
    def from_dict(cls, d: dict) -> "SendEnvelopeResponse":
        return cls(
            decision=d.get("decision", ""),
            reason=d.get("reason", ""),
            profile=d.get("profile", ""),
            node_id=d.get("node_id", ""),
            audit_root_hash=d.get("audit_root_hash", ""),
            peer_audit_root_hash=d.get("peer_audit_root_hash", ""),
        )


@dataclass
class ReceivedEnvelope:
    idempotency_key: str
    sender_region: str
    recipient_region: str
    trust_tier: int
    payload: Optional[bytes]
    timestamp: int

    @classmethod
    def from_dict(cls, d: dict) -> "ReceivedEnvelope":
        import base64
        raw = d.get("payload")
        payload = base64.b64decode(raw) if raw else None
        return cls(
            idempotency_key=d.get("idempotency_key", ""),
            sender_region=d.get("sender_region", ""),
            recipient_region=d.get("recipient_region", ""),
            trust_tier=d.get("trust_tier", 0),
            payload=payload,
            timestamp=d.get("timestamp", 0),
        )


@dataclass
class NodeStatusResponse:
    node_id: str
    region: str
    node_scope: str
    profile: str
    applicable_law: str
    app_version: str
    adr_version: str
    uptime_seconds: int

    @classmethod
    def from_dict(cls, d: dict) -> "NodeStatusResponse":
        return cls(
            node_id=d.get("node_id", ""),
            region=d.get("region", ""),
            node_scope=d.get("node_scope", ""),
            profile=d.get("profile", ""),
            applicable_law=d.get("applicable_law", ""),
            app_version=d.get("app_version", ""),
            adr_version=d.get("adr_version", ""),
            uptime_seconds=d.get("uptime_seconds", 0),
        )


@dataclass
class AuditEntry:
    seq: int
    timestamp: int
    decision: str
    reason: str
    trust_tier: int
    sender_region: str
    recipient_region: str
    policy_version: str
    profile: str
    entry_hash: str

    @classmethod
    def from_dict(cls, d: dict) -> "AuditEntry":
        return cls(
            seq=d.get("seq", 0),
            timestamp=d.get("timestamp", 0),
            decision=d.get("decision", ""),
            reason=d.get("reason", ""),
            trust_tier=d.get("trust_tier", 0),
            sender_region=d.get("sender_region", ""),
            recipient_region=d.get("recipient_region", ""),
            policy_version=d.get("policy_version", ""),
            profile=d.get("profile", ""),
            entry_hash=d.get("entry_hash", ""),
        )


@dataclass
class DlqEntry:
    index: int
    peer_addr: str
    attempts: int
    last_error: Optional[str]
    first_seen_unix: int
    last_attempt_unix: int
    envelope_id: str
    sender_region: str
    recipient_region: str

    @classmethod
    def from_dict(cls, d: dict) -> "DlqEntry":
        return cls(
            index=d.get("index", 0),
            peer_addr=d.get("peer_addr", ""),
            attempts=d.get("attempts", 0),
            last_error=d.get("last_error"),
            first_seen_unix=d.get("first_seen_unix", 0),
            last_attempt_unix=d.get("last_attempt_unix", 0),
            envelope_id=d.get("envelope_id", ""),
            sender_region=d.get("sender_region", ""),
            recipient_region=d.get("recipient_region", ""),
        )


@dataclass
class ReplayResult:
    decision: str
    reason: str

    @property
    def is_allowed(self) -> bool:
        return self.decision == "ALLOW"

    @classmethod
    def from_dict(cls, d: dict) -> "ReplayResult":
        return cls(decision=d.get("decision", ""), reason=d.get("reason", ""))


@dataclass
class CrlEntry:
    node_id: str
    reason: str
    sig_count: int
    is_effective: bool
    revoked_at_unix: int

    @classmethod
    def from_dict(cls, d: dict) -> "CrlEntry":
        return cls(
            node_id=d.get("node_id", ""),
            reason=d.get("reason", ""),
            sig_count=d.get("sig_count", 0),
            is_effective=d.get("is_effective", False),
            revoked_at_unix=d.get("revoked_at_unix", 0),
        )


@dataclass
class DiscoveryResult:
    node_id: str
    app_id: str
    user_id: str
    display_hint: str
    region: str
    opaque_token: str
    token_expires: int

    @classmethod
    def from_dict(cls, d: dict) -> "DiscoveryResult":
        return cls(
            node_id=d.get("node_id", ""),
            app_id=d.get("app_id", ""),
            user_id=d.get("user_id", ""),
            display_hint=d.get("display_hint", ""),
            region=d.get("region", ""),
            opaque_token=d.get("opaque_token", ""),
            token_expires=d.get("token_expires", 0),
        )


@dataclass
class ConnectResult:
    status: str
    session_id: Optional[str]
    expires_at: int

    @property
    def is_accepted(self) -> bool:
        return self.status == "ACCEPTED"

    @classmethod
    def from_dict(cls, d: dict) -> "ConnectResult":
        return cls(
            status=d.get("status", ""),
            session_id=d.get("session_id"),
            expires_at=d.get("expires_at", 0),
        )


@dataclass
class IssuedToken:
    token: str
    scope: str
    expires_at: int

    @classmethod
    def from_dict(cls, d: dict) -> "IssuedToken":
        return cls(
            token=d.get("token", ""),
            scope=d.get("scope", "read"),
            expires_at=d.get("expires_at", 0),
        )


@dataclass
class AppInfo:
    app_id: str
    webhook_url: str
    auto_accept: str

    @classmethod
    def from_dict(cls, d: dict) -> "AppInfo":
        return cls(
            app_id=d.get("app_id", ""),
            webhook_url=d.get("webhook_url", ""),
            auto_accept=d.get("auto_accept", "manual"),
        )


@dataclass
class RegisterAppRequest:
    app_id: str
    webhook_url: str = ""
    webhook_secret: str = ""
    auto_accept: AutoAcceptMode = AutoAcceptMode.MANUAL

    def to_dict(self) -> dict:
        return {
            "app_id": self.app_id,
            "webhook_url": self.webhook_url,
            "webhook_secret": self.webhook_secret,
            "auto_accept": self.auto_accept.value,
        }
