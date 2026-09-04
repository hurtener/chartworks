# Phase 22 — mcp-server (Wave 6)

> **Status:** draft
> **Owner:** orchestrator (Opus — plan authoring, convention 7)
> **Depends on:** phase-04-access-grants, phase-08-sources-core, phase-11-uploads-workspace,
> phase-13-engineering-pipelines, phase-15-topics-lifecycle … phase-20-charts-spec
> (same dependency set as phase 21 — the MCP surface is a thin caller of every shipped core).

---

## RFC / request sections

- **RFC-001-Chartworks.md §11.1** — the MCP tool surface (the binding tool table, the
  `mark3labs/mcp-go` library choice, the global-middleware scope gate, the fail-closed
  annotation allowlist test, typed results on the §9.7 envelope). **This is the phase's
  primary contract.**
- **RFC-001 §9.7** — the normalized answer envelope every query tool returns.
- **RFC-001 §9.5** — the typed validation error vocabulary that is normative on both
  surfaces (`statement.blocked`, `table.not_granted`, `table.not_in_topic`,
  `column.unknown`, `join.unreachable`, …).
- **RFC-001 §9.4 / §5.3** — the BYO context-vs-submit distinct capability scopes.
- **RFC-001 §4.2 / §5.2** — the frozen `identity.Envelope` + `EffectiveAccess` the
  middleware threads onto `ctx` before any core call.
