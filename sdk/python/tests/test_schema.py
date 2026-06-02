"""Tests for SchemaType, SendEnvelopeRequest schema fields, and Iso20022EnvelopeBuilder."""
from __future__ import annotations

import json

import pytest

from mrmi_gateway import SchemaType, SendEnvelopeRequest, Iso20022EnvelopeBuilder


# ── SchemaType enum ───────────────────────────────────────────────────────────

def test_schema_type_has_four_members():
    members = list(SchemaType)
    assert len(members) == 4


def test_schema_type_wire_values():
    assert SchemaType.MESSAGING.value == "messaging"
    assert SchemaType.ISO20022.value == "iso20022"
    assert SchemaType.HL7FHIR.value == "hl7fhir"
    assert SchemaType.EDIFACT.value == "edifact"


def test_schema_type_custom_returns_prefixed_string():
    assert SchemaType.custom("my-schema") == "custom:my-schema"
    assert SchemaType.custom("edi-x12") == "custom:edi-x12"


def test_schema_type_custom_is_plain_string():
    result = SchemaType.custom("test")
    assert isinstance(result, str)
    assert not isinstance(result, SchemaType)


# ── SendEnvelopeRequest defaults ──────────────────────────────────────────────

def test_send_request_defaults_to_messaging():
    req = SendEnvelopeRequest(
        idempotency_key="k1",
        sender_region="RS",
        recipient_region="RU",
    )
    assert req.schema_type == SchemaType.MESSAGING
    assert req.schema_version == "1.0.0"
    assert req.routing_hint is None


def test_send_request_to_dict_includes_schema_fields():
    req = SendEnvelopeRequest(
        idempotency_key="k1",
        sender_region="RS",
        recipient_region="RU",
        schema_type=SchemaType.ISO20022,
        schema_version="2019",
    )
    d = req.to_dict()
    assert d["schema_type"] == "iso20022"
    assert d["schema_version"] == "2019"


def test_send_request_default_schema_type_wire_is_messaging():
    req = SendEnvelopeRequest(
        idempotency_key="k1",
        sender_region="RS",
        recipient_region="RU",
    )
    d = req.to_dict()
    assert d["schema_type"] == "messaging"


@pytest.mark.parametrize("schema_type,expected_wire", [
    (SchemaType.MESSAGING, "messaging"),
    (SchemaType.ISO20022,  "iso20022"),
    (SchemaType.HL7FHIR,   "hl7fhir"),
    (SchemaType.EDIFACT,   "edifact"),
])
def test_send_request_all_schema_types_serialise_correctly(schema_type, expected_wire):
    req = SendEnvelopeRequest(
        idempotency_key="k",
        sender_region="RS",
        recipient_region="RU",
        schema_type=schema_type,
    )
    assert req.to_dict()["schema_type"] == expected_wire


def test_send_request_custom_schema_type_serialises():
    req = SendEnvelopeRequest(
        idempotency_key="k",
        sender_region="RS",
        recipient_region="RU",
        schema_type=SchemaType.custom("my-domain"),
    )
    assert req.to_dict()["schema_type"] == "custom:my-domain"


def test_send_request_routing_hint_included_when_set():
    req = SendEnvelopeRequest(
        idempotency_key="k",
        sender_region="RS",
        recipient_region="RU",
        routing_hint="BANKRS",
    )
    d = req.to_dict()
    assert d["routing_hint"] == "BANKRS"


def test_send_request_routing_hint_absent_when_none():
    req = SendEnvelopeRequest(
        idempotency_key="k",
        sender_region="RS",
        recipient_region="RU",
    )
    assert "routing_hint" not in req.to_dict()


def test_send_request_to_dict_is_json_serialisable():
    req = SendEnvelopeRequest(
        idempotency_key="k",
        sender_region="RS",
        recipient_region="RU",
        schema_type=SchemaType.ISO20022,
        routing_hint="BANKRS",
    )
    # Must not raise
    serialised = json.dumps(req.to_dict())
    parsed = json.loads(serialised)
    assert parsed["schema_type"] == "iso20022"
    assert parsed["routing_hint"] == "BANKRS"


# ── Iso20022EnvelopeBuilder ───────────────────────────────────────────────────

def test_iso20022_builder_sets_schema_type():
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
    ).build()
    assert req.schema_type == SchemaType.ISO20022
    assert req.to_dict()["schema_type"] == "iso20022"


def test_iso20022_builder_default_schema_version():
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
    ).build()
    assert req.schema_version == "1.0.0"
    assert req.to_dict()["schema_version"] == "1.0.0"


def test_iso20022_builder_custom_schema_version():
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
        schema_version="2019",
    ).build()
    assert req.to_dict()["schema_version"] == "2019"


def test_iso20022_builder_routing_hint_included():
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
        routing_hint="BANKRS",
    ).build()
    d = req.to_dict()
    assert d["routing_hint"] == "BANKRS"


def test_iso20022_builder_no_routing_hint_by_default():
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
    ).build()
    assert "routing_hint" not in req.to_dict()


def test_iso20022_builder_passes_payload():
    payload = b"\xde\xad\xbe\xef"
    req = Iso20022EnvelopeBuilder(
        idempotency_key="msg-001",
        sender_region="RS",
        recipient_region="RU",
        payload=payload,
    ).build()
    assert req.payload == payload


def test_iso20022_builder_full_round_trip():
    import base64
    payload = b"<iso20022:Document/>"
    req = Iso20022EnvelopeBuilder(
        idempotency_key="MSGID-20260602-001",
        sender_region="RS",
        recipient_region="RU",
        trust_tier=1,
        payload=payload,
        schema_version="2019",
        routing_hint="BANKRS",
    ).build()
    d = req.to_dict()
    assert d["idempotency_key"] == "MSGID-20260602-001"
    assert d["sender_region"] == "RS"
    assert d["recipient_region"] == "RU"
    assert d["trust_tier"] == 1
    assert d["schema_type"] == "iso20022"
    assert d["schema_version"] == "2019"
    assert d["routing_hint"] == "BANKRS"
    assert base64.b64decode(d["payload"]) == payload
