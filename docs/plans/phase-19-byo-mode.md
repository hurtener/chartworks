# Phase 19 — byo-mode

Status: in_progress. Owner: internal/nlq. Hard dependencies: 02, 10, 17.

## Authority and design

RFC-001 §9, D-052 and [COMMON.md](COMMON.md) apply. An external agent receives a context contract and submits SQL through the same validator/reader. A bundle reference is data lookup, never a new capability token.

## Brief findings incorporated

Briefs 03, 08, 12, 14: portable explicit constraints, provenance-labeled examples, independently useful context/submit surfaces and adversarial mode parity.

## Findings I'm departing from

Replace the old locally signed bundle handle with a bounded opaque stored reference. Pengui JWTs remain the only authority and are checked on every lookup/submission.

## Scope and implementation tasks

1. Publish versioned context/submit schemas with explicit SQL requirements, selected semantic versions, mandatory constraints and provenance-labeled examples.
2. Persist bounded expiring context bundles under opaque IDs; bind tenant/user/session/context and reauthorize every lookup/submission.
3. Use the identical validator/reader and document multi-step external agent analysis without building another agent runtime.

## Non-goals

No local signing key, separate query executor, implicit current-version substitution or internal analyst orchestration.

## Config and persistence

Query bundle TTL/byte/count ceilings and retention; no bundle signing key or alternate auth mode. Store the compact exact bundle and binding, not an unbounded prompt history or bearer token. Expired/invalid references require explicit new context construction.

## Acceptance criteria

1. **AC01** — Bundle schema/version and context contents have golden/backward-compatibility tests; context cannot hide mandatory constraints.
2. **AC02** — Opaque reference lookup rejects wrong tenant/user/session/context, expiry and changed data authority; no locally signed capability exists.
3. **AC03** — Submitted SQL rejects write/relation/parameter/dialect escapes through the same enumerated checks as internal generation.
4. **AC04** — Context-only authority cannot submit/execute; submit does not gain undeclared source reach or expose foreign bundle metadata.
5. **AC05** — Multi-step sessions retain per-step topic/version/provenance evidence and budgets without automatically orchestrating extra work.
6. **AC06** — Expired/invalid bundles return explicit replan requirements, never silent substitution with current semantics.

## Tests, coverage and smoke

Implement `TestPhase19/AC01` through `TestPhase19/AC06`, real stored references and query execution, arbitrary submitter negatives and an enumerated validator parity suite. COMMON.md supplies coverage; `scripts/smoke/phase-19.sh` requires all six results.

## Glossary, decisions and deviations

Opaque context reference has no authority independent of the current JWT. D-052 supersedes local signed-handle machinery. The implemented contract and retry semantics are documented in
[BYO SQL](../contracts/byo-sql.md); the [review record](../reviews/phase-19-byo-mode.md)
maps the six acceptance criteria to executable evidence. The phase stays
`in_progress` until its hosted gates and review complete; no full-release or
live-cloud qualification is implied.
