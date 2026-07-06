# Brief 13 — MCP-surface library choice: `mark3labs/mcp-go` vs Dockyard (D-011)

> Status: draft · 2026-07-06 · sources: hurtener/dockyard (pinned ref `v1.8.0` /
> commit `2452c090c717ba6c2960fd5d38083d064fb1de81`), soundings `internal/mcpserver`
> (mcp-go `v0.43.2`), upstream `mark3labs/mcp-go` (`v0.55.1`, checked live via
> `gh api`), upstream `modelcontextprotocol/go-sdk` (`v1.6.0`, local module cache)

## Summary

D-007's premise — "Dockyard is unavailable as a Go dependency" — no longer holds
(D-011). This brief re-checked it against the actual repository, not the prior
agent's note. Finding: Dockyard's `runtime/server` and `runtime/tool` packages
are a genuinely importable Go library, decoupled from its CLI/scaffold/codegen
tooling — a plain `main.go` can `server.New(...)`, register typed tools, and
call `HTTPHandler()` for a mountable `http.Handler`, exactly like an `mcp-go`
app does. It wraps the official `modelcontextprotocol/go-sdk`, sets HTTP
security explicitly, and gives contract-first schema generation with zero
required YAML or CLI step. Its two real gaps against a Chartworks fit: (1) no
global "wraps every tool call" middleware seam — cross-cutting auth/scope
enforcement is applied per-tool by convention, not structurally guaranteed the
way `mcp-go`'s `WithToolHandlerMiddleware` guarantees it; (2) a larger,
actively-evolving dependency surface (Tasks, obs/v1, Apps, cobra, sqlite in
adjacent packages) a headless V1 mostly won't touch but that raises the
update-surface / audit cost. `mcp-go` at its current upstream version
(`v0.55.1`) still exposes the exact seams Soundings proved — Soundings' pin is
stale (12 minor versions behind) but the API shape is unchanged.

**Provisional recommendation: Dockyard, with a Chartworks-side global tool-call
wrapper closing gap (1) — moderate confidence.** The ecosystem-alignment case is
real and the consumability blocker is gone; the missing global middleware seam
is a one-time, well-understood fix (a wrapper helper + a registration-time lint
check), not a structural mismatch. The RFC should treat this as re-opened, not
settled — see Open Questions.

---

## 1. Consumability