- **RFC-001 §14** — the config surface (`server:` HTTP/MCP addresses, `auth.audiences.mcp`).
- **RFC-001 §15** — RED metrics per tool; mechanical audited-tool coverage.
- **Decisions:** D-019 (mcp-go for V1, the binding tool table), D-020 (grants/resolver/
  scopes), D-006 (dual audiences), D-022 (BYO bundle + `submit_sql` through the identical
  core), D-030 (management-plane operations are HTTP-only — agents don't administer).

## Depends on

Same set as phase 21 (`http-api`): the tool handlers are **thin callers** (P7) of the
already-shipped cores — semantics (15–16), NLQ routing/context (17), generation/execution
(18), BYO mode (19), charts (20), datasets/sources (08, 11), the access resolver (04). No
tool re-implements a core; the surface only decodes typed input, calls one core method, and
shapes the §9.7 envelope. The auth validator + dual audiences (03/06) and the access
resolver (04) are hard prerequisites — without them the surface cannot authenticate or gate.

**Precondition (convention 10 / D-019):** the Wave-5 → Wave-6 Dockyard re-evaluation
checkpoint runs *before this phase starts* and files its own decision entry (mcp-go stays,
or Dockyard is adopted). This plan is written to D-019's standing answer (**mcp-go**); if the
re-check supersedes it, the plan is revised in the same PR that files the superseding
decision. The tool contracts are kept Dockyard-portable regardless (below).

## Informing briefs

- `docs/research/13-dockyard-mcp-surface.md` — **the backbone.** Library decision table,
  the global-middleware seam that makes the scope gate structural, transports (streamable-
  HTTP mountable handler, stdio, in-process), the "mcp-go now / Dockyard later" posture,
  the exact upstream version check.
- `docs/research/09-agents-repo-ideas.md` — finding #7: the fail-closed tool-annotation
  allowlist test (default-deny on an unclassified tool). Finding #8: read-aggregation tools
  may bundle reads but must never mask an access denial (P4).
- `docs/research/08-datus-agent-ideas.md` — the explicitly-curated read-only tool subset for
  external/BYO agents as a starting checklist; scope is derived server-side per tenant, never
  trusted from the caller (its self-declared-scope shape is named as an anti-pattern).

## Brief findings incorporated

- **Brief 13 — library + seams.** V1 builds `internal/mcpserver` on `mark3labs/mcp-go`,
  pinned exactly at the current upstream release **`v0.55.1`** (verified live against
  `repos/mark3labs/mcp-go/releases/latest`; Soundings' `v0.43.2` seams are shape-identical —
  a routine pin bump, not a re-architecture). The deciding factor: mcp-go's global
  `server.WithToolHandlerMiddleware` wraps **every** tool call — the scope/grant gate is
  *structural*, a missed wrap is impossible, which is load-bearing for P1a. Transports taken
  verbatim from the brief: `server.NewStreamableHTTPServer` → a mountable `http.Handler`
  hung off the shared HTTP mux; `server.ServeStdio` with a `WithStdioContextFunc` injecting a
  pre-validated dev envelope; `client.NewInProcessClient` for hermetic tests + the SDK's
  in-process mode. Contracts stay Dockyard-portable by discipline: Go structs as the schema
  source, **one** registration list, **one** `internal/mcpserver` package.
- **Brief 09 #7 — annotation allowlist.** A single `TestToolAnnotations` enumerates the
  live tool registry from the constructed server and asserts every tool carries an explicit
  read-only/write annotation; a `writeTools` allowlist (`submit_feedback` only) names the
  writers; any registered tool absent from the classification fails CI by default-deny — not
  by someone remembering to update a test.
- **Brief 09 #8 — bundled reads vs. access.** `describe_topic` / `describe_dataset` may
  aggregate several core reads, but a denied grant inside a bundle fails the whole tool
  loudly (typed `scope_blocked`), never silently omits the field (P4).
- **Brief 08 — curated read-only subset.** The 10 non-feedback tools are all read-only; no
  mutating/management tool is exposed on MCP (D-030). Scope is derived from the validated
  token + resolver server-side; the caller never self-declares scope (Datus' anti-pattern).

## Findings I'm departing from

- **Brief 13's provisional "Dockyard, with a wrapper" lean.** Not adopted for V1: D-019
  settled mcp-go on the structural-gate argument, and this plan implements D-019. Dockyard
  portability is preserved (structs-as-source, one list, one package) so the Wave-5 re-check
  can still flip cheaply. No departure from the settled decision — a departure from the
  brief's *provisional* recommendation, deliberately.
- **Brief 09 #8's "partial data with per-field error markers."** Adopted only for
  *availability* gaps, never for access. An access denial is always a whole-tool typed
  failure; the plan does not carry a per-field "denied" marker that could read as a soft
  omission.

## Scope

Delivers `internal/mcpserver`: the MCP tool surface as a thin P7 caller layer.

- **The RFC §11.1 tool set**, each with a typed Go input/output contract struct (the schema
  source of truth) registered through one list — see Design.
- **The one global tool-handler middleware** (`server.WithToolHandlerMiddleware`): trace-id
  threading → capability-scope gate (before any core call) → grant-relevant envelope on
  `ctx` → panic recovery → RED metrics → typed error-result mapping (§9.5 codes, §9.7
  envelope). Installed exactly once in `New()`.
- **Three transports:** streamable-HTTP (a mountable `http.Handler` behind the MCP-audience
  bearer middleware, mounted on the shared HTTP server at `server.mcp.path`), stdio (`chartworks
  mcp`, dev-envelope injected), and in-process (`client.NewInProcessClient`, for tests +
  `sdk/chartworks` in-process mode, phase 23).
- **Per-surface audience enforcement** (D-006): the streamable-HTTP transport validates the
  MCP audience; an HTTP-audience token is rejected 401 before the MCP machinery runs.
- **The closed surface error taxonomy** (domain-clean, P6) mapping core errors + §9.5
  validation codes onto typed `isError` results — distinct from internal core codes, which
  never reach the wire.
- **The fail-closed annotation allowlist test**, the structural single-registration-path
  test, the MCP-aud enforcement test, the in-process round-trip test, the panic-injection
  test, and the registry-driven cross-tenant probe.
- One config key (`server.mcp.path`) + the `chartworks mcp` subcommand's stdio serve.

## Non-goals

- **Management-plane tools** (sources, pipelines, grants, topic lifecycle transitions) — MCP
  is read/ask/feedback only in V1 (RFC §11.1, D-030). Those live on the HTTP surface (phase
  21).
- **The full three-surface parity suite** — a parity *skeleton* asserting the Ask/BYO tier
  is reachable through MCP lands here; the exhaustive HTTP↔MCP↔SDK table-driven suite is
  phase 23.
- **MCP Apps / `ui://` resources / MCP Tasks** — orthogonal, opt-in, post-V1 (brief 13 §4).
- **Dockyard migration** — deferred to the Wave-5 re-check outcome; not this phase.
- **New core logic of any kind** — a handler that needs business logic is a drift signal;
  the logic belongs in the core it calls.

## Design

### Package shape (one package, one list — Dockyard-portable)

```
internal/mcpserver/
  server.go       New(Deps) *Server; registerTools() (the ONE list); MCPServer();
                  Handler() (streamable-HTTP); ServeStdio(); InProcess()
  middleware.go   toolMiddleware (installed once via server.WithToolHandlerMiddleware)
  authn.go        httpAuthMiddleware (MCP-aud bearer validation); dev-token resolve
  tools.go        toolDefs() []mcp.Tool — one slice, Go contract structs as schema source
  contracts.go    the typed In/Out structs (the schema source of truth; Dockyard-portable)
  handlers.go     one handler per tool; each decodes In, calls one core, shapes Out
  errclass.go     the closed surface error taxonomy + core-error → code mapping
  annotations.go  the read-only/write annotation table + writeTools allowlist
  metrics.go      RED metrics per (tool, outcome); panic + scope-block counters
```

### The tool set (RFC §11.1 — binding table)

The RFC §11.1 table enumerates the tools below with their capability scope and effect. Each
carries an explicit read-only/write annotation (`mcp.ToolAnnotation` — `ReadOnlyHint` /
`DestructiveHint`). Typed `In`/`Out` structs are the schema source (`mcp.WithOutputSchema[Out]()`),
kept struct-first so a later Dockyard port is a re-registration, not a re-model.

| # | Tool | Scope | Annotation | `In` (key fields) | `Out` (normalized shape) |
|---|---|---|---|---|---|
| 1 | `list_topics` | `catalog.read` | read-only | `{}` (optional status filter) | `{ Topics []TopicSummary }` |
| 2 | `describe_topic` | `topic.read` | read-only | `{ TopicID string }` | `{ Topic TopicDetail }` (measures, dimensions, KPIs, join graph, sample values, capability-contract slice) |
| 3 | `list_datasets` | `catalog.read` | read-only | `{}` (optional source/origin filter) | `{ Datasets []DatasetSummary }` |
| 4 | `describe_dataset` | `dataset.read` | read-only | `{ DatasetID string }` | `{ Dataset DatasetDetail }` (schema, profile, freshness, lineage) |
| 5 | `preflight_question` | `query.preflight` | read-only | `{ Question string; Session string? }` | `{ Routability }` (topics, confidence, decision, clarification slots — no SQL) |
| 6 | `plan_query` | `query.plan` | read-only | `{ Question string; TopicID string?; EditBase string? }` | `{ Plan }` (routing evidence, SQL, validation report — **no execution**) |
| 7 | `run_query` | `query.execute` | read-only | `{ Question string; TopicID string?; IdempotencyKey string? }` | `{ Answer }` (the §9.7 envelope) |
| 8 | `refine_query` | `query.execute` | read-only | `{ Session string; Question string; IdempotencyKey string? }` | `{ Answer }` (session-scoped) |
| 9 | `get_query_context` | `query.context` | read-only | `{ Question string }` | `{ ContextBundle }` (bundle_version, routing result, contract slice, restated constraints, dialect + SQL requirements, clarification slots, provenance-labeled priors) |
| 10 | `submit_sql` | `query.submit` | read-only | `{ SQL string; BundleVersion string; IdempotencyKey string? }` | `{ Answer }` (external SQL validated + executed through the identical core, D-022) |
| 11 | `submit_feedback` | `feedback.write` | **write** | `{ QueryID string; Verdict string; CorrectedSQL string? }` | `{ Ack }` (feedback id + status) |

`Answer` is the RFC §9.7 envelope verbatim: routing evidence (topics, confidence, decision),
assumptions + ambiguity assessment, SQL (with provenance + validation report), result
preview, chart spec (§10), typed warnings. Ids are resolved to display labels before
returning (never an internal id as a user-facing label — P6). No tool leaks endpoint URLs,
dataset-storage identifiers, warehouse credentials, or model details.

**Ambiguity resolved — the "10 tools" count.** RFC §11.1's header reads "The V1 tool set
(10 tools)" but its own table enumerates **11 rows** (the four Discover, four Ask, two BYO,
one Feedback). The enumerated table is the more specific, binding artifact (D-019: "the
10-tool table is binding" — the *table*, not the prose count). This plan registers **every
tool the table names** (11); dropping one to satisfy the prose count would ship a missing
capability. The header's "10" is flagged as a prose miscount to reconcile in the RFC (a
one-word follow-up, filed in the PR description — CLAUDE.md §2 drift-flag, not a silent
divergence). The acceptance criteria bind to "every tool in the RFC §11.1 table," so the
count reconciliation cannot silently drop a registration.

### The one middleware (the structural scope gate — P1a)

Installed exactly once, in `New()`, via `server.WithToolHandlerMiddleware(s.toolMiddleware)`.
Because mcp-go applies it to **every** `AddTool`, no tool can bypass it — the load-bearing
D-019 property. Order, per call:

1. **Trace id** threaded onto `ctx` (telemetry).
2. **Capability-scope gate** — the tool's required scope (from the annotation table) is
   checked against the frozen envelope's token scopes **before any core call**. A missing
   scope short-circuits to a typed `scope_blocked` result + `access_decisions_total` metric;
   the core is never reached (asserted by core call-count == 0).
3. **Grant-relevant envelope on `ctx`** — the resolver's `EffectiveAccess` (attached at auth
   time, phase 04) rides the frozen envelope; the middleware guarantees it is on `ctx` for
   the handler. Resource-grant intersection stays **inside** the core query (P1 — never
   fetch-then-filter, never re-computed here). An empty effective set short-circuits to a
   typed `access_none` result, no query issued.
