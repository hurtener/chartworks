# Reporting delivery v1 — schedules and MCP Apps

This is the implemented Chartworks consumer contract for phases 30 and 31.
It extends [frozen runs](reviewed-engineering-and-frozen-runs.md) and
[report composition](reporting-composition-v1.md); it does not deliver phase-32
static exports, a standalone builder, or a new authentication system. Exact-source
verification and remaining qualification boundaries are recorded in the
[adversarial review](../reviews/phase-30-31-adversarial.md).

## One domain service, five portable operations

HTTP, the typed Go client, generic CLI dispatch and MCP all invoke the same
reporting service. Responses use `version: "reporting-view-v1"`. HTTP/MCP result
and error envelopes retain their existing transport conventions; the enclosed
reporting result and selected-output semantics agree.

| HTTP POST | MCP tool | Typed Go method | Effect |
|---|---|---|---|
| `/v1/reporting/search` | `reporting_search` | `SearchReporting` | Published metadata read; no SQL or retained values. |
| `/v1/reporting/describe` | `reporting_describe` | `DescribeReporting` | Published presentation/output/filter metadata, not query text or narrative prompts. |
| `/v1/reporting/run` | `reporting_run` | `RunReporting` | Explicit keyed execution; may query, call an opted-in model and retain results. |
| `/v1/reporting/runs` | `reporting_runs` | `SearchReportingRuns` | Bounded authorized retained catalog, never an occurrence trigger. |
| `/v1/reporting/view` | `reporting_view` | `ViewReporting` | One selected retained output/table page, with no source/model execution. |

Reads require `reporting.read`, current target/run read reach and the actual
retained execution-context reach. Execution independently requires
`reporting.execute` and all resolved target/dependency/query permissions. MCP also
requires `mcp.use` and its intended audience. UI visibility, a creator, a recipient,
a guessed ID or an execution binding is never an artifact grant.

A run request fixes a positive published revision, stable key, ordered output
selection, typed arguments/page filters, locale/timezone, explicit dynamic and
narrative consent, and partial policy. A changed request under the same key
conflicts. Identical replay does not make another query. `RunReporting` has no
automatic retry after an unknown outcome; inspect the existing catalog/receipt
before resubmitting intent. Earlier SDK `ReportingRunRequest` and
`ListReportingRuns` methods remain intact; delivery uses the distinct
`ReportingDeliveryRunRequest` and `SearchReportingRuns` names.

The selected view includes `selection`, `summary`, exact `page_bounds`, output
choices, permission-filtered report pages, business filters, trust/freshness,
locale/timezone and the selected output. `retained_digest` identifies the complete
retained output; projection/page digests do not pretend to be that complete hash.
Decimals and large integers remain strings in labels and tables. Approximate SVG
geometry is identified as such. Returned-row subtotals never imply full-source
totals. Private, partial, expired, omitted and unavailable states remain explicit.

The original block execution `policy` is included in an authorized view. Filter
reruns preserve `published` versus `certified_only`, and verify the described
resource kind, ID and revision before requesting execution. Current certification
and source authority are still checked server-side; historical display state is
not a grant. Pagination and redraw retain the same artifact and never rerun it.

## Apps resource and browser boundary

Enable `features.mcp` and include `reporting` in `mcp.groups`. The default group
set now contains discovery, query, byo, charts and reporting; only real installed
services are registered. The view tool advertises `_meta.ui.resourceUri` for
`ui://chartworks/report-viewer/v1`, with MIME
`text/html;profile=mcp-app`. The bundled resource declares empty external network,
resource/frame/base-URI domain lists and no device permissions. It contains no
tenant values, source/provider credentials, bearer, local account or token URL.

The read viewer uses the established Apps parent bridge. It pins the responding
parent origin, limits message size/pending calls, supports cancellation/teardown,
and ignores stale responses. It has no `fetch`, cookie or local-storage credential
channel. Tool arguments cannot contain SQL, arbitrary URLs, authorization headers
or replacement chart programs. The parent/BFF supplies fresh authority to each
provider call. Failed, mismatched, oversized or expired results clear visible data.

All fourteen kinds in the versioned [chart catalog](chart-specifications-v1.md)
are rendered and exercised by the real browser fixture matrix.
Tables, accessible chart values and stored narrative/text accompany safe geometry.
English/Spanish labels, host locale, light/dark theme, resize, table continuation,
page/widget/output navigation and explicit filter execution are exercised in the
actual bundled Chromium component. Missing Chrome or Node 22+ fails acceptance;
source-code scanning alone is not the browser proof. Hosts without UI support
receive useful equivalent structured/text results. No host-compatibility project
or protocol migration is required by this implementation.

