# MRMI Gateway .NET SDK

`MRMI.Gateway.Client` is a .NET client library for the MRMI Gateway management REST API and SSE stream.
`MRMI.Gateway.Hosting.Aspire` adds .NET Aspire AppHost support for running an MRMI Gateway node as an executable resource.

## Installation

Install from NuGet:

```bash
dotnet add package MRMI.Gateway.Client --version 0.3.0
dotnet add package MRMI.Gateway.Hosting.Aspire --version 0.3.0
```

## Quick start

```csharp
using MRMI.Gateway.Client;

var client = new MrmiClient(new MrmiClientOptions
{
    BaseUrl  = "http://localhost:8080",
    ApiKey   = "my-operator-key",
});

// Send a messaging envelope (default schema — backward-compatible with v0.8)
var response = await client.SendAsync(new SendEnvelopeRequest
{
    IdempotencyKey  = Guid.NewGuid().ToString(),
    SenderRegion    = "RS",
    RecipientRegion = "RU",
    TrustTier       = 1,
    Payload         = Encoding.UTF8.GetBytes("Hello!"),
});

Console.WriteLine(response.Decision);   // ALLOW
```

## Aspire AppHost

Add the hosting package to an Aspire AppHost project:

```bash
dotnet add package MRMI.Gateway.Hosting.Aspire --version 0.3.0
```

Then register the MRMI Gateway executable:

```csharp
using MRMI.Gateway.Hosting.Aspire;

var builder = DistributedApplication.CreateBuilder(args);

var gateway = builder.AddMrmiGateway("mrmi-rs", options =>
{
    options.ExecutablePath = "mrmi-gateway";
    options.ConfigPath = "../../configs/node.rs.local.toml";
    options.HttpPort = 8080;
    options.GrpcPort = 7777;
    options.MetricsPort = 9090;
    options.ApiKey = "demo-key";
});

builder.AddProject<Projects.MyApi>("api")
    .WithMrmiGatewayClientEnvironment(gateway, apiKey: "demo-key");

builder.Build().Run();
```

The gateway resource exposes endpoints named `http`, `grpc`, and `metrics`, uses `/healthz` for Aspire health checks, and displays the `metrics` endpoint as `/metrics` by default.

## Schema types (ADR-015)

Use the `SchemaType` enum to declare the payload domain. The gateway enforces
`jurisdiction_isolation` rules per schema type.

| Enum value            | Wire value                 | Description                          |
|-----------------------|----------------------------|--------------------------------------|
| `SchemaType.Messaging`  | `messaging`              | Default — v0.8 backward-compatible   |
| `SchemaType.Iso20022`   | `iso20022`               | ISO 20022 financial messages         |
| `SchemaType.Hl7Fhir`    | `hl7fhir`                | HL7 FHIR healthcare data             |
| `SchemaType.Edifact`    | `edifact`                | UN/EDIFACT trade documents           |
| `SchemaType.Custom`     | `custom:<CustomSchemaId>`| Custom domain adapter                |

```csharp
// ISO 20022 payment initiation
var response = await client.SendAsync(new SendEnvelopeRequest
{
    IdempotencyKey  = Guid.NewGuid().ToString(),
    SenderRegion    = "RS",
    RecipientRegion = "RU",
    SchemaType      = SchemaType.Iso20022,
    SchemaVersion   = "1.0.0",
    Payload         = iso20022XmlBytes,
});

// Custom domain adapter
var response = await client.SendAsync(new SendEnvelopeRequest
{
    IdempotencyKey  = Guid.NewGuid().ToString(),
    SenderRegion    = "RS",
    RecipientRegion = "RU",
    SchemaType      = SchemaType.Custom,
    CustomSchemaId  = "gov-rs-doc-exchange",
    Payload         = documentBytes,
});
```

`SchemaType` defaults to `SchemaType.Messaging` and `SchemaVersion` defaults to `"1.0.0"`,
so existing code that omits both fields continues to work unchanged.

## Receiving envelopes (SSE)

```csharp
await client.ReceiveAsync(async envelope =>
{
    Console.WriteLine($"Received from {envelope.SenderRegion}: {envelope.IdempotencyKey}");
    await Task.CompletedTask;
}, cancellationToken);

// Or as an async stream:
await foreach (var envelope in client.StreamAsync(cancellationToken))
{
    Console.WriteLine(envelope.IdempotencyKey);
}
```

## Authentication

| Method            | How to configure                              |
|-------------------|-----------------------------------------------|
| API key           | `new MrmiClientOptions { ApiKey = "..." }`    |
| JWT bearer token  | `new MrmiClientOptions { JwtToken = "..." }`  |

JWT takes precedence when both are set. Issue a short-lived JWT token:

```csharp
var issued = await client.IssueTokenAsync(scope: "operator", ttlMinutes: 60);
// use issued.Token as JwtToken in a new client
```

## Node status and audit log

```csharp
var status = await client.GetStatusAsync();
Console.WriteLine($"{status.NodeId} / {status.Profile} — up {status.UptimeSeconds}s");

var entries = await client.GetAuditLatestAsync(count: 50);
foreach (var e in entries)
    Console.WriteLine($"[{e.Decision}] {e.SenderRegion}→{e.RecipientRegion}");
```
