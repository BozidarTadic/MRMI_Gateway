using MRMI.Gateway.Client;

namespace MRMI.Demo.Blazor;

public sealed class DemoState : IAsyncDisposable
{
    public MrmiClient RsClient { get; }
    public MrmiClient RuClient { get; }

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
        var rsUrl = config["Demo:RsUrl"] ?? "http://localhost:8080";
        var ruUrl = config["Demo:RuUrl"] ?? "http://localhost:8081";
        RsClient = new MrmiClient(new MrmiClientOptions { BaseUrl = rsUrl });
        RuClient = new MrmiClient(new MrmiClientOptions { BaseUrl = ruUrl });
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
    public IReadOnlyList<JurisdictionRequest> JurisdictionRequests { get { lock (_lock) return [.. _jurisdictionRequests]; } }

    private readonly List<JurisdictionRequest> _jurisdictionRequests = [];

    public async Task<(string Decision, string Reason)> SendAsync(
        string rsId, string ruId, string fromRegion, string text)
    {
        var toRegion = fromRegion == RsRegion ? RuRegion : RsRegion;
        var key = $"demo-{_sessionPrefix}:{rsId}:{ruId}:{Interlocked.Increment(ref _seqCounter):D6}";
        var client = fromRegion == RsRegion ? RsClient : RuClient;

        try
        {
            var result = await client.SendAsync(new SendEnvelopeRequest
            {
                IdempotencyKey = key,
                SenderRegion = fromRegion,
                RecipientRegion = toRegion,
                TrustTier = 1,
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
        lock (_lock)
        {
            _log.Clear();
            _jurisdictionRequests.Clear();
        }
        OnChanged?.Invoke();
    }

    public DemoMetrics GetMetrics()
    {
        lock (_lock)
        {
            var entries = _log.ToList();
            var messages = _chats.Values.SelectMany(m => m).ToList();
            var requestCount = _jurisdictionRequests.Count;
            var allowedRequests = _jurisdictionRequests.Count(r => r.Decision == "ALLOW");
            var exposedPayloads = _jurisdictionRequests.Sum(r => r.ReturnedPayloadCount);
            var exposedBytes = _jurisdictionRequests.Sum(r => r.ReturnedBytes);

            return new DemoMetrics(
                TotalEnvelopes: entries.Count,
                AllowedEnvelopes: entries.Count(e => e.Decision == "ALLOW"),
                DeniedEnvelopes: entries.Count(e => e.Decision == "DENY"),
                ErrorEnvelopes: entries.Count(e => e.Decision == "ERROR"),
                RsVisibleMessages: messages.Count(m => m.FromRegion == RsRegion || m.ToRegion == RsRegion),
                RuVisibleMessages: messages.Count(m => m.FromRegion == RuRegion || m.ToRegion == RuRegion),
                JurisdictionRequests: requestCount,
                AllowedJurisdictionRequests: allowedRequests,
                DeniedJurisdictionRequests: requestCount - allowedRequests,
                PayloadsExposed: exposedPayloads,
                BytesExposed: exposedBytes,
                LatestAuditRootHash: entries.FirstOrDefault(e => !string.IsNullOrWhiteSpace(e.AuditRootHash))?.AuditRootHash,
                LatestPeerAuditRootHash: entries.FirstOrDefault(e => !string.IsNullOrWhiteSpace(e.PeerAuditRootHash))?.PeerAuditRootHash
            );
        }
    }

    public JurisdictionRequest SubmitJurisdictionRequest(string requesterRegion, string dataRegion, string dataClass)
    {
        requesterRegion = NormalizeRegion(requesterRegion);
        dataRegion = NormalizeRegion(dataRegion);
        dataClass = string.IsNullOrWhiteSpace(dataClass) ? "Messages" : dataClass.Trim();

        JurisdictionRequest request;
        lock (_lock)
        {
            var visible = ResolveVisibleData(requesterRegion, dataRegion, dataClass);
            var decision = visible.Count > 0 ? "ALLOW" : "DENY";
            var reason = decision == "ALLOW"
                ? requesterRegion == dataRegion
                    ? "local jurisdiction can inspect hosted data"
                    : "cross-border data exists only for allowed corridor traffic"
                : "no policy-visible data for requester and data holder";

            request = new JurisdictionRequest(
                Timestamp: DateTimeOffset.UtcNow,
                RequesterRegion: requesterRegion,
                DataRegion: dataRegion,
                DataClass: dataClass,
                Decision: decision,
                Reason: reason,
                ReturnedPayloadCount: visible.Count(v => v.Payload is not null),
                ReturnedBytes: visible.Sum(v => v.Payload?.Length ?? 0),
                ReturnedData: visible);

            _jurisdictionRequests.Insert(0, request);
            if (_jurisdictionRequests.Count > 100) _jurisdictionRequests.RemoveAt(_jurisdictionRequests.Count - 1);
        }
        OnChanged?.Invoke();
        return request;
    }

    private IReadOnlyList<ReturnedDataItem> ResolveVisibleData(string requesterRegion, string dataRegion, string dataClass)
    {
        if (dataClass.Equals("Audit metadata", StringComparison.OrdinalIgnoreCase))
        {
            return _log
                .Where(e => e.Direction.Contains(dataRegion, StringComparison.OrdinalIgnoreCase))
                .Take(20)
                .Select(e => new ReturnedDataItem(
                    Kind: "audit",
                    Direction: e.Direction,
                    Payload: requesterRegion == dataRegion || e.Direction.Contains(requesterRegion, StringComparison.OrdinalIgnoreCase)
                        ? e.Payload
                        : null,
                    Metadata: $"{e.Decision} / {e.Reason} / root {ShortHash(e.AuditRootHash)}"))
                .ToList();
        }

        var messages = _chats.Values.SelectMany(m => m)
            .Where(m => m.FromRegion == dataRegion || m.ToRegion == dataRegion);

        if (requesterRegion != dataRegion)
        {
            messages = messages.Where(m => m.FromRegion == requesterRegion || m.ToRegion == requesterRegion);
        }

        return messages
            .OrderByDescending(m => m.Timestamp)
            .Take(20)
            .Select(m => new ReturnedDataItem(
                Kind: "message",
                Direction: $"{m.FromRegion} -> {m.ToRegion}",
                Payload: m.Text,
                Metadata: m.IdempotencyKey))
            .ToList();
    }

    private static string NormalizeRegion(string value)
        => string.IsNullOrWhiteSpace(value) ? "RS" : value.Trim().ToUpperInvariant();

    private static string ShortHash(string? value)
        => string.IsNullOrWhiteSpace(value) ? "none" : value.Length > 10 ? value[..10] : value;

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

public sealed record DemoMetrics(
    int TotalEnvelopes,
    int AllowedEnvelopes,
    int DeniedEnvelopes,
    int ErrorEnvelopes,
    int RsVisibleMessages,
    int RuVisibleMessages,
    int JurisdictionRequests,
    int AllowedJurisdictionRequests,
    int DeniedJurisdictionRequests,
    int PayloadsExposed,
    int BytesExposed,
    string? LatestAuditRootHash,
    string? LatestPeerAuditRootHash
);

public sealed record JurisdictionRequest(
    DateTimeOffset Timestamp,
    string RequesterRegion,
    string DataRegion,
    string DataClass,
    string Decision,
    string Reason,
    int ReturnedPayloadCount,
    int ReturnedBytes,
    IReadOnlyList<ReturnedDataItem> ReturnedData
);

public sealed record ReturnedDataItem(
    string Kind,
    string Direction,
    string? Payload,
    string Metadata
);
