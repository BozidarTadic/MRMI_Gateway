# Sprint 11 Plan — Polish and Architecture

## Goal

Polish the existing app and harden the architecture without adding new product features. Sprint 11 focuses on operator experience, runtime reliability, API consistency, documentation drift, and enforceable architecture boundaries.

## Non-Goals

- No new corridor capabilities.
- No new public backend endpoints except tiny compatibility helpers required for consistency.
- No new protocol or data model expansion unless needed to preserve existing behavior safely.

## Tasks

| # | Title | Status |
|---|---|---|
| [#72](https://github.com/BozidarTadic/MRMI_Gateway/issues/72) | Dashboard polish and operator UX cleanup | Todo |
| [#73](https://github.com/BozidarTadic/MRMI_Gateway/issues/73) | Enforce architecture package boundaries | Todo |
| [#74](https://github.com/BozidarTadic/MRMI_Gateway/issues/74) | App lifecycle and shutdown hardening | Todo |
| [#75](https://github.com/BozidarTadic/MRMI_Gateway/issues/75) | Management API error and response consistency pass | Todo |
| [#76](https://github.com/BozidarTadic/MRMI_Gateway/issues/76) | Config, README, and operator docs polish | Todo |
| [#77](https://github.com/BozidarTadic/MRMI_Gateway/issues/77) | Test suite reliability and acceptance cleanup | Todo |
| [#78](https://github.com/BozidarTadic/MRMI_Gateway/issues/78) | Internal naming and logging polish pass | Todo |

## Workstreams

### App Polish

The dashboard and operator-facing text should feel consistent with the current v0.5 runtime. Focus on empty states, loading states, auth/API errors, responsive layout, and terminology. Avoid adding backend features during this pass.

### Architecture Hardening

The documented dependency rules should become executable checks. The most important boundaries are:

- `internal/core` must stay transport-agnostic.
- `internal/transport/grpc` must remain an adapter layer.
- `cmd/*` must stay thin and delegate behavior to packages.
- `internal/app` should remain wiring and lifecycle code, not business logic.

### Runtime Reliability

Startup, cancellation, shutdown, listener ownership, and error wrapping should be reviewed as one operational flow. Health and readiness behavior should be clear during startup and shutdown.

### API and Docs Consistency

The management API, CLI-facing responses, README, guides, ADR, and shipped TOML configs should agree on current behavior. Demo-only settings must be clearly separated from production expectations.

### Test Maintenance

Acceptance and integration tests should be easier to extend. Prefer shared helpers where they reduce duplication, `:0` listeners for test servers, explicit integration tags, and behavioral assertions over brittle timing.

## Acceptance Criteria

- All Sprint 11 issues are in the `MRMI Gateway` GitHub Project with `sprint-11` labels.
- `go test ./...` remains the default validation command.
- Dashboard, CLI, docs, and API behavior are consistent with the current v0.5 feature set.
- Architecture rules are documented and enforced by tests or a maintainable repository check.
- No Sprint 11 work introduces a new feature claim without implementation and tests.
