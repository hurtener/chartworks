# MCP service contract — phase 22

Chartworks exposes the installed analytical services at **POST `/v1/mcp`** on the
ordinary server port. The adapter uses the ecosystem's pinned
`github.com/modelcontextprotocol/go-sdk v1.6.1`; its source, not an independently
implemented protocol stack, owns initialization and MCP message dispatch.
[Phase 22](../plans/phase-22-mcp-server.md) owns this surface. The reporting Apps
viewer remains phase 31. This implementation does not requalify Harbor/Pengui
host compatibility or introduce a credential issuer.

## Authority and deployment

Enable `features.mcp` in the existing server configuration. Both `chartworks serve`
and `chartworks mcp` use the same composition root and listener; the latter requires
MCP to be enabled. There is no stdio authentication mode, second listener, token
exchange, local API key, cookie identity, or token-in-URL fallback.

Every request requires a **fresh Pengui bearer** verified for the configured MCP
intended audience and the signed `mcp.use` action. A tool additionally requires its
registered domain action and all resource/session/dependency restrictions enforced
by the ordinary service. Use distinct `auth.audiences.http` and `.mcp` values to
prevent cross-surface replay; the pre-existing explicit same-audience shorthand
continues to mean exactly what the operator configured. The outer phase-21 guard
uses the mount's MCP audience, not the HTTP audience. There is no special authority
for initialization, discovery, a transport session ID or an in-process caller.

`mcp.allowed_hosts` lists exact destination host names/IPs, without ports or
wildcards. The port in an HTTP Host must be valid, but does not create authority.
Forwarding headers cannot override that check. An Origin, when present, must
exactly match `server.cors_allowlist`; an empty list rejects all requests carrying
an Origin, including same-origin browser requests. Ordinary server CORS behavior
still applies outside this stricter MCP boundary. Deploy behind the existing TLS
proxy with explicit host/origin configuration. These network settings never grant
analytical access. See the [configuration excerpt](../../examples/chartworks.mcp.json)
and [Pengui registration guide](pengui-provider-registration.md).

## Real tool inventory

Only installed services in enabled groups are registered. `tools/list` filters
this metadata by the caller's current domain actions. Resource restrictions are
checked again at invocation, not inferred from tool visibility. A production
composition with all implemented services has eighteen bindings:

| Group | MCP tool | Existing HTTP operation ID | Signed domain action |
|---|---|---|---|
| discovery | `list_sources` | `listSources` | `sources.read` |
| discovery | `list_datasets` | `listDatasets` | `sources.read` |
| discovery | `describe_dataset` | `describeDataset` | `sources.read` |
| discovery | `list_topics` | `listTopics` | `topics.read` |
| discovery | `describe_topic` | `getPublishedTopic` | `topics.read` |
| query | `preflight_question` | `preflightNLQ` | `query.preflight` |
| query | `plan_question` | `planNLQ` | `query.plan` |
| query | `run_question` | `runNLQ` | `query.execute` |
| query | `refine_question` | `refineNLQ` | `query.execute` |
| query | `submit_feedback` | `feedbackNLQ` | `feedback.write` |
| byo | `get_query_context` | `getQueryContext` | `query.context` |
| byo | `read_query_context` | `readQueryContext` | `query.context` |
| byo | `submit_sql` | `submitSQL` | `query.submit` |
| charts | `chart_catalog` | `chartCatalog` | `charts.read` |
| charts | `select_chart` | `selectChart` | `charts.select` |
| charts | `specify_chart` | `specifyChart` | `charts.bind` |
| charts | `build_chart` | `buildChart` | `charts.bind` |
| charts | `rebind_chart` | `rebindChart` | `charts.bind` |

The eleven established contracts are list/describe topics and datasets;
preflight/plan/run/refine question; get context/submit SQL; and submit feedback.
Source listing, retained context lookup and chart specification bindings are real
additional consumers, not substitutes for those eleven. Context creation is
absent when its routing service is unavailable. No reporting, scheduling, admin,
renderer, shell or unimplemented tool is advertised.

`Bind` requires an existing HTTP registration with a closed request schema,
matching typed response, action, known effect, error inventory and audit semantics.
POST argument schemas must agree with their HTTP DTOs; GET bindings use a typed
argument projection for path coordinates. Unknown effect classifications, duplicate
tool names/operation IDs and ambiguous resource templates fail construction.
`tools/list` and `Registry.Manifest()` are the generated source of truth; there is
no manually maintained second business registry. Registration and acceptance tests
compare all current bindings to the actual HTTP definitions.

## Effects and outcomes

MCP annotations and `chartworks/*` metadata describe actual work, not merely
whether a source SQL statement is read-only. Retained metadata, context lookup and
deterministic caller-data transformations are read-only and idempotent. Optional
chart ranking may spend remote-model budget. Question preflight **persists session
evidence and may invoke routing models**; planning/refinement, execution, feedback
and BYO operations also carry conservative persisted/paid annotations. Feedback
with corrected SQL may perform native validation. No analytical mutation is
advertised as harmless simply because it cannot modify the warehouse.

