# MRMI Gateway — Code Standards

## Package structure

| Package | Role |
|---------|------|
| `cmd/mrmi-gateway/` | Binary entry point only — flag parsing, signal handling, `app.Run`. No business logic. |
| `internal/app/` | Wiring layer — constructs components, starts servers, handles shutdown. No decisions. |
| `internal/core/` | Use-case layer — `Gateway` struct, `SendEnvelope`/`GetNodeInfo` orchestration. Domain types only; no proto, no gRPC, no HTTP imports. |
| `internal/<domain>/` | One package per domain concept: `audit`, `config`, `dedup`, `policy`. Owns its own types. |
| `internal/transport/grpc/` | gRPC adapter — translates proto ↔ core types, registers the gRPC service, manages the server/client lifecycle. No business decisions. |
| `internal/delivery/` | Forwarding, retry, DLQ — wires the gRPC client into the send path with backoff and dead-letter storage. |
| `internal/tlsutil/` | TLS loading helpers (`LoadServerTLS`, `LoadClientTLS`). No business logic. |
| `proto/mrmi/v1/` | `.proto` source files + generated `.pb.go` / `_grpc.pb.go`. Generated files are committed; never edited by hand. |

**Rule:** packages in `internal/transport/grpc/` must not import `internal/core` directly; the adapter receives a `GatewayService` interface. This keeps the transport layer testable without constructing a real core.

## Naming

- No stutter: `dedup.Index` not `dedup.DedupIndex`.
- `New(...)` for a single constructor; `NewXxx(...)` only when a package exports multiple.
- Package alias: import `internal/transport/grpc` as `grpctransport` everywhere — this is established in `app.go` and the integration tests.
- Exported errors: `var ErrXxx = errors.New(...)` at package level. Never bare `errors.New` inside a function where wrapping applies.
- Test helpers: first arg `*testing.T`, first statement `t.Helper()`.

## Error handling

- Wrap errors with context at every layer boundary: `fmt.Errorf("create policy engine: %w", err)`.
- `log.Fatalf` only at startup in `main`/`app.Run`.
- `status.Error(codes.Xxx, ...)` calls belong exclusively in `internal/transport/grpc/`. The core layer returns plain errors; the adapter converts them to gRPC status errors.
- Never discard errors silently. `_ = x` is acceptable only for `defer file.Close()` where the caller already has the content.

## Testing

- **Table-driven** for multiple input variants:
  ```go
  tests := []struct{ name, input, want string }{ ... }
  for _, tc := range tests {
      t.Run(tc.name, func(t *testing.T) { ... })
  }
  ```
- **White-box** (`package foo`) for same-package unit tests; **black-box** (`package foo_test`) for integration/external-behaviour tests.
- **Test server binding:** always `:0`; capture address via `srv.Addr()`. Never hard-code ports.
- **Build tag** for network/filesystem tests: `//go:build integration` as the first line.
- **Helper pattern** — established model to follow:
  ```go
  func startTestServer(t *testing.T) (addr string, client *Client) {
      t.Helper()
      // ... construct components, register t.Cleanup, return handles
  }
  ```

## Protobuf codegen workflow

**Prerequisites (install once):**
```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```
Both generators are pinned in `tools.go`. Ensure `$GOPATH/bin` (or `$GOBIN`) is on `$PATH`.

**Regenerate:**
```
make proto
```
This runs `protoc` from the repo root and writes `proto/mrmi/v1/contracts.pb.go` and `proto/mrmi/v1/contracts_grpc.pb.go`.

**Rules:**
- Generated files are committed alongside the `.proto` source.
- Never edit generated files by hand.
- The `.proto` file is the source of truth. When adding a field: update the proto first, run `make proto`, then update consuming Go code.
- After regeneration, run `go test ./...` to confirm no breakage.

## Config extension guide

To add a new TOML section (e.g., `[peers]` for Sprint 2):

1. Add a struct field with a `toml:` tag to the relevant raw struct in `internal/config/config.go`.
2. Add the corresponding typed field to the top-level `Config` (or sub-struct).
3. Add an `apply` branch that copies the raw value to the typed config.
4. Add a `Validate()` case for any required fields.
5. Update example files in `configs/` and `docs/ADR_v0.5.md` Appendix A.
6. Add a table-driven test case in `internal/config/config_test.go` using `t.TempDir()`.

## Security guidelines

- **mTLS is mandatory** for all inter-node gRPC (ADR-003). `grpc.WithInsecure()` is removed; use `insecure.NewCredentials()` only when `tls.insecure = true` is explicitly set in config.
- **Private keys** (`.pem`, `.key`) are in `.gitignore`. Never commit key material.
- **Test certificates** are generated in-process using Go's `crypto/x509` APIs and placed in `t.TempDir()` — no `openssl` dependency, no committed certs.
- The `ed25519:REPLACE_ME` placeholder in presets is intentional. Document in README that it must be replaced before production deployment. No code should silently accept this value in production.
- TLS config loading (in `internal/tlsutil`): if cert/key/CA paths are set and `insecure = false`, return an error on load failure — never silently fall back to insecure.
- Audit log entries contain no PII by design. The hash function covers region strings and decisions, not payload or identity bytes.

## Anti-patterns — what not to do

| Anti-pattern | Why |
|---|---|
| Hand-rolled TOML parsers | Cannot handle dotted-key sections or map values needed for `[peers]`. Use `BurntSushi/toml`. |
| Business logic in transport packages | `internal/transport/grpc/` handles encoding only. Decisions belong in `internal/core/`. |
| Duplicate proto types as hand-written Go structs | JSON codec hides field-name mismatches at compile time; silently breaks on proto schema changes. |
| `grpc.WithInsecure()` in non-test code | Prohibited by ADR-003. Use `internal/tlsutil.LoadClientTLS`. |
| Hard-coded ports in tests | Use `:0` and capture `srv.Addr()`. |
| Committed binary artifacts | `*.exe` and `*.exe~` are in `.gitignore`. Build artifacts are never committed. |
| Three representations of `Decision` | After proto codegen, `PolicyResult_Decision` (the proto enum) is canonical. Do not re-introduce string or custom-type duplicates. |
| `grpc.DialContext` with `grpc.WithBlock()` in tests | Causes flaky hangs if the server races. Migrate to `grpc.NewClient` in Sprint 2. |
