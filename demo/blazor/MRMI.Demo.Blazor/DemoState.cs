using MRMI.Gateway.Client;

namespace MRMI.Demo.Blazor;

public sealed class DemoState : IAsyncDisposable
{
    public MrmiClient RsClient { get; }
    public MrmiClient RuClient { get; }
    public string RsBaseUrl { get; }
    public string RuBaseUrl { get; }

    public const string RsRegion = "RS";
    public const string RuRegion = "RU";

    public IReadOnlyList<DiscoveryResult> RsUsers => _rsUsers;
    public IReadOnlyList<DiscoveryResult> RuUsers => _ruUsers;
    private List<DiscoveryResult> _rsUsers = [];
    private List<DiscoveryResult> _ruUsers = [];

    // key format: demo-{session}:{rsUserId}:{ruUserId}:{seq}
    // ':' used as separator because user IDs contain '-' but never ':'
    private readonly Dictionary<(string RsId, string RuId), List<ChatMessage>> _chats = new();
    private readonly List<LogEntry> _log = [];
    private readonly object _lock = new();

    public bool RsOnline { get; private set; }
    public bool RuOnline { get; private set; }

    public event Action? OnChanged;

    private CancellationTokenSource? _cts;
    private static int _seqCounter;
    private static readonly string _sessionPrefix =
        DateTimeOffset.UtcNow.ToUnixTimeMilliseconds().ToString("x");

    public DemoState(IConfiguration config)
    {
        RsBaseUrl = config["Demo:RsUrl"] ?? "http://localhost:8080";
        RuBaseUrl = config["Demo:RuUrl"] ?? "http://localhost:8081";
        RsClient = new MrmiClient(new MrmiClientOptions { BaseUrl = RsBaseUrl });
        RuClient = new MrmiClient(new MrmiClientOptions { BaseUrl = RuBaseUrl });
    }

    public async Task LoadUsersAsync()
    {
        try
        {
            var rsTask = RsClient.DiscoverAsync("");
            var ruTask = RuClient.DiscoverAsync("");
            await Task.WhenAll(rsTask, ruTask);
            lock (_lock)
            {
                _rsUsers = [.. rsTask.Result];
                _ruUsers = [.. ruTask.Result];
            }
        }
        catch { }
        OnChanged?.Invoke();
    }

    public void StartStreaming()
    {
        if (_cts is not null) return;
        _cts = new CancellationTokenSource();
        _ = Task.Run(() => StreamNode(RsClient, RsRegion, _cts.Token));
        _ = Task.Run(() => StreamNode(RuClient, RuRegion, _cts.Token));
    }

