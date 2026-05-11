using System.Net;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using MRMI.Gateway.Client;
using Xunit;

namespace MRMI.Gateway.Client.Tests;

/// <summary>
/// Unit tests for MrmiClient using a stub HttpMessageHandler so no real gateway is needed.
/// </summary>
public sealed class MrmiClientTests
{
    private static MrmiClient BuildClient(HttpMessageHandler handler) =>
        new(new MrmiClientOptions
        {
            BaseUrl = "http://localhost:8080",
            HttpClient = new HttpClient(handler),
        });

    // ── SendAsync ────────────────────────────────────────────────────────────

    [Fact]
    public async Task SendAsync_ReturnsAllowDecision()
    {
        var responseJson = """
            {"decision":"ALLOW","reason":"POLICY_ACCEPTED","profile":"balanced","node_id":"rs-node","audit_root_hash":"sha256:abc","peer_audit_root_hash":""}
            """;

        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var result = await client.SendAsync(new SendEnvelopeRequest
        {
            IdempotencyKey = "test-001",
            SenderRegion = "RS",
            RecipientRegion = "RU",
        });

        Assert.Equal("ALLOW", result.Decision);
        Assert.True(result.IsAllowed);
        Assert.Equal("rs-node", result.NodeId);
    }

    [Fact]
    public async Task SendAsync_ThrowsOnNonSuccess()
    {
        var handler = new StubHandler(HttpStatusCode.BadRequest, "idempotency_key required", "text/plain");
        using var client = BuildClient(handler);

        await Assert.ThrowsAsync<HttpRequestException>(() =>
            client.SendAsync(new SendEnvelopeRequest
            {
                IdempotencyKey = "",
                SenderRegion = "RS",
                RecipientRegion = "RU",
            }));
    }

    // ── GetStatusAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task GetStatusAsync_ParsesAllFields()
    {
        var responseJson = """
            {"node_id":"rs-01","region":"RS","node_scope":"regional","profile":"balanced","applicable_law":"RS-GDPR","app_version":"0.1.0","adr_version":"0.8","uptime_seconds":42}
            """;

        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var status = await client.GetStatusAsync();

        Assert.Equal("rs-01", status.NodeId);
        Assert.Equal("RS", status.Region);
        Assert.Equal(42L, status.UptimeSeconds);
        Assert.Equal("0.1.0", status.AppVersion);
    }

    // ── GetAuditLatestAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task GetAuditLatestAsync_ReturnsList()
    {
        var responseJson = """
            [{"seq":1,"timestamp":1000,"decision":"ALLOW","reason":"POLICY_ACCEPTED","trust_tier":1,"sender_region":"RS","recipient_region":"RU","policy_version":"v1","profile":"balanced","entry_hash":"sha256:xxx"}]
            """;

        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var entries = await client.GetAuditLatestAsync();

        Assert.Single(entries);
        Assert.Equal("ALLOW", entries[0].Decision);
        Assert.Equal("RS", entries[0].SenderRegion);
    }

    // ── GetDlqEntriesAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task GetDlqEntriesAsync_ReturnsList()
    {
        var responseJson = """
            [{"index":0,"peer_addr":"localhost:7777","attempts":3,"last_error":"dial timeout","first_seen_unix":1000,"last_attempt_unix":2000,"envelope_id":"env-01","sender_region":"RS","recipient_region":"RU"}]
            """;

        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var entries = await client.GetDlqEntriesAsync();

        Assert.Single(entries);
        Assert.Equal(3, entries[0].Attempts);
        Assert.Equal("dial timeout", entries[0].LastError);
    }

    // ── RemoveDlqEntryAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task RemoveDlqEntryAsync_Succeeds()
    {
        var handler = new StubHandler(HttpStatusCode.NoContent, "", "application/json");
        using var client = BuildClient(handler);

        // Should not throw
        await client.RemoveDlqEntryAsync(0);
    }

    // ── ReplayDlqEntryAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task ReplayDlqEntryAsync_ReturnsResult()
    {
        var responseJson = """{"decision":"ALLOW","reason":"POLICY_ACCEPTED"}""";
        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var result = await client.ReplayDlqEntryAsync(0);

        Assert.Equal("ALLOW", result.Decision);
        Assert.True(result.IsAllowed);
    }

    // ── GetCrlEntriesAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task GetCrlEntriesAsync_ReturnsList()
    {
        var responseJson = """
            [{"node_id":"bad-node","reason":"compromised","sig_count":2,"is_effective":true,"revoked_at_unix":9999}]
            """;

        var handler = new StubHandler(HttpStatusCode.OK, responseJson, "application/json");
        using var client = BuildClient(handler);

        var entries = await client.GetCrlEntriesAsync();

        Assert.Single(entries);
        Assert.True(entries[0].IsEffective);
        Assert.Equal("bad-node", entries[0].NodeId);
    }

    // ── PublishRevocationSignatureAsync ──────────────────────────────────────

    [Fact]
    public async Task PublishRevocationSignatureAsync_PostsCorrectPayload()
    {
        string? capturedBody = null;
        var handler = new CapturingHandler(body => capturedBody = body,
            HttpStatusCode.OK, """{"node_id":"n1","is_effective":false}""", "application/json");

        using var client = BuildClient(handler);

        await client.PublishRevocationSignatureAsync("n1", "key compromise", new byte[] { 1, 2, 3 });

        Assert.NotNull(capturedBody);
        var doc = JsonDocument.Parse(capturedBody!);
        Assert.Equal("n1", doc.RootElement.GetProperty("node_id").GetString());
        Assert.Equal("key compromise", doc.RootElement.GetProperty("reason").GetString());
        var expectedSig = Convert.ToBase64String(new byte[] { 1, 2, 3 });
        Assert.Equal(expectedSig, doc.RootElement.GetProperty("signature_b64").GetString());
    }

    // ── MrmiProfile enum ─────────────────────────────────────────────────────

    [Fact]
    public void MrmiProfile_HasThreeValues()
    {
        var values = Enum.GetValues<MrmiProfile>();
        Assert.Equal(3, values.Length);
        Assert.Contains(MrmiProfile.Strict, values);
        Assert.Contains(MrmiProfile.Balanced, values);
        Assert.Contains(MrmiProfile.Performance, values);
    }
}

// ── Test helpers ─────────────────────────────────────────────────────────────

internal sealed class StubHandler(HttpStatusCode status, string body, string contentType)
    : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        var response = new HttpResponseMessage(status)
        {
            Content = new StringContent(body, Encoding.UTF8, contentType),
        };
        return Task.FromResult(response);
    }
}

internal sealed class CapturingHandler(
    Action<string> capture,
    HttpStatusCode status,
    string body,
    string contentType)
    : HttpMessageHandler
{
    protected override async Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        if (request.Content is not null)
        {
            var content = await request.Content.ReadAsStringAsync(cancellationToken);
            capture(content);
        }
        return new HttpResponseMessage(status)
        {
            Content = new StringContent(body, Encoding.UTF8, contentType),
        };
    }
}