## Real scheduling targets

Reporting extends the existing durable `reporting.scheduled` submission kind.
The request contains an opaque approved `binding_id` and a closed `reporting`
target. No second queue, local signer, stronger-account selector or hidden report
is created.

| Target type | Existing object and execution rule |
|---|---|
| `saved_sql` | Reviewed published SQL-bearing block; exact revision by default; selected saved outputs. This does not claim certification. |
| `block` | Exact positive certified block revision and selected output IDs. Floating latest is rejected. |
| `saved_question` | One explicitly selected replayable dynamic query widget in a published report. Requires dynamic opt-in and model budget; unavailable/session-only context is rejected. |
| `report` | Published report and its existing frozen/dynamic/text composition. Dynamic and narrative lanes remain explicit and governed. |

The saved-question representation deliberately reuses the already governed report
widget and phase-18 execution service, rather than creating another mutable saved
question/IAM store. A direct SQL/block target creates no report at all. Report
children delegate one counted root execution slot with immutable parentage and
parent/child fences, not a second root queue admission.

`latest_published:true` requires revision zero and resolves once when each new
occurrence is accepted. Exact revisions, report child pins, accepted due instant,
half-open window, request hash, binding and resource coordinates survive edits,
retries and newer publications. Missing definitions can leave an explicitly blocked
occurrence rather than disappearing from history. Missing current approval,
outputs, source context or renewed authority cannot silently repin or broaden it.

## Authority, recurrence and lifecycle

Creation/replacement checks the caller's signed binding-use and target/dependency
permissions. Execution uses the existing
[Pengui execution-authority adapter](execution-authority-v1.md): request shape,
trusted endpoint, opaque `auth.Execution`, intended jobs audience and short-lived
issuer signature are unchanged. Every attempt obtains and verifies fresh authority
for its exact binding/job/manifest and service identity. Model reservations also
match tenant, actor and session. The scope set must actually be provisioned by
Pengui; a fixture-backed consumer test is not evidence of production issuer setup.
Refusal blocks, transient outages retry within policy, and expired authority never
renews itself locally. No user bearer is persisted in schedules or artifacts.

Supported triggers are manual, anchored interval and five-field minute cron.
Interval bounds are 60 seconds through 31 days, with an explicit whole-second
anchor. Timezones use the checked-in archive pinned by
`jobs.timezone_database_version`; host OS timezone changes cannot rewrite windows.
Cron selects real instants: missing local times have no invented occurrence and
repeated local times remain distinct due instants. Parameter periods separately
retain their declared first-occurrence, DST and month policy.

Missed occurrences are `skip` or bounded `catch_up` (1–32); overlap is `skip` or
bounded `queue`. Defaults do not imply unbounded catch-up or concurrency. Stored
occurrence and revision streams expose the actual accepted/skipped decisions.
Relative periods resolve from accepted logical time, not a retry's wall clock.

| Operation | HTTP | Typed client |
|---|---|---|
| Create | `POST /v1/schedules` with Idempotency-Key | `CreateSchedule` |
| Read | `GET /v1/schedules/{id}` | `Schedule` |
| Replace future intent | `PUT /v1/schedules/{id}` with key and expected revision | `ReplaceSchedule` |
| Pause/resume | `PUT /v1/schedules/{id}/state` with expected revision | `SetScheduleState` |
| Permanently retire | `POST /v1/schedules/{id}/retire` with expected revision | `RetireSchedule` |
| Real test execution | `POST /v1/schedules/{id}/test` with key and expected revision | `TestSchedule` |
| Manual fire | `POST /v1/schedules/{id}/runs` with key | `FireSchedule` |
| Bounded history | `POST /v1/schedules/{id}/history` | `ScheduleHistory` |
| Cancel accepted work | `POST /v1/jobs/{id}/cancel` | `CancelJob` |

Test execution is cost-bearing, not a dry run. Pause/replace/retire changes future
unaccepted work, not an accepted job; cancel that job explicitly. Read/history,
pause and retirement remain usable without a model or running dispatcher.
Creation/test/admission requiring a dispatcher are not registered as success stubs
when it is unavailable. Event, condition, condition-check and custom-code triggers
are rejected and absent from capabilities.