4. **Panic recovery** — a `defer`/`recover` converts any core panic into a typed `internal`
   result (content-free code + trace id in a structured log, never the panic value). A panic
   **never** crosses the MCP boundary (P4/§13).
5. **RED metrics** — `(tool, outcome)` timed; `outcome ∈ {ok, tool_error, scope_blocked,
   access_none, internal}`.
6. **Typed error mapping** — a core error is routed through `errclass` into an `isError`
   `CallToolResult` carrying the closed surface code + (for validation) the §9.5 code in the
   envelope's validation report. A returned Go `error` or a raised stack never reaches the
   protocol layer.

### The closed surface error taxonomy (`errclass.go`)

Domain-clean, content-free codes distinct from internal core codes (P6):
`not_found` · `scope_blocked` · `access_none` · `validation_failed` (carries the §9.5 code +
report) · `clarification_needed` · `no_route` · `provider_unavailable` · `token_invalid` ·
`internal`. Each carries ids/flags only — never a message body, a data row, an internal id,
or a stack. The mapping from each core sentinel to a surface code is table-driven and
golden-tested.

### Transports + per-surface audience (D-006)

- **streamable-HTTP** — `server.NewStreamableHTTPServer(s.mcp)` wrapped by
  `httpAuthMiddleware` (validates the **MCP** audience, builds the frozen envelope, injects
  it via `r.WithContext`, `WithHTTPContextFunc`). Mounted on the shared HTTP server at
  `server.mcp.path` (default `/mcp`) — one binary, one listener. An HTTP-audience token, or
  an absent/invalid token, is refused **401 before** the MCP machinery runs. `Handler()`
  fails loud if no validator was wired.
