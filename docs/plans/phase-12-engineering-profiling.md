# Phase 12 — engineering-profiling

Status: in_progress. Owner: internal/engineering. Hard dependencies: 05, 06, 08, 10.

## Authority and design

RFC-001 §7/8, D-049/D-052 and [COMMON.md](COMMON.md) apply. Profiles are versioned evidence, not approved semantic meaning. Deterministic profiling remains useful without an optional model summary.

## Brief findings incorporated

Briefs 02, 09, 11, 12, 14: quality dimensions, freshness, value families, selective profiling and dependency health.

## Findings I'm departing from

A bounded returned sample is not proof of a bounded source scan. Do not publish incomplete inferred semantics or rewrite approved reports because a profile changes.

## Scope and implementation tasks

1. Create versioned profiles with sampling provenance, normalized summaries/value families, quality checks, observation timestamps and freshness buckets.
2. Implement schema rediscovery/diff and dependent-topic/report health signals; avoid treating profile generation as semantic publication.
3. Run optional gateway summaries on sanitized bounded context; keep useful deterministic profiling available without an LLM.

## Non-goals

No full automatic data catalog, invisible scan-cost assumptions, autonomous semantic publication or required LLM for deterministic quality checks.

## Config and persistence

Profiling sample rows/bytes/cost/time, sensitive-field policy references, freshness thresholds, optional summary enablement. Persist profile version, sampling strategy, source context/version, observation time and findings. Use idempotent dependency events and shared queue operations.

## Acceptance criteria

1. **AC01** — Profile schema and quality dimensions match synthetic goldens; sensitive values are excluded/redacted before model calls.
2. **AC02** — Configured sample/cost/time limits are enforced by adapter capability; a row sample is not mislabeled as proof of no source scan.
3. **AC03** — Freshness classification, unknown freshness and timezone boundaries are deterministic and traceable.
4. **AC04** — Schema diff identifies affected dependencies and emits idempotent health/invalidation events without auto-editing approved SQL.
5. **AC05** — Cancellation/retry/resume preserves profile version and stage evidence; source failures are typed.
6. **AC06** — Profile outputs feed real semantic generation/inspection consumers with scoped integration fixtures and measured cost/latency.

## Tests, coverage and smoke

Implement `TestPhase12/AC01` through `TestPhase12/AC06`. Real source profiling/limits and source-change fixtures supplement pure summary/freshness tests; semantic consumers close the boundary before their phase closure. COMMON.md supplies coverage; `scripts/smoke/phase-12.sh` requires all six results.

## Glossary, decisions and deviations

Observation time, freshness and sampling provenance are distinct. D-049/D-052 apply. No runtime completion is claimed.
