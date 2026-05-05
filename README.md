# MRMI Gateway

MRMI Gateway is a regional federation middleware for regulated cross-border messaging corridors. This repository is being built from the architecture in [MRMI_Gateway_ADR_v0.5.md](./MRMI_Gateway_ADR_v0.5.md).

## Current status

The repository now contains the initial bootstrap needed to start implementation:

- `cmd/mrmi-gateway/`: process entrypoint
- `internal/config/`: configuration model and defaults
- `internal/policy/`: first in-memory policy engine
- `internal/server/`: health and audit bootstrap endpoints
- `proto/mrmi/v1/`: v0.1 protobuf contracts
- `configs/`: sample operator configuration assets

## Run

```powershell
go run ./cmd/mrmi-gateway
```

This starts the bootstrap HTTP server on `:8080` with:

- `/healthz`
- `/readyz`
- `/.well-known/mrmi-audit`

## Next implementation slices

1. TOML parsing and signed config verification
2. gRPC transport and protobuf code generation
3. Idempotency, dedup, retry, ACK, and DLQ
4. Merkle audit log persistence and verification
5. CRL gossip and blacklist quorum flows
