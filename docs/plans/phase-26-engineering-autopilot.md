# Phase 26 — engineering-autopilot

Status: in_progress. Owner: internal/engineering. Hard dependencies: 13, 15, 16, 21, 27.

## Authority and design

RFC-001 §7/19, D-051/D-052 and [COMMON.md](COMMON.md) apply. L2 is a reviewed proposal workflow over existing primitives, not a second agent engine, IAM service or distributed transaction manager.

## Brief findings incorporated

Briefs 09, 11, 12, 14: demand-driven planning, retrieve/verify/confirm matching, canonical identifiers, evidence and managed outputs.

## Findings I'm departing from

No universal atomic apply/revert promise and no local autonomy access-policy CRUD. L3 and a new internal analyst are explicit post-cutover extensions, not gates for reporting parity.

## Scope and implementation tasks

1. Implement L2 goal -> scope-bounded blind planning -> retrieve/verify/confirm -> proposed pipeline/dataset/topic/schedule changes with decision records.
2. Support review/edit/approve/reject and staged apply/compensation through ordinary publication and managed-write gates.
3. Turn schema/freshness/quality failures into deduplicated amendment proposals; keep L3/new internal analyst orchestration as explicit later work.

## Non-goals

No autonomous business meaning publication, baseline writes, generic workflow system, L3 policy store or invented owner authority.

## Config and persistence

Autonomy L2 proposal step/call/token/time limits and amendment dedup interval; no local IAM/autonomy access-policy store. Persist proposals and evidence/decision records, exact intended effects, review actor and staged actual effects. Use the existing queue/runner and separate propose/apply signed scopes.

## Acceptance criteria

1. **AC01** — Every proposed object has evidence, alternatives, match/build rationale, author/model version and bounded usage.
2. **AC02** — Planner sees only authorized scope; matching/building cannot widen source reach or access unrelated tenant catalog data.
3. **AC03** — Separate propose/apply scopes and human semantic publication are enforced; the submitter does not self-approve by changing a body field.
4. **AC04** — Staged apply records actual completed effects, validates managed ownership and blocks conflicting/dependent revert instead of claiming global atomicity.
5. **AC05** — Drift creates one reviewable amendment and report/topic impact evidence; no published query/meaning changes automatically.
6. **AC06** — L2 goal-to-managed-data path is functional; no exposed L3 policy CRUD or unsupported analyst/stub behavior is included in parity claims.

## Tests, coverage and smoke

Implement `TestPhase26/AC01` through `TestPhase26/AC06` with the real runner/store and bounded model fixtures. Kill at effect boundaries, exercise dependency-blocked compensation and competing reviewers, and verify immutable reporting definitions. COMMON.md sets coverage; `scripts/smoke/phase-26.sh` requires all six results.

## Glossary, decisions and deviations

Proposal approval and actual effect completion are separate. D-051/D-052 apply. No runtime completion is claimed.

## Runtime completion evidence

See the [runtime contract](../contracts/reviewed-engineering-and-frozen-runs.md) and
[review and verification ledger](../reviews/phase-26-28-runtime.md). Exact-source
acceptance and coverage are required before closure.