- **stdio** — `server.ServeStdio` with `WithStdioContextFunc` injecting a pre-validated dev
  envelope (`chartworks mcp`, local dev; the caller resolves + validates the dev token
  first — no fail-open path).
- **in-process** — `client.NewInProcessClient(s.mcp)`; the envelope is injected per
  `CallTool` ctx. Backs hermetic tests + the parity harness + `sdk/chartworks` in-process
  mode (phase 23).

HTTP transport hardening (timeouts, body limits, Origin/Content-Type) is set explicitly by
the shared server (phase 21) — never inherited from an SDK default (CLAUDE.md §7); the MCP
mount inherits that hardened server, it does not relax it.

### Version pin verification

`go.mod` pins `github.com/mark3labs/mcp-go v0.55.1` (verified current upstream release at
authoring time — convention 8: pin the exact tag, verified against the real release asset).
A `TestLibraryPin` reads the pinned version out of `go.mod`/`debug.BuildInfo` and asserts it
equals the recorded pin, so a silent drift bump fails CI. The pin is bumped only deliberately,
with the seam re-verified.

### How it upholds P1–P7

- **P1a** — the scope gate is structural (global middleware); resource grants intersect
  inside the core query, never here; empty set short-circuits.
- **P2** — identity is read from the validated MCP-audience token into the frozen envelope;
  no `X-*` header consulted; per-call the envelope rides `ctx`.
