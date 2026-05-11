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