    private async Task StreamNode(MrmiClient client, string nodeRegion, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                await foreach (var env in client.StreamAsync(ct))
                {
                    var text = env.Payload?.Length > 0
                        ? System.Text.Encoding.UTF8.GetString(env.Payload).TrimEnd('\0')
                        : "(empty)";

                    var (rsId, ruId) = ParseChatKey(env.IdempotencyKey);
                    if (rsId is null || ruId is null) continue;

                    var msg = new ChatMessage(
                        DateTimeOffset.UtcNow,
                        env.SenderRegion,
                        env.RecipientRegion,
                        text,
                        env.IdempotencyKey,
                        rsId, ruId);

                    lock (_lock)
                    {
                        var key = (rsId, ruId);
                        if (!_chats.ContainsKey(key)) _chats[key] = [];
                        // Deduplicate: both RS and RU streams fire for the same message
                        if (_chats[key].All(m => m.IdempotencyKey != msg.IdempotencyKey))
                            _chats[key].Add(msg);
                    }

                    if (nodeRegion == RsRegion) RsOnline = true;
                    else RuOnline = true;

                    OnChanged?.Invoke();
                }
            }
            catch (OperationCanceledException) { break; }
            catch
            {
                if (nodeRegion == RsRegion) RsOnline = false;
                else RuOnline = false;
                OnChanged?.Invoke();
                try { await Task.Delay(3000, ct); } catch (OperationCanceledException) { break; }
            }
        }
    }

    private static (string? rsId, string? ruId) ParseChatKey(string? key)
    {
        if (key is null) return (null, null);
        var parts = key.Split(':');
        return parts.Length >= 3 ? (parts[1], parts[2]) : (null, null);
    }

    public void EnsureChat(string rsId, string ruId)
    {
        lock (_lock)
        {
            var key = (rsId, ruId);
            if (!_chats.ContainsKey(key)) _chats[key] = [];
        }
        OnChanged?.Invoke();
    }

    public IReadOnlyList<ChatMessage> GetMessages(string rsId, string ruId)
    {
        lock (_lock)
        {
            return _chats.TryGetValue((rsId, ruId), out var msgs) ? [.. msgs] : [];
        }
    }

    public IReadOnlyList<(string RsId, string RuId)> ChatList
    {
        get { lock (_lock) return [.. _chats.Keys]; }
    }

    public string GetDisplayName(string region, string userId)
    {
        var list = region == RsRegion ? _rsUsers : _ruUsers;
        return list.FirstOrDefault(u => u.UserId == userId)?.DisplayHint ?? userId;
    }

    public IReadOnlyList<LogEntry> Log { get { lock (_lock) return _log.AsReadOnly(); } }

    public async Task<(string Decision, string Reason)> SendAsync(
        string rsId, string ruId, string fromRegion, string text)
    {
        var toRegion = fromRegion == RsRegion ? RuRegion : RsRegion;
        var key = $"demo-{_sessionPrefix}:{rsId}:{ruId}:{Interlocked.Increment(ref _seqCounter):D6}";
        var client = fromRegion == RsRegion ? RsClient : RuClient;
        var result = await SendEnvelopeAsync(client, key, fromRegion, toRegion, 1, text);
        return (result.Decision, result.Reason);
    }

    public async Task<ScenarioResult> RunScenarioAsync(DemoScenario scenario)
    {
        var request = scenario switch
        {
            DemoScenario.AllowedRsToRu => ScenarioRunRequest.AllowedCorridor(),
            DemoScenario.DeniedBlockedJurisdiction => ScenarioRunRequest.BlockedJurisdiction(),
            DemoScenario.LowTrustTier => ScenarioRunRequest.LowTrustTier(),
            DemoScenario.UnknownRegion => ScenarioRunRequest.UnknownRegion(),
            DemoScenario.DuplicateIdempotency => ScenarioRunRequest.AllowedCorridor() with
            {
                Name = "Duplicate idempotency key",
                Payload = "First and second sends reuse the same idempotency key",
            },
            _ => throw new ArgumentOutOfRangeException(nameof(scenario), scenario, null),
        };

        return scenario == DemoScenario.DuplicateIdempotency
            ? await RunDuplicateScenarioAsync(request)
            : await RunScenarioAsync(request);
    }

    public async Task<ScenarioResult> RunScenarioAsync(ScenarioRunRequest request)
    {
        if (string.Equals(request.Slug, "duplicate", StringComparison.OrdinalIgnoreCase))
            return await RunDuplicateScenarioAsync(request);

        var nodeLabel = string.Equals(request.Node, "RU", StringComparison.OrdinalIgnoreCase) ? "RU" : "RS";
        var client = nodeLabel == "RU" ? RuClient : RsClient;
        var key = $"scenario-{_sessionPrefix}:{request.Slug}:{Interlocked.Increment(ref _seqCounter):D6}";
        return await RunSingleScenarioAsync(
            request.Name,
            client,
            nodeLabel,
            key,
            request.SenderRegion,
            request.RecipientRegion,
            request.TrustTier,
            request.Payload);
    }

    public async Task<IReadOnlyList<NodeSnapshot>> LoadNodeSnapshotsAsync()
    {
        var rsTask = LoadNodeSnapshotAsync("RS", RsBaseUrl, RsClient);
        var ruTask = LoadNodeSnapshotAsync("RU", RuBaseUrl, RuClient);
        await Task.WhenAll(rsTask, ruTask);
        return [rsTask.Result, ruTask.Result];
    }

    private async Task<NodeSnapshot> LoadNodeSnapshotAsync(string label, string baseUrl, MrmiClient client)
    {
        try
        {
            var statusTask = client.GetStatusAsync();
            var auditTask = client.GetAuditLatestAsync(8);
            var dlqTask = client.GetDlqEntriesAsync();
            await Task.WhenAll(statusTask, auditTask, dlqTask);

            return new NodeSnapshot(
                Label: label,
                BaseUrl: baseUrl,
                Online: true,
                Status: statusTask.Result,
                Audit: auditTask.Result,
                Dlq: dlqTask.Result,
                Error: null);
        }
        catch (Exception ex)
        {
            return new NodeSnapshot(label, baseUrl, false, null, [], [], ex.Message);
        }
    }

    private async Task<ScenarioResult> RunSingleScenarioAsync(
        string name,
        MrmiClient client,
        string nodeLabel,
        string key,
        string fromRegion,
        string toRegion,
        uint trustTier,
        string text)
    {
        var result = await SendEnvelopeAsync(client, key, fromRegion, toRegion, trustTier, text);
        return new ScenarioResult(
            name,
            result.Decision,
            result.Reason,
            key,
            nodeLabel,
            fromRegion,
            toRegion,
            trustTier,
            text,
            result.AuditRootHash,
            result.PeerAuditRootHash,
            result.Profile,
            result.NodeId);
    }

    private async Task<ScenarioResult> RunDuplicateScenarioAsync(ScenarioRunRequest request)
    {
        var key = $"scenario-{_sessionPrefix}:duplicate:{Interlocked.Increment(ref _seqCounter):D6}";
        var nodeLabel = string.Equals(request.Node, "RU", StringComparison.OrdinalIgnoreCase) ? "RU" : "RS";
        var client = nodeLabel == "RU" ? RuClient : RsClient;
        await SendEnvelopeAsync(client, key, request.SenderRegion, request.RecipientRegion, request.TrustTier, request.Payload);
        var result = await SendEnvelopeAsync(client, key, request.SenderRegion, request.RecipientRegion, request.TrustTier, request.Payload);
        return new ScenarioResult(
            request.Name,
            result.Decision,
            result.Reason,
            key,
            nodeLabel,
            request.SenderRegion,
            request.RecipientRegion,
            request.TrustTier,
            request.Payload,
            result.AuditRootHash,
            result.PeerAuditRootHash,
            result.Profile,
            result.NodeId);
    }

    private async Task<ScenarioSendResult> SendEnvelopeAsync(
        MrmiClient client,
        string key,
        string fromRegion,
        string toRegion,
        uint trustTier,
        string text)
    {
        try
        {
            var result = await client.SendAsync(new SendEnvelopeRequest
            {
                IdempotencyKey = key,
                SenderRegion = fromRegion,
                RecipientRegion = toRegion,
                TrustTier = trustTier,
                Payload = System.Text.Encoding.UTF8.GetBytes(text),
            });

            lock (_lock)
            {
                _log.Insert(0, new LogEntry(
                    DateTimeOffset.UtcNow, $"{fromRegion} → {toRegion}",
                    result.Decision, result.Reason, key, Payload: text,
                    AuditRootHash: result.AuditRootHash,
                    PeerAuditRootHash: result.PeerAuditRootHash,
                    Profile: result.Profile, NodeId: result.NodeId,
                    TrustTier: trustTier));
                if (_log.Count > 200) _log.RemoveAt(_log.Count - 1);
            }
            OnChanged?.Invoke();
            return new ScenarioSendResult(
                result.Decision,
                result.Reason,
                result.AuditRootHash,
                result.PeerAuditRootHash,
                result.Profile,
                result.NodeId);
        }
        catch (Exception ex)
        {
            lock (_lock)
            {
                _log.Insert(0, new LogEntry(DateTimeOffset.UtcNow,
                    $"{fromRegion} → {toRegion}", "ERROR", ex.Message, key));
                if (_log.Count > 200) _log.RemoveAt(_log.Count - 1);
            }
            OnChanged?.Invoke();
            return new ScenarioSendResult("ERROR", ex.Message, null, null, null, null);
        }
    }

    public void ClearChat(string rsId, string ruId)
    {
        lock (_lock)
        {
            if (_chats.ContainsKey((rsId, ruId))) _chats[(rsId, ruId)].Clear();
        }
        OnChanged?.Invoke();
    }

    public void ClearLog()
    {
        lock (_lock) { _log.Clear(); }
        OnChanged?.Invoke();
    }

    public async ValueTask DisposeAsync()
    {
        if (_cts is not null)
        {
            await _cts.CancelAsync();
            _cts.Dispose();
        }
        RsClient.Dispose();
        RuClient.Dispose();
    }
}

