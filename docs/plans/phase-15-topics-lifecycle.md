# Phase 15 — topics-lifecycle

Status: in_progress. Owner: internal/semantics. Hard dependencies: 04, 05, 07, 12, 21.

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

Implement `TestPhase15/AC01` through `TestPhase15/AC06` with real PostgreSQL/pgvector publication races, source changes, draft mutations and neutral portability goldens. Use recorded gateway fixtures and a separately reported live semantic-quality run. COMMON.md sets coverage; `scripts/smoke/phase-15.sh` requires all six results.

## Glossary, decisions and deviations

D-045/D-049/D-052 preserve lifecycle outcomes but remove local policy decisions. No runtime completion is claimed.

## Bounded draft service stage

`internal/semantics/drafts`, PostgreSQL migration 012, `internal/topicapi` and the Go SDK now provide private draft creation, exact CAS revisions, scoped reads/history/diff and neutral mapped export/import. Admission consumes real source discovery and active private profile evidence; storage fences them at commit. The [draft service contract](../contracts/topic-drafts-v1.md) defines authority, limits and typed failure behavior. `TestTopicDraftAPIAndSDK`, `TestTopicDraftCASAndScopeFences`, `TestTopicDraftCommitFencesAndErasure`, `TestTopicDraftProfileHeadAndAuditFences` `TestTopicDraftAdmissionLimitsAndIndependentActions` and `TestTopicDraftMultipleDatasetScopeAndAdmissionBounds` exercise this partial AC02/AC03/AC06 consumer against real PostgreSQL and HTTP/SDK schemas. They do not satisfy the six cumulative `TestPhase15` criteria.

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

Entity/onboarding APIs, source-reference rewrite workflows and full lifecycle
portability remain unimplemented. No phase acceptance stub or completed-phase claim
is added.
