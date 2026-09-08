# Phase 19 — external context and SQL review

Review date: 2026-09-08. Scope: PR #12, recovered exact implementation head
`c8c17906f851c770e9cd0d5fef50706f28fc8f97`, followed by the fixes in this delivery.
The base is merged main `a26e026b07bd42047422edeb47118dffac784277`.
This document records inspected behavior and executable regression coverage;
final exact-head hosted results belong to the [PR checks](https://github.com/hurtener/chartworks/pull/12/checks)
and its verification comment. It does not assert that an unobserved run passed.

## Findings and corrections

1. **Context-only access incorrectly required query authority.** The BYO source
   accessor reused `sources.Service.Binding`, which deliberately requires
   `sources.query` and source-query reach for validator use. Replaced only the
   context metadata port with `ContextBinding`, requiring source-read and exact
   execution-context reach. Both accessors share metadata loading; the original
   validator/query gate is unchanged. AC04 exercises context creation/lookup with
   no query authority, direct query-binding/validator denial, submit-without-source
   denial and separately authorized submission of the same data snapshot.
2. **Stale-rule and archived-topic negatives interfered.** AC06 constructed an
   additional context after retiring mandatory rules and failed before checking
   archive behavior. Archive now has an independent healthy fixture, with both
   lookup and submission returning replan and zero execution calls. The original
   stale-rule and exact-expiry checks remain.
3. **Schema regression inventory omitted migration 021.** Added the two owned BYO
   tables to Phase02/AC04's exact schema inventory. The equality assertion and
   prohibition on local IAM tables remain intact.
4. **Intentional nil-context rejection triggered Staticcheck SA1012.** The one
   deliberately invalid test call has a narrowly explained Staticcheck directive;
   production context handling and repository-wide lint remain enabled.
5. **Durability/limits lacked direct PostgreSQL failure probes.** Added
   `TestBYOStoreFencesAndAudit`: failed bundle/step audit admission rolls back;
   failed terminal audit leaves an accepted receipt retryable; pinned input and
   sealed evidence cannot be rewritten; operation replay and changed-input
   conflicts preserve the step budget; concurrent session and tenant ceilings
   hold; retention deletion cascades receipts without crossing tenants.
6. **Consumer documentation and named CI invocation were incomplete.** Added the
   version-1 wire/retry/authority contract, typed configuration defaults/bounds and
   example excerpt. CI explicitly invokes all Phase19 criteria and the bounded
   reference/receipt fuzz target, without generating or repairing source.

## Acceptance map

| Criterion | Executable evidence |
|---|---|
| AC01 | Mandatory rules/metrics, English/Spanish context, detached exact stored snapshot, unreviewed caller examples, schema rejection; API golden and SDK compatibility tests. |
| AC02 | Real Pengui-verifier envelopes; foreign tenant/user/session/context, changed data reach and reference-only authority negatives. |
| AC03 | Enumerated shared-validator unsafe SQL/relations/parameters/dialect negatives; narrower semantic column projection; allowed parameter, CTE, window and set-query parity. |
| AC04 | Useful context-only permissions without native planning/execution; no manufactured source reach; real HTTP/SDK create/read/submit and closed wire schemas. |
| AC05 | Model-free restarted service, concurrent idempotent steps, no replayed values, changed-input conflict, exact step limit and content-free semantic receipts. |
| AC06 | Exact expiry, absent reference, retired rules and independent archived-topic replanning; no current-version substitution or warehouse execution on denial. |

## Security and lifecycle review

Inspected the three transport operations through runtime registration, SDK, service,
shared validator/native planner, ordinary opaque-plan executor, PostgreSQL
transactions and migration fences. Bundle references are private data coordinates,
not signing credentials. All live operation authority is current Pengui authority.
The captured data reach cannot be expanded or narrowed through a new action-only
JWT; grant changes require explicit fresh context. Submit retains normal source,
context, dependency and physical-binding enforcement.

No local signer, raw-SQL execution port, automatic SQL correction, model call on
lookup/submission, new job queue or result cache is introduced. Replays do not
pretend to recover lost values. Post-dispatch receipt failure suppresses result
return; unresolved accepted evidence does not authorize a second execution. Source
and topic/rule pins are checked before work, with a second semantic check after
native planning. Arbitrary external SQL is not certified as business-correct by
passing the enumerated safety validator.

## Verification gate and limitations

Local review uses the recovered Git pack, formatting, whitespace, planning,
mirror and drift checks. The local environment lacks the required Go 1.26.4/native
parser build dependencies; build, lint, all-package race coverage, strict named
acceptance, binary smoke, native Linux/macOS builds and cumulative development
preflight must pass on the actual committed-source GitHub Actions pipeline before
PR #12 is marked ready. Coverage thresholds are unchanged, including the existing
owner-approved 84.5% PostgreSQL band.

Recorded cloud/model fixtures remain recorded fixtures, not new paid/live-provider
qualification. Later planned phases and the strict phase-25 full-release gate are
not claimed complete. No temporary source export or source-editing workflow is
part of this delivery, and readiness is not permission to merge the PR.
