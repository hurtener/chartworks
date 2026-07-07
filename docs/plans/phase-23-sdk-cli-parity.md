# Phase 23 — sdk-cli-parity (Wave 6)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-21-http-api, phase-22-mcp-server

---

## RFC / request sections

- **RFC-001 §11.3** (SDK, CLI, parity) — the phase's charter: `sdk/chartworks`
  HTTP + in-process modes with identical typed results; the `chartworks admin`
  command family on stdlib `flag`; the standing three-surface parity suite.
- **RFC-001 §11.1 / §11.2** — the MCP tool table and HTTP route table the parity
  suite and the SDK method set are mechanically derived from.
- **RFC-001 §5.4** — scope-debug (`chartworks admin scope-debug`): replays a
  hypothetical `(principal, resource | question)` through the one resolver and
  reports the failed predicate; admin-only, read-only, never user-facing.
- **RFC-001 §15** (Erasure) — `chartworks admin erase --tenant` hard-deletes a
  tenant's store rows, facet vectors, workspace database contents, and uploaded
  files; audited; **loud on partial failure**.
- **RFC-001 §4.3 / §4.4, §17** — `chartworks admin bootstrap` (local-only
  first-admin provisioning; no header-exchange, no first-caller-becomes-admin
  path); the CGo-free single-binary operational shape the SDK in-process mode
  shares.
- **Decisions relied on:** D-008 (CLI on stdlib `flag`, `run(args, stdout,
  stderr) int` dispatch; cobra rejected), D-030 (no local user management;
  self-issue = API keys; admin bootstrap is a local CLI action), D-020 (the one
  resolver scope-debug replays), D-006 (bearer-token identity the SDK injects;
  never an `X-*` header — P2).

## Depends on

- **phase-21-http-api** — the SDK's HTTP transport calls the registered `/v1`
  routes; the parity suite enumerates one half of its capability registry from
  the HTTP route table. Must be shipped so the routes exist to call and to
  enumerate.
- **phase-22-mcp-server** — the parity suite's MCP leg drives the in-process MCP
  client (mcp-go in-process session) and enumerates the other half of the
  capability registry from the tool registration table. Must be shipped so the
  tools exist to exercise.

Both are the surfaces this phase proves are at parity; it ships no new capability
of its own (P7 — thin surfaces over the one core), so it cannot precede them.

## Informing briefs

- **Brief 01** (`01-predecessor-architecture.md`) — predecessor CLI / API-surface
  evidence: the sprawling `/v1/tenants/*`, `/v1/admin/*`, and GEPA prompt-pack
  admin surfaces, the platform auto-auth middleware, and the in-process shape.

## Brief findings incorporated

