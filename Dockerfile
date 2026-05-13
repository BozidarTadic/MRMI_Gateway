# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY proto ./proto
COPY web ./web
RUN go build -o /out/mrmi-gateway ./cmd/mrmi-gateway

FROM mcr.microsoft.com/dotnet/sdk:10.0 AS dotnet-build
WORKDIR /src

COPY sdk/dotnet/MRMI.Gateway.Client/MRMI.Gateway.Client.csproj sdk/dotnet/MRMI.Gateway.Client/
COPY demo/blazor/MRMI.Demo.Blazor/MRMI.Demo.Blazor.csproj demo/blazor/MRMI.Demo.Blazor/
RUN dotnet restore demo/blazor/MRMI.Demo.Blazor/MRMI.Demo.Blazor.csproj

COPY sdk/dotnet ./sdk/dotnet
COPY demo/blazor ./demo/blazor
RUN dotnet publish demo/blazor/MRMI.Demo.Blazor/MRMI.Demo.Blazor.csproj \
    -c Release \
    -o /out/blazor \
    --no-restore

FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app

COPY --from=go-build /out/mrmi-gateway /app/mrmi-gateway
COPY --from=dotnet-build /out/blazor /app/blazor
COPY configs/node.rs.demo.toml /app/configs/node.rs.demo.toml
COPY configs/node.ru.demo.toml /app/configs/node.ru.demo.toml
COPY scripts/demo-start.sh /app/demo-start.sh

RUN chmod +x /app/demo-start.sh

ENV ASPNETCORE_ENVIRONMENT=Production
ENV Demo__RsUrl=http://127.0.0.1:8082
ENV Demo__RuUrl=http://127.0.0.1:8083

EXPOSE 8080
ENTRYPOINT ["/app/demo-start.sh"]
