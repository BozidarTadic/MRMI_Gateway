using MRMI.Gateway.Client;

namespace MRMI.Demo.Blazor;

public sealed class DemoState : IAsyncDisposable
{
    public MrmiClient RsClient { get; }
    public MrmiClient RuClient { get; }

    public const string RsUser = "Marko Petrović";
    public const string RsRegion = "RS";
    public const string RuUser = "Иван Иванов";
    public const string RuRegion = "RU";

    private readonly List<ChatMessage> _rsMessages = [];
    private readonly List<ChatMessage> _ruMessages = [];
    private readonly List<LogEntry> _log = [];
    private readonly object _lock = new();

    public IReadOnlyList<ChatMessage> RsMessages { get { lock (_lock) return [.. _rsMessages]; } }
    public IReadOnlyList<ChatMessage> RuMessages { get { lock (_lock) return [.. _ruMessages]; } }
    public IReadOnlyList<LogEntry> Log { get { lock (_lock) return _log.AsReadOnly(); } }

    public bool RsOnline { get; private set; }
    public bool RuOnline { get; private set; }

    public event Action? OnChanged;

    private CancellationTokenSource? _cts;
    private static int _seqCounter;

    public DemoState(IConfiguration config)
    {
        var rsUrl = config["Demo:RsUrl"] ?? "http://localhost:8080";
        var ruUrl = config["Demo:RuUrl"] ?? "http://localhost:8081";
        RsClient = new MrmiClient(new MrmiClientOptions { BaseUrl = rsUrl });
        RuClient = new MrmiClient(new MrmiClientOptions { BaseUrl = ruUrl });
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
                        ? System.Text.Encoding.UTF8.GetString(env.Payload)
                        : "(empty)";

                    var msg = new ChatMessage(
                        DateTimeOffset.UtcNow,
                        env.SenderRegion,
                        env.RecipientRegion,
                        text,
                        env.IdempotencyKey);

                    lock (_lock)
                    {
                        if (nodeRegion == RsRegion) _rsMessages.Add(msg);
                        else _ruMessages.Add(msg);
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

    public async Task<(string Decision, string Reason)> SendAsync(
        string fromRegion, string toRegion, string text)
    {
        var key = $"demo-{Interlocked.Increment(ref _seqCounter):D6}";
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
                _log.Insert(0, new LogEntry(DateTimeOffset.UtcNow,
                    $"{fromRegion} → {toRegion}", result.Decision, result.Reason, key));
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

    public void ClearLog()
    {
        lock (_lock) { _log.Clear(); }
        OnChanged?.Invoke();
    }

    public void ClearMessages()
    {
        lock (_lock) { _rsMessages.Clear(); _ruMessages.Clear(); }
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

public sealed record ChatMessage(
    DateTimeOffset Timestamp,
    string FromRegion,
    string ToRegion,
    string Text,
    string IdempotencyKey
);

public sealed record LogEntry(
    DateTimeOffset Timestamp,
    string Direction,
    string Decision,
    string Reason,
    string IdempotencyKey
)
{
    public bool IsAllow => Decision == "ALLOW";
}
