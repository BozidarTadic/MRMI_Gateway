namespace MRMI.Gateway.Client;

/// <summary>Options for configuring an <see cref="MrmiClient"/> instance.</summary>
public sealed class MrmiClientOptions
{
    /// <summary>Base URL of the MRMI Gateway node, e.g. http://localhost:8080</summary>
    public required string BaseUrl { get; init; }

    /// <summary>
    /// API key sent as <c>X-MRMI-Key</c>. Ignored when <see cref="JwtToken"/> is set.
    /// </summary>
    public string? ApiKey { get; init; }

    /// <summary>
    /// JWT bearer token sent as <c>Authorization: Bearer …</c>.
    /// Takes precedence over <see cref="ApiKey"/>.
    /// </summary>
    public string? JwtToken { get; init; }

    /// <summary>
    /// Optional pre-configured <see cref="HttpClient"/>. When provided the SDK uses it
    /// directly; <see cref="Timeout"/> is ignored. Useful for testing with a mock handler.
    /// </summary>
    public HttpClient? HttpClient { get; init; }

    /// <summary>Request timeout. Defaults to 10 seconds. Not applied to SSE streams.</summary>
    public TimeSpan Timeout { get; init; } = TimeSpan.FromSeconds(10);
}
