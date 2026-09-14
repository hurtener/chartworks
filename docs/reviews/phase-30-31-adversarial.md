# Phase 30/31 adversarial review and verification ledger

Scope: the `feat/phase-30-31-reporting-delivery` implementation, preserving merged
phase29/main `bce498536c8cd51e5c4b2509365c2edb0359ef68`. This is a source and
real-consumer adversarial review, not a live deployment/provider qualification.
The [runtime contract](../contracts/reporting-delivery-v1.md) owns the public
behavior. Phase statuses remain in_progress for review rather than concealing
implemented code behind planned-phase skips.

## Findings addressed

| Finding | Correction and regression evidence |
|---|---|
| Reporting admission was only partially connected to the original queue. | Actual immutable dispatch pinning, repository admission/sealing, production composition-root wiring and four real target paths. Phase30/AC01, AC04 and AC08. |
| Nested report children could consume another root queue slot or outlive ownership. | Immutable parentage, sequential delegation and both live parent/child fences; original phase29 regressions and concurrent phase30/AC05. |
| Reporting budget errors were hidden as generic storage unavailability. | Preserve public budget/attention sentinels through wrappers/joins; pre-call physical query/model reservations survive retries. Error unit tests and phase30/AC05. |
| Retry classification conflated failed infrastructure with invalid evidence. | Storage-outage injection uses SQLSTATE58000; invalid retained manifests/invariants remain attention. Existing assertions still prove rollback, retry and no resubmission of uncertain source work. |
| Query/artifact success could be confused with catalog publication. | Read actual retained-stage evidence; independent replay-safe catalog publication and explicit not_requested notification. Real injected publication failure/recovery and phase30/AC06. |
| A schedule artifact's ordinary catalog lacked accepted occurrence provenance. | Context-authorized content-free summary; no bindings, actors, recipients, hidden-page-derived status or authority bytes. Phase30/AC06 ordinary catalog/viewer tests. |
| Filter describe responses only matched revision, not target identity. | Require exact resource kind/ID/revision and consistent artifact selection/summary. Actual Chromium security component tests. |
| Filter reruns discarded certified-only policy. | Propagate the original sealed block policy into the authorized view and rerun request; current server eligibility still applies. Certified scheduled view and browser request assertions. |
| Catalog response bytes were not bounded on both storage paths. | Apply the same serialized-message ceiling to frozen and composition catalogs, returning no partial metadata on refusal. Real multi-run catalog fixtures in phase31/AC08. |
| Model reservation callbacks only compared tenant. | Opaque matcher binds current tenant, actor and session; foreign/expired/zero principal negatives. Real scheduled consumers retain their domain permissions. |
| A history cursor's nullable schema and fixtures diverged from actual contracts. | Correct closed schema option; preserve archive pointer invariants and nonoverlapping grid fixture. Phase21 registration and phase29 regressions. |
| Migration and production-coverage inventories omitted new consumers. | Explicit new 031–033 identities, calendar and web viewer bands; existing thresholds unchanged. Full coverage also includes browser-backed acceptance. |

## Acceptance ownership

Phase30/AC01 covers all four targets; AC02 current broker authority and denied
admission; AC03 calendar/window semantics; AC04 immutable pins and CAS lifecycle;
AC05 concurrency, cancellation, attempts, budgets and uncertainty; AC06 independent
catalog delivery and ordinary viewer provenance; AC07 unavailable/withdrawn
business dependencies; AC08 cumulative API/SDK contracts and unsupported kinds.

Phase31/AC01 covers actual UI metadata/resources; AC02 authorized metadata and
retained reads; AC03 artifact states; AC04 fourteen actual chart kinds; AC05
explicit filter execution; AC06 locale/theme/resize/accessibility/paging; AC07
HTTP/MCP/SDK parity without a scheduler; AC08 real component injection/identity
checks plus provider request/output/catalog bounds.

## Exact-source evidence

Candidate `0ee5f2ef0e3a7352d72e74caaeff43754ce1854d`, Actions run
`34882308894`: all eight strict phase30 criteria and all eight strict phase31
criteria passed with no unimplemented skips. Race-enabled original phase06/21/29
regressions passed. Planning checks passed. Broader unit verification caught an
outdated assertion expecting 30 migrations; it was corrected to explicitly verify
all 33 identities rather than weakening the migration-history guard.