- **The admin surface is deliberately tiny.** Brief 01 inventories the
  predecessors' admin sprawl — tenant/membership/invite CRUD, service-account
  rotation, platform-admin user listing, GEPA prompt-pack CRUD/hot-reload/
  autopilot. Per D-030 none of it transfers: the `chartworks admin` family is
  exactly four verbs — `bootstrap`, `scope-debug`, `keys`, `erase`. Everything
  brief 01 lists as admin-plane tenant/topic/source/pipeline management is
  HTTP-only management-plane (RFC §11.1: "agents ask questions; they don't
  administer tenants"), reached through the SDK, never the CLI.
- **No auto-provisioning identity path.** Brief 01's "platform auto-auth
  middleware… auto-provisions a user" is the named counterexample; `bootstrap`
  is a local-only, store-direct operator action with no network surface and no
  header trust (P2, D-030).
- **In-process is the same process shape, not a second server.** Brief 01 notes
  both predecessors run one in-process API-plus-workers shape; the SDK
  in-process transport reuses the one `core` service stack the HTTP and MCP
  surfaces wrap — no parallel handler tree (P7).
- **Mechanically-derived registries, never hand-maintained lists.** Brief 01's
  observability scar (and the RFC §5.5 / §15 mechanical-registry discipline)
  drives the parity suite to enumerate capabilities from the route/tool
  registration tables themselves, so a surface gap fails without anyone editing a
  checklist.

## Findings I'm departing from

- **The predecessors' broad admin CLI/API is not ported.** Brief 01 documents a
  rich admin surface (tenants, memberships, invites, GEPA); this phase ships none
  of it. This is a deliberate D-030 departure, not an oversight — the surface's
  attack area is deleted, not migrated. No other brief finding is departed from.

## Scope

- **`sdk/chartworks`** — the public Go client:
  - One `Client` interface covering the Discover / Ask / BYO / Feedback
    capabilities plus the management-plane operations (sources, datasets,
    pipelines, topics, rules, grants, sessions, schedules, saved queries, audit,
    scope-debug) as typed methods returning the normalized §9.7 / §10 / §11.2
    result shapes (no bespoke per-caller format — P7).
  - Two transports behind that one interface: an **HTTP transport** (calls the
    phase-21 `/v1` routes, bearer-token auth injected per call) and an
    **in-process transport** (constructs and calls the one `core` service stack
    directly, threading a frozen identity envelope — for embedders and for the
    parity suite's SDK leg).
  - Auth injection: a bearer token (self-issue or external) on every HTTP
    request; the resolved envelope in in-process mode. Never an `X-*` identity
    header (P2).
- **`cmd/chartworks` admin command family** — `bootstrap`, `scope-debug`,
  `keys` (create / list / revoke), `erase`, on stdlib `flag` with a
  `run(args []string, stdout, stderr io.Writer) int` dispatch (D-008), wired
  under the existing `admin` subcommand stub from phase 01.
- **The three-surface parity suite** (`test/integration/`) — a shared
  **capability registry** derived from the HTTP route table and the MCP tool
  table; a table-driven suite that exercises every Discover / Ask / BYO /
  Feedback capability through HTTP, MCP (in-process), and the SDK (both
  transports), asserting the same typed result; a capability present on one
  surface but absent on another fails the suite (and the wave gate) mechanically.
- **The erase cascade** — the `erase` implementation in the core (invoked by the
  CLI and the HTTP admin route): store rows (every tenant-scoped table in the §12
  inventory) → `facet_vectors` (vindex) → upload-workspace database contents →
  uploaded files on disk under the tenant-scoped path; each step audited;
  **loud on partial failure** (typed error + non-zero exit + audit event, never a
  silent or partial success).

## Non-goals

- **No new capability.** This phase adds no Discover/Ask/BYO/Feedback/management
  behavior; it only re-exposes and proves the existing surfaces (P7). A parity
  gap is fixed by adding the missing surface wiring, not by inventing a
  capability here.
- **No local user / password / invite / signup management** (D-030) — out of
  scope permanently, not deferred.
- **No management-plane MCP tools** — management stays HTTP + SDK only (RFC
  §11.1); the parity suite does not require management capabilities on MCP.
- **No new erase policy** — soft-delete, retention windows, and per-resource
  (non-tenant) erase are not in V1; erase is tenant-grained hard delete.
- **No CLI framework dependency** (cobra rejected, D-008).
- **No new config keys, migrations, gateway roles, or MCP tools.**

## Design

**Data flow.** Both SDK transports and both agent-facing surfaces converge on the
one `core` service stack:

```
                 ┌───────────────── sdk/chartworks.Client (interface) ─────────────────┐
 caller ─────────┤  httpTransport ── bearer ──▶ internal/api ─┐                         │
                 │  inprocTransport ── envelope ──────────────┼──▶ core service stack ──┼──▶ store / sources / exec / …
 agent ──────────┤  mcp in-process client ──▶ internal/mcpserver ┘  (validation, audit, │
 CLI  ───────────┤  cmd/chartworks admin ── run(args,…) ──▶ core (bootstrap/keys/erase) │  access, events, cache)
                 └──────────────────────────────────────────────────────────────────────┘
```

**`sdk/chartworks`.** The `Client` interface is the contract; `var _ Client =
(*httpTransport)(nil)` and `var _ Client = (*inprocTransport)(nil)`
compile-time assertions prove both satisfy it, so a method added to one but not
the other fails to build. The in-process transport takes a constructed `core`
service and an `identity.Envelope`; an **architecture test** asserts the
in-process transport imports the core service package and *not* `internal/api`
or a re-implemented handler — it shares the one stack (P7), it does not fork it.
Auth: the HTTP transport sets `Authorization: Bearer <token>` per request and
sets no `X-*` identity header (a test scans outgoing headers); a call with no
token is a typed pre-transport error (P4). The in-process transport carries the
frozen envelope (P2) and never mutates it.

**The admin CLI.** `admin.Run(args []string, stdout, stderr io.Writer) int`
parses the leading subcommand by hand, dispatches to a `flag.FlagSet` per verb
(D-008), and returns a process exit code — fully testable without spawning a
process. Verbs:

- `bootstrap` — provisions the first admin principal + tenant directly against
  the store (local-only, no network, no header exchange — D-030/§4.3);
  idempotent-guarded (a second run is a typed no-op, never a duplicate admin).
- `scope-debug` — replays `(principal, resource | question)` through the **one
  resolver** (D-020) and, for topics, routing eligibility; prints the failed
  predicate (tenant mismatch / no grant at grain / scope missing / topic not
  published / source unavailable). Admin-only, read-only; output is a diagnostic,
  never a user-facing string (P6/§5.4).
- `keys` — `create` / `list` / `revoke` API keys against the store. Secret key
  material is printed **once** on `create` and never appears in `list` (the
  `list` shape has no secret field — a type-level guard mirrors §6.2's
  secret-free read shape); compares are constant-time in the auth core, never in
  the CLI.
- `erase` — invokes the core erase cascade below.

**The parity suite.** A `capabilityRegistry` is assembled at test time from two
mechanically-enumerated sources: the phase-21 HTTP route table and the phase-22
MCP tool table (the same registration tables the RFC §5.5 cross-tenant probe
enumerates — never a hand-maintained list). Each Discover/Ask/BYO/Feedback
capability names its HTTP route, its MCP tool, and its SDK method. The suite is
table-driven over the registry: for each capability it drives the HTTP client, an
in-process MCP client, and the SDK (both transports) against the mock gateway +
Docker Postgres stack, then asserts the normalized result shapes match. A
capability enumerated on one surface's table but missing a binding on another
(no route, no tool, or no SDK method) is a mechanical FAIL — this is the P7
parity proof, asserted from the registries, not by convention.

**The erase cascade.** A single core `EraseTenant(ctx, tenantID)` runs the steps
in order, each tenant-scoped: (1) delete every tenant row across the §12 store
tables; (2) delete `facet_vectors` via the vindex seam; (3) drop the tenant's
upload-workspace tables (via the workspace adapter, D-024); (4) remove uploaded
files under the tenant-scoped disk path (§12 / §7.4). Each step emits a
content-free audit event (§15). A failure at any step returns a typed error
naming the step, increments an erase-failure metric, and yields a non-zero CLI
exit — **never** a swallowed partial success (P4). An integration test injects a
partial failure and asserts the loud outcome, then a clean run asserts every
store table, the vector set, the workspace, and the file path are empty
afterward.

**Seams touched (CLAUDE.md §4.4).** `store` (bootstrap/keys/erase reads+writes,
scoped), `vindex` (erase facet deletion — the phase-07 tenant-scoped delete),
`auth`/`identity` (bearer + envelope injection, bootstrap principal), the
workspace adapter (erase step 3), `telemetry`/audit (erase + scope-debug
events). No gateway calls except through whatever a driven capability itself
makes (mock in tests). P1/P3: every store and workspace call in this phase
carries non-optional scope parameters; erase is tenant-grained.

## Config keys added

**None.** The SDK, the admin CLI, and the parity suite reuse existing config: the
HTTP transport uses the caller-supplied base URL + token (not config); `erase`
reuses the phase-11 upload-workspace DSN and the tenant-scoped upload file path
already in the §14 `workspace` domain; `bootstrap`/`keys` reuse the §14 `store`
and `auth` config. No key is introduced, so none is added to the example config.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none — this phase adds no config key |

## Acceptance criteria

1. **SDK one-interface, two-transport parity.** `sdk/chartworks` exposes a single
   `Client` interface; compile-time assertions prove both the HTTP and in-process
   transports satisfy it (a method on one but not the other fails to build). An
   architecture test asserts the in-process transport imports the core service
   package and **not** `internal/api` or a parallel handler (shares the one core
   stack — P7). *(TestClientInterfaceParity, TestInProcessSharesCoreStack)*
2. **SDK auth injection, no header identity.** The HTTP transport sets a bearer
   token on every request and sets **no** `X-*` identity header (header-scan
   test); a call with no token is a typed pre-transport error; the in-process
   transport carries the frozen envelope unmutated (P2). *(TestSDKAuthInjection)*
3. **CLI `run(args, stdout, stderr) int` dispatch.** The admin family dispatches
   `bootstrap|scope-debug|keys|erase` via stdlib `flag` (D-008) through a
   `run([]string, io.Writer, io.Writer) int`; a table-driven test drives each
   verb capturing exit code + streams; an unknown subcommand ⇒ non-zero exit +
   usage on stderr. *(TestAdminRunDispatch)*
4. **scope-debug replays the one resolver.** `admin scope-debug` routes a
   hypothetical principal/resource (and question) through the same resolver the
   query path uses and prints the failed predicate; output is diagnostic-only,
   never a user-facing string (P6). *(TestAdminScopeDebug)*
5. **keys round-trip, secret shown once.** `admin keys` create/list/revoke
   round-trips through the store; secret material prints once on create and never
   appears in the `list` shape (type-level secret-free guard). *(TestAdminKeysRoundTrip)*
6. **bootstrap is local-only and idempotent-guarded.** `admin bootstrap`
   provisions a first admin against the store with no network/header path; a
   second run is a typed no-op, never a duplicate admin (D-030/§4.3).
   *(TestAdminBootstrap)*
7. **Three-surface parity from the registries.** The parity suite enumerates the
   Discover/Ask/BYO/Feedback capabilities from the HTTP route table and MCP tool
   table (not a hand list) and exercises each through HTTP, MCP (in-process), and
   the SDK, asserting matching typed results; a capability present on one surface
   but absent on another fails mechanically (P7). *(TestThreeSurfaceParity)*
8. **erase cascades end-to-end, loud on partial failure.** `admin erase
   --tenant` deletes store rows + facet vectors + workspace tables + uploaded
   files; an injected partial failure ⇒ non-zero exit + typed error + audit
   event (never silent); a clean run leaves every store table, vector set,
   workspace, and file path empty (integration). *(TestEraseCascade)*

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven CLI dispatch (`TestAdminRunDispatch` — every verb, bad
  verb, `--help`), the SDK interface-satisfaction assertions, the SDK header-scan
  and no-token tests, the `keys` secret-free list shape (type-level).
- **Integration:** required — this phase's `Deps` name phases 21 and 22 and it
  closes the parity seam over both surfaces (§17). Real drivers: Docker Postgres
  (`make pg-up`) for store/workspace/vindex, real self-issued tokens for the HTTP
  leg, the mcp-go in-process session for the MCP leg; the gateway `mock` driver
  is the one sanctioned boundary mock (paired with the recorded-fixture tests
  already owned by phase 05). `TestThreeSurfaceParity` (identity/scope
  propagation across all three surfaces) and `TestEraseCascade` (≥1 failure mode:
  the injected partial-failure path) run under `-race`.
- **Adversarial:** this phase touches auth/identity injection and the erase path.
  The SDK no-`X-*`-header and no-token guards (P2), and a cross-tenant erase
  probe (an erase scoped to tenant A leaves tenant B intact — the standing
  cross-tenant obligation) are asserted. The registry-driven cross-tenant probe
  suite (RFC §5.5) already covers the routes/tools this phase re-exposes; this
  phase adds no new route or tool, so it inherits that coverage rather than
  duplicating it.
- **Fuzz:** n/a — this phase introduces no new parse/decode surface (JWT parsing,
  NLQ payloads, and SQL validation are owned by phases 03/17/09 and keep their
  fuzz targets; the SDK marshals typed Go structs through stdlib `encoding/json`).
- **Bench:** n/a — the SDK client and CLI are not hot reusable artifacts on a
  request hot path; the core stack they call carries its own benchmarks in its
  owning phases.

## Coverage targets

Per CLAUDE.md §11 defaults: 70% for `cmd/` / CLI tooling; 80% for a new public
package (`sdk/chartworks`). Entries added to `scripts/coverage-bands.conf` in this
PR:

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `sdk/chartworks` | 80 | default for a new public package |
| `cmd/chartworks` | 70 | default CLI/tooling band (covers the `admin` family; the `run(args,…)` shape makes it table-testable) |

The parity and erase integration tests live in `test/integration/` and count
toward the touched packages' coverage via their driven paths; the erase core
logic (if it lands in an `internal/` package rather than `cmd/`) is covered at
that package's existing band.

## Smoke checks

`scripts/smoke/phase-23.sh` maps each criterion to a `go test -run` assertion via
`run_group` (SKIPs cleanly per-criterion when a test isn't present yet).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestClientInterfaceParity` + `TestInProcessSharesCoreStack` in `./sdk/chartworks` pass |
| 2 | `TestSDKAuthInjection` in `./sdk/chartworks` passes |
| 3 | `TestAdminRunDispatch` in `./cmd/chartworks` passes |
| 4 | `TestAdminScopeDebug` in `./cmd/chartworks` passes (SKIPs without a store URL) |
| 5 | `TestAdminKeysRoundTrip` in `./cmd/chartworks` passes (SKIPs without a store URL) |
| 6 | `TestAdminBootstrap` in `./cmd/chartworks` passes (SKIPs without a store URL) |
| 7 | `TestThreeSurfaceParity` in `./test/integration` passes (SKIPs without a store URL) |
| 8 | `TestEraseCascade` in `./test/integration` passes (SKIPs without a store URL) |

## Glossary additions

- **Capability registry** — the mechanically-derived list of Discover/Ask/BYO/
  Feedback capabilities the three-surface parity suite enumerates from the HTTP
  route table and the MCP tool table (never a hand-maintained list); each entry
  names a capability's HTTP route, MCP tool, and SDK method so a missing binding
  fails the parity gate (P7, RFC §11.3).
- **Three-surface parity suite** — the standing test that exercises every
  capability in the registry through HTTP, MCP (in-process), and the SDK,
  asserting matching typed results; a capability shipping on one surface only
  fails the wave gate (RFC §11.3).
- **In-process transport** — the `sdk/chartworks` transport that calls the one
  `core` service stack directly (with a frozen identity envelope) instead of over
  HTTP, for embedders and the parity suite; the counterpart to the HTTP transport
  behind the same `Client` interface (RFC §11.3).

## Decisions filed

No new decision. This phase relies on existing entries: **D-008** (stdlib `flag`
CLI, `run(args, stdout, stderr) int` dispatch), **D-030** (no local user
management; admin bootstrap is a local CLI action; self-issue = API keys),
**D-020** (the one resolver scope-debug replays), **D-006** (bearer-token
identity the SDK injects — never an `X-*` header, P2), **D-019** (the mcp-go
in-process session the parity MCP leg drives), **D-024** (the upload workspace
the erase cascade clears).

## Deviation log

<!-- Filled DURING implementation. Every reasonable deviation (CLAUDE.md §4.3):
     what changed, why, and confirmation this file was updated in the same PR. -->

n/a — not yet in implementation.
