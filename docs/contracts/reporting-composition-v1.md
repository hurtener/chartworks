# Report/dashboard composition v1

Phase 29 implements definitions and retained composition over the existing block,
NLQ and request-operation services. Pengui remains the sole issuer and policy
owner. No new source driver, model client, SQL parser or general work queue is
introduced. The canonical operation registry is the HTTP/OpenAPI/SDK authority.

## Definitions, review and publication

`POST /v1/reports` and `POST /v1/dashboards` accept `{id, definition}` and create
private revision 1. `PUT /v1/{reports|dashboards}/{id}` accepts `expected_version`,
`from` and `definition`; it creates a new immutable revision. Review, publish,
reject and archive are separate POST suffixes with `expected_version`, `revision`
and `note`. Use each returned version for the next compare-and-swap operation.
Publish/reject require `reporting.publish`; create/edit/review/archive require
`reporting.write`, plus corresponding signed target resource reach. A pending
review, newer draft and older publication retain independent pointers.

`GET /v1/{reports|dashboards}/{id}` defaults to the published revision. `revision`
selects an exact version; `stage` selects a lifecycle pointer, but both cannot be
combined. Private definition reads require preview authority even for the
creator. `GET /v1/{reports|dashboards}?after=...&limit=...` lists only authorized,
unarchived, published metadata; the limit is 1–100. It does not execute widgets.
Dashboard page redaction occurs in SQL before hidden names/content leave storage;
authorized root access can legitimately return zero visible pages.

Reports contain the closed `block`, `query` or `text` widget union on a bounded
12-column grid. Block widgets reference an exact revision or a floating published
pointer and an explicit saved-output subset. A query widget carries exact semantic
pins, execution context and replayable intent or a private originating-session
reference. Saved routing selections are business choices, not authority. Text is
inert plain/restricted Markdown, not HTML, script or a renderer extension.
Dashboards contain ordered page IDs/titles and **exact published report revisions**.

Legacy section-format imports preserve their original immutable bytes and expose
a canonical projection. External source/object/version coordinates identify an
import; they grant nothing. Unsupported versions are private quarantine records,
not executable or discoverable reports.

## Execution and results

`POST /v1/{reports|dashboards}/{id}/runs` reserves a body `key` before resolving
floating inputs. The first successful seal pins references, resolved parameters,
timezone/instant, context partitions, output unions and privacy. Explicit replay
with the same key and input returns that seal; changed input conflicts.

`POST /v1/composition-runs/{id}/execute` accepts `{resume:false}` or explicit
`{resume:true}`. Shared block widgets execute one query only when revision,
parameters, source/context, policy, privacy, locale and resolution semantics agree.
The child executes the union of selected outputs; each widget retains its subset.
An unavailable optional narrative cannot erase a successful table in partial mode.
Opt-in and full declared model-call/token budgets remain mandatory. Strict reports
record failure and do not expose complete-looking retained widget payloads.

Dynamic execution defaults **off**. Enable `reporting.composition.live_queries`
explicitly and `session_bound` separately where needed. Configuration does not
supply `query.plan`, `query.execute`, topic/source/dataset/context or target reach.
A report publication cannot certify dynamic SQL or impersonate a saved session.
A durable generation-start marker prevents automatic regeneration after an
uncertain response. Existing saved plan evidence can be recovered; missing
plan evidence remains explicitly indeterminate, with no replacement model call.

`GET /v1/composition-runs/{id}` returns bounded metadata, not SQL, rows, narrative
text or raw parameter values. `GET /v1/composition-runs/{id}/receipt` is the
actor/session-private execution summary. `GET /v1/composition-runs/{id}/widget`
requires `page` and `widget` query coordinates and returns only the addressed,
currently visible retained subset. Current artifact/page/context reach is checked
before values are loaded. Retained reads do not invoke sources or models; private
previews stay private after later publication.

`POST /v1/composition-runs/{id}/cancel` requires `jobs.cancel` and actual target
reach. It records durable intent and propagates cancellation to owned read
attempts, without asserting that native database execution has already stopped.

`POST /v1/composition-retention` accepts `{limit:1..100}` and requires both
`reporting.retention` and `cw.tenant.erase:<signed-tenant>`. It atomically erases
expired composition-owned manifests, group results and static widget payloads,
releases retention bytes, and appends audit. It preserves content-free tombstone
identity/page/reach metadata. A failed audit rolls back erasure. Child frozen runs
have their own retention ownership; parent expiry is not a cross-owner purge.
Expired reads never silently regenerate values.

## Operational limits

All fields below are under `reporting.composition`; none is secret or grants access.

| Setting | Default | Accepted bound |
|---|---:|---|
| `max_widgets`, `max_filters`, `max_pages` | 100 each | 1–100 each |
| `max_text_bytes` | 32768 bytes | 1–131072 |
| `max_definition_bytes` | 1048576 bytes | At least `max_text_bytes`, at most 2097152 |
| `max_queries` | 16 groups | 1–100 |
| `max_retained_bytes` | 8388608 bytes | 1024–16777216 |
| `timeout` | `60s` | `1s`–`60s` |
| `live_queries`, `session_bound` | false | `session_bound` requires `live_queries` |
| `partial_failure` | `fail_closed` | `fail_closed` or `allow_partial` |

The existing `reporting.execution` settings also govern row/result bounds, optional
narrative budgets, shared tenant artifact quotas and retention. Report/dashboard
counts and revision ceilings use existing `reporting.max_blocks` and
`reporting.max_revisions` respectively. Lowering authoring limits does not rewrite
or invalidate already retained immutable metadata.

## Example and client flow

Use [report-create.json](../../examples/report-create.json) for a text-only draft
that needs no warehouse/model configuration and
[dashboard-create.json](../../examples/dashboard-create.json) after publishing
its referenced report revision. They are synthetic request fixtures.

Given an existing SDK client with a caller-supplied Pengui token provider, use
`CreateDocument`, `TransitionDocument` (review, then publish), `AdmitComposition`,
`ExecuteComposition`, `ReadComposition`, and `ReadCompositionWidget`. Always carry
the returned identity/revision/version into the next call; do not retry mutations
implicitly or manufacture credentials in the example. See
[GETTING-STARTED.md](../../GETTING-STARTED.md) for the existing client and server setup.

Reports/dashboards are API-first. Phase 29 does not provide drag-and-drop authoring,
report schedules, the MCP Apps viewer, static chart rendering or export formats.
Those remain owned by their later phases.
