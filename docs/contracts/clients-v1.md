# Chartworks clients v1

Status: implemented for phase-23 review. [D-070](../decisions/2026-09-09-client-parity.md)
and [Pengui authority](pengui-authority.md) apply. This contract adds consumers of
real services, not another query executor, credential issuer or business store.

## Typed HTTP and verified in-process calls

Use the existing `sdk/chartworks.Client` typed domain methods for application code.
`New(baseURL, httpClient, TokenProvider)` pins the backend origin and optional
configured mount. HTTPS is required except for an explicit loopback IP over HTTP.
URLs cannot contain credentials, queries, fragments, encoded path aliases or dot
segments. Redirects and cookie jars are disabled on a copy of the HTTP client.

The provider supplies current **Pengui HTTP-audience** authority for each request.
It may delegate renewal to the application's existing Pengui integration; the SDK
never signs, exchanges or extends a token, caches a bearer between requests, or
silently retries a 401. The provider must be concurrency-safe, honor its context
and preserve the logical caller's identity and intended authority across a retry.
Its raw errors are not exposed. Separate MCP calls use a separately supplied
MCP-audience token with `mcp.use` and the applicable domain scopes.

```go
// Inside application code. currentPenguiToken is your existing integration's
// context-aware callback, not a token value or a Chartworks exchange endpoint.
client, err := chartworks.New(
    "https://analytics.example/warehouse", nil, currentPenguiToken,
)
if err != nil { return err }
topics, err := client.ListTopics(ctx, chartworks.TopicListRequest{Limit: 20})
if err != nil { return err }
// Consume topics as typed data; the service already enforced signed reach.
_ = topics
```

`NewInProcess` and `NewInProcessWithOptions` run those same client methods through
an application's **production authenticated HTTP handler**, synchronously and
without a listener. `InProcessOptions.BasePath` must match the server; the empty
value selects `/`. No envelope, tenant, user, role or source-secret constructor is
exposed. Ambient context values are discarded at the simulated wire boundary;
deadlines and cancellation remain. The token provider receives the original
caller context before the wire boundary. The handler still verifies the fresh
bearer and resolves every signed resource restriction. Supplying an unprotected
handler or a malicious custom transport is not a supported authentication setup.

The synchronous adapter waits for the context-aware handler to finish; it never
spawns detached mutation work. It buffers at most 32 MiB, preserves committed
response headers, rejects protocol upgrades and implements HEAD/bodyless statuses.
It is not an SSE streaming implementation. The established network MCP endpoint
remains stateless bounded JSON; durable work is inspected and explicitly cancelled
through its existing operation endpoints, not by pretending a client disconnect
proves server termination.

## One installed-operation matrix

`Operations(ctx)` parses the running server's generated `/openapi.json`.
`ParseOperations(document)` accepts the same exported document offline.
`OperationMatrix(ctx, mcpClient)` optionally joins the separately authorized
`tools/list` result by exact operation ID/action/effect/audit. No MCP client means
MCP was not queried; an empty MCP column is not a compatibility verdict.

Each row includes method/path, action, resource loader, audit/effect, replay
classification, body bound, parameter and request/response schemas, the concrete
SDK dispatch method and CLI command. Every HTTP operation is represented;
MCP's protocol mount is explicitly dispatched by `MCP`, not generic HTTP `Invoke`.
The matrix describes installed contracts, **not resource access permission**.
Later reporting/domain owners extend registration and their tests in their own
change. No report, artifact, rendition or identity-management operation is
invented merely because its phase is planned.

`Invoke(ctx, operationID, CallOptions)` obtains the current contract; it accepts no
arbitrary URL or authorization headers and does not trust editable returned rows.
Path and query coordinates, required keys and closed JSON bodies are checked
against that contract. `application/octet-stream` operations receive bounded raw
bytes. Requests cannot select a tenant/user outside their verified bearer.
Responses retain exact bytes and numerical spelling; generic JSON results are
validated against the actual output schema. Metadata is not persisted or cached
as another authoritative entity store.

## Replay and cancellation

OpenAPI publishes `x-chartworks-replay`, derived from `Definition.Replay`:

| Value | Meaning |
| --- | --- |
| `never` | Default for every mutation and unknown contract; no SDK retry loop. |
| `read` | Registered GET/HEAD only; eligible for explicitly requested bounded retries. |
| `keyed` | Explicit owner classification plus a required bounded Idempotency-Key header and an existing deduplicated service implementation. |

A header alone is not replay proof. The currently classified keyed operations are
`sweepRetention`, `submitJob`, `createSchedule` and `fireSchedule`. Their existing
service ledgers remain authoritative. Query/model calls with operation IDs in their
body do **not** gain automatic retry eligibility. Older documents without this
extension remain non-replayable for mutations.

The default is one attempt. `CallOptions.Attempts` may explicitly request two or
three attempts for an eligible operation. Only HTTP 429, 502, 503 and 504 can enter
that loop, with bounded 100/200 ms delay and the earlier caller deadline. The SDK
freezes path, body and logical key before the first attempt and reacquires a token
only from the supplied provider. Authorization failures, 404/409/410, other errors,
ambiguous transport failures and cancellation do not enter the loop. Explicit
replay is not external exactly-once execution or a guarantee of retained values.

