from pathlib import Path
import hashlib
r = Path('.')
def edit(path, old, new):
    p = r/path
    s = p.read_text()
    assert old in s, path
    p.write_text(s.replace(old,new))
def append(path,text):
    p=r/path
    p.write_text(p.read_text().rstrip()+'\n\n'+text.strip()+'\n')
p=r/'README.md'; s=p.read_text(); start=s.index('**Merged baseline:'); stop=s.index('| Capability |',start)
s=s[:start]+'''**Merged baseline: phases 01–21 are implemented** — 21 of 34 workstreams and
130 named acceptance criteria. PR #13 merged the phase-20 output specifications
and cumulative phase-21 HTTP hardening at
`2fa80a404518e59db5d6157b50a7521aa9c1512f`.

**Phase 22 is implemented and under final verification in PR #14.** It adds a
bounded Pengui-authenticated MCP transport, eighteen real tools and three metadata
resource bindings, plus cumulative HTTP/Go SDK discovery. Its six named criteria
exercise the eleven established core operations through actual services; the
[adversarial and verification record](docs/reviews/phase-22-adversarial.md) separates
local checks from full CI evidence. No release or deployment is implied.

**Twelve workstreams remain planned: 23–34**, including the phase-25 final release
gate. There are 224 acceptance criteria across the full plan. Recorded cloud and
model fixtures do not constitute live-provider or production-cutover qualification.

'''+s[stop:]
s=s.replace('| Output specifications — this change |','| Output specifications |')
s=s.replace('A runtime guard rejects unregistered paths before handler dispatch. |','A runtime guard rejects unregistered paths before handler dispatch and selects the declared HTTP/MCP intended audience. |\n| MCP — phase 22 | Eighteen installed-service tools, the eleven core contracts, three pure metadata resource bindings, fresh per-request authority, closed schemas, bounded work and explicit effects. No Apps viewer or local credentials. |')
s=s.replace('The later Chartworks MCP tools, retained-artifact viewer and exports are implementation work, not a reason to reopen host compatibility.','Chartworks MCP tools use that established ecosystem; the retained-artifact viewer and exports remain later implementation work, not a reason to reopen host compatibility.')
s=s.replace('Still planned: the full MCP/CLI surface,','Still planned: full SDK/CLI parity,')
s=s.replace('No merge, deployment or full replacement qualification is implied by this branch.','Phase 22 adds its own [MCP contract](docs/contracts/mcp-v1.md) and [review record](docs/reviews/phase-22-adversarial.md); no deployment or full replacement qualification is implied.')
section='''## MCP tools and metadata resources

Enable `features.mcp` to mount **POST `/v1/mcp`** on the existing server. The
[configuration excerpt](examples/chartworks.mcp.json) and [MCP contract](docs/contracts/mcp-v1.md)
define exact hosts/origins, request/response/concurrency limits and operator setup.
Each request requires a fresh Pengui bearer for the MCP intended audience and
`mcp.use`; each operation then enforces its ordinary domain action and exact reach.
The transport is stateless and returns JSON, with no bearer persistence or separate
credential channel. HTTP and in-process clients use the same guarded dispatch.

The eleven core tools list/describe topics and datasets; preflight/plan/run/refine
questions; obtain context/submit SQL; and submit feedback. Source listing, retained
context lookup and five chart-specification tools bring the installed inventory to
eighteen. Paid or persisted calls are annotated accordingly: preflight is **not** a
pure metadata read. Disabled or unbuilt services are absent, never success stubs.

The chart catalog, published topic and registered dataset metadata can also be read
as three resource bindings through the same services, without source/model calls.
These are JSON metadata, not rendered Apps or retained reporting artifacts.
Phase 21 and the typed Go SDK additionally expose `POST /v1/topics/list`,
`POST /v1/datasets/list` and `POST /v1/datasets/describe`. Their database queries apply
all signed dependency/context restrictions before pagination, not after broad reads.

'''
s=s.replace('## Portable output specifications\n',section+'## Portable output specifications\n');p.write_text(s)
p=r/'docs/plans/README.md';s=p.read_text();start=s.index('Current merged baseline:');end=s.index('## Fixed decisions',start)
s=s[:start]+'''Current merged baseline: phases **01–21 are shipped** (21 workstreams,
130 implemented acceptance criteria). PR #13 merged phase 20 and cumulative
phase-21 HTTP hardening at `2fa80a404518e59db5d6157b50a7521aa9c1512f`.
Phase 22 now implements six additional criteria, eighteen real MCP bindings and
three metadata resources, with HTTP/SDK discovery extensions. It remains
`in_progress` until final verification/review closes in PR #14; twelve workstreams
(23–34) remain planned, including the phase-25 final release gate.

The [phase-22 review](../reviews/phase-22-adversarial.md) records findings and actual
execution evidence. The registry is a status ledger; real named tests and
exact-source CI establish acceptance, not a green documentation check. Historical
superseded plans remain under `docs/archive/phase0-plans/`.

'''+s[end:]
s=s.replace('Phases01–19 and21 are shipped; phase20 is implemented in review, leaving thirteen planned workstreams.','Phases01–21 are shipped; phase22 is implemented under verification, leaving twelve planned workstreams.');p.write_text(s)
edit('docs/plans/phase-20-charts-spec.md','The implementation is in review, not a merged release or rendered-pixel claim.','The implementation merged in PR #13 at `2fa80a404518e59db5d6157b50a7521aa9c1512f`.\nThis is a specification capability, not a rendered-pixel or full-release claim.')
edit('docs/plans/phase-20-charts-spec.md','readiness. The phase remains `in_progress` pending review/merge. Renderer and full','readiness. The phase is shipped following PR #13; its current criteria also run\nin cumulative CI. Renderer and full')
append('docs/plans/phase-21-http-api.md','''## Phase-22 cumulative consumers, 2026-09-09

Three retained-metadata operations (`listTopics`, `listDatasets`, `describeDataset`)
now share the ordinary protected registry, DTO schemas and Go SDK with their MCP
bindings. Signed topic and every dependency restriction are applied before SQL
pagination; dataset access names its exact source/context. These operations neither
probe a warehouse nor expose private drafts. The checked-in source/topic manifests
and exhaustive registry tests include them.

`POST /v1/mcp` is a registered protected transport, not a guard exception.
`Definition.Surface` selects the current MCP intended audience and `mcp.use` at
the composition boundary; domain dispatch still requires its own action/resources.
OpenAPI describes that audience, bounded JSON-RPC request/response and empty 202
notification success. The bodyless 405 contract remains unchanged. NLQ preflight's
existing effect is corrected to paid routing plus persisted session evidence.

`TestPhase21` reruns cumulative denial/registration coverage, while `TestPhase22`
exercises the real HTTP/MCP/SDK services and per-request authority. The
[phase-22 review](../reviews/phase-22-adversarial.md) records evidence. No reporting,
render/export, Apps-viewer or credential-issuance placeholder is added.''')
append('docs/plans/phase-22-mcp-server.md','''## Implemented continuation, 2026-09-09

`internal/mcpserver` supplies one immutable typed binding registry, bounded
stateless Streamable HTTP mount and verified in-process adapter. Source, topic,
NLQ, BYO and chart owners bind their actual services to shared phase-21 schemas,
actions, error inventories and effect/audit metadata. All eleven core contracts
have real consumers; source listing, retained context lookup and five chart
operations bring the all-services inventory to eighteen. Three metadata resource
bindings invoke those same pure services. No reporting Apps resource is claimed.

Configuration defaults MCP off and bounds groups, hosts, request/response bytes,
concurrency and timeout. The request deadline/cancellation and verified envelope
are preserved through the SDK; transport sessions never carry analytical authority.
The same listener is used by `serve` and the explicit MCP-enabled `mcp` command.
Fresh caller-supplied tokens are required by HTTP and in-process clients.

`TestPhase22/AC01`–`AC06` are implemented with real PostgreSQL/pgvector, native
validation/read execution and recorded Bifrost provider responses. They exercise
all eleven contracts, binding parity/effects, three resource reads, private/foreign
reach denials, replay and concurrent caller isolation. Additional package tests
cover malformed envelopes, ambiguous resource templates, protocol/panic redaction,
expiry/cancellation, output bounds and overload; `FuzzMCPBoundaries` covers the parse
surface. The read-only MCP workflow enforces all six phase-22 and six cumulative
phase-21 results. Full CI also retains the complete race/coverage, native/container,
lint and preflight obligations. Status remains in progress pending those results.

See the [MCP v1 contract](../contracts/mcp-v1.md),
[configuration excerpt](../../examples/chartworks.mcp.json),
[operator handoff](../contracts/pengui-provider-registration.md) and
[adversarial record](../reviews/phase-22-adversarial.md). Later domain owners extend
this registry and its tests; phase 23 still owns full SDK/CLI parity, and phase 31
owns the reporting Apps resource/viewer.''')
edit('docs/plans/phase-22-mcp-server.md','D-046 establishes host support; D-050 permits early shell delivery. No runtime completion is claimed.','D-046 establishes host support; D-050 permits early shell delivery. The continuation\nbelow implements Chartworks behavior without changing either decision.')
p=r/'RFC-001-Chartworks.md';s=p.read_text();start=s.index('Status:');end=s.index('\n\nAuthority:',start)
s=s[:start]+'''Status: implementation design, revised 2026-09-09 for merged phases 01–21 and
phase-22 MCP implementation under final verification in PR #14. PR #13 merged at
`2fa80a404518e59db5d6157b50a7521aa9c1512f`. The actionable
[phase ledger](docs/plans/README.md) separates implemented scope from twelve planned
workstreams. [MCP v1](docs/contracts/mcp-v1.md) binds real services and cumulative
HTTP discovery without a new issuer or host qualification. Exact-source CI
establishes readiness; phase 25 and rendering phases 31/32 remain unimplemented.
Recorded cloud/model fixtures do not claim live qualification.'''+s[end:];p.write_text(s)
edit('docs/configuration.md','NLQ, reporting and full MCP transports remain in their owning phases.','NLQ and optional MCP use the same verification core; reporting remains in its owning later phases.')
edit('docs/configuration.md','| `server.write_timeout` | duration string | `30s` |','| `server.write_timeout` | duration string | `1m15s` |')
edit('docs/configuration.md','| `features.mcp` | boolean | false | True rejected until the MCP surface phase is implemented. |','| `features.mcp` | boolean | false | Mounts the real stateless MCP adapter at `/v1/mcp`; no extra listener or credential channel. |')
edit('docs/configuration.md','later reporting, rendering and MCP surfaces remain unavailable until their owning phases.','MCP availability follows its configured installed-service mount; reporting and rendering remain unavailable until their owning phases.')
append('docs/configuration.md','''## MCP shared-port transport (phase 22)

All settings below are non-secret. The [excerpt](../examples/chartworks.mcp.json)
merges into the ordinary operator-owned configuration; it does not create authority.

| Key | Type / units | Default | Bounds and behavior |
|---|---|---|---|
| `mcp.max_request_bytes` | integer bytes | 10 MiB | 1 KiB–10 MiB; bounded before SDK decoding, with non-queued admission before body read. The ordinary server body cap also applies. |
| `mcp.max_response_bytes` | integer bytes | 16 MiB | 16 KiB–32 MiB; text plus structured results and final protocol encoding are bounded. |
| `mcp.max_concurrent` | integer calls | 16 | 1–64; exhaustion returns 429, with no unbounded waiting queue. |
| `mcp.timeout` | duration | `1m5s` | 1–65 seconds; when enabled, strictly shorter than `server.write_timeout`. Caller cancellation and current bearer expiry may shorten it. |
| `mcp.groups` | string array | discovery, query, byo, charts | 1–4 distinct implemented group names. Only installed services are exposed; selecting no real bindings fails startup. |
| `mcp.allowed_hosts` | string array | localhost, 127.0.0.1, ::1 | 1–16 unique canonical lower-case DNS names or IP literals; no wildcard, scheme, port or forwarding-header substitution. |

Every request uses `auth.audiences.mcp` (or the explicitly configured shared
`auth.audience`) and requires `mcp.use`. Tools also require their existing domain
actions and actual session/resource/context reach. MCP rejects query credentials,
cookie-based identity, transport-session/resume headers, compressed bodies and noncanonical URIs.
It returns JSON; GET/SSE is not provided. The original HTTP context is restored
inside the SDK so disconnects and deadlines remain effective.

An MCP Origin must be a single exact entry in `server.cors_allowlist`, including
same-origin requests; empty means no browser-origin access. Host/origin allowlists
are deployment boundaries, not authority policies. The default listener remains
loopback-only and a TLS proxy must preserve an explicitly allowed Host.
`TestMCPConfigurationBoundsAndIsolation` and decoder/admission tests cover positive
round trips, all limit edges, invalid hosts/groups and detached configuration.''')
append('docs/contracts/pengui-provider-registration.md','''## MCP and retained discovery (phase 22)

Operators may now deliberately register the opaque **`mcp.use`** action for the
Chartworks MCP capability. Each request to `POST /v1/mcp` requires a freshly supplied
Pengui bearer for `auth.audiences.mcp`, then its ordinary tool action and complete
resource/session reach. Use distinct HTTP and MCP audiences to prevent unintended
cross-surface replay. The transport never issues, renews or persists bearer tokens.
A transport session or enabled tool group cannot confer permission. Existing
same-audience configuration remains an explicit operator choice, not an alias
created by the adapter.

The [MCP contract](mcp-v1.md) maps eighteen real tools to the actual HTTP operation
IDs and signed action spellings. `tools/list` derives schemas/effects/audit metadata
from those registrations and filters by current actions. Topic and dataset metadata
resources reuse the same services and require their normal reach; none is a local
public/anonymous capability or an Apps viewer.

Three cumulative HTTP/SDK operations add no new domain action: `POST /v1/topics/list`
uses `topics.read`, while `POST /v1/datasets/list` and `/v1/datasets/describe` use
`sources.read`. Topic list requires signed topic-read, source-read, dataset-query
and execution-context-use selections, applied to every publication dependency before
pagination. Dataset operations require source-read, exact execution-context-use
and selected dataset-query reach. They read retained metadata without connecting
to the warehouse. Their source/topic manifests and phase-21 denial tests extend
in the same change.

This is a verified consumer integration, not evidence that a production Pengui
policy has been provisioned. No Pengui issuer change or separate Chartworks
credential channel is required for these opaque action strings.''')
edit('docs/contracts/pengui-provider-registration.md','MCP transport/tools remain phase 22; both intended-audience verifier paths use the same verification core.','Phase 22 implements MCP transport/tools through the same intended-audience verification core; see its operator requirements below.')
append('GETTING-STARTED.md','''## MCP tools on the existing server

Merge [examples/chartworks.mcp.json](examples/chartworks.mcp.json) into your existing
configuration, retaining the trusted Pengui issuer/JWKS, intended HTTP/MCP audiences,
metadata connection and installed domain services. Use exact `mcp.allowed_hosts`
for your deployment; allow browser origins deliberately through
`server.cors_allowlist`. The default 75-second server write timeout exceeds the
65-second MCP call limit; existing deployments with shorter timeouts must adjust
it or lower `mcp.timeout`. No provider is contacted merely by enabling MCP.

```bash
./bin/chartworks config-check --config /path/to/chartworks.json
./bin/chartworks mcp --config /path/to/chartworks.json
# Equivalently: chartworks serve --config ... with features.mcp=true.
```

The explicit `mcp` command fails when MCP is disabled. Both commands use the same
listener, health/capability services and authenticated route registry. There is no
stdio token store or separate public authentication endpoint. The host/client
supplies a current Pengui bearer carrying `mcp.use` plus the needed domain actions
and signed resource/session/context restrictions. Use the MCP intended audience,
not an HTTP-only bearer. Never put that bearer in an MCP URL or resource URI.

A client initializes using a JSON-RPC object such as the following, sent to
`POST /v1/mcp` with `Content-Type: application/json`,
`Accept: application/json, text/event-stream`, and its current Authorization bearer:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"analytics-client","version":"1"}}}
```

Send `notifications/initialized` next (202, no body), then use the negotiated
`Mcp-Protocol-Version` on subsequent requests. `tools/list` returns the permitted
installed bindings. A model-free chart catalog call is:

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"chart_catalog","arguments":{}}}
```

This additionally requires `charts.read` and `cw.tenant.read:<signed-tenant>`.
There is no GET/SSE stream or transport-session cookie. A successful tool result
contains a typed `result`; inspect `isError`, error outcome and domain receipts
before retrying anything that may have persisted or spent budget.
`sdk/chartworks.Client.MCP` obtains the bearer from its caller-supplied token
provider on every message. Its typed `ListTopics`, `ListDatasets` and
`DescribeDataset` methods instead use their ordinary HTTP endpoints/audience.
See the [MCP contract](docs/contracts/mcp-v1.md) for all eighteen tools, metadata
resource URIs and exact restrictions. The reporting Apps viewer remains phase 31.''')
append('CHANGELOG.md','''## Phase 22 MCP and cumulative phase 21 discovery

- Optional stateless Streamable HTTP mount and authenticated in-process client,
  eighteen actual service bindings including all eleven established core operations,
  and three pure metadata resources through the same domain core.
- Fresh MCP-audience Pengui verification, registered tool actions/resources, bounded
  concurrent admission, closed schemas, typed safe errors and honest paid/persisted
  annotations. No local issuer, credential channel, analytical transport state or
  reporting viewer placeholder.
- HTTP/OpenAPI/public Go SDK topic and dataset discovery with signed dependency
  restrictions applied before database pagination. Existing NLQ preflight effects
  correctly include routing cost and persisted session evidence.
- Six phase-22 criteria, cumulative phase-21 guards, ordinary MCP functional tests,
  malformed-input/expiry/cancellation/panic/isolation regressions and protocol fuzzing.
  Final evidence is recorded in docs/reviews/phase-22-adversarial.md.''')
