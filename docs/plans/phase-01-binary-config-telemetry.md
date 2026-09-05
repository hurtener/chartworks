# Phase 01 — binary-config-telemetry

Status: in_progress. Owner: cmd/chartworks, internal/config, internal/telemetry. Hard dependencies: none.

## Authority and design

RFC-001, RFC-002, the Pengui authority contract and D-044–D-052 apply. [COMMON.md](COMMON.md) supplies the binding implementation, testing, coverage and smoke workflow. Phase numbers are identifiers, not execution order; register concrete operations with the early transport/SDK shells in the same feature change.

## Brief findings incorporated

Briefs 01, 04, 14; retain the product behavior below and its coverage-map evidence. Historical detailed notes in `docs/archive/phase0-plans/` are background, not competing instructions.

## Findings I'm departing from

Pengui issues all authority; no local IAM, token issuance or host-compatibility qualification. Discard successful stubs and do not inherit automatic source-feature deferrals.

## Scope and implementation tasks

1. Build the serve/mcp/version/config-check command dispatch, cancellable lifecycle and explicit configuration loader; expose only real health/capability endpoints.
2. Reject retired auth-mode/signing/bootstrap settings; wire verification configuration and feature-specific readiness. A disabled optional model/renderer must not break healthy artifact reads.
3. Provide content-free audit/metrics interfaces, exported-counter conformance and limits; document each configuration field with units and defaults.

## Non-goals

No local IAM, token issuance, or unrelated subsystem implementation. L3 and a new internal analyst are not prerequisites.

## Config and persistence

Server body/time limits; telemetry log format/metrics/optional OTel; feature enablement; auth verification settings from the authority contract. Defaults and examples must come from the typed configuration schema. Put exact implemented keys/types/defaults in the typed reference/example. Domain schema changes ship with their first consumer; no IAM relations. All domain methods carry verified tenant/authority context.

## Acceptance criteria

1. **AC01** — Unknown or retired configuration is rejected with a safe field-specific error; no implicit development authority is created.
2. **AC02** — Boot/readiness distinguish invalid configuration, stale verification keys, unavailable store and optional dependency health; shutdown joins workers.
3. **AC03** — Every registered metric exports; seeded secrets and data never appear in ordinary logs/errors; labels remain bounded.
4. **AC04** — HTTP health/capabilities report only implemented/enabled functionality; unavailable features are not successful stubs.
5. **AC05** — Command dispatch and configuration source precedence have deterministic fixtures, correct exit codes and injected I/O.
6. **AC06** — Race-safe concurrent telemetry/configuration use and startup/idle benchmarks record the environment without asserting invented targets.

## Tests, coverage and smoke

Implement `TestPhase01/AC01` through `TestPhase01/AC06` against the real owning services; follow COMMON.md for real-store/source fixtures, unit/fuzz/race tests and 85/80/70% coverage bands. `scripts/smoke/phase-01.sh` requires every named acceptance result. Missing/skipped runtime tests are not passes. Feature/gate ownership is in `coverage.json`.

## Glossary, decisions and deviations

Update the shared glossary for new terms. D-044–D-052 govern this revision. No runtime completion/deviation is claimed; record implementation findings and equivalent behavior here before closure.
