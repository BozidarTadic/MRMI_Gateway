using System.Text.Json.Serialization;

namespace MRMI.Gateway.Client;

// ── Envelope send ────────────────────────────────────────────────────────────

/// <summary>Request to send an envelope through the gateway.</summary>
public sealed class SendEnvelopeRequest
{
    /// <summary>Unique key for at-least-once delivery. Must be stable across retries.</summary>
    [JsonPropertyName("idempotency_key")]
    public required string IdempotencyKey { get; init; }

    /// <summary>ISO-3166-1 alpha-2 region code of the sending application.</summary>
    [JsonPropertyName("sender_region")]
    public required string SenderRegion { get; init; }

    /// <summary>ISO-3166-1 alpha-2 region code of the target application.</summary>
    [JsonPropertyName("recipient_region")]
    public required string RecipientRegion { get; init; }

    /// <summary>Trust tier of the sender (0 = anonymous, 3 = legal entity).</summary>
    [JsonPropertyName("trust_tier")]
    public uint TrustTier { get; init; }

    /// <summary>Opaque application payload bytes.</summary>
    [JsonPropertyName("payload")]
    public byte[]? Payload { get; init; }

    /// <summary>Opaque sender identity bytes (used for Ed25519 verification).</summary>
    [JsonPropertyName("sender_identity")]
    public byte[]? SenderIdentity { get; init; }
}

/// <summary>Response from a <see cref="MrmiClient.SendAsync"/> call.</summary>
public sealed class SendEnvelopeResponse
{
    [JsonPropertyName("decision")]
    public string Decision { get; init; } = "";

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = "";

    [JsonPropertyName("profile")]
    public string Profile { get; init; } = "";

    [JsonPropertyName("node_id")]
    public string NodeId { get; init; } = "";

    [JsonPropertyName("audit_root_hash")]
    public string AuditRootHash { get; init; } = "";

    [JsonPropertyName("peer_audit_root_hash")]
    public string PeerAuditRootHash { get; init; } = "";

    /// <summary>True when the gateway allowed the envelope.</summary>
    [JsonIgnore]
    public bool IsAllowed => Decision == "ALLOW";
}

// ── Inbox / SSE ──────────────────────────────────────────────────────────────

/// <summary>An envelope received from the SSE stream.</summary>
public sealed class ReceivedEnvelope
{
    [JsonPropertyName("idempotency_key")]
    public string IdempotencyKey { get; init; } = "";

    [JsonPropertyName("sender_region")]
    public string SenderRegion { get; init; } = "";

    [JsonPropertyName("recipient_region")]
    public string RecipientRegion { get; init; } = "";

    [JsonPropertyName("trust_tier")]
    public uint TrustTier { get; init; }

    [JsonPropertyName("payload")]
    public byte[]? Payload { get; init; }

    [JsonPropertyName("timestamp")]
    public long Timestamp { get; init; }
}

// ── Status ───────────────────────────────────────────────────────────────────

public sealed class NodeStatusResponse
{
    [JsonPropertyName("node_id")]
    public string NodeId { get; init; } = "";

    [JsonPropertyName("region")]
    public string Region { get; init; } = "";

    [JsonPropertyName("node_scope")]
    public string NodeScope { get; init; } = "";

    [JsonPropertyName("profile")]
    public string Profile { get; init; } = "";

    [JsonPropertyName("applicable_law")]
    public string ApplicableLaw { get; init; } = "";

    [JsonPropertyName("app_version")]
    public string AppVersion { get; init; } = "";

    [JsonPropertyName("adr_version")]
    public string AdrVersion { get; init; } = "";

    [JsonPropertyName("uptime_seconds")]
    public long UptimeSeconds { get; init; }
}

// ── Audit ─────────────────────────────────────────────────────────────────────

public sealed class AuditEntry
{
    [JsonPropertyName("seq")]
    public ulong Seq { get; init; }

    [JsonPropertyName("timestamp")]
    public long Timestamp { get; init; }