public enum DemoScenario
{
    AllowedRsToRu,
    DeniedBlockedJurisdiction,
    LowTrustTier,
    UnknownRegion,
    DuplicateIdempotency,
}

public sealed record ScenarioRunRequest(
    string Name,
    string Slug,
    string Node,
    string SenderRegion,
    string RecipientRegion,
    uint TrustTier,
    string Payload)
{
    public static ScenarioRunRequest AllowedCorridor() => new(
        "Allowed corridor",
        "allow",
        "RS",
        DemoState.RsRegion,
        DemoState.RuRegion,
        1,
        "Allowed demo payload");

    public static ScenarioRunRequest BlockedJurisdiction() => new(
        "Blocked jurisdiction",
        "blocked",
        "RS",
        DemoState.RsRegion,
        "US",
        1,
        "This should not leave the allowed corridor");

    public static ScenarioRunRequest LowTrustTier() => new(
        "Low trust tier",
        "low-trust",
        "RS",
        DemoState.RsRegion,
        DemoState.RuRegion,
        0,
        "Low-trust request should be denied by policy");

    public static ScenarioRunRequest UnknownRegion() => new(
        "Unknown region",
        "unknown-region",
        "RS",
        DemoState.RsRegion,
        "ZZ",
        1,
        "Unknown recipient region should be denied by policy");
}

