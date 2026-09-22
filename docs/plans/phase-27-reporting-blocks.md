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
3. Implement typed parameter declarations, fixed-slot lists and dialect-dispositioned assisted period parameterization with preserved authoring intent, selected output definitions, dependency-impact classification and SQL-read protection.
4. Bind certification to visible localized period-language findings and persist bounded reviewed-intent duplicate/overlap/unique evidence after authorization filtering.

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
5. **AC05** — All parameter types, defaults/ranges/fixed lists/dimension references and dialect-bound period proposals validate; assisted changes cannot alter unrelated SQL/filters and unsupported dialects return a non-mutating disposition.
6. **AC06** — Chart/KPI/table/narrative output definitions, output-subset selection and separate SQL-read projection are exposed without leaking SQL by default.
7. **AC07** — Cosmetic/rename/review-required/unavailable impacts are computed from exact dependencies; rename creates a new draft and never rewrites publication.
8. **AC08** — Authoring/preview/archive/reference errors enforce parent/resource reach and private scope; API and SDK provide the complete lifecycle.
9. **CW09-AC01** — English and Spanish contradictory/ambiguous period wording is visible before certification and requires exact immutable reviewer acknowledgement.
10. **CW09-AC02** — Reviewed intent distinguishes paraphrase duplicates, semantic overlap and unique questions over authorized bounded candidates; lexical fallback and incomplete scans are explicit and evidence is durable.

## Tests, coverage and smoke

Implement `TestPhase27/AC01` through `TestPhase27/AC08`. Use real source validation and PostgreSQL CAS races, edit-after-validation, revoked attestation, unauthorized SQL view, exact rename and missing-dependency fixtures. Parameter goldens cover every preserved type and period policy. COMMON.md requires security/store conformance bands and functional API/SDK checks. `scripts/smoke/phase-27.sh` requires all eight results.

## Glossary, decisions and deviations

Block, revision, validation evidence, attestation and current health are distinct shared terms. D-047 establishes required reporting scope; D-045 leaves access decisions in Pengui. The implementation and all eight named acceptance criteria are present. Review/merge status remains separate from runtime evidence; see [the adversarial record](../reviews/phase-27-adversarial.md) and [the HTTP/SDK contract](../contracts/reporting-blocks-v1.md).

## CW-03 output-intent and evidence-policy continuation

AC01/AC02/AC03/AC06/AC08: definition v2 and detached legacy migration; output-level localized intent; restrictive result policy; native SQL-authorized definition export/import; immutable publication and closed API/SDK schemas.

Use [D-073](../decisions/2026-09-16-reporting-output-policies.md) and the
[v2 field-level contract](../contracts/reporting-output-intent-v2.md). The
[scoped adversarial record](../reviews/cw-03-adversarial.md) links real PostgreSQL,
source execution, provider-fixture and browser checks. Keep the existing named
phase criteria and phase status; this assignment closes only its three owned gap
entries, not the whole phase, other reporting work, or full migration/release.

## CW-09 governed authoring depth continuation

BLK-03/04/06 are implemented by certification-time localized period evidence,
reviewed structured question-intent assessment, fixed-slot list binds and a
dialect-bound assistance proposal. Findings, reviews, candidate scope and authoring
dispositions are digest-bound and durable. They never grant source reach, rewrite a
published revision, execute during refresh, or turn question metadata into SQL.
Use [D-080](../decisions/2026-09-22-governed-block-depth.md) and the
[block contract](../contracts/reporting-blocks-v1.md).

## CW-06 rule-snapshot continuation

BLK-02 is implemented by exact immutable rule pins on definition v2. Validation,
publication/certification health, source-impact rechecks and protected native
definition transfer bind the same rule dependency digest. Replacement or
retirement invalidates current health without rewriting the published revision
or historical attestation. Query capture transfers the sealed reviewed template
selection as a bounded topic-ordered set of complete topic/ruleset coordinates
and fences current rule heads in the block commit transaction, so a
concurrent replacement fails stale without partial state. See [D-076](../decisions/2026-09-22-rule-scopes-reporting-snapshots.md),
the [block contract](../contracts/reporting-blocks-v1.md), and the
[CW-06 review](../reviews/cw-06-adversarial.md).

## CW-05 rich output continuation

AC01/AC02/AC06/AC08: immutable block revisions may store closed v3 KPI/table and
column display intent. Existing JSON digests and drift checks include the complete
mapping; migration 043 adds forward bounds without rewriting v1/v2 publications.
Definition transfer remains native and exact. Foreign mapping belongs to Phase 34.
See [D-079](../decisions/2026-09-22-rich-output-display.md).
