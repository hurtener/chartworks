# Phase 34 — migration-parity-cutover

Status: in_progress. Owner: internal/migration, internal/migrationapi, test/acceptance. Hard dependencies: 14, 16, 18, 19, 23, 24, 26, 27, 28, 29, 30, 31, 32, 33.

## Authority and design

RFC-001 §16/17, RFC-002 §9, `docs/reporting/implementation-plan.md`, D-050 and [COMMON.md](COMMON.md) apply. This phase closes source-feature parity and cutover, then phase 25 closes release. The 63-row feature map is mandatory; a source code inventory or planning script cannot mark it implemented.

## Brief findings incorporated

Briefs 01–06 and 14: all retained source outcomes, privacy, schema/temporal/visual contracts, scheduling and explicit unsupported-source debt.

## Findings I'm departing from

No private source code/names/schemas/tokens in this repository, no automatic import of old authority or certifications, and no cutover with two uncontrolled schedule streams. Q11 source stubs are deliberately discarded with explicit rejection behavior, not claimed functional.

## Scope and implementation tasks

1. Implement neutral dry-run/import/export tooling and a private source-to-canonical mapping; import definitions, metadata/revisions, outputs, reports/dashboard pages, schedules and retained history.
2. Validate every B/R/Q/N feature disposition with real-driver semantic/authorization/temporal/visual comparisons; preserve evidence levels and quarantine unsupported records.
3. Rehearse cohort cutover and rollback: stop duplicate schedule dispatch, preserve last occurrence/references/retention and distinguish irreversible external effects.

Use stable external-reference mappings and idempotent batches. Dry-run reports every transformed, unknown or rejected field; default uncertain lifecycle to private draft. Parameter precedence, time policies, exact pins, older section layout and visibility must normalize with equivalent outcomes. Private source comparison data stays in owner-controlled environments; commit only newly authored synthetic goldens. A new Pengui token supplies authority after import; old permissions/tokens do not become local policy.

## Non-goals

No mass destructive migration without a dry run, silent feature omission, source-brand compatibility API or claimed ability to undo every external effect.

## Config and persistence

Migration batch limits, dry_run=true default, cohort/source mappings, explicit lifecycle mapping and cutover occurrence boundary; no imported auth tokens/users/grants. Add only required neutral import manifest/reference/checkpoint state. Historical artifacts retain origin and privacy but do not automatically receive new current attestation.

## Acceptance criteria

1. **AC01** — Dry-run lists every transformed/dropped/unsupported field and stable external reference; duplicate imports/revisions are idempotent and secrets/source names do not enter repository fixtures.
2. **AC02** — Private/public/review states, output mappings, filter precedence, exact pins, parameter windows and localized metadata survive normalization.
3. **AC03** — Historical certificates/artifacts preserve origin and cannot automatically become current Chartworks approval or broader Pengui authority.
4. **AC04** — Shadow comparisons cover all retained NLQ/semantic/report/schedule/driver behaviors with semantic-result equivalence, not SQL string equality alone.
5. **AC05** — Artifact retention/expiry/erasure and revoked/expired scoped reads remain correct across imported data and generated renditions, within the JWT validity model.
6. **AC06** — Cutover hands off one logical schedule occurrence stream; rollback restores accepted pointers/routing without claiming to unsend messages or undo arbitrary DDL.
7. **AC07** — Every required source feature has a passed target acceptance/evidence record; Q11 is intentionally discarded and unsupported engines/cohorts cannot be declared migrated.
8. **AC08** — Operator runbook, observed limitations and rollback drill results are complete; final production readiness passes to phase 25, not claimed by documentation tests.

## Tests, coverage and smoke

Implement `TestPhase34/AC01` through `TestPhase34/AC08`; test import replay/CAS, malformed/unknown records, exact values/windows/layout, certificate privacy and cohort schedule handoff on real services. Owner-run source comparisons are explicit evidence artifacts, never invented from test-file presence. COMMON.md sets coverage; `scripts/smoke/phase-34.sh` requires all eight results.

## Current implementation boundary

The in-progress runtime is owned by `internal/migration`, `internal/migrationapi`,
`internal/store/postgres/migration.go`, migration 050 and the typed SDK. It exposes
seven registered HTTP/CLI operations and seven optional MCP bindings. The
[v1 contract](../contracts/migration-cutover-v1.md),
[operation manifest](../contracts/chartworks-migration-operations.json) and
[operator runbook](../runbooks/migration-cutover.md) define the manifest, authority,
loss ledger, quarantine, retention and schedule handoff behavior.

`TestPhase34/AC01`–`AC08` exercise the immutable bundle, full dependent graph,
per-apply current-authority/retention/owner revalidation, cross-revision reservation
and checkpoint fencing, current source validation at cutover, all 63 feature dispositions,
live owner evidence resolution, PostgreSQL replay/CAS, bounded erasure and the
worker-consumed occurrence cutover/rollback fence. Phases 24 and 33 are integrated. Evaluation
suites and server-owned runtime packs import through the Phase 24 public service as
drafts and reconcile exact retry conflicts without importing acceptance or selection.
Runtime packs and suites remain drafts; calibration objects become durable private
optimization candidates and cannot transfer review or selection. A top-level
calibration member is rejected as non-operative. Imported schedules are disabled in
their creation transaction until an exact two-route cutover. Owner comparison
results are resolved from distinct held-out feature cases in accepted live Phase 24
reports, with source revision and engine binding, rather than manifest status text.
Phase 34 remains `in_progress` until the complete private evidence set and Phase 25
release gates pass.

## Glossary, decisions and deviations

A cohort is migrated only when its required behavior and engine evidence are complete. D-050 applies. No migration, rollout or runtime completion is claimed by this plan.

## CW-04 import substrate

Neutral topic portability now includes rich aliases, semantic roles, units,
governed values, explicit geography designation, temporal policy, filters and relationship decisions, and remaps
every reference-bearing rich field to destination coordinates before compilation.
This supplies the semantic field-level substrate for AC01/AC02. It does not implement
phase 34's external manifest, cohort dry run, history/state normalization, schedule
handoff, owner-run shadow comparison or rollback drill.

## CW-07 retained route evidence

Portable/cutover evaluation must preserve or explicitly transform the selected
topic policy version, locale/parser/anchor, exact topic/source pins, interpretation
digest and reviewed correction edits. A foreign raw span or topic score cannot be
trusted directly: import replays it against current authority and publication/source
state. The external bundle and cohort migration remain phase-34 work.

## CW-08 learning portability substrate

Versioned learning rows have protected neutral export and destination-side import
that reroutes current authority, rechecks exact semantic/source/context/rule/template
origin, validates SQL natively and creates only a review candidate. Replays are
idempotent. Phase 34 still owns external manifests, coordinate remapping, cohort
dry-runs, unsupported-record ledgers and cutover/rollback evidence.

## CW-10 selectable filter binding portability

The optional filter-options binding is ordinary immutable document JSON and
therefore survives the existing versioned import/export path. Phase 34 must map
its exact block/topic/dataset/column coordinates, quarantine unresolved bindings
and prove source-revision/current-authority revalidation; it must not replace the
binding with profile samples or a label-derived physical column.
