# Phase 13 — engineering-pipelines

Status: in_progress. Owner: internal/engineering. Hard dependencies: 06, 09, 10, 12.

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

The current candidate uses the D-067 minimal fork derived from Bruin `v0.11.749`; its current source baseline is `83f04505f5a257e7dbd89b98b8276cb6dcb1ec8b`; the final qualified source commit, build version and executable SHA-256 remain acceptance evidence rather than an unmodified-stock qualification. Defaults are disabled, 45-second timeout, concurrency 1, eight steps, 64 KiB SQL per step and 1 MiB runner output. Enabling requires absolute runner/private temporary paths. Definitions support only SQL steps, declared predecessor placeholders, bounded schemas/checks and the six named strategy contracts. Bruin's validator invokes its embedded Python parser/runtime for SQL-only assets; deployment therefore supplies 512 MiB of executable private tmpfs per configured worker while the public asset contract still rejects user Python/R assets. The measured cold fixture wrote 228,013,251 runtime bytes in 1.297 seconds. Exit zero is insufficient: the complete payload must deny malformed/multiple JSON, any critical issue and overflow. The phase's first `pipeline_draft` gateway consumer receives governed schema and a bounded instruction without source rows or secrets; valid output enters the same draft-only validation path and has no publication or execution side effect. See the [candidate runtime contract](../contracts/managed-pipelines.md) and [current evidence ledger](../reviews/phase-13-current-evidence.md).

## Acceptance criteria

1. **AC01** — Undeclared inputs/outputs or invalid/cyclic pipelines fail before execution; all supported strategies have declared per-engine capability tests.
2. **AC02** — NLQ adapters cannot reach managed-write operations; model drafting creates only a proposal/draft.
3. **AC03** — Both definition and rendered execution reject baseline objects and falsely prefixed unmanaged targets; credentials independently restrict writes.
4. **AC04** — Quality failure leaves an explicit failed/staged outcome and no silent active partial dataset; lineage records actual completed effects.
5. **AC05** — Subprocess timeout/cancel/crash, telemetry-disable and secret-safe invocation pass; unsupported runner assets never execute.
6. **AC06** — Incremental/merge/interval/scd2 and replace/append semantics are tested where claimed; retry/revert never assumes a distributed transaction.

## Tests, coverage and smoke

Implement `TestPhase13/AC01` through `TestPhase13/AC06` against the real pinned runner and managed PostgreSQL fixture. Observe rendered config, process environment/output handling, blocked baseline targets and failed-check activation. Driver support claims need their strategy tests. COMMON.md supplies coverage; `scripts/smoke/phase-13.sh` requires all six results.

The owner approved an exact 84.5% statement coverage exception for
`internal/store/postgres` on 2026-09-07. All other package bands retain their
existing thresholds. The [approval and measurement scope](../reviews/phase-13-current-evidence.md#owner-approved-package-coverage-band)
record the partial-suite evidence; the exception does not waive complete native
CI or turn that measurement into a passing full-suite result.

Implementation has been submitted for acceptance. The phase remains in progress until all named criteria, real runner/workspace boundaries, race/coverage and cumulative exact-head gates pass without skips.

## Glossary, decisions and deviations

Managed object, staged effect and compensation are not universal rollback. D-051/D-052 apply. Record the actually adopted runner version and license/build evidence in the implementation PR; no runtime completion is claimed here.

The round-one pipeline fixes retain a same-physical-database execution boundary:
`TestPipelineRejectsDifferentInputDatabase` uses two real databases with the same
relation name and distinct rows, and requires rejection without execution effects.
No cross-database transfer is implemented. `TestPipelinePublishesTwoOutputsWithOneReadConnection`
uses the real runner with a single read pool connection and requires the complete
two-step output manifest and actual retained values. Publication holds all exact
output locks in one native transaction through the existing atomic metadata commit.
