"""ISO 20022 financial message envelope builder (ADR-016)."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

from ..models import SchemaType, SendEnvelopeRequest


@dataclass
class Iso20022EnvelopeBuilder:
    """Constructs a :class:`~mrmi_gateway.SendEnvelopeRequest` pre-configured for
    ISO 20022 financial messages.

    Sets ``schema_type="iso20022"`` and enforces the ADR-016 defaults:
    72-hour dedup TTL and ``retain_long`` audit flag are applied server-side
    automatically when ``schema_type=iso20022``.

    :param idempotency_key: Globally unique message identifier (e.g. MsgId from the ISO 20022 header).
    :param sender_region:   ISO 3166-1 alpha-2 region code of the sending institution.
    :param recipient_region: ISO 3166-1 alpha-2 region code of the receiving institution.
    :param trust_tier:      Trust tier of the sending node (default 0).
    :param payload:         Serialised ISO 20022 XML/JSON payload.
    :param schema_version:  ISO 20022 message version string (default ``"1.0.0"``).
    :param routing_hint:    Optional BIC prefix (first 6 characters) for path optimisation.
                            Never used for identity verification — purely advisory.

    Usage::

        from mrmi_gateway.adapters import Iso20022EnvelopeBuilder

        req = Iso20022EnvelopeBuilder(
            idempotency_key="MSGID-20260602-001",
            sender_region="RS",
            recipient_region="RU",
            payload=xml_bytes,
            routing_hint="BANKRS",  # BIC prefix
        ).build()

        response = client.send(req)
    """

    idempotency_key: str
    sender_region: str
    recipient_region: str
    trust_tier: int = 0
    payload: Optional[bytes] = None
    schema_version: str = "1.0.0"
    routing_hint: Optional[str] = None

    def build(self) -> SendEnvelopeRequest:
        """Return a :class:`~mrmi_gateway.SendEnvelopeRequest` with iso20022 defaults applied."""
        return SendEnvelopeRequest(
            idempotency_key=self.idempotency_key,
            sender_region=self.sender_region,
            recipient_region=self.recipient_region,
            trust_tier=self.trust_tier,
            payload=self.payload,
            schema_type=SchemaType.ISO20022,
            schema_version=self.schema_version,
            routing_hint=self.routing_hint,
        )
