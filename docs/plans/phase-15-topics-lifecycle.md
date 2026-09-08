# Phase 15 — topics-lifecycle

Status: in_progress. Owner: internal/semantics. Hard dependencies: 04, 05, 07, 12, 21. Current cumulative evidence: [phases 15–18 and 21](../reviews/phase-15-18-current-evidence.md).

## Authority and design

RFC-001 §8, D-045/D-049/D-052, the bounded [semantic foundation contract](../contracts/semantic-foundation-v1.md), and [COMMON.md](COMMON.md) apply. Published semantic meaning and matching facets become active together; a failed background step leaves the prior publication usable. All permissions are Pengui-signed.

## Brief findings incorporated

Briefs 03, 05, 07, 14: versioned packs, compact contracts, full entity/reference mutation, health/recheck, canonical registry and portability.

## Findings I'm departing from

No local sharing/role machinery, asynchronous active-pointer/facet inconsistency, or silent default semantics for missing model output.

## Scope and implementation tasks

1. Implement pack/entity schemas, stable IDs, canonical registry, generation and per-entity editing/moves with exact reference validation.
2. Implement immutable published versions, draft/review/publish/rollback/archive transitions and version-fenced facet activation.
3. Carry source-health/table-rename rewriting, recheck, history/diff, onboarding profiles and neutral portability; permissions come only from signed scopes.

## Non-goals

No automatic semantic publication, local topic grant management or replacement of stable identifiers with display names.

## Config and persistence

Semantics batch/token/concurrency limits, compact-card caps, supported locale list and publication activation timeout. Topic/version/audit/entity/profile state is domain data. Keep immutable versions, staged ready facet generation and a CAS active pointer; reference changes happen in a new draft. Register all lifecycle and entity operations through phase 21.

## Acceptance criteria

1. **AC01** — Concurrent publication has one active version with matching ready facets; failed generation leaves the prior publication usable.
2. **AC02** — Published versions cannot mutate/discard; draft edits, review, rollback and archival preserve typed actor/change evidence.
3. **AC03** — Measures/dimensions/KPIs/joins and source references are validated and rewritten consistently on a draft, never hot-patched in publication.
4. **AC04** — Unavailable/archived sources/topics are excluded from query contracts; recheck clears only verified resolved health issues.
5. **AC05** — Generation is bounded/resumable, retains unresolved semantics, and produces compact contracts with stable entity IDs.
6. **AC06** — Export/import and entity/onboarding-profile APIs preserve semantic meaning without source identifiers, credentials or local sharing/role machinery.

## Tests, coverage and smoke

`TestPhase15/AC01` through `TestPhase15/AC06` exercise real PostgreSQL/pgvector publication races, source changes, draft mutations and neutral portability goldens. Recorded gateway fixtures are the reproducible acceptance path; live semantic quality has not been measured and is not inferred from those fixtures. COMMON.md sets coverage; `scripts/smoke/phase-15.sh` requires all six results.

## Glossary, decisions and deviations

D-045/D-049/D-052 preserve lifecycle outcomes but remove local policy decisions. The current runtime candidate and its remaining release gates are recorded below.

## Bounded draft service stage

`internal/semantics/drafts`, PostgreSQL migration 012, `internal/topicapi` and the Go SDK provide private draft creation, exact CAS revisions, scoped reads/history/diff and neutral mapped export/import. Admission consumes real source discovery and active private profile evidence; storage fences them at commit. The [draft service contract](../contracts/topic-drafts-v1.md) defines authority, limits and typed failure behavior. `TestTopicDraftAPIAndSDK`, `TestTopicDraftCASAndScopeFences`, `TestTopicDraftCommitFencesAndErasure`, `TestTopicDraftProfileHeadAndAuditFences`, `TestTopicDraftAdmissionLimitsAndIndependentActions` and `TestTopicDraftMultipleDatasetScopeAndAdmissionBounds` exercise the real PostgreSQL and HTTP/SDK consumer. Their earlier bounded evidence is now supplemented by the cumulative `TestPhase15` acceptance described below.

## Bounded publication lifecycle stage

The [publication contract](../contracts/topic-publication-v1.md) adds immutable
review receipts, explicit publication, retained current/exact reads, rollback,
archive and a separate current source contract read. Publication obtains the full
embedding-space descriptor from the active Bifrost route, stages complete
per-context facet generations invisibly, then commits the semantic head and every
old/new context vector head in one PostgreSQL transaction. Rollback restores the
retained exact generation set without a gateway call. Contract and rollback compose
their registered topic action with the existing `sources.read` action required by
live discovery; archive performs no discovery. Managed facet search resolves
the verified Pengui envelope and every persisted source/dataset/context dependency
inside the same repeatable-read snapshot before selecting facet bodies.