## Budgets, uncertainty and delivery

Each target supplies `timeout_ms`, `max_rows`, `max_bytes`, `query_attempts`,
`model_calls`, and `model_tokens`. These are ceilings, not observed usage. Durable
pre-call reservations cover actual read and Bifrost attempts across retries; they
are not reset after a worker failure. Target time/row/byte limits only narrow the
existing deployment and current-authority limits. Frozen work with narrative off
uses zero model calls/tokens. Model receipts preserve unknown costs as unknown.

A source operation accepted before its result checkpoint cannot be declared
successful or silently resubmitted when evidence is lost. A retained result can
resume output/catalog publication without another query. Live parent/child fences,
cancellation and current authority guard writes at effect/checkpoint boundaries.
Corrupted manifests and violated invariants require attention; genuine storage
outages may retry. Fault-injection tests distinguish these failure classes.

Catalog pull is the implemented delivery baseline. `query`, `artifact`, `catalog`
and `notification` outcomes are independent; an artifact may be retained while
catalog publication is pending. A successful publication transaction is replay-safe.
Recipient IDs are descriptive metadata only; this release emits **no outbound
email/chat notification** and records `notification: "not_requested"`. It neither
fabricates a sent receipt nor adds an unimplemented notification endpoint.

Authorized ordinary catalog/view responses may include `summary.scheduled`:
schedule ID/revision, due/window instants, execution and independent delivery
stages, and actual publication time. Bindings, actors, recipients, tokens and
hidden report-page derived status are omitted. Same-tenant missing-context and
cross-tenant readers do not acquire artifact data through this metadata.

## Document deletion and catalog presentation

Archive and deletion are distinct. `GET /v1/reports/{id}/delete-impact` and the
dashboard equivalent return bounded counts and currently readable matching
schedule identifiers without loading definitions or values. `POST
/v1/reports/{id}/delete` and its dashboard equivalent require exact current
version, replay key, nonempty reason, `reporting.write`, and target-write reach.
Every matching schedule additionally requires current `scheduling.write` and
schedule-write reach before the transaction changes anything.

Deletion scrubs live definition payloads and external import mappings, erases retained composition payloads,
expires their bounded receipts, retires matching report/saved-question schedules,
and retains schedule history plus a deletion tombstone. Stale execution cannot
complete after the tombstone. Shared blocks/topics are preserved. Dashboard
deletion does not infer ownership or cascade to reports; report deletion preserves
dashboard history while current page projection omits the deleted report. Identical
replay returns the tombstone and changed intent conflicts. Backups, replicas and
WAL are outside the live-data erasure result.

Document catalog summaries carry bounded block/topic/schedule relationships and
descriptive creator/last-editor presentations. Actor IDs remain protected audit
coordinates and are not serialized in the summary. An optional Pengui-owned label
resolver supplies known labels; missing/deleted and service actors receive safe
fallbacks. Labels, recipients and relationship membership never grant access.
Schedule relations require current schedule-read action and resource reach before
their identifiers enter the SQL result.

## Deployment and examples

Merge the [configuration excerpt](../../examples/chartworks.reporting-delivery.json)
into the existing operator-owned configuration. It enables no dispatcher and
creates no credentials. The
[saved SQL schedule example](../../examples/reporting-schedule.json) is a request
for already existing reviewed resources and a Pengui-approved binding, not a
bootstrap script. Supply it through the ordinary authenticated SDK/API; never put
a literal bearer in arguments, a resource URL or browser storage.

```go
// client is the application's existing *chartworks.Client with a current
// caller-supplied Pengui TokenProvider. No scheduler or model is needed to read.
page, err := client.SearchReportingRuns(ctx, chartworks.ReportingRunsRequest{
    Kind: "block", Resource: "sales-summary", Limit: 20,
})
if err != nil { return err }
if len(page.Items) == 0 { return nil }
view, err := client.ViewReporting(ctx, chartworks.ReportingViewRequest{
    Kind: "block", Run: page.Items[0].Run, Output: "table-main", Limit: 100,
})
if err != nil { return err }
// Consume view.Output and view.Summary.Scheduled as authorized retained data.
```

New migrations 031–033 add fenced report-child relationships, the timezone archive
coordinate, occurrence delivery and budget state. Existing migrations 001–030 are
unchanged. Apply the normal service migration path; do not edit applied migration
checksums or manually manufacture a successful occurrence. Required build/runtime
fixtures and the current package coverage gates remain in the ordinary CI.
