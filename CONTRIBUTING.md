# Contributing to MRMI Gateway

Thank you for your interest in contributing. MRMI Gateway is an infrastructure project — correctness, auditability, and regulatory compliance are non-negotiable constraints, so we hold contributions to a high bar.

## Before You Start

Read the [Architecture Decision Record](docs/ADR_v0.5.md). Every design choice in the codebase traces back to it. If you want to propose something that contradicts an ADR decision, open an issue first — don't send code that will immediately conflict.

## What Needs Work

The [v0.1 milestone tracker](docs/ADR_v0.5.md#12-next-steps--v01-milestones) lists open work. Items explicitly marked as "open for contributors":

- **Milestone 8** — CLI reference client
- **Milestone 9** — Extension API design  
- **Milestone 10** — Java SDK

Before picking up anything else, check open issues to avoid duplication.

## Development Setup

```bash
# Prerequisites: Go 1.21+, protoc, protoc-gen-go, protoc-gen-go-grpc

git clone https://github.com/tadicbb/mrmi-gateway
cd mrmi-gateway

go mod download
go build ./...
go test ./...
```

Generate protobuf code after modifying `.proto` files:

```bash
protoc --go_out=. --go-grpc_out=. proto/mrmi/v1/contracts.proto
```

## Contribution Process

1. Open an issue describing what you want to do and why.
2. Wait for a maintainer to acknowledge it — this avoids wasted effort.
3. Fork, branch from `main`, implement.
4. All new code needs tests. No exceptions for gateway-critical paths.
5. Run `go vet ./...` and `go test ./...` — both must pass clean.
6. Open a pull request referencing the issue.

## Code Standards

- Go standard formatting (`gofmt`). No exceptions.
- No external dependencies without prior discussion in an issue.
- Cryptographic code (Merkle log, signatures, mTLS) requires a second review from a maintainer before merge — do not ship crypto changes unreviewed.
- All audit-log-touching code must preserve the Merkle chain invariant: a new entry must chain to the SHA-256 of the previous entry's raw bytes.

## Reporting Security Issues

Do **not** open a public issue for security vulnerabilities. Email the maintainer directly at tadicbb@gmail.com with subject `[MRMI Security]`. Include a description, reproduction steps, and your assessment of impact. We aim to respond within 72 hours.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
