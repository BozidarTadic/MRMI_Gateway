namespace MRMI.Gateway.Hosting.Aspire;

/// <summary>
/// Options used to add an MRMI Gateway executable resource to an Aspire AppHost.
/// </summary>
public sealed class MrmiGatewayResourceOptions
{
    /// <summary>
    /// Path to the MRMI Gateway executable. Defaults to resolving <c>mrmi-gateway</c>
    /// from the current process PATH.
    /// </summary>
    public string ExecutablePath { get; set; } = "mrmi-gateway";

    /// <summary>
    /// Working directory for the MRMI Gateway process. Defaults to the AppHost directory.
    /// </summary>
    public string? WorkingDirectory { get; set; }

    /// <summary>
    /// Optional TOML config file path passed as <c>-config &lt;path&gt;</c>.
    /// </summary>
    public string? ConfigPath { get; set; }

    /// <summary>
    /// Optional API key injected into the gateway process as <c>MRMI_API_KEY</c>.
    /// </summary>
    public string? ApiKey { get; set; }

    /// <summary>
    /// Host port for the HTTP management API endpoint. Leave null for Aspire allocation.
    /// </summary>
    public int? HttpPort { get; set; }

    /// <summary>
    /// Port where the MRMI process listens for HTTP management traffic.
    /// </summary>
    public int HttpTargetPort { get; set; } = 8080;

    /// <summary>
    /// Host port for the gRPC transport endpoint. Leave null for Aspire allocation.
    /// </summary>
    public int? GrpcPort { get; set; }

    /// <summary>
    /// Port where the MRMI process listens for gRPC transport traffic.
    /// </summary>
    public int GrpcTargetPort { get; set; } = 7777;

    /// <summary>
    /// Host port for Prometheus metrics. Leave null for Aspire allocation.
    /// </summary>
    public int? MetricsPort { get; set; }

    /// <summary>
    /// Port where the MRMI process exposes Prometheus metrics.
    /// </summary>
    public int MetricsTargetPort { get; set; } = 9090;

    /// <summary>
    /// Path served by the MRMI Prometheus metrics endpoint.
    /// </summary>
    public string MetricsPath { get; set; } = "/metrics";

    /// <summary>
    /// Extra command-line arguments appended after generated arguments.
    /// </summary>
    public IList<string> Args { get; } = [];

    internal string WorkingDirectoryOrDefault => WorkingDirectory ?? ".";
}