Mutation requests do not expose a rewind callback to Go's default HTTP transport;
this prevents the standard keyed-body retry mechanism from independently replaying
them outside the explicit loop. Standard HTTP transports can still transparently
retry certain safe reads, and custom transports/providers are caller-owned. The
SDK's attempt option is not a universal count of all physical network transmissions.

`errors.Is(err, context.Canceled/DeadlineExceeded)` survives credential acquisition,
network/body reads and retry delays. `StatusError` retains HTTP status and the
existing bounded owner usage receipt where defined; its string never contains the
server body, token or native diagnostic. A rejected/expired artifact or context
reference never turns into a new warehouse or model execution.

## Operator CLI

The binary preserves `version`, `config-check`, `serve` and `mcp` server commands.
Client operations are under `chartworks client` and do not initialize stores,
workers, source pools, model clients or a listener.

| Command | Behavior |
| --- | --- |
| `client config` | Safe resolved URL/timeout/token-source category only; no connection or token read. |
| `client operations [--schemas] [--mcp-token-env NAME]` | Generated installed-operation matrix; optional separately authorized MCP join. |
| `client diagnostics` | Real read-only `ops.inspect` diagnostics, not local policy simulation. |
| `client call OPERATION --execute` | Invoke one registered HTTP operation with explicit effect acknowledgement. |
| `client mcp --execute --input -` | Bounded raw MCP JSON-RPC; tool/protocol failures do not return a success exit merely because HTTP was 200. |

`--url` or `CHARTWORKS_CLIENT_URL` supplies the trusted backend and matching mount.
`--timeout` or `CHARTWORKS_CLIENT_TIMEOUT` defaults to 75 seconds; the positive
maximum is 15 minutes. Flags override environment configuration. An earlier caller
context always wins. SDK zero HTTP timeout selects the bounded default. Cooperative
provider callbacks and injected readers/handlers must honor cancellation; the CLI
closes closable stdin on cancellation and joins that cleanup callback.

A caller-provided environment source defaults to `CHARTWORKS_TOKEN`; `--token-env`
selects another environment **name**, not a literal bearer. Alternatively,
`--token-fd N` consumes an explicitly inherited regular-file descriptor 3–1024,
re-reading at most 64 KiB per call and closing it when the command ends. Pipes,
sockets and stdin/stdout/stderr descriptors are rejected. An explicitly selected
environment and descriptor source are mutually exclusive. Token values never
appear in printed configuration or errors and are not persisted by this program.
A launcher may provide the descriptor from its secret store without putting the
bearer in command history. Do not commit token files or enable shell tracing.

```bash
# The application's existing Pengui integration supplies CHARTWORKS_TOKEN.
# These example names/values are synthetic and contain no bearer value.
chartworks client operations --url https://analytics.example/warehouse
chartworks client diagnostics --url https://analytics.example/warehouse

# Pure topic discovery still requires explicit acknowledgement for generic calls.
printf '%s\n' '{"limit":20,"after":""}' |
  chartworks client call listTopics --url https://analytics.example/warehouse \
    --execute --input -

# A sweep is a real, bounded destructive maintenance request. The provided token
# must have ops.maintain plus exact tenant erasure reach. Reuse this same key for
# the same logical request; changing it requests a different effect.
chartworks client call sweepRetention --url https://analytics.example/warehouse \
  --execute --idempotency-key synthetic-sweep-001 --attempts 2

# The launcher has inherited a regular-file descriptor containing current
# Pengui authority; this command does not accept a token value as an argument.
chartworks client diagnostics --url https://analytics.example/warehouse --token-fd 3
```

Bodies are read from `--input -` with the registered byte bound. `--id` supplies
one registered path identifier; repeated `--query NAME=VALUE` supplies distinct
registered query names, never an arbitrary header. JSON, SQL and bearer values
are not accepted through dedicated command-line flags. `--execute` acknowledges
possible persistence/spend/erasure but grants no authority. Inspection is distinct
from invocation, and absence of an endpoint is an error rather than a placeholder.

Exit statuses are 0 success, 1 dependency/output/protocol failure, 2 invalid
invocation/unknown operation/unsafe retry, 3 HTTP authority denial, 4 conflict or
expired object, 124 deadline and 130 cancellation. Authorized result bytes go to
stdout; safe diagnostics go to stderr. Short writes fail. MCP tool faults retain
their bounded owner outcome/receipt; raw protocol diagnostics are not echoed.

## Bounds and evidence

The metadata document is at most 4 MiB and 256 operations. Per-operation input
byte/schema limits are unchanged. Generic and in-process output is at most 32 MiB;
typed calls retain their original lower bounds. Output-only decoding supports
100,000 collection items to match the configured read row ceiling. The original
65,536-item model/request decoder, 32 nesting levels, duplicate-key rejection,
finite numeric validation and exact number representation remain unchanged.

`TestPhase23/AC01`–`AC06` and the cumulative phase-21/22 tests exercise real
PostgreSQL, native read validation/execution, topic publication and recorded model
providers. Unit, fuzz and race tests exercise malformed catalogs, replay drift,
provider concurrency, ambient authority, cancellation, descriptor handling and
output bounds. Recorded provider fixtures do not establish live-provider quality;
reporting/rendering and the final release gate remain owned by later phases.
