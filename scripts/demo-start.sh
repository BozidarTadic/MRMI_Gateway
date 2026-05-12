#!/usr/bin/env bash
set -euo pipefail

port="${PORT:-8080}"

/app/mrmi-gateway -config /app/configs/node.rs.demo.toml &
rs_pid="$!"

/app/mrmi-gateway -config /app/configs/node.ru.demo.toml &
ru_pid="$!"

cleanup() {
  kill "$rs_pid" "$ru_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cd /app/blazor
ASPNETCORE_URLS="http://0.0.0.0:${port}" dotnet MRMI.Demo.Blazor.dll &
blazor_pid="$!"

wait -n "$rs_pid" "$ru_pid" "$blazor_pid"