- **P3** — the tenant predicate lives in the cores' store/warehouse queries; the surface only
  forwards the frozen envelope.
- **P4** — every failure is a typed result + a metric; a panic is recovered; no silent
  omission; management ops are simply absent, not soft-failed.
- **P5** — no provider SDK here; gateway calls stay in the cores.
- **P6** — tool names, fields, and error codes are domain vocabulary; no plumbing term on the
  wire; ids resolved to labels.
- **P7** — one core per capability; the surface is thin; a capability shipping on MCP ships
  on HTTP too (parity skeleton here, full suite phase 23).

## Config keys added

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `server.mcp.path` | string | `/mcp` | no | Mount path of the streamable-HTTP MCP surface on the shared HTTP server. Validated at boot (must start with `/`, no whitespace); an invalid value is a refused boot (fail-loud). Documented here, in the example config, and smoke-checked (added in the implementation PR per CLAUDE.md §4.2). |

Referenced (owned elsewhere, **not** added by this phase): `auth.audiences.mcp` (phase 03 /
§14 — the MCP audience this surface validates); `server.*` HTTP address/timeouts/body limits
(phase 01/21). No new secret.

## Acceptance criteria

1. **Full tool registration through one list.** Every tool in the RFC §11.1 table is
   registered exactly once via the single `registerTools()` list; the live tool registry
   enumerated from the constructed server equals the annotation table (no orphan tool, no
   registered-but-unclassified tool). (The table names 11 tools; the §11.1 header's "10" is a
   flagged prose miscount — criterion binds to the table.)
2. **Fail-closed annotation allowlist.** `TestToolAnnotations` enumerates the registry and
   asserts every tool carries an explicit read-only/write annotation; `writeTools` =
   `{submit_feedback}` (the only writer), all others read-only. **A registered tool absent
   from the classification fails CI** (default-deny). An accidentally-writable read tool
   fails.
3. **Middleware-bypass unregistrable (structural).** A structural test asserts (a) `AddTool`
   is reachable only inside `registerTools()` (grep/AST guard over the package — no other
   call site), and (b) a `*Server` is constructible only via `New()`, which always installs
   `server.WithToolHandlerMiddleware(s.toolMiddleware)`; a server or a tool registered
   without the middleware is not expressible. Every tool call demonstrably passes the scope
   gate.
