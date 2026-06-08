using MRMI.Gateway.Hosting.Aspire;

var builder = DistributedApplication.CreateBuilder(args);

var repoRoot = FindRepositoryRoot(AppContext.BaseDirectory);
var generatedConfigDirectory = Path.Combine(
    repoRoot,
    "logs",
    "aspire-demo",
    "generated-configs");

Directory.CreateDirectory(generatedConfigDirectory);

var rsHttpPort = 18080;
var ruHttpPort = 18081;
var rsGrpcPort = 17787;
var ruGrpcPort = 17788;
var rsMetricsPort = 19092;
var ruMetricsPort = 19093;

var rsConfig = WriteGeneratedConfig(
    sourcePath: Path.Combine(repoRoot, "configs", "node.rs.demo.toml"),
    targetPath: Path.Combine(generatedConfigDirectory, "node.rs.aspire.toml"),
    httpPort: rsHttpPort,
    grpcPort: rsGrpcPort,
    metricsPort: rsMetricsPort,
    peerRegion: "RU",
    peerPort: ruGrpcPort);

var ruConfig = WriteGeneratedConfig(
    sourcePath: Path.Combine(repoRoot, "configs", "node.ru.demo.toml"),
    targetPath: Path.Combine(generatedConfigDirectory, "node.ru.aspire.toml"),
    httpPort: ruHttpPort,
    grpcPort: ruGrpcPort,
    metricsPort: ruMetricsPort,
    peerRegion: "RS",
    peerPort: rsGrpcPort);

var rsGateway = builder.AddMrmiGateway("mrmi-rs", options =>
{
    options.ExecutablePath = "go";
    options.WorkingDirectory = repoRoot;
    options.HttpPort = rsHttpPort;
    options.HttpTargetPort = rsHttpPort;
    options.GrpcPort = rsGrpcPort;
    options.GrpcTargetPort = rsGrpcPort;
    options.MetricsPort = rsMetricsPort;
    options.MetricsTargetPort = rsMetricsPort;
    options.Args.Add("run");
    options.Args.Add("./cmd/mrmi-gateway");
    options.Args.Add("-config");
    options.Args.Add(rsConfig);
});

var ruGateway = builder.AddMrmiGateway("mrmi-ru", options =>
{
    options.ExecutablePath = "go";
    options.WorkingDirectory = repoRoot;
    options.HttpPort = ruHttpPort;
    options.HttpTargetPort = ruHttpPort;
    options.GrpcPort = ruGrpcPort;
    options.GrpcTargetPort = ruGrpcPort;
    options.MetricsPort = ruMetricsPort;
    options.MetricsTargetPort = ruMetricsPort;
    options.Args.Add("run");
    options.Args.Add("./cmd/mrmi-gateway");
    options.Args.Add("-config");
    options.Args.Add(ruConfig);
});

var rsBaseUrl = "http://localhost:" + rsHttpPort;
var ruBaseUrl = "http://localhost:" + ruHttpPort;

builder.AddProject<Projects.MRMI_Demo_Blazor>("mrmi-demo-ui")
    .WithEnvironment("Demo__RsUrl", rsBaseUrl)
    .WithEnvironment("Demo__RuUrl", ruBaseUrl)
    .WithReference(rsGateway.GetEndpoint("http"))
    .WithReference(ruGateway.GetEndpoint("http"))
    .WaitFor(rsGateway)
    .WaitFor(ruGateway);

builder.Build().Run();

static string FindRepositoryRoot(string startDirectory)
{
    var directory = new DirectoryInfo(startDirectory);
    while (directory is not null)
    {
        if (File.Exists(Path.Combine(directory.FullName, "go.mod")))
        {
            return directory.FullName;
        }

        directory = directory.Parent;
    }

    throw new InvalidOperationException("Could not find MRMI_Gateway repository root from AppHost output directory.");
}

static string WriteGeneratedConfig(
    string sourcePath,
    string targetPath,
    int httpPort,
    int grpcPort,
    int metricsPort,
    string peerRegion,
    int peerPort)
{
    var config = File.ReadAllText(sourcePath);
    config = ReplaceRegex(config, @"grpc_listen_addr\s*=\s*""0\.0\.0\.0:\d+""", $"grpc_listen_addr = \"0.0.0.0:{grpcPort}\"");
    config = ReplaceRegex(config, @"http_listen_addr\s*=\s*""0\.0\.0\.0:\d+""", $"http_listen_addr = \"0.0.0.0:{httpPort}\"");
    config = ReplaceRegex(config, @"metrics_addr\s*=\s*""0\.0\.0\.0:\d+""", $"metrics_addr     = \"0.0.0.0:{metricsPort}\"");
    config = ReplaceRegex(config, $@"(\[peers\.{peerRegion}\]\s+addr\s*=\s*)""localhost:\d+""", $"$1\"localhost:{peerPort}\"");

    File.WriteAllText(targetPath, config);
    return targetPath;
}

static string ReplaceRegex(string input, string pattern, string replacement) =>
    System.Text.RegularExpressions.Regex.Replace(
        input,
        pattern,
        replacement,
        System.Text.RegularExpressions.RegexOptions.Multiline);
