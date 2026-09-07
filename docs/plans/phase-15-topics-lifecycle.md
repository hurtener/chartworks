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