4. **MCP-audience enforcement.** The streamable-HTTP transport rejects a token minted for the
   **HTTP** audience (and an absent/invalid/expired token) with **401 before** any MCP
   machinery runs; only an MCP-audience token passes. `Handler()` errors loud if no validator
   is wired.
5. **In-process round-trip.** An in-process client lists tools and calls one read tool
   (`list_topics`) end-to-end, receiving a well-formed typed §9.7-shaped result — green under
   `-race`.
6. **Panic never crosses the boundary.** With a panic-injected core fake, a tool call returns
   a typed `internal` `isError` result (content-free code) — no panic propagates, no stack
   trace or panic value appears in the result or a user-facing string.
7. **Scope gate denies before the core.** A call missing the tool's capability scope
   short-circuits to a typed `scope_blocked` result before any core call (asserted via core
   call-count == 0); an empty effective-access set short-circuits to typed `access_none`, no
   query issued; each increments its access-decision metric.
8. **Typed errors, never a raise.** A core sentinel error routes through `errclass` into an
   `isError` result carrying the closed surface code (and, for a validation error, the §9.5
   code in the report); no Go `error` and no stack ever reaches the protocol layer
   (golden-tested error mapping).
9. **Registry-driven cross-tenant probe.** Every tenant-sensitive tool, enumerated from the
   registration list itself (never a hand-maintained list — §5.5), is probed cross-tenant and
   returns a typed denial / nothing; a bare success carrying another tenant's data fails.
10. **Management ops are absent, not soft-failed.** The registry contains no sources /
    pipelines / grants / lifecycle-transition tool (D-030); a negative test asserts none is
    registered.
11. **mcp-go pinned & verified.** `go.mod` pins `github.com/mark3labs/mcp-go v0.55.1`;
    `TestLibraryPin` asserts the pinned tag equals the recorded pin (drift bump fails CI).
12. **Config fail-loud + parity skeleton.** An invalid `server.mcp.path` is a refused boot
    (fail-loud); the Ask-tier tools (`plan_query`, `run_query`) are reachable through the
    in-process MCP client with the same typed result the HTTP surface returns (parity
    skeleton — full suite deferred to phase 23).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven per tool (input decode → one core call → §9.7 envelope shaping);
  the `errclass` core-sentinel → surface-code mapping (golden); the annotation table vs.
  registry (criterion 2); the structural single-registration-path guard (criterion 3);
  `TestLibraryPin` (criterion 11); config validator fail-loud (criterion 12).
- **Integration:** required — this phase *consumes* every core seam (P7 thin surface) and
  closes the MCP half of the surface parity seam (§17). In-process client against the real
  constructed server with **real cores** where cheap and the gateway `mock` driver at the one
  sanctioned boundary; the full plan→run round-trip driven through the in-process client
  (criterion 5, 12). MCP-audience enforcement with a **real token** (no auth mock at the
  boundary — criterion 4). Lives in-package (`internal/mcpserver` *is* the wiring boundary);
  the cross-surface parity table is phase 23 in `test/integration/`.
- **Adversarial:** required (ACL/auth surface). The registry-driven cross-tenant probe
  (criterion 9), the empty-effective-access short-circuit (criterion 7), a forged-`X-*`-header
  attempt (must have no effect — envelope comes only from the validated token), and the
  fetch-then-filter regression guard (the gate denies before the core, proven by call-count).
  Once phase 18/19 exist, the injection / schema-escape corpus rides the BYO `submit_sql`
  path here as an end-to-end surface probe.
- **Fuzz:** n/a at the surface — the JWT parse fuzz lives in `auth` (phase 03) and the SQL /
  BYO-submission fuzz in `exec` (phase 09) / BYO (phase 19). The MCP surface decodes typed
  structs via the SDK's schema-validated path; it introduces no new hand-rolled parse
  surface. (If a hand-rolled pre-decode is added during implementation, a `FuzzXxx` lands
  with it — logged in the deviation log.)