Additional response-identity, certified-policy, scheduled-provenance and catalog
cap regressions are included after that candidate. They require fresh exact-source
results; earlier passes are not attributed to later edits. Full race coverage,
real PostgreSQL/MySQL/SQL Server fixtures, native dependencies and typed lint run
through the read-only verification workflow. The final evidence addendum records
the actual final result, not a planned command presented as a pass.

## Qualification limits and intentional boundaries

Catalog pull is implemented. Recipient metadata neither grants access nor proves
an email/chat message was sent; notifications remain not_requested, with no stub
sender. No new account, signer, issuer endpoint, bearer persistence, BFF credential
service or alternate learned-model client is introduced. Pengui remains the sole
issuer, and production binding provisioning is separate deployment work.

The saved-question target is an explicitly selected replayable published report
widget, not a second saved-query authoring or IAM model. Direct block/saved-SQL
execution creates no hidden report. The Apps viewer does not depend on a scheduler.
Static rendering/export/BFF embedding, live model quality, cloud cutover, migration
and final release are not claimed by these phase30/31 tests.

## Final verification and submission — 2026-09-14

Verified implementation and test revision:
`5bafeb6e6bc354d94ef6ddd398550a59e9037503`.

The completed [reviewed-source workflow](https://github.com/hurtener/chartworks/actions/runs/34894620229)
and independent [reporting-contract workflow](https://github.com/hurtener/chartworks/actions/runs/34894620278)
both succeeded on that exact revision. Downloaded evidence identifies the same
`TESTED_COMMIT`; it is not inferred from a green parent job or a source filename.

| Verification | Observed result |
|---|---|
| Strict phase30 acceptance | AC01–AC08 passed; `unimplemented_skips=0`. |
| Strict phase31 acceptance | AC01–AC08 passed, including actual Chromium component tests; `unimplemented_skips=0`. |
| Original phase06/21/29 race regressions | Passed, including cumulative API and composition behavior. |
| Complete race-instrumented coverage suite | Passed with real PostgreSQL, MySQL and SQL Server fixtures and pinned native dependency. |
| Production coverage gate | Every registered band passed; no threshold was lowered. |
| Go build, vet and formatting | Passed on committed source. |
| Typed golangci-lint | Passed using v2.12.2. |
| Planning coherence | Passed; planning is recorded separately from runtime acceptance. |

Measured statement coverage was 84.3% overall. Relevant exact band results were:
`internal/reporting` 80.77%, `internal/reportingapi` 82.86%, `internal/jobs` 81.47%,
`internal/jobs/pengui` 96.61%, `internal/calendars` 89.19%,
`internal/gateway/bifrost` 86.95%, `internal/store/postgres` 84.12%,
`internal/workapi` 85.49%, and `sdk/chartworks` 86.51%. PostgreSQL retains the
previously owner-approved 84% band. The Go resource embed wrapper measured 100%;
that figure is **not JavaScript coverage**. Browser behavior is proven separately
by the real component criteria, not by the wrapper's four instrumented statements.

The final review rechecked accepted revision/window integrity, creator versus
renewed execution authority, durable pre-call usage, parent/child publication
fences, independent delivery stages, current-context catalog filtering, escaped
viewer content, parent-origin binding, and certified-only filter reruns. No
unresolved blocking finding remains in this reviewed scope. Added boundary tests
cover transactional failures, nested ownership, partial delivery and corruption
of the pinned timezone distribution as part of the verified revision above.

Submission cleanup removes all five remaining phase30/31 source-transfer,
diagnostic and temporary verification workflows. The temporary write-capable
editor and editing script were already removed in the verified revision. The
permanent read-only `reporting.yml` and full `ci.yml` retain strict acceptance,
real-browser checks, full regression coverage and typed lint; CI also rejects
reintroduction of recovery and editing aids. This cleanup changes documentation and
CI bookkeeping only: production code, tests, dependencies, migrations and coverage
bands are byte-identical to the verified implementation. The PR's own checks
remain a separate run and are not retroactively represented by the earlier SHA.

The phases remain `in_progress` until review/merge, as required for submissions.
That status does not indicate a skipped or missing criterion. Notification
sending, phase32 static exports and production issuer provisioning remain the
explicit out-of-scope qualification boundaries described above.