expected={
'CHANGELOG.md': '59b17f2476657b68194aefc310782b421847d9ccabd48f7a1624bbf2ae6360cf',
'GETTING-STARTED.md': 'a6a6f74d105109142f3b606a1f812eed23d758264a0db267e811f80b80e250d2',
'README.md': 'd8c8d3bd82a964d2151307e394bff96e429d3b8b2369fa4261bcfdb7c1b31ea2',
'RFC-001-Chartworks.md': 'fd031cc23a168e2ce3a09a574b1a72be456e14db79a895b4361a9288dc4b52a4',
'docs/configuration.md': '90af865cfaaa71a4f95874985c6f7c133d440c8dc3f2297049f5b0cf0d16a107',
'docs/contracts/pengui-provider-registration.md': '4314ea77b9b8a24c0a8d108fd19b241f7e6913a6a151f36473d2f82fd3b03890',
'docs/plans/README.md': 'cf031a5d6455e690b6f9b08ac6135f41cb23313f58769047dd88b5040dd4a695',
'docs/plans/phase-20-charts-spec.md': 'e446ad8da31a864fe6f6b69e7361782a226da48a08c7876b824dad7730c27fd3',
'docs/plans/phase-21-http-api.md': 'fe0c6d76cc6548bc7ecc82400f56572ae40a6706f086c89d2ca33c7e8b603701',
'docs/plans/phase-22-mcp-server.md': '1bdb0f799adfdf64372d4153a76da47ecfc838140bfda7a3d5c0b36fb8ffbeb0',
}
for path, digest in expected.items():
    assert hashlib.sha256((r/path).read_bytes()).hexdigest() == digest, path
