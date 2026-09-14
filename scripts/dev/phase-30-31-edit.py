"""Documentation, public aliases and positive/negative operator configuration tests."""
from pathlib import Path
import json
import subprocess
changed=set()
def replace(path,old,new,count=1):
    p=Path(path); text=p.read_text()
    if new in text:return
    if text.count(old)!=count:raise RuntimeError(f'Unexpected anchor {path}: {old[:120]!r}')
    p.write_text(text.replace(old,new));changed.add(path)
def write(path,content):
    p=Path(path)
    if p.exists():
        if p.read_text()!=content:raise RuntimeError(f'Refusing unexpected overwrite {path}')
        return
    p.write_text(content);subprocess.run(['git','add','-N','--',path],check=True);changed.add(path)
def append(path,content):
    p=Path(path);text=p.read_text()
    if content in text:return
    p.write_text(text.rstrip()+'\n\n'+content);changed.add(path)

write('docs/contracts/reporting-delivery-v1.md','''# Reporting delivery v1 — schedules and MCP Apps

This is the implemented Chartworks consumer contract for phases 30 and 31.
It extends [frozen runs](reporting-execution-v1.md) and
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
`ui://chartworks/report-viewer/v1.html`, with MIME
`text/html;profile=mcp-app`. The bundled resource declares empty external network,
resource/frame/base-URI domain lists and no device permissions. It contains no
tenant values, source/provider credentials, bearer, local account or token URL.

The read viewer uses the established Apps parent bridge. It pins the responding
parent origin, limits message size/pending calls, supports cancellation/teardown,
and ignores stale responses. It has no `fetch`, cookie or local-storage credential
channel. Tool arguments cannot contain SQL, arbitrary URLs, authorization headers
or replacement chart programs. The parent/BFF supplies fresh authority to each
provider call. Failed, mismatched, oversized or expired results clear visible data.

All fourteen catalog kinds are rendered: area, bar, column, donut, grouped bar,
heatmap, KPI, line, pie, scatter, stacked bar, stacked column, table and treemap.
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
''')
write('examples/chartworks.reporting-delivery.json',json.dumps({
    'mcp':{'groups':['discovery','query','byo','charts','reporting']},
    'reporting':{'viewer':{'max_message_bytes':2097152,'max_rows':500,'page_rows':100,'max_outputs':32,'max_points':5000}},
    'jobs':{'timezone_database_version':'go1.26.4-sha256:8f55634d05f8bca1f7bc7c69c5933428c69357e0bdf565e5ba224e3f88ff12e8'}},indent=2)+'\n')
write('examples/reporting-schedule.json',json.dumps({
    'target':{'kind':'reporting.scheduled','binding_id':'approved-reporting-binding','reporting':{
        'type':'saved_sql','id':'sales-summary','revision':1,'latest_published':False,'outputs':['table-main'],
        'arguments':[],'locale':'en','timezone':'America/Argentina/Buenos_Aires','dynamic':False,'narrative':False,
        'partial_failure':'fail_closed','budget':{'timeout_ms':10000,'max_rows':1000,'max_bytes':1048576,'query_attempts':1,'model_calls':0,'model_tokens':0}}},
    'spec':{'type':'cron','cron':'0 9 * * 1-5','timezone':'America/Argentina/Buenos_Aires','missed':'skip','overlap':'skip'}},indent=2)+'\n')
