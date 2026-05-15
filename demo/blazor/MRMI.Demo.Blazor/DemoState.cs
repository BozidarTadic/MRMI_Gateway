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
        return await SendEnvelopeAsync(client, key, fromRegion, toRegion, 1, text);
    }

    public async Task<ScenarioResult> RunScenarioAsync(DemoScenario scenario)
    {
        return scenario switch
        {
            DemoScenario.AllowedRsToRu => await RunSingleScenarioAsync(
                "Allowed RS -> RU",
                RsClient,
                $"scenario-{_sessionPrefix}:allow:{Interlocked.Increment(ref _seqCounter):D6}",
                RsRegion,
                RuRegion,
                1,
                "Allowed demo payload"),

            DemoScenario.DeniedBlockedJurisdiction => await RunSingleScenarioAsync(
                "Denied RS -> US",
                RsClient,
                $"scenario-{_sessionPrefix}:deny-us:{Interlocked.Increment(ref _seqCounter):D6}",
                RsRegion,
                "US",
                1,
                "This should not leave the allowed corridor"),

            DemoScenario.DuplicateIdempotency => await RunDuplicateScenarioAsync(),

            _ => throw new ArgumentOutOfRangeException(nameof(scenario), scenario, null),
        };
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
        string key,
        string fromRegion,
        string toRegion,
        uint trustTier,
        string text)
    {
        var (decision, reason) = await SendEnvelopeAsync(client, key, fromRegion, toRegion, trustTier, text);
        return new ScenarioResult(name, decision, reason, key);
    }

    private async Task<ScenarioResult> RunDuplicateScenarioAsync()
    {
        var key = $"scenario-{_sessionPrefix}:duplicate:{Interlocked.Increment(ref _seqCounter):D6}";
        await SendEnvelopeAsync(RsClient, key, RsRegion, RuRegion, 1, "First send with reusable idempotency key");
        var (decision, reason) = await SendEnvelopeAsync(RsClient, key, RsRegion, RuRegion, 1, "Second send with same idempotency key");
        return new ScenarioResult("Duplicate idempotency key", decision, reason, key);
    }

    private async Task<(string Decision, string Reason)> SendEnvelopeAsync(
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
                    Profile: result.Profile, NodeId: result.NodeId));
                if (_log.Count > 200) _log.RemoveAt(_log.Count - 1);
            }
            OnChanged?.Invoke();
            return (result.Decision, result.Reason);
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
            return ("ERROR", ex.Message);
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
    DuplicateIdempotency,
}

public sealed record ScenarioResult(
    string Name,
    string Decision,
    string Reason,
    string IdempotencyKey
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
    string? NodeId = null
)
{
    public bool IsAllow => Decision == "ALLOW";
}