    [JsonPropertyName("decision")]
    public string Decision { get; init; } = "";

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = "";

    [JsonPropertyName("trust_tier")]
    public uint TrustTier { get; init; }

    [JsonPropertyName("sender_region")]
    public string SenderRegion { get; init; } = "";

    [JsonPropertyName("recipient_region")]
    public string RecipientRegion { get; init; } = "";

    [JsonPropertyName("policy_version")]
    public string PolicyVersion { get; init; } = "";

    [JsonPropertyName("profile")]
    public string Profile { get; init; } = "";

    [JsonPropertyName("entry_hash")]
    public string EntryHash { get; init; } = "";
}

// ── DLQ ──────────────────────────────────────────────────────────────────────

public sealed class DlqEntry
{
    [JsonPropertyName("index")]
    public int Index { get; init; }

    [JsonPropertyName("peer_addr")]
    public string PeerAddr { get; init; } = "";

    [JsonPropertyName("attempts")]
    public int Attempts { get; init; }

    [JsonPropertyName("last_error")]
    public string? LastError { get; init; }

    [JsonPropertyName("first_seen_unix")]
    public long FirstSeenUnix { get; init; }

    [JsonPropertyName("last_attempt_unix")]
    public long LastAttemptUnix { get; init; }

    [JsonPropertyName("envelope_id")]
    public string EnvelopeId { get; init; } = "";

    [JsonPropertyName("sender_region")]
    public string SenderRegion { get; init; } = "";

    [JsonPropertyName("recipient_region")]
    public string RecipientRegion { get; init; } = "";
}

public sealed class ReplayResult
{
    [JsonPropertyName("decision")]
    public string Decision { get; init; } = "";

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = "";

    [JsonIgnore]
    public bool IsAllowed => Decision == "ALLOW";
}

// ── CRL ──────────────────────────────────────────────────────────────────────

public sealed class CrlEntry
{
    [JsonPropertyName("node_id")]
    public string NodeId { get; init; } = "";

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = "";

    [JsonPropertyName("sig_count")]
    public int SigCount { get; init; }

    [JsonPropertyName("is_effective")]
    public bool IsEffective { get; init; }

    [JsonPropertyName("revoked_at_unix")]
    public long RevokedAtUnix { get; init; }
}

// ── Discovery / Connect (v0.2) ────────────────────────────────────────────────

/// <summary>A user returned by a discovery query.</summary>
public sealed class DiscoveryResult
{
    [JsonPropertyName("node_id")]
    public string NodeId { get; init; } = "";

    [JsonPropertyName("app_id")]
    public string AppId { get; init; } = "";

    [JsonPropertyName("user_id")]
    public string UserId { get; init; } = "";

    [JsonPropertyName("display_hint")]
    public string DisplayHint { get; init; } = "";

    [JsonPropertyName("region")]
    public string Region { get; init; } = "";

    /// <summary>Short-lived token to use in <see cref="MrmiClient.ConnectAsync"/>.</summary>
    [JsonPropertyName("opaque_token")]
    public string OpaqueToken { get; init; } = "";

    [JsonPropertyName("token_expires")]
    public long TokenExpires { get; init; }
}

/// <summary>Result of a <see cref="MrmiClient.ConnectAsync"/> call.</summary>
public sealed class ConnectResult
{
    /// <summary>ACCEPTED, PENDING, or DENIED.</summary>
    [JsonPropertyName("status")]
    public string Status { get; init; } = "";

    [JsonPropertyName("session_id")]
    public string? SessionId { get; init; }

    [JsonPropertyName("expires_at")]
    public long ExpiresAt { get; init; }

    [JsonIgnore]
    public bool IsAccepted => Status == "ACCEPTED";
}

/// <summary>Options for a connect request.</summary>
public sealed record ConnectOptions
{
    /// <summary>How the target node should auto-accept the request.</summary>
    public AutoAcceptMode AutoAccept { get; init; } = AutoAcceptMode.Manual;
}
