namespace MRMI.Gateway.Client;

/// <summary>Options for configuring an <see cref="MrmiClient"/> instance.</summary>
public sealed class MrmiClientOptions
{
    /// <summary>Base URL of the MRMI Gateway node, e.g. http://localhost:8080</summary>
    public required string BaseUrl { get; init; }

    /// <summary>
    /// Optional pre-configured <see cref="HttpClient"/>. When provided the SDK uses it
    /// directly; <see cref="Timeout"/> is ignored. Useful for testing with a mock handler.
    /// </summary>
    public HttpClient? HttpClient { get; init; }

    /// <summary>Request timeout. Defaults to 10 seconds. Not applied to SSE streams.</summary>
    public TimeSpan Timeout { get; init; } = TimeSpan.FromSeconds(10);
}