Tool results have matching text and structured content. Success is
`{"result": <typed domain result>}`. A tool failure sets `isError` and contains
`{"error":{"code":"...","outcome":"not_started|unknown"}}`, with an optional
bounded owner-supplied usage receipt. `not_started` means this invocation was
rejected before entering the domain service. `unknown` conservatively means it
entered the service and may have incurred cost or persisted state; it is not a
claim of rollback. Domain receipts and BYO idempotency still govern replay. Retrying
an accepted BYO step returns its original content-free receipt without executing
SQL again or inventing retained values.

Resource/protocol errors expose fixed codes only. Native driver messages, panic
values, stack traces, bearer bytes and foreign metadata cannot enter these errors.
An unexpected mapper code is replaced with `unavailable`. Panic, cancellation,
expiry, malformed output and response-limit paths do not fabricate success or
silently retry operations. The transport SDK receives only protocol headers;
the original bearer is not copied into its request extras or stored as a session.

## Metadata resources

Three resource bindings invoke the same pure service adapters as their tools:

| URI or template | Tool |
|---|---|
| `chartworks://charts/catalog` | `chart_catalog` |
| `chartworks://topics/{topic}` | `describe_topic` |
| `chartworks://datasets/{source}/{context}/{dataset}` | `describe_dataset` |

`resources/list` returns the catalog resource; `resources/templates/list` returns
the two templates, filtered by current actions. A read returns JSON with the same
result wrapper as the tool. Canonical IDs only: no credentials, query strings,
fragments, percent aliases, traversal, remote URLs or ad-hoc source connections.
Every actual topic/dataset read checks current tenant and dependency/context reach.
Topic reads expose immutable publication DTOs, never private draft payloads.
These are retained metadata, not live source-health promises, analytical artifacts,
HTML Apps resources or rendered charts. Reads make no source/model call.

## Transport profile, limits and clients

The Streamable HTTP deployment is **stateless, JSON-response only**. POST clients
advertise both `application/json` and `text/event-stream` in Accept, as required by
the SDK transport. Responses use JSON; initialized notifications return 202 with
no body. GET returns 405 rather than opening SSE. Session/resume headers are
rejected; transport state neither retains analytical runs nor authorizes requests.
The SDK negotiates supported protocol versions; subsequent requests supply the
negotiated `Mcp-Protocol-Version`. The public Go adapter uses `2025-11-25`; omitted
headers follow the SDK's `2025-03-26` fallback. The package is pinned, not upgraded
as a host compatibility prerequisite.

Supported messages are initialize, initialized notification, ping, tools list/call,
resources list/templates/read. Requests are single closed JSON-RPC objects, not
batches. IDs are bounded nonempty safe strings or nonnegative exact integers;
Accept is limited to 1 KiB across all its header values; encoding headers are not
accepted. Duplicate JSON fields, invalid UTF-8, extra envelope fields and malformed input
are rejected. Every tool's arguments receive their own closed schema validation.

Admission occurs before body reading: default sixteen concurrent requests, no
unbounded queue, and 429 when full. Default request/response bounds are 10/16 MiB;
text and structured output both count. The call deadline is the earliest of caller
cancellation, current verified authority expiry and the configured 65-second limit.
The adapter restores the original request context inside the SDK and retains no
ambient token. This is not durable job cancellation: already committed domain
work is reconciled through its normal receipts and control operations.

`internal/mcpserver.Client` is an in-process adapter requiring a fresh caller token
provider; it still verifies the MCP audience and uses the same admission/dispatch.
`sdk/chartworks.Client.MCP` sends bounded JSON-RPC over HTTP with a freshly obtained
caller bearer for each message, preserves tool-level errors for inspection and does
not retry. The public SDK's three typed discovery methods use ordinary HTTP
intended authority. Complete SDK/CLI product parity remains phase 23, not a second
scope or credential path added here.

## Verification

`TestPhase22/AC01`–`AC06` cover the eleven real operations, eighteen binding
contracts, resource reads, action/audience/tenant/session/context isolation,
provider/source work boundaries, exact output schemas, BYO replay and shared-client
concurrency. Fixtures use real PostgreSQL/pgvector, the pinned native validator/read
executor and recorded provider responses through the actual Bifrost SDK.
`TestPhase21` includes the new discovery routes and MCP-audience mount in cumulative
registration/denial tests. Package adversarial tests cover malformed requests,
URI collisions, panic redaction, output bounds, capacity, cancellation and expiry.
The read-only MCP workflow validates all twelve named criteria and fuzzes the
protocol; full CI separately runs the complete race/coverage and native/container
suite. The [adversarial record](../reviews/phase-22-adversarial.md) records actual
execution evidence, not live-cloud, independent human-review or host qualification.
