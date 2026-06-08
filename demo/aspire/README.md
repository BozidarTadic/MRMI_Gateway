# MRMI Aspire Demo

This Aspire AppHost orchestrates the local MRMI demo corridor:

- `mrmi-rs`: RS gateway node from `configs/node.rs.demo.toml`
- `mrmi-ru`: RU gateway node from `configs/node.ru.demo.toml`
- `mrmi-demo-ui`: existing Blazor demo UI
- `MRMI.Demo.ServiceDefaults`: shared Aspire defaults for the Blazor UI

Each gateway resource also exposes an Aspire `metrics` endpoint that opens the node's Prometheus `/metrics` output.
The Blazor UI uses the standard Aspire defaults for OpenTelemetry, health checks, service discovery, and resilient HTTP client defaults.

## Prerequisites

- Go 1.25+
- .NET 10 SDK
- Free local ports: `18080`, `18081`, `17787`, `17788`, `19092`, `19093`, `5294`

## Run

From the repository root:

```powershell
dotnet run --project demo\aspire\MRMI.Demo.AppHost\MRMI.Demo.AppHost.csproj
```

Open the Aspire dashboard URL printed by `dotnet run`, then open `mrmi-demo-ui` from the dashboard.

The AppHost starts both gateway nodes with `go run ./cmd/mrmi-gateway -config <config>` and injects these UI settings:

- `Demo__RsUrl=http://localhost:18080`
- `Demo__RuUrl=http://localhost:18081`

The Blazor UI also exposes Aspire default endpoints in development:

- `/health`
- `/alive`
