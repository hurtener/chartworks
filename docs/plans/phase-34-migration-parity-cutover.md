# Phase 34 — migration-parity-cutover

Status: planned. Owner: internal/reporting, test/integration, scripts/migration. Hard dependencies: 14, 16, 18, 19, 23, 24, 26, 27, 28, 29, 30, 31, 32, 33.

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

## Glossary, decisions and deviations

A cohort is migrated only when its required behavior and engine evidence are complete. D-050 applies. No migration, rollout or runtime completion is claimed by this plan.
