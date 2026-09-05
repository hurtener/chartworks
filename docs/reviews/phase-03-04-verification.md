# Phase 03/04 verification record

Date: 2026-09-05. Execution environment: GitHub Actions Ubuntu 24.04 / linux-amd64, pinned Go 1.26.4, PostgreSQL 17 with real pgx transactions and PostgreSQL client recovery tools. These are hosted execution results, not a claim of a local PostgreSQL checkout, production Pengui session or customer warehouse test.

## Runtime evidence baseline

Normalized commit `0feb589663f32680bcbb696b10c094da1408d9d9`, tested by preparation run `33974190050`, passed:

- CGo-free shipping compilation and go vet.
- The complete race-enabled real PostgreSQL suite, including earlier migration/CAS/fencing/backup/recovery tests.
- All six `TestPhase03/ACxx` and six `TestPhase04/ACxx` results, without missing/skipped criteria.
- All twelve earlier phase01/02 criteria; cumulative preflight reported four passing phases and thirty explicitly unimplemented later phases.
- `TestCompiledAuthorityLifecycle`: actual compiled binary, trusted ephemeral TLS JWKS, valid and rejected bearer calls, real PostgreSQL mutation/read/sweep, public SDK diagnostics/metrics, and joined SIGTERM shutdown.
- All 31 Python repository-tool regression tests and planning/mirror coherence.

Its only remaining lint finding was the repository source-scan test's race-prone filesystem walk/read. That has been changed to a confined `os.Root` read, not a blanket linter exclusion. The default key HTTP transport was also hardened without a type-assertion panic and with bounded response headers. Final source is recompiled and rechecked rather than inheriting a green status from that earlier head.

## Measured baseline coverage

These are full-suite instrumented values from that run, not per-package test-only guesses. No threshold was lowered.

| Package | Measured | Required |
|---|---:|---:|
| internal/auth | 94.83% | 85% |
| internal/identity | 100.00% | 85% |
| internal/access | 96.39% | 85% |
| internal/securityapi | 87.50% | 85% |
| sdk/chartworks | 96.97% | 80% |
| internal/config | 95.56% | 80% |
| internal/foundation | 93.07% | 80% |
| internal/store | 100.00% | 85% |
| internal/store/postgres | 88.41% | 85% |
| internal/maintenance | 88.24% | 80% |
| internal/telemetry | 95.00% | 80% |
| cmd/chartworks | 75.00% | 70% |

## Final exact-head checks before opening the PR

The PR records the reviewed commit and its completed read-only `.github/workflows/ci.yml` run. The temporary normalization workflow and patch programs are absent from the final tree. Final CI has only contents-read permissions, refuses dependency/source drift and verifies the working tree remains unchanged.

Required commands include `make build`, `make vet`, `make coverage`, each phase01–04 acceptance runner, `make foundation-smoke`, `make planning-check`, `make preflight-full`, strict golangci-lint, actual linux/amd64 and darwin/arm64 CGo-free cross-builds, and two bounded mutation campaigns:

```sh
go test -race ./test/acceptance -run '^$' -fuzz '^FuzzAuthorityJSON$' -fuzztime=5s -parallel=2
go test -race ./test/acceptance -run '^$' -fuzz '^FuzzActualVerifier$' -fuzztime=5s -parallel=2
```

Campaign execution counts are in the workflow log. Seed-only runs are not described as mutation coverage. Short bounded fuzzing is useful regression evidence, not exhaustive security proof. macOS cross-compilation does not claim macOS runtime execution.

## Scope of the claim

Phases 03/04 have an actual reusable verifier/enforcer, first protected operational consumers and SDK. Full MCP transport, analytical source/reporting services, and fresh delegated authority for durable work remain assigned to later phases. The operator handoff matches an inspected Pengui serializer using synthetic signing fixtures; it does not assert a platform deployment or live user-session registration. No new IAM store, issuer, local model, token renewal or browser-auth mechanism is included.