Dockyard is a proper Go module: `module github.com/hurtener/dockyard`, `go
1.26.2`, tagged releases (`v1.3.0`…`v1.8.0`; `v1.8.0` is current `HEAD` on
`main`, 2026-06-30). The pieces relevant to an MCP tool surface live under
`runtime/` — `runtime/server`, `runtime/tool`, `runtime/obs`, `runtime/apps`,
`runtime/tasks`, `runtime/store` — ordinary importable packages, not
generated-only output. This is the key correction to D-007: the prior
rejection ("its tooling scaffolds standalone MCP-App servers with embedded
UIs") described the `dockyard new` scaffold and `cmd/dockyard` CLI
(`internal/cli`/`scaffold`/`codegen` driving the `dockyard.app.yaml` manifest
workflow) — a real, separate half of the repository — but conflated it with
the runtime, which needs no manifest, CLI, or code generation to function.

Concretely: `runtime/server.New(Info, *Options) (*Server, error)` constructs a
server from an `Info{Name, Title, Version}` and a small `Options` struct
(logger, obs emitter, extensions, Tasks engine). `runtime/tool.New[In,
Out](name)` is a fluent builder (`.Describe().UI().Handler().Register(srv)`)
that infers JSON Schema from the Go contract structs via reflection
(`internal/codegen.SchemaFor`, using `github.com/google/jsonschema-go` — the
same schema engine the official MCP SDK uses internally) — no `dockyard
generate`, no `.yaml`, no Node toolchain in that path. The manifest and its
TypeScript/drift-check generation are consumed only by `internal/cli` (the
`dockyard` binary's `generate`/`validate`/`test`/`new`/`build` subcommands) —
useful for Dockyard's quality-gate tooling, optional otherwise.

Architecturally, Dockyard's runtime does not assume it owns `main()`. Every
worked example (`examples/backend-tools-only`, `examples/combined-patterns`,
`examples/prompts-demo`) has a hand-written `cmd/*/main.go` that constructs the
logger, the server, registers tools, and picks a transport off an env var or a
flag — indistinguishable in shape from a hand-rolled `mcp-go` `main.go`. There
is no `dockyard.Run()` entry point that takes over process lifecycle; `Server`
is a value the caller drives. **Version to pin: `v1.8.0`** — current `main`
HEAD at research time, so pinning the tag and the commit are equivalent today.

## 2. Transport support

All three transports Chartworks needs exist in `runtime/server`, each a thin,
explicit wrapper over `modelcontextprotocol/go-sdk`:

- **stdio** — `(*Server).ServeStdio(ctx) error`, wrapping `mcpsdk.StdioTransport`.
- **streamable-HTTP** — `(*Server).HTTPHandler(*HTTPOptions) (http.Handler,
  error)`, wrapping `mcpsdk.NewStreamableHTTPHandler`. Returns a plain
  `http.Handler` meant to be mounted ("Mount it on an *http.Server", per the
  doc comment) — the shape a one-binary dual-surface server needs to hang the
  MCP path off the same `http.ServeMux`/router the HTTP API uses.
- **in-process** — `(*Server).ServeInMemory(ctx) mcpsdk.Transport`, wrapping
  `mcpsdk.NewInMemoryTransports()`; starts the server in a background
  goroutine and hands back the client-side transport — the direct analog of
  `mcp-go`'s `client.NewInProcessClient(s.mcp)` Soundings' test/parity harness uses.

A notable Dockyard-specific addition: `HTTPOptions.ServerForRequest
func(*http.Request) *Server` — a per-request server-selection hook ("enabling
per-session or multi-tenant wiring") — plus `HTTPOptions.Stateless`. Neither is
strictly needed for Chartworks' P3 tenant model (isolation lives in the
identity envelope + store predicate, not per-tenant `*Server` selection), but
both are available if wanted later.

HTTP security is set explicitly rather than inherited from the SDK default —
`DefaultHTTPSecurity()` turns on DNS-rebinding protection, cross-origin (CSRF)
protection, and Content-Type verification, citing a real upstream regression
("cross-origin protection was on in v1.4.1 and off again in v1.6.0") as the
reason it never trusts the SDK's shipped posture — a direct match for CLAUDE.md
§7's "set explicitly — never inherited from an SDK default" rule; `mcp-go`
leaves this to the caller (Soundings' `httpAuthMiddleware` does it by hand).

## 3. Middleware / context seams — typed identity injection, typed errors

**Injecting identity/auth.** Dockyard has no per-tool-handler middleware
registration option analogous to `mcp-go`'s `WithToolHandlerMiddleware`. What
it has instead:

- At the HTTP transport, `HTTPHandler()` returns a bare `http.Handler` — a
  caller wraps it in ordinary `net/http` middleware exactly as it would wrap
  any other handler (parse/validate the bearer JWT, build the frozen identity
  envelope, `r.WithContext(...)`, call `next`). This works end-to-end because
  the underlying SDK's streamable-HTTP transport reads `req.Context()` as the
  base context for the session (`go-sdk@v1.6.0/mcp/streamable.go:943`, upstream
  comment: "Pass req.Context() here, to allow middleware to add context
  values") and threads it to the tool-call handler — the P2 identity-envelope
  pattern is directly portable.
- At the tool-registration layer, Dockyard exposes context-carried,
  request-scoped values through exported helpers — `RequestMeta(ctx)` /
  `WithRequestMeta` (the inbound MCP `_meta` a host attaches per-call,
  read-only, opaque to the runtime) and `RawArguments(ctx)` (undecoded call
  arguments, for edge-schema validation before the typed handler runs).
  Neither is an auth seam, but both show the context-carrying pattern is
  first-class.
- There is **no built-in "runs before every registered tool" hook.** Each
  `tool.New[...]().Handler(fn)` is registered independently; a scope check
  must be applied by the app author wrapping `fn`, or by a Chartworks-owned
  helper every registration site is required to call. Real difference from
  Soundings' pattern: its single `toolMiddleware` (installed once via
  `server.WithToolHandlerMiddleware`) enforces the capability-scope gate
  *before any core call*, for *every* tool, structurally — a reviewer cannot
  forget to wire it in. Reproducing that on Dockyard needs either (a) a
  lint/test failing if a tool isn't wrapped by the shared guard, or (b) moving
  the gate into HTTP middleware by parsing the JSON-RPC method + tool name out
  of the body (more invasive, and duplicates onto the stdio path).

**Typed tool errors.** Both frameworks are equally thin here — neither hands
you a taxonomy, both let you build one. In Dockyard, `ToolFunc[In, Out]`
returns `(Out, error)`; a non-nil error becomes an `isError` `CallToolResult`
via the underlying SDK (`go-sdk@v1.6.0/mcp/tool.go:343`), using the error's own
message text unless the app supplies its own — practically identical to
`mcp-go`'s `NewToolResultError(msg)`. Soundings' closed §8.1 error taxonomy
(`CodeNotFound`, `CodeScopeBlocked`, `CodeProviderUnavailable`, `CodeInternal`,
…, carrying ids/flags only, never a message or content) is entirely
Soundings-authored code sitting *on top of* `mcp-go`, not an `mcp-go` feature —
the identical taxonomy would sit on Dockyard's `ToolOutputFunc` (a
`ToolOutput[Out]{Text, Structured, Meta}` split) the same way. One
Dockyard-side improvement worth banking: panic recovery across the MCP
boundary is **already a toolchain guarantee** — every handler invocation is
wrapped in `guardHandler`, recovering a panic into a typed `*panicError`
(wrapping `ErrHandlerPanic`) with full logging — enforced by the framework
rather than depending on the app remembering to install a recover-based
middleware, as Soundings' `toolMiddleware` does by hand today.

## 4. The MCP Apps / UI-resource layer

Not relevant to a headless V1 (`runtime/apps`, `ui://` resources, `_meta.ui`),
but genuinely orthogonal — `apps.Register` and `tool.Builder.UI(name)` are
opt-in; a server that registers no App advertises no
`io.modelcontextprotocol/ui` extension capability and behaves as a plain MCP
server (package doc: "graceful degradation is mandatory and automatic … a
host that does not negotiate … simply ignores `_meta.ui`"). Importing
`runtime/server`/`runtime/tool` does not pull in `runtime/apps` for a tool
that never calls `.UI(...)`.

It is a plausible **future asset**, not a speculative one: if Chartworks ever
wants an inline chart/result renderer in a host's chat (rather than only a
chart-spec the caller renders), the App layer is a drop-in extension of the
same `Server` — no rearchitecture, just a later `apps.Register` + `.UI(name)`
call on tools already built with `runtime/tool`. `runtime/tasks` (MCP Tasks —
long-running, resumable, human-in-the-loop calls) is a second future fit: an
NLQ→SQL stage that runs long (large-warehouse queries, an approval step
before executing generated SQL) maps naturally onto it — a concrete Dockyard
argument, since `mcp-go`/the official SDK has no equivalent Tasks abstraction.

## 5. Coupling cost

Adopting Dockyard's **runtime only** (server + tool, no CLI, no Apps, no
Tasks, no manifest) drags into `go.mod`: `modelcontextprotocol/go-sdk`,
`google/jsonschema-go` (pure-Go schema inference, no codegen step), and
`go.opentelemetry.io/otel*` (a no-op unless `Options.Obs` is wired). A modest,
justifiable set given P5's schema-constrained structured-output posture
already implies a JSON Schema dependency somewhere.

Adopting the **full Dockyard workflow** (scaffold, `dockyard.app.yaml`,
`dockyard generate`/`validate`/`test`, the Apps web toolchain, `cobra`,
`fsnotify` dev loop, `modernc.org/sqlite`) drags in more: a CLI dependency
tree, an embedded pure-Go SQLite driver (`runtime/store/sqlitestore`, the
inspector's own persistence — irrelevant to Chartworks' Postgres-only store,
D-004), a Node/Vite toolchain for any App UI bundle, and Dockyard's own
quality-gate posture (the manifest's `quality:` block) layered on top of
Chartworks' `preflight`/`drift-audit`/coverage-band machinery (§4, §11). None
of it is required for transports and typed-tool registration — it is an
opt-in second tier adoptable later without committing to it from day one.

The genuine "what it gives": contract-first tooling keeping Chartworks'
NLQ/SQL/chart-spec output types and their JSON Schemas byte-verifiably in
sync (`internal/codegen`'s `ErrSchemaTSDrift` check), explicit and audited
HTTP security defaults, and a family-maintained runtime — bug fixes and
MCP-spec-revision tracking become a shared cost across adopters rather than
each repo tracking `mcp-go`'s and the official SDK's churn independently —
plus Tasks and Apps without a second framework migration later.

## 6. `mark3labs/mcp-go` at its current version

Soundings pinned `v0.43.2`; upstream is now `v0.55.1` — twelve minor versions
ahead, confirmed live via `gh api repos/mark3labs/mcp-go/releases/latest`. A
direct read of the current upstream source (`server/server.go`,
`server/stdio.go`, `server/streamable_http.go`) confirms the exact seams
Soundings' `internal/mcpserver` relies on are unchanged in shape:
`WithToolHandlerMiddleware`, `ToolHandlerFunc`, `WithStdioContextFunc`,
`WithHTTPContextFunc`, and `NewInProcessClient`. Upstream `mcp-go` now depends
on `google/jsonschema-go` too (schema-engine convergence with the official
SDK, same as Dockyard). Nothing observed suggests re-pinning to `v0.55.1`
would require a rewrite of Soundings' pattern — a routine version bump, not a
re-architecture. One structural note: `mcp-go` is not a wrapper over the
official `modelcontextprotocol/go-sdk` — it is an independent protocol
implementation, the real fork point between the two options. Picking
`mcp-go` means tracking its own protocol-compliance velocity; picking
Dockyard means tracking the official SDK's velocity through Dockyard's
wrapper — one layer further from the wire, but the layer the family already
invests in keeping current (§5).

## 7. A hybrid option

**Dockyard contracts/patterns over `mcp-go` transport:** not realistic as a
maintained posture. Dockyard's contract-first schema generation
(`internal/codegen.SchemaFor`) is not exported as a transport-agnostic library
separable from `runtime/server`'s `AddToolWithSchemas` — the builder in
`runtime/tool` imports `runtime/server` directly; the two are designed
together. Re-deriving just the schema-generation half onto `mcp-go`'s own
`AddTool` reimplements the split anyway, forfeiting the "family's own
framework" argument while still paying an integration cost. Not recommended.

**Dockyard now vs. later:** the more realistic hybrid. Ship V1 on `mcp-go`
(the Soundings-proven, lower-risk path) with the Chartworks tool contracts
defined the way Dockyard would want them (Go structs as the source of truth,
one `internal/mcpserver` package, one tool registration list) — that
discipline is CLAUDE.md P7 regardless of framework and costs nothing to keep
framework-portable. Migrate to Dockyard at a later wave once (a) the missing
global tool-middleware seam has a settled answer (a shared wrapper helper,
proven in a spike) and (b) a second adopter validates the maintenance-sharing
argument beyond Dockyard's own examples. Cost: a possible one-time migration.
Benefit: not gating Chartworks' first ship on an unproven-for-this-shape
framework choice. This is the brief's lean behind "moderate confidence" —
either "Dockyard now" or "`mcp-go` now, Dockyard later" is defensible; the RFC
should pick based on the family's appetite for being the first non-trivial
consumer of Dockyard's runtime in a headless posture.

---

## Decision table

| Criterion | `mark3labs/mcp-go` | Dockyard (`runtime/server` + `runtime/tool`) |
|---|---|---|
| Importable, no owned `main()` | Yes — proven in Soundings | Yes — confirmed here, decoupled from CLI/scaffold |
| stdio transport | Yes (`server.ServeStdio`) | Yes (`(*Server).ServeStdio`) |
| streamable-HTTP transport | Yes, mountable `http.Handler` | Yes (`(*Server).HTTPHandler`), mountable `http.Handler` |
| In-process transport | Yes (`client.NewInProcessClient`) | Yes (`(*Server).ServeInMemory`) |
| HTTP security defaults | Caller sets explicitly (Soundings' own middleware) | Framework sets explicitly (`DefaultHTTPSecurity`), never SDK-inherited |
| Global per-tool-call middleware (structural auth/scope gate) | Yes — `WithToolHandlerMiddleware` wraps every tool | No — per-registration wrapping only; a missed wrap isn't caught |
| Context injection at HTTP boundary | Yes — `WithHTTPContextFunc` | Yes — plain `net/http` middleware; SDK propagates `req.Context()` |
| Typed/structured tool errors | App-built taxonomy on top (§8.1 pattern) | Same — app-built taxonomy on top, equivalent effort |
| Panic-across-boundary safety | App-installed (Soundings' middleware) | Framework-guaranteed (`guardHandler` wraps every call) |
| Contract-first schema generation | Not built in | Built in (`internal/codegen`, reflection, no CLI needed) |
| MCP Apps / inline UI | Not available | Available, opt-in, zero cost if unused |
| MCP Tasks (long-running/human-in-loop) | Not available | Available (`runtime/tasks`) |
| Dependency footprint, runtime-only | Minimal (`jsonschema-go`, `google/uuid`, …) | Modest (`go-sdk`, `jsonschema-go`, OTel no-op unless wired) |
| Dependency footprint, full workflow | N/A | Larger (`cobra`, embedded sqlite, Node/Vite) — avoidable if unused |
| Version currency vs. Soundings' pin | Pinned `v0.43.2`; upstream `v0.55.1`, same shape | `v1.8.0` is itself current `HEAD` |
| Ecosystem-maintenance sharing | None — each repo tracks upstream independently | Real if a second adopter exists — unproven beyond own examples |
| Track record for this exact shape | Proven — Soundings ships it today | Unproven in production; proven only in Dockyard's own examples |

---

## Open questions for the RFC

1. Gate Chartworks' first ship on being the first production, non-Dockyard-repo
   consumer of this exact one-binary/dual-surface/JWT-at-boundary shape, or
   sequence it as "`mcp-go` now, Dockyard reconsidered later" (§7)?
2. If Dockyard is chosen, what closes the missing global tool-call-middleware
   gap (§3) — a Chartworks wrapper helper enforced by a lint/registration-time
   check, a contribution back to Dockyard adding a
   `WithToolMiddleware`-equivalent seam, or an HTTP-layer scope gate (with the
   stdio-path duplication that implies)? Pin this before any phase plan assumes
   it.
3. Does Chartworks want any part of the full Dockyard workflow (manifest,
   `dockyard generate`, quality gates) in V1, or strictly the runtime packages
   — and either way, does CLAUDE.md's `preflight`/`drift-audit`/coverage-band
   machinery need a note reconciling with Dockyard's own `quality:` gate so a
   future adopter doesn't run two redundant gates, and does the family want a
   decision entry tracking "first non-Dockyard production consumer" learnings
   for the next candidate?
4. Is the MCP Tasks engine (§4) a plausible fit for the SQL-safety /
   long-running-query / approval-before-execute shape P1 hints at — a concrete
   Dockyard argument with no `mcp-go` equivalent, worth a forward-looking RFC
   note either way?
5. Should the RFC pin the exact Dockyard ref (`v1.8.0` / the commit above),
   given Dockyard's rapid phase velocity means a later minor bump could move
   `runtime/server`'s API before Chartworks' phase-0 build lands?
