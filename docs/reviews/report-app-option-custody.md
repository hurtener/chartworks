# Governed authoring option lookup qualification

2026-10-04. This is a bounded native option-lookup checkpoint for the manual
report app, not complete Builder or browser qualification.

## Implemented contract

`DatasetOptions` reads choices for one allocated new block and an exact reviewed
topic/dataset/dimension. `ReportOptions` derives the dimension from the saved
report revision, digest, page, filter and actual parameter bindings. Its explicit
report policy is `private_preview` or `published`. A private parent requires its
actor and preview authority independently of its dependencies. Each block pin
keeps its own policy: published dependencies need no private-preview permission;
private dependencies preserve their actor and preview checks after publication.

The first lane supports direct PostgreSQL text dimensions from the finite
prepared origin. Mandatory dimension filters, temporal policies, active rules,
group/population policies, legacy origins without proof and floating block
revisions produce explicit unsupported results. Different immutable published
block revisions remain eligible; no latest revision is substituted. The existing
published `FilterOptions` operation and its wildcard authority behavior are
unchanged. New option operations require exact signed reach.

Search is an explicit bounded read over the complete reviewed source relation.
It is not a filter over an initial client page, and chart defaults/runtime
selections do not implicitly cross-filter the option population. Text search is
bound and escaped. Keyset cursors bind authority, target, semantics, source
revision, search, limit and locale, and expire after five minutes. SQL NULL is
excluded; the empty text category remains a valid exact value.

The maximum page is **199 choices plus one sentinel**, preserving the native
200-row preview ceiling. The existing validator/executor enforce source cost,
deadline and concurrency. Both source results and the final serialized
value/label response have a 512 KiB ceiling. Oversize values/results reject;
category values are never truncated.

## Admission and recovery

The client generates `option:<canonical Unix seconds>:<32 lowercase hex>`.
Fresh admission permits at most five minutes of age and thirty seconds of future
skew. Migration 086 stores coordinate/digest metadata without SQL, raw search,
cursor values, choices or tokens. Operation identity is unique per tenant/actor;
the original session remains required for its custody. Changed inputs conflict.

The accepted reservation seals one content-free validator receipt. The native
`BeginRead` trigger binds actor/session, operation, exact receipt, source/context,
attempt one, preview mode, rows, bytes and deadline. It rechecks report/block,
topic/rule-absence and source pins. Reservation cancellation is serialized with
native attempt insertion. Final value release repeats current authority and
pin checks transactionally.

`OptionStatus` is metadata-only. `OptionControl` explicitly cancels or reconciles
the original native attempt and can contact the source control lane. Neither
operation restarts queries. No choices are cached. Lost/replayed replies return
`values_available=false`, distinct from a genuine successful empty page. A new
query requires a new explicit operation after terminal source outcome is proven.
Actor/target liability spans changed searches, cursors, revisions and sessions;
a new login cannot bypass an unresolved old attempt. Reconciliation currently
requires the original session's fresh authority.

Reservation-time pruning removes at most 100 proven terminal records older than
24 hours. Active/unknown liability is retained. Expired operation keys cannot
become fresh after metadata or native journal cleanup. Limits are 512 records and
16 MiB per actor, and 10,000 records and 128 MiB per tenant, with 256 KiB maximum
record size. The final quota correction reserves that maximum for every pending
record before its receipt is added. Source, report revision and policy-bearing
block revision foreign keys preserve ownership; unresolved metadata is not
silently removed to permit resource erasure.

## Exact qualification

Production/test source `9f0f4f94ccfe3d799e75f954494dbb25fbcd4fdf` passed:

- Focused reporting race tests, including new operation-age, null/keyset,
  lost-value, exact-pin and filter metadata tests plus existing option tests:
  13.504 seconds.
- PostgreSQL store schema/manifest race test: 1.086 seconds.
- Actual PostgreSQL race acceptance roots below: 24.066 seconds total.
  - `TestReportAppAuthoringOptions`
  - `TestReportAppOptionCancellationDuringValidation`
  - `TestReportAppOptionReceiptAndRuleFences`
  - `TestReportAppOptionRetention`

The happy journey includes full-population pagination, literal search, exact
operation replay without another read, private report filters, publication and
explicit rebind, a Consumer without preview grants, same-revision mixed policies,
repeated mixed published revisions, native `Describe.definition_digest`, floating
refusal, source overflow and duplicated escaped JSON overflow. Negatives cover
cross-session reuse, current source reach, native receipt/identity/limit mismatch,
cancellation during validation, rule activation during a read, expired-key cleanup
and retained unknown liability. All use synthetic source data, without a model
call in the measured option journeys.

The dedicated PostgreSQL process stopped cleanly after terminal success. The
captured run is `qualification-final.log`; earlier failed runs remain separate.
The earlier filter/preparation regression set passed at the initial option
checkpoint, not after every later delta. Independent read-only review found and
verified corrections for mixed-revision deduplication, per-block policy custody
and floating references; it did not execute tests.

`f28633b` subsequently corrects pending-byte reservation accounting and adds its
actual-store quota regression. That final delta awaits the integrated Go/PG run;
the successful timings above must not be attributed to it. HTTP/MCP/SDK, app
integration and browser qualification are separate owner checkpoints. No main
application, fixture export, remote publication or live-provider run was performed
by this isolated workstream.

## Integrated follow-through

At `ea86fdc`, the final pending-byte quota change passed the integrated run:
reporting API/MCP/SDK full race suites (41.162s/3.881s/39.332s), focused reporting
and store race tests, and all four PostgreSQL option roots (24.109s). The fixture
stopped after terminal success. A later exact native request/response capture
passed UI module conformance at `617a62f`; it stayed local and synthetic. See
[manual filter integration](report-app-filter-integration.md) for UI/resource limits.
