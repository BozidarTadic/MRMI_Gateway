using Aspire.Hosting;
using Aspire.Hosting.ApplicationModel;

namespace MRMI.Gateway.Hosting.Aspire;

/// <summary>
/// Aspire AppHost extensions for MRMI Gateway.
/// </summary>
public static class MrmiGatewayResourceBuilderExtensions
{
    /// <summary>
    /// Adds an MRMI Gateway node as an executable resource.
    /// </summary>
    /// <param name="builder">The distributed application builder.</param>
    /// <param name="name">The Aspire resource name.</param>
    /// <param name="configure">Optional resource configuration.</param>
    /// <returns>The MRMI Gateway executable resource builder.</returns>
    public static IResourceBuilder<ExecutableResource> AddMrmiGateway(
        this IDistributedApplicationBuilder builder,
        string name = "mrmi-gateway",
        Action<MrmiGatewayResourceOptions>? configure = null)
    {
        ArgumentNullException.ThrowIfNull(builder);
        ArgumentException.ThrowIfNullOrWhiteSpace(name);

        var options = new MrmiGatewayResourceOptions();
        configure?.Invoke(options);

        var args = new List<string>();
        if (!string.IsNullOrWhiteSpace(options.ConfigPath))
        {
            args.Add("-config");
            args.Add(options.ConfigPath);
        }

        args.AddRange(options.Args);

        var resource = builder
            .AddExecutable(name, options.ExecutablePath, options.WorkingDirectoryOrDefault, [.. args])
            .WithHttpEndpoint(
                port: options.HttpPort,
                targetPort: options.HttpTargetPort,
                name: "http",
                isProxied: false)
            .WithEndpoint(
                port: options.GrpcPort,
                targetPort: options.GrpcTargetPort,
                scheme: "tcp",
                name: "grpc",
                isProxied: false)
            .WithHttpEndpoint(
                port: options.MetricsPort,
                targetPort: options.MetricsTargetPort,
                name: "metrics",
                isProxied: false)
            .WithHttpHealthCheck("/healthz", endpointName: "http");

        if (!string.IsNullOrWhiteSpace(options.MetricsPath))
        {
            resource.WithUrlForEndpoint("metrics", url =>
            {
                url.Url = options.MetricsPath;
                url.DisplayText = "Metrics";
            });
        }

        if (!string.IsNullOrWhiteSpace(options.ApiKey))
        {
            resource.WithEnvironment("MRMI_API_KEY", options.ApiKey);
        }

        return resource;
    }

    /// <summary>
    /// Injects MRMI Gateway client environment variables into another Aspire resource.
    /// </summary>
    /// <typeparam name="T">The destination resource type.</typeparam>
    /// <param name="builder">The destination resource builder.</param>
    /// <param name="gateway">The MRMI Gateway resource returned by <see cref="AddMrmiGateway"/>.</param>
    /// <param name="baseUrlEnvironmentVariableName">
    /// Environment variable that receives the HTTP endpoint URL.
    /// Defaults to <c>MRMI_GATEWAY_BASE_URL</c>.
    /// </param>
    /// <param name="apiKey">
    /// Optional API key to inject into the destination as <c>MRMI_GATEWAY_API_KEY</c>.
    /// </param>
    /// <returns>The destination resource builder.</returns>
    public static IResourceBuilder<T> WithMrmiGatewayClientEnvironment<T>(
        this IResourceBuilder<T> builder,
        IResourceBuilder<ExecutableResource> gateway,
        string baseUrlEnvironmentVariableName = "MRMI_GATEWAY_BASE_URL",
        string? apiKey = null)
        where T : IResourceWithEnvironment, IResourceWithWaitSupport
    {
        ArgumentNullException.ThrowIfNull(builder);
        ArgumentNullException.ThrowIfNull(gateway);
        ArgumentException.ThrowIfNullOrWhiteSpace(baseUrlEnvironmentVariableName);

        builder
            .WithReference(gateway.GetEndpoint("http"))
            .WithEnvironment(baseUrlEnvironmentVariableName, gateway.GetEndpoint("http"))
            .WaitFor(gateway);

        if (!string.IsNullOrWhiteSpace(apiKey))
        {
            builder.WithEnvironment("MRMI_GATEWAY_API_KEY", apiKey);
        }

        return builder;
    }
}