The registered HTTP and Go SDK operations are exercised by focused real PostgreSQL,
pgvector and recorded gateway fixtures, including concurrent publication, gateway
failure, multi-context retirement/restoration, archive and current-health boundaries.
This is bounded AC01/AC02/AC04/AC05 evidence; it is not a substitute for the six
cumulative `TestPhase15` criteria. The exact-head review and root verification are
recorded in the [bounded publication evidence](../reviews/phase-15-topic-publication.md).

## Bounded canonical registry stage

D-068 and the publication contract define global canonical meaning separately from
topic-local physical keys. Existing draft/import operations accept collision-checked
proposals, and existing reviewed publication atomically approves a new ID at revision
one or the exact next revision. Exact immutable revision reuse needs no registry write.
New normalized terms remain reserved to their first entity ID, and canonical facets
are split by their actual source/context. This is partial AC01/AC02/AC06 evidence and
adds no route or action.

No phase acceptance stub or completed-phase claim is added.

## Bounded draft consumer operations stage

The draft service, HTTP registry and Go SDK now expose deterministic onboarding,
atomic entity CRUD and reviewed dataset rebind operations. Onboarding creates an
unresolved one-dataset scaffold from active private profile evidence without model
generation. Entity batches compile as one new revision, so deletes cannot strand
measure, dimension, KPI, join or canonical references. Rebind preserves stable
semantic column IDs, derives every physical/source coordinate from active evidence,
rewrites all dataset-qualified references and uses the existing Save admission and
transaction fences. These operations reuse `topics.write` with the existing
secondary `engineering.read` and `sources.read` actions and require no migration.

Focused pure and real PostgreSQL HTTP/SDK lifecycle tests cover these AC03/AC06
operations, including an enhanced draft whose unresolved semantic reference survives
a reviewed dataset rebind with its stable ID and logical column unchanged.

## Health, generation and cumulative acceptance candidate

Migration 016 adds current topic-health snapshots and immutable generation
checkpoints. Publication and rollback establish a healthy observation inside the
same transaction as their source-revision fence. A recheck observes every public
source/dataset binding without consulting private profiles, then replaces the
snapshot only while the same publication remains current. An unhealthy observation
records drift; clearing issues additionally locks and verifies every current source
revision. Archive removes the current snapshot while retaining immutable topic
versions and events.

The draft enhancement operation sends at most 32 stable dataset/column coordinates
through the existing Bifrost `enhance` role. The closed output classifies each input
exactly once as a measure, dimension or unresolved authoring gap. Entity IDs are
server-derived from stable coordinates. Every successful step is a normal immutable
draft revision with an atomically stored cursor, completion bit and gateway receipt;
the next step must match that persisted cursor, while skips, rewinds, completed
checkpoints and stale heads fail before a model request. Unresolved gaps survive later steps,
publication projection and neutral export/import remapping without becoming
executable entities.

`TestPhase15/AC01` through `AC06` compose the real publication race/failure,
immutable lifecycle, draft mutation/rebind, durable health, recorded Bifrost
generation and neutral portability consumers. The pinned Linux/native-parser,
PostgreSQL/pgvector race run at `c3ccd730ac6537c65f3d439aa9b9564c982f0289`
passed all six children; the root cumulative race run also passed all packages and
acceptance in 173.985 seconds. The author fix `8291e84cf5bcb04c16ecbbae6cbf3d8e561d9d43`
then added the unresolved-rebind regression and passed the full six-child phase run in
14.491 seconds before integration as `39bc68c753ff65e1d382102dcd6bcfdd0ea8387b`.
One independent review was clear at `e90ac24`; the second found that rebind defect,
and its required narrow follow-up review cleared the fix. The later unresolved-rebind
correction `6884f23126dbf01e45c2d799e8f10dfee03c2955` is integrated and root's
semantic checks passed. Exact testing at `39bc68c` also
found that migration 017 had replaced the audit-action constraint without preserving
the migration 016 `topic.health_rechecked` action. Forward migration 019 in
`d103ba95af8a4f951b0a4d2589292ec269905a67` restores the complete closed union; the
author's strict Phase 02 and Phase 15 runs passed all six children with zero skips, and
the repair integrated as `dd6f79e`. Root then verified the committed-source archive
(SHA-256 `6801a28f69d6179e7c3e3bee7c32e7949c194f1fdafba0dac110cf255d5d9aba`):
strict Phase 02 and Phase 15 each passed all six children with zero skips, and the native
race `TestSafeErrors` passed. The final exact integrated-head coverage, full lint/preflight,
hosted CI and release integration remain
gates, so the phase stays `in_progress`. The approved 84.5% coverage exception applies only to
`internal/store/postgres`; no other phase 15 package inherits it.