write('internal/config/reporting_delivery_test.go','''package config

import (
    "bytes"
    "encoding/json"
    "os"
    "testing"
)

func TestReportingViewerConfigurationBounds(t *testing.T) {
    valid:=[]ReportingViewer{DefaultReportingViewer(),{MaxMessageBytes:16384,MaxRows:1,PageRows:1,MaxOutputs:1,MaxPoints:1},{MaxMessageBytes:4<<20,MaxRows:1000,PageRows:1000,MaxOutputs:64,MaxPoints:10000}}
    for _,v:=range valid{if err:=v.Validate();err!=nil{t.Fatal("valid closed bounds rejected",v,err)}}
    for _,tc:=range []struct{name string;edit func(*ReportingViewer)}{
        {"message-low",func(v *ReportingViewer){v.MaxMessageBytes=16383}},
        {"message-high",func(v *ReportingViewer){v.MaxMessageBytes=4<<20+1}},
        {"rows-low",func(v *ReportingViewer){v.MaxRows=0}},
        {"rows-high",func(v *ReportingViewer){v.MaxRows=1001}},
        {"page-low",func(v *ReportingViewer){v.PageRows=0}},
        {"page-high",func(v *ReportingViewer){v.PageRows=v.MaxRows+1}},
        {"outputs-low",func(v *ReportingViewer){v.MaxOutputs=0}},
        {"outputs-high",func(v *ReportingViewer){v.MaxOutputs=65}},
        {"points-low",func(v *ReportingViewer){v.MaxPoints=0}},
        {"points-high",func(v *ReportingViewer){v.MaxPoints=10001}},
    }{t.Run(tc.name,func(t *testing.T){v:=DefaultReportingViewer();tc.edit(&v);if v.Validate()==nil{t.Fatal("unbounded setting accepted",v)}})}
}

func TestReportingDeliveryOperatorExcerpt(t *testing.T) {
    data,err:=os.ReadFile("../../examples/chartworks.reporting-delivery.json")
    if err!=nil{t.Fatal(err)}
    var v struct{MCP MCP `json:"mcp"`;Reporting struct{Viewer ReportingViewer `json:"viewer"`} `json:"reporting"`;Jobs Jobs `json:"jobs"`}
    v.MCP,v.Jobs=DefaultMCP(),DefaultJobs();v.Reporting.Viewer=DefaultReportingViewer()
    decoder:=json.NewDecoder(bytes.NewReader(data));decoder.DisallowUnknownFields()
    if err:=decoder.Decode(&v);err!=nil{t.Fatal("unknown or invalid example setting",err)}
    if err:=ValidateMCP(v.MCP);err!=nil{t.Fatal(err)}
    if err:=ValidateJobs(v.Jobs,Auth{});err!=nil{t.Fatal(err)}
    if err:=v.Reporting.Viewer.Validate();err!=nil{t.Fatal(err)}
    if v.Jobs.Enabled||len(v.Jobs.Credentials)!=0{t.Fatal("excerpt bootstraps authority")}
}
''')
replace('sdk/chartworks/reporting_delivery.go','type ReportingRunSummary = reporting.ReportingRunSummary','''type ReportingRunSummary = reporting.ReportingRunSummary

// ReportingScheduledProvenance describes accepted windows and independent delivery stages.
type ReportingScheduledProvenance = reporting.ScheduledProvenance''')