- **Bench:** `BenchmarkToolMiddleware` on the hot per-call middleware path (scope gate +
  envelope threading) — a baseline, not a CI gate. The `*Server` is a shared reusable
  artifact: a `-race` concurrent-reuse test drives many concurrent in-process `CallTool`s.

## Coverage targets

Per CLAUDE.md §11 defaults. `internal/mcpserver` is a new `internal/` package (not a
store/auth/conformance driver), so the 80% band applies — the middleware scope gate + error
mapping + annotation test carry the security-load and must be well within it.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/mcpserver` | 80% | Default new-package band (convention 4); added to `scripts/coverage-bands.conf` in the implementation PR. |

## Smoke checks

`scripts/smoke/phase-22.sh` (`PHASE="22"`) SKIPs entirely until the surface is built (no
`chartworks mcp` subcommand / no registered tools), then one assertion per criterion.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `chartworks mcp --list-tools` prints exactly the RFC §11.1 table's tools; registry == annotation table (no orphan). |
| 2 | The tool list marks every tool read-only except `submit_feedback` (write); no unclassified tool. |
| 3 | A marker (build tag / test-only assertion surfaced via `chartworks mcp --self-check`) reports the middleware installed and the single registration path intact. |
| 4 | An HTTP-audience token against the `/mcp` mount is rejected 401; an MCP-audience token is accepted. |
| 5 | `chartworks mcp --self-check` runs the in-process round-trip (`list_topics`) and reports OK. |
| 6 | `--self-check` panic-injection path reports a typed `internal` result, no stack in output. |
| 7 | `--self-check` reports the scope-gate deny (core call-count 0) and the empty-access short-circuit. |
| 8 | `--self-check` reports a core error surfaced as a typed `isError` code, never a raised trace. |
| 9 | `--self-check` cross-tenant probe over the enumerated registry reports no cross-tenant leak. |
| 10 | `--list-tools` contains no sources/pipelines/grants/lifecycle tool. |
| 11 | `go list -m github.com/mark3labs/mcp-go` reports `v0.55.1` (pin check). |
| 12 | An invalid `server.mcp.path` config ⇒ non-zero boot with a typed error; a valid one mounts the surface. |

## Glossary additions

- **Tool-handler middleware** — the single seam (`server.WithToolHandlerMiddleware`) every
  MCP tool call passes: capability-scope gate → grant-relevant envelope on `ctx` → panic
  recovery → RED metrics → typed error mapping. Structural (no tool can bypass it) — the
  load-bearing P1a property of the MCP surface (D-019).
- **Annotation allowlist test** — the fail-closed CI gate that enumerates the live MCP tool
  registry and asserts every tool carries an explicit read-only/write annotation; an
  unclassified tool fails by default-deny (brief 09 #7).
- **In-process transport** — the network-free MCP client bound directly to the server
  (`client.NewInProcessClient`), backing hermetic tests, the parity harness, and the SDK's
  in-process mode.

## Decisions filed

- **Relies on** (no new entry): D-019 (mcp-go for V1, the binding tool table, the structural
  middleware gate), D-020 (grants/resolver/capability scopes), D-006 (dual audiences), D-022
  (BYO bundle + `submit_sql` through the identical core), D-030 (management-plane HTTP-only).
- **Precondition (not filed here):** the convention-10 / D-019 Wave-5 → Wave-6 Dockyard
  re-evaluation decision entry is filed by the checkpoint audit **before** this phase starts.
  This plan implements D-019's standing answer (mcp-go); if that re-check supersedes it, the
  plan is revised in the superseding PR.
- **Flagged for RFC reconciliation** (PR description, not a new decision): RFC §11.1's "(10
  tools)" header vs. its 11-row table — the table is binding; the header count is corrected in
  a follow-up.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). -->