public sealed record ScenarioResult(
    string Name,
    string Decision,
    string Reason,
    string IdempotencyKey,
    string Node,
    string SenderRegion,
    string RecipientRegion,
    uint TrustTier,
    string Payload,
    string? AuditRootHash,
    string? PeerAuditRootHash,
    string? Profile,
    string? NodeId
);

public sealed record ScenarioSendResult(
    string Decision,
    string Reason,
    string? AuditRootHash,
    string? PeerAuditRootHash,
    string? Profile,
    string? NodeId
);

public sealed record NodeSnapshot(
    string Label,
    string BaseUrl,
    bool Online,
    NodeStatusResponse? Status,
    IReadOnlyList<AuditEntry> Audit,
    IReadOnlyList<DlqEntry> Dlq,
    string? Error
);

public sealed record ChatMessage(
    DateTimeOffset Timestamp,
    string FromRegion,
    string ToRegion,
    string Text,
    string IdempotencyKey,
    string? RsUserId = null,
    string? RuUserId = null
);

public sealed record LogEntry(
    DateTimeOffset Timestamp,
    string Direction,
    string Decision,
    string Reason,
    string IdempotencyKey,
    string? Payload = null,
    string? AuditRootHash = null,
    string? PeerAuditRootHash = null,
    string? Profile = null,
    string? NodeId = null,
    uint? TrustTier = null
)
{
    public bool IsAllow => Decision == "ALLOW";
}
