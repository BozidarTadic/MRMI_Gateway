using MRMI.Gateway.Client;

namespace MRMI.Demo.Blazor;

/// <summary>
/// Singleton that holds both gateway clients and the shared corridor log.
/// </summary>
public sealed class DemoState
{
    // ── Clients ───────────────────────────────────────────────────────────────

    public MrmiClient RsClient { get; }
    public MrmiClient RuClient { get; }

    // ── Users ─────────────────────────────────────────────────────────────────

    public const string RsUser = "Marko Petrović";
    public const string RsRegion = "RS";
    public const string RuUser = "Иван Иванов";
    public const string RuRegion = "RU";

    // ── Corridor log ──────────────────────────────────────────────────────────

    private readonly List<LogEntry> _log = [];
    private readonly object _logLock = new();

    public IReadOnlyList<LogEntry> Log
    {
        get { lock (_logLock) { return _log.AsReadOnly(); } }
    }

    public event Action? OnLogChanged;

    public DemoState(IConfiguration config)
    {
        var rsUrl = config["Demo:RsUrl"] ?? "http://localhost:8080";
        var ruUrl = config["Demo:RuUrl"] ?? "http://localhost:8081";

        RsClient = new MrmiClient(new MrmiClientOptions { BaseUrl = rsUrl });
        RuClient = new MrmiClient(new MrmiClientOptions { BaseUrl = ruUrl });
    }

    public void AddLog(LogEntry entry)
    {
        lock (_logLock)
        {
            _log.Insert(0, entry);
            if (_log.Count > 100) _log.RemoveAt(_log.Count - 1);
        }
        OnLogChanged?.Invoke();
    }

    public void ClearLog()
    {
        lock (_logLock) { _log.Clear(); }
        OnLogChanged?.Invoke();
    }
}

public sealed record LogEntry(
    DateTimeOffset Timestamp,
    string Direction,   // "RS → RU" or "RU → RS"
    string Decision,
    string Reason,
    string IdempotencyKey
)
{
    public bool IsAllow => Decision == "ALLOW";
}
