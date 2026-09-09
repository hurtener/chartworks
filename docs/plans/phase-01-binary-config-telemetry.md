# Phase 01 — binary-config-telemetry

Status: shipped. Owner: cmd/chartworks, internal/config, internal/telemetry. Hard dependencies: none.

## Authority and design

RFC-001, RFC-002, the Pengui authority contract and D-044–D-052 apply. [COMMON.md](COMMON.md) supplies the binding implementation/testing workflow. D-056/D-058 record the concrete foundation boundary and configuration format. Shipped here means implemented and verified in this PR, not deployed or all-product release.

## Brief findings incorporated

Briefs 01, 04, 14: bounded configuration, content-free exported observability, explicit unavailable capabilities and no development-authority fallback. Prior notes in `docs/archive/phase0-plans/` remain historical.

## Findings I'm departing from

Pengui alone issues authority. Actual JWT acceptance is phase 03, not a local implementation in this phase. The foundation is loopback health-only; the usable public-key health probe is not a token verifier. The original MCP-unavailable dispatch was superseded by the real [phase-22 transport](phase-22-mcp-server.md): it remains opt-in, rejects disabled startup with exit 2, and otherwise invokes the common protected service lifecycle. Optional OTel configuration fails explicitly until its adapter is implemented. Typed JSON is the implemented configuration format; no competing YAML or implicit environment alias regime is introduced.

## Scope and implementation tasks

1. Delivered serve/mcp/version/config-check dispatch with injected environment/I/O/lifecycle, deterministic exits, bounded JSON configuration, deep-copy snapshots and redacted error/value handling.
2. Delivered real boot/store migration dependency, trusted public-key health with bounded freshness, `/healthz`, `/readyz`, `/capabilities`, explicit transport limits and joined graceful shutdown.
3. Delivered slog plus a real Prometheus registry/exporter with closed labels and export conformance. No metrics/business endpoint is mounted without future Pengui enforcement.

## Non-goals

No local IAM, token issuance, inference, reporting, rendering, authentication bypass or host compatibility qualification. Bifrost-only remote inference remains phase 05; inactive provider configuration is structurally checked but never called here.

## Config and persistence

[Configuration reference](../configuration.md) lists every implemented typed key, units, bounds, defaults and secret classification. `examples/chartworks.foundation.json` is the executable configuration shape; `config-check --defaults` generates typed defaults. [GETTING-STARTED.md](../../GETTING-STARTED.md) covers setup, dependency health, safety boundaries and commands. Phase 02 supplies actual durable metadata; this phase invents no IAM relations.

## Acceptance criteria

1. **AC01** — Unknown or retired configuration is rejected with a safe field-specific error; no implicit development authority is created.
2. **AC02** — Boot/readiness distinguish invalid configuration, stale verification keys, unavailable store and optional dependency health; shutdown joins workers.
3. **AC03** — Every registered metric exports; seeded secrets and data never appear in ordinary logs/errors; labels remain bounded.
4. **AC04** — HTTP health/capabilities report only implemented/enabled functionality; unavailable features are not successful stubs.
5. **AC05** — Command dispatch and configuration source precedence have deterministic fixtures, correct exit codes and injected I/O.
6. **AC06** — Race-safe concurrent telemetry/configuration use and startup/idle benchmarks record the environment without asserting invented targets.

## Tests, coverage and smoke

`test/acceptance/phase01_test.go` implements `TestPhase01/AC01` through `AC06`; shared adversarial tests also start the actual service against disposable PostgreSQL and check connection/bind failures, key staleness and shutdown. Unit/fuzz tests cover malformed configuration/key responses, redaction and immutable snapshots. The complete suite runs under `-race`; coverage includes real cross-package callers at unchanged thresholds.

`scripts/smoke/phase-01.sh` requires all six actual named results; `make foundation-smoke` additionally launches the compiled executable, checks health/capability negative routes and sends SIGTERM. No acceptance child may skip. See [the adversarial review](../reviews/phase-01-02-adversarial.md) and [verification record](../reviews/phase-01-02-verification.md).

## Glossary, decisions and deviations

D-056 defines health-only exposure until phase 03/04. D-058 records JSON and cross-package coverage. The key probe hands off through a dependency-check seam; it is not a competing auth implementation. Later-phase source/provider failures are not manufactured to demonstrate currently nonexistent artifact operations: their explicit disabled state is exercised here, and real artifact resilience remains assigned to reporting phases.

All six acceptance criteria, actual PostgreSQL startup/shutdown, coverage, vet and lint passed on the reviewed Go source before this status changed. The final read-only CI rechecks the exact PR tree. Other phases remain planned and all-product release is not claimed.

## Cumulative MCP regression contract (2026-09-09)

Phase 22 makes `features.mcp=true` valid; AC01 now tests that positive case and
keeps negative coverage for missing issuer configuration, unimplemented reporting,
renderer/OTel, invalid MCP limits/groups/hosts and a transport deadline that would
outlive the HTTP writer. Existing authentication/secret/listener negatives remain.
AC05 checks disabled MCP returns 2 with zero lifecycle calls, enabled MCP preserves
its caller context/configuration/overrides/I/O, and missing or failing lifecycles
return sanitized failures. `make foundation-smoke` additionally exercises both the
compiled `serve` (MCP disabled) and `mcp` (real charts group) commands: exact
OpenAPI/401/404 distinctions, capability advertisement, readiness and joined
SIGTERM shutdown. These assertions replace obsolete unimplemented-feature
expectations, not security checks. Exact-source CI remains the merge gate.