# Keep original feature mappings and the reviewed source contracts; update only
# current implementation bookkeeping, not release/cutover qualification.
p=Path('README.md');text=p.read_text();start=text.index('## Current status');end=text.index('| Capability |',start)
text=text[:start]+'''## Current status

**The merged baseline through phase 29 implements 172 named acceptance criteria.**
Phases **30 and 31 add sixteen criteria for reporting schedules and the MCP Apps
read viewer**, bringing the implemented/in-progress inventory to 188 across 29 of
34 workstreams. The phase registry retains `in_progress` for review submissions;
actual named tests and exact-source CI, not that status label, establish readiness.
The [delivery contract](docs/contracts/reporting-delivery-v1.md) and
[adversarial evidence](docs/reviews/phase-30-31-adversarial.md) describe this change.

**Five workstreams remain planned: 24–25 and 32–34**, including evaluation,
static rendering/BFF exports, onboarding, migration and the final release gate.
There are 224 criteria in the complete plan. Catalog pull delivery is implemented;
no outbound notification service, static-export implementation, live-provider
qualification or production-cutover claim is implied.

'''+text[end:]
text=text.replace('| MCP — phase 22 | Eighteen installed-service tools, the eleven core contracts, three pure metadata resource bindings, fresh per-request authority, closed schemas, bounded work and explicit effects. No Apps viewer or local credentials. |','''| Reporting schedules — phase 30 | Four real target types on the shared queue; fresh Pengui authority, immutable due/window/revision pins, CAS lifecycle/history, fenced budgets and independent artifact/catalog delivery. No local issuer or fabricated notification. |
| MCP Apps — phases 22/31 | Eighteen existing tools plus five reporting tools when installed; a versioned read viewer, fourteen visual kinds, typed filter reruns, exact retained paging, theme/locale and safe structured/text fallback. No browser credentials or separate scheduler. |''')
text=text.replace('Harbor/Pengui MCP Apps support is established by the owner. Chartworks MCP tools use that established ecosystem; the retained-artifact viewer and exports remain later implementation work, not a reason to reopen host compatibility. Iframe credentials belong in the Pengui/client BFF.','Harbor/Pengui MCP Apps support is established by the owner. The Chartworks retained-artifact viewer uses that ecosystem; static rendering and exports remain phase 32. Iframe credentials belong in the Pengui/client BFF, not the public Apps resource.')
text=text.replace('These are JSON metadata, not rendered Apps or retained reporting artifacts.','These three resources remain JSON metadata. The reporting group additionally supplies the versioned credential-free Apps HTML resource and authorized retained-artifact tools.')
text=text.replace('eighteen. Paid or persisted calls are annotated accordingly:', 'eighteen. The reporting group adds `reporting_search`, `reporting_describe`, `reporting_run`, `reporting_runs` and `reporting_view`. Paid or persisted calls are annotated accordingly:')
start=text.index('Still planned:');end=text.index('\n\nUseful evidence',start)
text=text[:start]+'''Still planned: evaluation and release gates, static rendering/BFF embeds/exports,
guided onboarding and migration/cutover. Schedule catalog publication does not
mean an email was sent; retained artifact viewing does not execute a new query.
See the [runtime delivery guide](docs/contracts/reporting-delivery-v1.md) for
operator setup, target/lifecycle semantics, API/SDK methods and tested boundaries.'''+text[end:]
p.write_text(text);changed.add(str(p))
replace('docs/plans/README.md','Phases 23, 26, 27, 28 and 29 are in progress;\nthe remaining seven workstreams are planned','Phases 23, 26, 27, 28, 29, 30 and 31 are in progress;\nthe remaining five workstreams are planned')
replace('docs/plans/README.md','The registry maps 172 criteria','The registry maps 188 criteria')
replace('docs/plans/README.md','phases23, 26, 27, 28 and 29 are in progress, leaving seven planned workstreams','phases23, 26, 27, 28, 29, 30 and 31 are in progress, leaving five planned workstreams')
replace('docs/plans/README.md','[phase-29 review](../reviews/phase-29-adversarial.md) record verification','[phase-29 review](../reviews/phase-29-adversarial.md), and\n[phase-30/31 review](../reviews/phase-30-31-adversarial.md) record verification')
for n,slug in [('30','reporting-schedules'),('31','reporting-mcp-apps')]:
    replace(f'docs/plans/phase-{n}-{slug}.md','No runtime completion is claimed.','''The implementation and concrete representation choices are documented in
[reporting delivery v1](../contracts/reporting-delivery-v1.md). All eight named
criteria have executable real-consumer tests. Exact-source results and review
findings are recorded in the [adversarial ledger](../reviews/phase-30-31-adversarial.md);
status remains in_progress pending review/merge, not a planned-phase skip.''')
append('docs/configuration.md','''## Reporting delivery and timezone archive — phases 30/31

The [operator excerpt](../examples/chartworks.reporting-delivery.json) merges into
existing configuration without enabling a dispatcher or provisioning credentials.
See [reporting delivery v1](contracts/reporting-delivery-v1.md) for lifecycle,
authority and catalog-only delivery. All new fields below are non-secret.

| Key | Units / default | Closed bounds and behavior |
|---|---|---|
| `mcp.groups` | Array; discovery, query, byo, charts, reporting | 1–5 distinct implemented names. The reporting group registers real services only; `features.mcp` still defaults false. |
| `reporting.viewer.max_message_bytes` | Bytes; 2097152 | 16384–4194304. Enforced for search/describe/runs/view; over-budget replies contain no partial data. MCP's transport cap independently applies. |
| `reporting.viewer.max_rows` | Rows; 500 | 1–1000 per selected table page, not a new source limit. |
| `reporting.viewer.page_rows` | Rows; 100 | 1 through max_rows; default page selection. |
| `reporting.viewer.max_outputs` | Count; 32 | 1–64. Rejects oversized output collections rather than silently dropping outputs. |
| `reporting.viewer.max_points` | Count; 5000 | 1–10000 chart points; exact accessible values remain attached. |
| `jobs.timezone_database_version` | String; bundled Go 1.26.4 archive SHA-256 coordinate | Empty normalizes to bundled; any other version is rejected. Persistence protects mixed-replica calendar behavior. No host-zonefile lookup or silent database-version replacement. |

The exact archive coordinate is
`go1.26.4-sha256:8f55634d05f8bca1f7bc7c69c5933428c69357e0bdf565e5ba224e3f88ff12e8`.
The archive license and verification live in `internal/calendars`. Timezone
rollouts require reviewed migration/deployment changes, not editing accepted
occurrences. Tests cover pinned archive integrity, changed host environment,
wrong version, invalid names, DST gaps/folds, leap dates and first windows.

Each reporting target's budget is request intent constrained again by deployment
limits: timeout_ms 1000–60000, max_rows 1–10000, max_bytes 1024–4194304,
query_attempts 1–800, model_calls 0–64, model_tokens 0–16777216. Zero calls requires
zero tokens; enabled calls require at least 64 reserved tokens. Deterministic
frozen requests leave dynamic/narrative false and model ceilings zero. Reservations
are durable pre-call ceilings, not invented usage/cost observations.

`TestReportingViewerConfigurationBounds`, `TestReportingDeliveryOperatorExcerpt`,
calendar/config tests and phase30/31 acceptance cover positive and negative
settings, non-secret examples, actual payload caps and runtime attempts. Existing
jobs/reader/gateway deadlines, tenant quotas and signed authority are not enlarged.
''')
replace('docs/configuration.md','| `mcp.groups` | string array | discovery, query, byo, charts | 1–4 distinct implemented group names.','| `mcp.groups` | string array | discovery, query, byo, charts, reporting | 1–5 distinct implemented group names.')
append('docs/contracts/execution-authority-v1.md','''## Reporting consumer extension — phase 30

Reporting uses the same v1 Basic-authenticated trusted broker adapter, body,
short-lived signed execution proof and jobs audience. No target-scope or identity
fields are added to the exchange request. The accepted local kind is
`reporting.scheduled`; target-specific publication/dependency pins are part of the
immutable manifest hash. The issuer must independently approve binding-use,
`reporting.execute`, exact run execution and all actual block/report/source/topic/
dataset/context reach; dynamic queries additionally need their ordinary query
actions. The consumer never derives these permissions from a creator or recipient.

The producer's deployment/policy must support that approved scope set before
reporting dispatch is enabled. Chartworks fixture-backed tests do not claim to have
provisioned production Pengui bindings or changed its issuer configuration.
`TestPhase30/AC01`–`AC08` exercises the real adapter, verifier, queue and reporting
paths, including wrong manifest/binding/audience/service, expiry, refusal, narrowed
context, retries and zero protected work on denial. Metadata-only catalog reads
are independent of that broker. The [delivery contract](reporting-delivery-v1.md)
describes the four target representations and distinct effect/receipt states.
''')
append('docs/reporting/delivery.md','''## Implemented phase-30/31 runtime boundary

The executable schedule and Apps consumer is now documented in
[reporting delivery v1](../contracts/reporting-delivery-v1.md). That guide separates
implemented retained-catalog delivery and the actual bundled read viewer from
this design's later phase-32 SSR/BFF/export work. No notification-sent or production
cutover claim follows from a catalog artifact or browser rendering test.
''')
append('GETTING-STARTED.md','''## Reporting schedules and the Apps read viewer

Use [reporting delivery v1](docs/contracts/reporting-delivery-v1.md) for the exact
operator setup, four governed schedule targets, real test/pause/update/retire
semantics and the five shared HTTP/MCP/SDK operations. Merge
`examples/chartworks.reporting-delivery.json` into the existing trusted deployment;
it contains no credentials and does not enable workers. Configure the actual
Pengui execution binding before enabling reporting dispatch. Retained viewing
needs neither that worker nor a model provider.

Browser acceptance requires Node 22+ and an installed Chrome/Chromium executable;
`CHARTWORKS_CHROME_BIN` may select an existing binary. The acceptance harness
executes the bundled resource rather than a DOM substitute. Run both strict
owners with the existing database/native fixtures:

```bash
python3 scripts/run_phase_acceptance.py --phase 30
python3 scripts/run_phase_acceptance.py --phase 31
```

A returned catalog artifact does not prove an outbound notification was sent.
Catalog pull is the implemented baseline; static export/iframe rendering remains
with phase 32.
''')
write('docs/reviews/phase-30-31-adversarial.md','''# Phase 30/31 adversarial review and verification ledger

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
''')
# Check every newly written local documentation link against this exact checkout.
import re
for path in [p for p in changed if p.endswith('.md')]:
    for link in re.findall(r'\]\(([^)]+)\)',Path(path).read_text()):
        target=link.split('#')[0]
        if not target or '://' in target:continue
        if not (Path(path).parent/target).exists():raise RuntimeError(f'Missing documentation target: {path}: {target}')
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['git','diff','--check'],check=True)
