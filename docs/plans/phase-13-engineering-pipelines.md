# Phase 13 — engineering-pipelines

Status: planned. Owner: internal/engineering. Hard dependencies: 06, 09, 10, 12.

## Authority and design

RFC-001 §7, D-048/D-051/D-052 and [COMMON.md](COMMON.md) apply. Retain the accepted Bruin runner seam rather than inventing a generic workflow platform. Pengui-signed scope and actual managed-object ownership both constrain the write path.

## Brief findings incorporated

Briefs 02, 10, 11, 14: declared input/output graph, quality before activation, managed writes, lineage, resumable execution and bounded assistance. The initial mine-only dependency verdict is historical; later runner adoption remains the direction.

## Findings I'm departing from

No baseline write permission inferred from a name prefix, silent partial publication or cross-engine atomic rollback promise. No generated arbitrary shell or Python assets.

## Scope and implementation tasks

1. Implement versioned SQL-only definitions/strategy/check contracts, dependency ordering and explicit human publication before managed writes.
2. Wire the pinned Bruin subprocess runner: argv-only invocation, disabled telemetry, bounded environment/tmpfs secret handling, validate/lineage output and per-asset results.
3. Stage materializations and publish dataset lineage/freshness only after required quality checks; record partial effects and compensations.

## Non-goals

No new pipeline engine, unrestricted scripts, automatic semantic publication or write flag on the NLQ read interface.

## Config and persistence

Pipeline runner path/version, strategy allowlist, timeout/concurrency, managed-object registry, telemetry disabled and quality policy; no arbitrary shell/Python assets. Persist definition versions, declared dependencies, staged effects, completion/compensation records and dataset publication pointer. Secret material is never a durable pipeline payload.

## Acceptance criteria

1. **AC01** — Undeclared inputs/outputs or invalid/cyclic pipelines fail before execution; all supported strategies have declared per-engine capability tests.
2. **AC02** — NLQ adapters cannot reach managed-write operations; model drafting creates only a proposal/draft.
3. **AC03** — Both definition and rendered execution reject baseline objects and falsely prefixed unmanaged targets; credentials independently restrict writes.
4. **AC04** — Quality failure leaves an explicit failed/staged outcome and no silent active partial dataset; lineage records actual completed effects.
5. **AC05** — Subprocess timeout/cancel/crash, telemetry-disable and secret-safe invocation pass; unsupported runner assets never execute.
6. **AC06** — Incremental/merge/interval/scd2 and replace/append semantics are tested where claimed; retry/revert never assumes a distributed transaction.

## Tests, coverage and smoke

Implement `TestPhase13/AC01` through `TestPhase13/AC06` against the real pinned runner and managed PostgreSQL fixture. Observe rendered config, process environment/output handling, blocked baseline targets and failed-check activation. Driver support claims need their strategy tests. COMMON.md supplies coverage; `scripts/smoke/phase-13.sh` requires all six results.

## Glossary, decisions and deviations

Managed object, staged effect and compensation are not universal rollback. D-051/D-052 apply. Record the actually adopted runner version and license/build evidence in the implementation PR; no runtime completion is claimed here.
