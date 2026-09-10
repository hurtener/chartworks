# Phase 27 — reporting-blocks

Status: in_progress. Owner: internal/reporting. Hard dependencies: 15, 20, 21.

## Authority and design

RFC-002 §§2–5, the Pengui authority contract, D-045/D-047 and [COMMON.md](COMMON.md) apply. This is the reusable analytical-definition lifecycle, not just a saved chart. Phase 28 owns recurring execution and retained results.

## Brief findings incorporated

Briefs 05, 06, 14; source coverage B01–B06/B08–B09/B11–B15/B18–B19 and related rows in coverage.json. Preserve independent publication/certification/health, typed parameters, question discovery and dependency evidence.

## Findings I'm departing from

Do not flatten trust into one boolean, treat successful NLQ as certified, expose raw SQL to every reader or rewrite an approved definition on source drift. No local block grant store.

## Scope and implementation tasks

1. Add block identities/revisions, localized questions/aliases, exact semantic/template/dependency references and an authoring API/SDK.
2. Implement CAS drafts, capture-from-query/manual authoring, duplicate-question assessment, validation/preview/publication/rejection/restore/archive and separate attestations.
3. Implement typed parameter declarations and assisted period parameterization, selected output definitions, dependency-impact classification and SQL-read protection.

The service constructs dependency manifests during validation. Content supplied by a client cannot omit a dependency and thereby authorize it. Validation evidence includes canonicalization/validator version, exact execution/revision/dependency hashes, observed result schema, actor/time and query attempt. Publication checks the same draft version in its transaction. Certification references a published immutable revision and a real evidence record.

## Non-goals

No visual authoring application, automatic certification, local IAM, semantic self-publication or query rewrite during refresh.

## Config and persistence

Reporting authoring SQL/schema/options/alias bounds, validation timeout and question-assessment threshold; no separate block auth system. Add block/revision, validation-evidence and attestation domain state with tenant-composite references and immutable published payloads. Reuse common audit/CAS and the source-safe validator. The authoring output union contains chart/KPI/table/narrative specifications with stable IDs, not arbitrary frontend HTML.

## Acceptance criteria

1. **AC01** — Identity/revision/output IDs, localized metadata and aliases are stable and tenant-scoped; unsupported fields fail closed.
2. **AC02** — CAS edits/publication have one winner; published content is immutable and rejection/restore/amendment preserves history.
3. **AC03** — Validation executes the exact draft under current reach and binds content/dependency/result-schema evidence; material edits invalidate it.
4. **AC04** — Publication and certification are distinct scoped actions; stale/withdrawn approval and current health remain separate from historical attestation.
5. **AC05** — All parameter types, defaults/ranges/dimension references and period proposals validate; assisted changes cannot alter unrelated SQL/filters.
6. **AC06** — Chart/KPI/table/narrative output definitions, output-subset selection and separate SQL-read projection are exposed without leaking SQL by default.
7. **AC07** — Cosmetic/rename/review-required/unavailable impacts are computed from exact dependencies; rename creates a new draft and never rewrites publication.
8. **AC08** — Authoring/preview/archive/reference errors enforce parent/resource reach and private scope; API and SDK provide the complete lifecycle.

## Tests, coverage and smoke

Implement `TestPhase27/AC01` through `TestPhase27/AC08`. Use real source validation and PostgreSQL CAS races, edit-after-validation, revoked attestation, unauthorized SQL view, exact rename and missing-dependency fixtures. Parameter goldens cover every preserved type and period policy. COMMON.md requires security/store conformance bands and functional API/SDK checks. `scripts/smoke/phase-27.sh` requires all eight results.

## Glossary, decisions and deviations

Block, revision, validation evidence, attestation and current health are distinct shared terms. D-047 establishes required reporting scope; D-045 leaves access decisions in Pengui. The implementation and all eight named acceptance criteria are present. Review/merge status remains separate from runtime evidence; see [the adversarial record](../reviews/phase-27-adversarial.md) and [the HTTP/SDK contract](../contracts/reporting-blocks-v1.md).
