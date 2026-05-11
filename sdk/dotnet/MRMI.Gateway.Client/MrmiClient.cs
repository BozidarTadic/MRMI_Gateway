using System.Net.Http.Json;
using System.Runtime.CompilerServices;
using System.Text;
using System.Text.Json;

namespace MRMI.Gateway.Client;

/// <summary>
/// Client for the MRMI Gateway management REST API.
/// Thread-safe; share a single instance per base URL.
/// </summary>
public sealed class MrmiClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly bool _ownsClient;
    private readonly string _baseUrl;

    private static readonly JsonSerializerOptions _json = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    public MrmiClient(MrmiClientOptions options)
    {
        ArgumentNullException.ThrowIfNull(options);
        _baseUrl = options.BaseUrl.TrimEnd('/');

        if (options.HttpClient is not null)
        {
            _http = options.HttpClient;
            _ownsClient = false;
        }
        else
        {
            _http = new HttpClient { Timeout = options.Timeout };
            _ownsClient = true;
        }
    }

    // ── Envelope send ────────────────────────────────────────────────────────

    /// <summary>Send an envelope through the gateway.</summary>
    public async Task<SendEnvelopeResponse> SendAsync(
        SendEnvelopeRequest request,
        CancellationToken cancellationToken = default)
    {
        var response = await _http.PostAsJsonAsync(
            $"{_baseUrl}/api/v1/envelopes", request, _json, cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<SendEnvelopeResponse>(_json, cancellationToken)
               ?? throw new InvalidOperationException("Empty response from gateway.");
    }

    // ── SSE receive ──────────────────────────────────────────────────────────

    /// <summary>
    /// Connect to the gateway SSE stream and invoke <paramref name="handler"/> for each
    /// received envelope until <paramref name="cancellationToken"/> is cancelled.
    /// </summary>
    public async Task ReceiveAsync(
        Func<ReceivedEnvelope, Task> handler,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(handler);

        // Use an HttpClient with no timeout for the long-lived stream.
        using var streamClient = new HttpClient { Timeout = Timeout.InfiniteTimeSpan };
        using var request = new HttpRequestMessage(HttpMethod.Get, $"{_baseUrl}/api/v1/stream");
        request.Headers.Accept.ParseAdd("text/event-stream");

        using var response = await streamClient.SendAsync(
            request,
            HttpCompletionOption.ResponseHeadersRead,
            cancellationToken);
        response.EnsureSuccessStatusCode();

        using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
        using var reader = new StreamReader(stream, Encoding.UTF8);

        while (!cancellationToken.IsCancellationRequested)
        {
            var line = await reader.ReadLineAsync(cancellationToken);
            if (line is null) break;

            if (!line.StartsWith("data: ", StringComparison.Ordinal)) continue;

            var json = line["data: ".Length..];
            var envelope = JsonSerializer.Deserialize<ReceivedEnvelope>(json, _json);
            if (envelope is not null)
                await handler(envelope);
        }
    }

    /// <summary>
    /// Async enumerable variant of <see cref="ReceiveAsync"/>.
    /// </summary>
    public async IAsyncEnumerable<ReceivedEnvelope> StreamAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        using var streamClient = new HttpClient { Timeout = Timeout.InfiniteTimeSpan };
        using var request = new HttpRequestMessage(HttpMethod.Get, $"{_baseUrl}/api/v1/stream");
        request.Headers.Accept.ParseAdd("text/event-stream");

        using var response = await streamClient.SendAsync(
            request,
            HttpCompletionOption.ResponseHeadersRead,
            cancellationToken);
        response.EnsureSuccessStatusCode();

        using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
        using var reader = new StreamReader(stream, Encoding.UTF8);

        while (!cancellationToken.IsCancellationRequested)
        {
            var line = await reader.ReadLineAsync(cancellationToken);
            if (line is null) yield break;

            if (!line.StartsWith("data: ", StringComparison.Ordinal)) continue;

            var json = line["data: ".Length..];
            var envelope = JsonSerializer.Deserialize<ReceivedEnvelope>(json, _json);
            if (envelope is not null)
                yield return envelope;
        }
    }

    // ── Status ───────────────────────────────────────────────────────────────

    public async Task<NodeStatusResponse> GetStatusAsync(CancellationToken cancellationToken = default)
    {
        var response = await _http.GetAsync($"{_baseUrl}/api/v1/status", cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<NodeStatusResponse>(_json, cancellationToken)
               ?? throw new InvalidOperationException("Empty response.");
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<AuditEntry>> GetAuditLatestAsync(
        int count = 20,
        CancellationToken cancellationToken = default)
    {
        var response = await _http.GetAsync(
            $"{_baseUrl}/api/v1/audit/latest?n={count}", cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<List<AuditEntry>>(_json, cancellationToken)
               ?? [];
    }

    // ── DLQ ──────────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<DlqEntry>> GetDlqEntriesAsync(
        CancellationToken cancellationToken = default)
    {
        var response = await _http.GetAsync($"{_baseUrl}/api/v1/dlq", cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<List<DlqEntry>>(_json, cancellationToken)
               ?? [];
    }

    public async Task RemoveDlqEntryAsync(int index, CancellationToken cancellationToken = default)
    {
        var response = await _http.DeleteAsync(
            $"{_baseUrl}/api/v1/dlq/{index}", cancellationToken);
        response.EnsureSuccessStatusCode();
    }

    public async Task<ReplayResult> ReplayDlqEntryAsync(
        int index,
        CancellationToken cancellationToken = default)
    {
        var response = await _http.PostAsync(
            $"{_baseUrl}/api/v1/dlq/{index}/replay", null, cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<ReplayResult>(_json, cancellationToken)
               ?? throw new InvalidOperationException("Empty response.");
    }

    // ── CRL ──────────────────────────────────────────────────────────────────

    public async Task<IReadOnlyList<CrlEntry>> GetCrlEntriesAsync(
        CancellationToken cancellationToken = default)
    {
        var response = await _http.GetAsync($"{_baseUrl}/api/v1/crl", cancellationToken);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<List<CrlEntry>>(_json, cancellationToken)
               ?? [];
    }

    /// <summary>
    /// Submit a revocation signature for <paramref name="nodeId"/>.
    /// The gateway requires ≥2 distinct signatures to mark a node as effectively revoked.
    /// </summary>
    public async Task PublishRevocationSignatureAsync(
        string nodeId,
        string reason,
        byte[] signature,
        CancellationToken cancellationToken = default)
    {
        var body = new
        {
            node_id = nodeId,
            reason,
            signature_b64 = Convert.ToBase64String(signature),
        };
        var response = await _http.PostAsJsonAsync(
            $"{_baseUrl}/api/v1/crl", body, _json, cancellationToken);
        response.EnsureSuccessStatusCode();
    }

    public void Dispose()
    {
        if (_ownsClient) _http.Dispose();
    }
}
