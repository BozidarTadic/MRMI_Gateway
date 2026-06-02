namespace MRMI.Gateway.Client;

/// <summary>Compliance profile that governs jitter, padding, and dedup TTL.</summary>
public enum MrmiProfile
{
    /// <summary>Maximum compliance — 152-ФЗ / GDPR strict mode.</summary>
    Strict,

    /// <summary>Balanced compliance — default for most corridors.</summary>
    Balanced,

    /// <summary>Performance mode — minimal overhead, reduced privacy guarantees.</summary>
    Performance,
}

/// <summary>Discovery query type for <see cref="MrmiClient.DiscoverAsync"/>.</summary>
public enum DiscoveryQueryType
{
    /// <summary>Match against the user's display hint (partial, case-insensitive).</summary>
    DisplayHint,

    /// <summary>Return all users registered under a specific app ID.</summary>
    AppId,
}

/// <summary>Domain adapter that classifies the envelope payload format (ADR-015).</summary>
public enum SchemaType
{
    /// <summary>Default messaging envelope — backward-compatible with v0.8 envelopes that omit schema_type.</summary>
    Messaging,

    /// <summary>ISO 20022 financial messaging (pacs, camt, pain, …).</summary>
    Iso20022,

    /// <summary>HL7 FHIR healthcare data exchange.</summary>
    Hl7Fhir,

    /// <summary>UN/EDIFACT trade and business document exchange.</summary>
    Edifact,

    /// <summary>
    /// Custom domain adapter. Set <see cref="SendEnvelopeRequest.CustomSchemaId"/> to the
    /// adapter identifier; the wire value becomes <c>custom:&lt;id&gt;</c>.
    /// </summary>
    Custom,
}

/// <summary>Auto-accept policy sent with a connect request.</summary>
public enum AutoAcceptMode
{
    /// <summary>Operator must manually approve the connection.</summary>
    Manual,

    /// <summary>Accept automatically if the requester is on the app's whitelist.</summary>
    AutoWhitelist,

    /// <summary>Accept automatically when both parties are in each other's allowed regions.</summary>
    AutoMutual,

    /// <summary>Accept all connect requests unconditionally.</summary>
    AutoAll,
}
