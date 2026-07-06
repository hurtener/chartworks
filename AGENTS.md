# Chartworks — Contributor & Agent Normatives

> This file is **binding** for anyone — human or AI — modifying this repository.
> It is mirrored **verbatim** in `AGENTS.md` so all agent tooling picks it up
> automatically. If the two files diverge, the most recent commit timestamp wins;
> flag the drift in your PR.
>
> Chartworks is at **bootstrap**: the RFC and the phase plans are not yet authored, and
> the kickoff interview will refine the product frame in §1. Where this file states a
> rule as *inherited-binding now*, it holds today; where it states one as *RFC-pending*,
> it is a provisional default the RFC (or a superseding decision) will confirm or replace.
> Both kinds are marked. If a rule below conflicts with the RFC or a phase plan, the
> **RFC wins**, then the **phase plan**, then this file. Update whichever artifact is
> wrong; never silently ignore the conflict.

---

## Starting a new session — orientation (READ THIS FIRST)

Chartworks is a multi-phase, doc-driven build. The design surface is large on purpose:
hygiene up front is cheaper than retrofitting it. Before substantive work, skim, in
order:

1. **§1 — What Chartworks is.** The product and its binding properties.
2. **§2 — Authoritative sources.** The priority chain: RFC > phase plans > master plan >
   this file > a consumer request (if the kickoff produces one) > research briefs > code
   comments.
3. **§16 — Authoring a phase plan.** The binding workflow for any contributor touching a
   phase. Skipping it is the single largest source of design drift.

**Drift-hygiene artifacts (live references):**

- `RFC-001-Chartworks.md` — the design source of truth. *(Not yet authored — the RFC is
  the next setup step after the kickoff interview; until it lands, this file and
  `docs/decisions.md` are the standing authority.)*
- `00_KICKSTART-PROMPT.md` — the **bootstrap artifact** that framed this repo and the
  kickoff interview. Authoritative for *bootstrap intent* until the RFC absorbs it; a
  `docs/decisions.md` entry may revise a specific point.
- `docs/decisions.md` — append-only log of settled decisions (`D-NNN`). At bootstrap the
  seed entries are *proposed* (inherited from the sibling Soundings build); the kickoff
  flips them to accepted or superseded. When tempted to re-litigate something, grep here
  first.
- `docs/glossary.md` — Chartworks vocabulary. New terms land here in the same PR.
- `docs/research/INDEX.md` — subsystem → research-brief reverse index *(created with the
  first brief)*.
- `docs/plans/_template.md` — phase plan template; new phases start as a copy.
- `scripts/drift-audit.sh` — mechanical drift checks (`make drift-audit`).

If asked to do something that doesn't fit a phase (a one-off fix, a question, a small
doc edit), proceed without the full §16 ritual — but mention any drift risk you spot.

**Predecessor hygiene.** Chartworks is a clean-room redesign, not a port. It has **two**
Python predecessors, both living under `_ref/` (gitignored — never copied, vendored, or
committed here):

- **the client predecessor** — `_ref/original_wayfinder/`: the client-tailored
  original NLQ-to-SQL router-first platform, carrying
  production fixes worth mining — **especially the topic lifecycle**. The client's
  name, schemas, and data samples are confidential: they never appear — even
  paraphrased — in any brief, plan, commit, or doc in this repo.
- **the generalistic predecessor** — `_ref/forked_wayfinder_explorer/` (the "Explorer"
  fork): the generalistic descendant of the same codebase; the two share most
  functionality.

Refer to them only as "the client predecessor" and "the generalistic predecessor." Ideas
are inherited **only** through `docs/research/` briefs — never by reading files across
into this tree. A dedicated **diff brief** comparing the client fixes against the fork
(especially the topic-lifecycle divergence) is a **mandatory phase-0 research artifact**.

---

## 1. What Chartworks is

> **Provisional frame — the kickoff interview + RFC will refine this. Marked accordingly
> throughout.**

Chartworks is the ecosystem's **Explorer seat**: a Go-native **structured-data analytics**
service. It is the sixth and (for now) final product in the family, and a clean-room Go
migration of an NLQ-to-SQL product (the two Python predecessors above), **extended** with
an upfront **data-engineering stage** before the NLQ-to-SQL functionality. Where Soundings
serves *unstructured documents*, Chartworks serves *structured data* — the boundary the
consumer request draws explicitly ("not structured CSV/XLSX — that is Explorer's domain").

```text
Portico   — the MCP gateway         (connects and governs tools)
Harbor    — the agent framework     (builds and runs agents; owns the MCP client)
Dockyard  — the MCP Apps framework  (builds the MCP servers and apps users touch)
Stowage   — memory infrastructure   (remembers, reconciles, retrieves, forgets)
Soundings — the KnowledgeProvider   (unstructured docs: extract + index + retrieve + ACL)
Chartworks— the Explorer            (structured data: engineer → model → NLQ-to-SQL →
                                      charts)
```

The provisional pipeline direction is **engineer → model → NLQ-to-SQL → charts**: an
upfront data-engineering stage shapes and prepares structured sources; a semantic model
(the predecessors' "topic packs" — measures, dimensions, KPIs, join graphs) grounds them;
natural-language questions route through that model to **validated, read-only SQL**; and
results render as charts. The final capability inventory, surface set, and scope
boundaries are **owned by the RFC** — this frame is the starting point for the kickoff,
not a settled contract.

Chartworks is expected to present a **dual surface** — an **MCP tool surface** for agents
(consumed through Harbor's MCP southbound driver) and an **HTTP surface** for a Console /
consumer UI — from **one binary** (`chartworks`), running **both standalone** (its own
token issuer, for API-key callers) **and as an ecosystem citizen** (validating tokens
Pengui issues). Like Soundings, it is intended as a **swappable capability**: any Explorer
that implements the tool set + HTTP surface + JWT validation + access contract is a
drop-in. *(All of this paragraph is RFC-pending in its specifics; the ecosystem-citizen
posture and one-binary dual-surface shape are inherited defaults.)*

**Binding properties.** These distil the ecosystem invariants into product law. The P2–P7
set is **inherited-binding now** — a change that weakens one is wrong; reach for the RFC
(or a decision entry), not the keyboard. **P1 is a stub**: its access model is inherited
in principle but its concrete shape (and the Chartworks-specific SQL-safety property it
implies) is **RFC-pending**.

1. **P1 — Deny-by-default access, computed in the query path *(inherited principle;
   concrete model RFC-pending)*.** Access to a **data source** or a **dataset** is
   deny-by-default: absence of an explicit grant means no access, and the access
   restriction is applied **inside** the query that reaches the data — never
   fetch-then-filter. An empty effective-access set short-circuits to "no results" and
   never issues a query. **The exact access primitive** (whether Chartworks carries an
   ACL-style claim like Soundings, or scopes by data-source/dataset grants, or both) **is
   for the RFC to define.** Additionally, and specific to this product: **SQL-generation
   safety** — read-only execution, schema allowlisting, and injection guardrails — **will
   be a Chartworks-specific binding property the RFC must define.** It is called out here
   so no phase ships generated-SQL execution before that property is settled; until then,
   treat any generated SQL as untrusted and non-executable by default.
2. **P2 — Identity lives in the signed token, never in headers *(inherited-binding
   now)*.** Asymmetric JWT only (RS/ES `256|384|512`); `HS*` and `none` are rejected **at
   the parser**, before any business logic. `aud` is mandatory and per-instance (replay
   defense). The caller's **resolved identity/access claim** is the source of truth, read
   once into a **frozen per-request envelope** and never mutated mid-request. Chartworks
   is **stateless about identity**: it never recomputes sharing and never trusts `X-*`
   headers for identity or access.
3. **P3 — Multi-isolation on `(tenant, user, session)` *(inherited-binding now)*.** The
   `tenant` claim is mandatory; a query never matches across tenants regardless of any
   other prefix. Isolation is enforced at the storage/index layer (an inescapable tenant
   predicate), not by a filter a code path could forget.
4. **P4 — Fail loud, never silently degrade *(inherited-binding now)*.** A denied scope,
   an unreachable dependency, a missing identity, or a failed SQL validation yields a
   **typed error + a metric** — never an empty result that reads as "nothing found," never
   a silent fallback that widens access or executes an unvalidated query, never an
   empty-catch.
5. **P5 — One intelligence seam *(inherited-binding now)*.** Every embedding / LLM /
   rerank / SQL-generation model call goes through the `gateway` seam (Bifrost driver
   first). No package outside `internal/gateway` imports a provider SDK or constructs a
   provider HTTP request. Structured outputs use schema-constrained calls; free-text JSON
   parsing of model output is forbidden.
6. **P6 — Domain vocabulary only on the wire and in the UI *(inherited-binding now;
   forbidden-word list RFC-pending)*.** User-facing surfaces use domain terms only.
   Plumbing words (representative, generic examples pending the predecessor research —
   `collection`, `wiring`, `repair`, `index`, `shard`, `embedding`, `namespace`, `sync`)
   never appear in an API field, a UI string, an error message, or a human-read log. The
   **authoritative forbidden-word list is RFC-pending** and will be informed by the
   predecessor diff brief (the client predecessor grew a user-facing "repair"-class
   surface exactly because plumbing ids leaked — the anti-pattern this property exists to
   prevent).
7. **P7 — One primitive family, no parallel paths *(inherited-binding now)*.** One auth
   model, one access representation, one query/routing contract, implemented once in the
   core and exposed through thin surfaces (§6). Two of anything is the failure mode we are
   correcting — the predecessors' monolith scars (a router core wired into many bespoke
   call paths) are what the rewrite sheds.

---

## 2. Authoritative sources (in priority order)

1. `RFC-001-Chartworks.md` — product intent and design decisions *(pending; see §1)*.
2. `docs/plans/phase-NN-*.md` — implementation specifications. Acceptance criteria are
   binding.
3. `docs/plans/README.md` — the master phase plan: cross-cutting conventions and the
   phase index.
4. This file (`CLAUDE.md` / `AGENTS.md`) — operational rules.
5. A **consumer request doc**, if the kickoff interview produces one (a Pengui/Explorer
   contract request). Until it exists, this slot is empty and the chain skips to briefs.
6. `docs/research/*.md` — phase-planning research briefs. Authoritative for *context*, not
   for design. (The phase-0 predecessor diff brief is the first of these.)
7. Code comments and godoc — last and least authoritative.

`00_KICKSTART-PROMPT.md` is the bootstrap artifact that seeded the repo; it sits outside
this chain as historical intent and is superseded section-by-section as the RFC lands.
When a phase plan and the RFC drift, the RFC wins. File a follow-up to fix the plan.

---

## 3. Repository layout

> Provisional, in the sibling Soundings shape. **The RFC owns the final package
> inventory** — the domain packages below are marked `TBD-by-RFC` and will be renamed,
> split, or dropped as the RFC settles the pipeline. The infrastructure packages
> (`api`, `mcpserver`, `auth`, `identity`, `config`, `gateway`, `store`, `telemetry`) are
> inherited-shape and unlikely to move.

```text
.
├── RFC-001-Chartworks.md             # design RFC — source of truth (pending)
├── 00_KICKSTART-PROMPT.md            # the bootstrap / kickoff artifact
├── README.md
├── CHANGELOG.md                      # release notes (Keep a Changelog)
├── CLAUDE.md / AGENTS.md             # this file (verbatim copies)
├── Makefile                          # canonical build / test / lint commands
├── go.mod / go.sum                   # module github.com/hurtener/chartworks (placeholder, D-002)
├── docker-compose.yml                # local Postgres for dev & tests (D-004)
├── .github/                          # CI, PR template
├── .golangci.yml / .editorconfig / .gitignore
├── _ref/                             # the two Python predecessors — gitignored, never
│                                     #   copied/vendored/committed; ideas via briefs only
├── cmd/
│   └── chartworks/                   # the `chartworks` binary (serve, mcp, CLI)
├── internal/
│   ├── api/                          # HTTP surface: routing, validation
│   ├── mcpserver/                    # MCP tool surface
│   ├── auth/                         # JWT validation; self-issue + external-issuer; JWKS
│   ├── identity/                     # (tenant,user,session) triple + frozen envelope
│   ├── config/                       # typed config, env indirection, fail-loud validation
│   ├── gateway/                      # the intelligence seam + drivers {bifrost, mock} (P5)
│   ├── store/                        # the Store seam + driver {postgres} — Chartworks' OWN
│   │                                 #   state, distinct from the customer data sources (D-004)
│   ├── telemetry/                    # slog, metrics, per-decision counters, audit, optional OTel
│   ├── sources/                      # TBD-by-RFC: customer data-source connections it queries
│   ├── engineering/                  # TBD-by-RFC: the upfront data-engineering stage
│   ├── semantics/                    # TBD-by-RFC: the semantic model (topics/measures/dims/KPIs)
│   ├── nlq/                          # TBD-by-RFC: NL → routing → SQL generation
│   ├── exec/                         # TBD-by-RFC: read-only, guarded SQL execution
│   └── charts/                       # TBD-by-RFC: chart / visualization spec generation
├── sdk/
│   └── chartworks/                   # public Go client (HTTP + in-process modes)
├── eval/                             # NLQ/routing/SQL-quality harness (`chartworks eval`)
├── examples/
├── test/integration/
├── scripts/
│   ├── preflight.sh                  # the preflight gate
│   ├── drift-audit.sh                # design-coherence checks
│   ├── smoke/                        # per-phase smoke scripts
│   ├── hooks/pre-commit
│   └── install-hooks.sh
└── docs/
    ├── plans/                        # master plan (README.md) + phase plans + _template.md
    ├── research/                     # research briefs + INDEX.md
    ├── decisions.md                  # append-only D-NNN log
    └── glossary.md
```

Directories are created as the phases that own them land. Anything that doesn't have a
home above is wrong — if you need a new top-level directory, propose it in the RFC first;
once the RFC lands, `§3` (as the RFC amends it) is the binding layout.

---

## 4. Build, test, lint, run

All targets are canonical and run by CI. Targets no-op gracefully before the code they act
on exists.

```bash
make build         # build the chartworks binary
make test          # go test -race ./...
make coverage      # per-package coverage profile + the mechanical band gate
make bench         # run the Go benchmarks (on demand — not a CI gate)
make vet           # go vet ./...
make lint          # golangci-lint run
make pg-up         # start the local Postgres (docker compose) for tests
make pg-down       # stop and remove it
make drift-audit   # design-coherence checks (RFC/plans/briefs/mirror/forbidden names)
make check-mirror  # verify AGENTS.md == CLAUDE.md
make preflight     # build + smoke checks + drift-audit
make install-hooks # install the pre-commit hook (one-time, per clone)
```

### 4.1 Preflight gate — non-negotiable

`make preflight` is the same gate the pre-commit hook and CI enforce: it builds, runs
every per-phase smoke script (which SKIP gracefully where the surface isn't built yet),
and runs `drift-audit`. Do not bypass the pre-commit hook with `--no-verify` outside a
documented emergency.

### 4.2 Phase implementor contract

A phase is **done** only when: (a) every acceptance criterion in its plan passes; (b)
coverage targets for touched packages are met; (c) `scripts/smoke/phase-NN.sh` reports
`OK ≥ count(criteria)` and `FAIL = 0`; (d) prior phases' smoke scripts still pass. A new
CLI command, HTTP endpoint, MCP tool, or public API ⇒ a smoke check in the **same** PR. A
new config key ⇒ documented in the plan, the example config, and a smoke check.

### 4.3 Reasonable plan deviations

Plans are specifications, not straitjackets. A reasonable deviation discovered during
implementation is fine — document it in the PR description and update the plan file **in
the same PR**. Silent divergence from a plan or the RFC is drift.

### 4.4 Extensibility seams (project-wide policy)

Any subsystem with a plausible alternate backend lives behind an **interface + factory +
driver** pattern; drivers register via `init()` blank-import. V1 mandates this for:

- the **`gateway`** intelligence seam — `bifrost` + `mock` (P5, D-003);
- the **`store`** — `postgres` only in V1 (the seam exists for future backends; SQLite is
  explicitly out of scope, D-004). *This is Chartworks' own durable state — see the
  D-004 note distinguishing it from the customer data sources it queries.*
- the **auth issuer** — `self_issue` and `external_issuer`, selectable and combinable
  (D-006);
- the **telemetry/events** emitter;
- any **data-source / warehouse adapter** the RFC introduces (the predecessors carried
  Databricks/Postgres/BigQuery/Snowflake adapters — the seam shape is inherited, the V1
  driver set is RFC-owned).

A **vector-search seam** is *conditionally* mandated: if the RFC's routing/retrieval needs
vector search, it ships behind a seam with a single V1 driver, exactly as the store does.
It is not carried speculatively.

A seam with a single V1 driver still ships as a seam: the interface is the contract, so a
second driver never means a rewrite.

**CGo posture — CGo-free by default (D-005).** Unlike the sibling Soundings (which took a
deliberate CGo exception for its extraction library), Chartworks is **CGo-free**:
`CGO_ENABLED=0`, single static binary, no C toolchain prerequisite. A genuine CGo need
would **reverse** this posture and therefore requires its **own decision entry** before any
CGo dependency enters the tree — never a silent `CGO_ENABLED=1`.

---

## 5. Code conventions (Go)

- **Toolchain.** Go 1.26, pinned in every `go.mod`. Module path
  `github.com/hurtener/chartworks` *(placeholder pending the kickoff interview, D-002)*.
- **CGo posture.** CGo-free by default (§4.4, D-005). `CGO_ENABLED=0` builds a single
  static binary; a new CGo dependency needs a decision entry that reverses D-005.
- **Style.** `gofmt -s`; `go vet` and `golangci-lint run` clean. Generated code is marked
  with a `// Code generated … DO NOT EDIT.` header and stays boring and readable.
- **Errors.** `errors.Is`/`errors.As`, `%w` wrapping, sentinel errors, `errors.Join`. Wrap
  with context. **Never `panic` for control flow** and never panic across the API or MCP
  boundary.
- **Context.** `context.Context` is the first parameter of any call that does I/O, blocks,
  or can be cancelled. Honour cancellation.
- **Logging.** `log/slog` only — no `log.Printf`, no `logrus`/`zap`. JSON handler in
  production, text in dev. No unredacted secrets, no token bytes, no customer data rows in
  logs.
- **Concurrency.** Race detector mandatory on tests. A reusable artifact (a server, a
  store, a gateway driver, a pipeline stage) must be safe under concurrent use; prove it.
  Per-request state lives in `ctx` and parameters, never receiver fields; shared instances
  are immutable after construction. The frozen per-request identity envelope (P2) is
  constructed once and never mutated.
- **Tests.** Table-driven where it fits; golden tests for prompt/contract output; `-race`
  always.
- **JSON.** Stdlib `encoding/json` (v1).

---

## 6. The non-negotiable product rules

These enforce P1–P7 (§1). They are binding on every phase. Where a rule depends on a model
the RFC has not yet defined, the rule states the *principle* now and the RFC pins the
*mechanism*.

- **Deny-by-default access, computed in the query path (P1 — mechanism RFC-pending).**
  Every read of a data source / dataset applies the caller's effective access as a
  restriction **inside** the query — never fetch-then-filter. An empty effective-access
  set short-circuits — no query. A store or data-source query method without a scope
  parameter is rejected in review. The precise access primitive is RFC-owned.
- **SQL-generation safety (P1 — Chartworks-specific, RFC must define).** Generated SQL is
  untrusted until validated. The RFC defines the binding safety property: **read-only
  execution** (no DDL/DML/side effects), **schema allowlisting** (a query may only touch
  tables/columns the semantic model and the caller's grants expose), and **injection
  guardrails** (parameterization / canonicalization, never string-concatenated identifiers
  from model output). No phase executes generated SQL against a real data source before
  this property is settled and implemented; validation failure is a typed error (P4), never
  a silent skip-to-execute.
- **Token-carried identity (P2).** Both surfaces validate the same token the same way:
  asymmetric-only, `iss` matches, `aud` mandatory and per-instance, `exp` enforced, JWKS
  past `jwks_max_stale` fails **closed**. Identity/access are read from the validated claim
  into the frozen envelope — never from headers, never recomputed. Service-to-service calls
  (rare) use a `svc:` identity model, never a bespoke shared secret.
- **Tenant isolation (P3).** The tenant predicate is applied at the storage/index layer on
  every read and write of Chartworks' own state, and on every query to a customer data
  source. No unscoped query API exists.
- **Fail loud (P4).** No degraded-on-failure without a loud signal; an upstream `403` is
  surfaced, never swallowed; a dead filter that silently excludes everything is a bug; an
  unvalidated query is never executed as a fallback. Every access branch increments a
  metric and emits a structured, content-free log, surfaced only through an admin
  diagnostic — never into a tool result or a user-facing string.
- **One intelligence seam (P5).** No provider SDK or provider HTTP request outside
  `internal/gateway`. Any embedding model + dimensions are pinned per index and validated
  at boot; a model change is an explicit reindex, never silent. Every gateway call is
  metered (tokens, cost). SQL and structured routing outputs use schema-constrained
  generation.
- **Domain vocabulary (P6 — list RFC-pending).** Wire/field/UI/error/log strings use domain
  terms only. Pengui (or the eventual consumer) owns any external id map in **one** place;
  Chartworks receives ids and never mirrors, "wires", or "repairs" them. The authoritative
  forbidden-word list lands with the RFC, informed by the predecessor diff brief.
- **One logic core, thin surfaces (P7).** Every capability is implemented once in the
  core/service layer; `sdk/chartworks`, `internal/api` (HTTP), and `internal/mcpserver`
  (MCP) are thin callers, and a capability's side effects (validation, audit, events, cache
  invalidation) live in the core so no surface can omit them. A new capability ships on all
  of its tier's surfaces in the same PR with a parity test (MCP included).
- **Outputs are first-class, normalized shapes.** Routing evidence, generated SQL, result
  previews, and chart specs are returned in normalized, provider-agnostic shapes — never a
  bespoke per-caller format. Tools return **typed error results**, never a raised stack
  trace, and never leak internal endpoint URLs, index names, warehouse credentials, or
  model details into a result the model or user sees.
- **The Store schema is budgeted.** A table or column outside the RFC's schema inventory
  requires an RFC amendment first (guardrail against sprawl) — the same discipline that
  keeps the predecessors' "id in seven columns across six tables" sprawl from recurring.

---

## 7. Security — non-negotiable rules

- No hardcoded secrets, anywhere — including tests. Config secrets use `env.VAR`
  indirection and fail closed at boot. **Warehouse / data-source credentials** are secrets:
  never logged, never echoed into an error, never returned in a result.
- Asymmetric JWT only (RS/ES); `HS*`/`none` rejected at the parser. `aud` mandatory. Stale
  JWKS past the max-stale ceiling fails closed. Identity/access never ride `X-*` headers.
- Access scoping is enforced in the store / data-source query layer; handler-layer
  filtering is not a substitute. Access is intersected inside the query, never
  fetch-then-filter.
- **Generated SQL is executed read-only, against allowlisted schema, with injection
  guardrails (P1).** Until the RFC settles this property, generated SQL is not executed
  against real data.
- In self-issue mode, API keys are compared in constant time and never logged.
- HTTP transport: timeouts, body limits, Origin/Content-Type and cross-origin protections
  are set **explicitly** — never inherited from an SDK default.
- The HTTP audience may be a distinct `aud` from the MCP audience, so a UI token cannot be
  replayed as an agent token. No unauthenticated endpoints except `/healthz` / `/readyz`.
- Audit records (source connection, model/query changes, access denials) are
  **content-free** — ids + principals + decision, never data rows, never token bytes.
  Redaction profiles apply before any gateway call.

---

## 8. Observability — the rules

- Metrics: per-access-decision counters (§6), ingest/engineering throughput and latency,
  NLQ routing and SQL-generation latency, execution latency, cache hit rates, and the
  standard RED metrics. Prefer Prometheus/OTel so it composes with the ecosystem's
  telemetry.
- Health: `/healthz` + `/readyz` (ready = JWKS fetched + Chartworks' own storage reachable;
  data-source reachability is reported, never a boot gate). Pengui derives
  "connected/healthy" **live** from this; it persists only the *intent* to connect.
- A **scope-debug diagnostic** is the sanctioned way to answer "why didn't the caller see
  data X / route to topic Y" — admin-only, read-only, never a user-facing "repair" surface
  (the exact anti-pattern the predecessors grew).
- OTel export is an adapter behind the telemetry seam, off by default; it is never a
  prerequisite to observe locally.

---

## 9. Persistence — the `Store` seam rules

- All durable state goes through the `Store` interface. **V1 ships exactly one driver:
  `postgres` (pgx/v5) (D-004).** SQLite / an embedded store is **explicitly out of scope** —
  do not spend effort on it; the seam exists for a *future* backend, not a second V1
  driver.
- **Chartworks' own store is distinct from the customer data sources it queries (D-004).**
  The `Store` seam holds Chartworks' state (semantic models, jobs, sessions, audit, etc.).
  The **customer data warehouses** Chartworks reads to answer NLQ are a *separate* concern,
  reached through data-source adapters (§4.4), read-only and access-scoped — never mixed
  into the `Store` seam. The RFC pins this boundary; do not conflate the two.
- Local development and CI run against a **Docker Postgres** (`make pg-up`); there is no
  in-memory shortcut that bypasses the real store for integration tests.
- A new persistence concern adds a method to the seam and is covered by the store
  conformance suite. Migrations are forward-only; never edit a migration after it merges.

---

## 10. The gateway seam rules (P5)

- `internal/gateway` is the only package that knows provider wire formats. The Bifrost
  driver wraps `github.com/maximhq/bifrost/core` (D-003); a `mock` driver backs every test
  that must not call a paid API — paired with at least one recorded-fixture test against
  the real wire format.
- Any embedding model + dimensions are pinned per index and validated at boot; a model
  change is an explicit reindex operation, never silent.
- Every gateway call is metered (tokens, cost) and surfaced to telemetry.
- Structured outputs (routing decisions, generated SQL, chart specs) use
  JSON-schema-constrained calls; free-text JSON parsing of model output is forbidden.

---

## 11. Testing rules

- `-race` on every test run. CI fails on a race.
- API contracts and routing / SQL-generation / chart-spec output are covered by **golden
  tests** (fixed input → fixed output).
- A phase that consumes another subsystem's surface, or closes a cross-subsystem seam,
  ships an **integration test** with real drivers against the Docker Postgres — see §17.
  The gateway `mock` driver is the one sanctioned boundary mock (pair it with a
  recorded-fixture test against the real wire format).
- Coverage defaults (override per phase): 80% new packages; 85% the `store` driver, the
  `auth`/access packages, and conformance-tested subsystems; 70% CLI / tooling. **The
  bands are a mechanical gate** (`make coverage`); a regression, or a new package with no
  configured threshold, fails the build. A band genuinely unreachable hermetically gets a
  documented override (class + reason) and a decision entry — never a silent lowering.
- The **access and SQL-safety paths carry adversarial tests**: a cross-tenant probe, an
  empty access set, a forged-header attempt, a fetch-then-filter regression guard, and —
  once SQL execution exists — a write/DDL-injection and a schema-escape probe are standing
  test obligations, not optional.
- Prime parse/decode surfaces (JWT, NLQ payloads, generated-SQL validation) carry Go
  `FuzzXxx` **fuzz targets** with a seed corpus and an asserted invariant; the corpus runs
  as an ordinary CI test. Hot reusable artifacts carry `BenchmarkXxx` **benchmarks**
  (`make bench` — a baseline, not a CI gate).

---

## 12. Commit and PR conventions

- **Commits:** imperative mood, scoped (`feat(nlq): …`, `fix(exec): …`, `chore: …`,
  `docs: …`). Small and coherent. Commits are **unsigned** in this repository
  (`commit.gpgsign=false` is set locally; do not enable signing). Author with the personal
  GitHub identity, never a work email.
- **Branches:** never commit feature work directly to `main`; use `feat/phase-NN-*` (or
  `chore/*`, `docs/*`). Once past scaffolding, do not modify `main` directly — use a
  worktree or branch.
- **PRs:** reference the RFC section(s) (or the bootstrap / consumer-request section, until
  the RFC lands) and the phase. State any plan deviation and update the plan in the same
  PR. The pre-merge checklist (§14) gates the PR.
- **Merge:** squash unless history is meaningful. CI green is mandatory.

---

## 13. Forbidden practices

- Hardcoded secrets, including in tests; logging warehouse / data-source credentials.
- `panic` for control flow; panicking across the API or MCP boundary.
- Copying or vendoring code/files from either Python predecessor; reading across from
  `_ref/` into this tree; naming a predecessor by its product name (refer to "the client
  predecessor" and "the generalistic predecessor").
- Carrying identity/access in `X-*` headers (violates P2).
- Fetch-then-filter access, or any store / data-source query API without a scope parameter
  (violates P1/P3).
- Executing generated SQL that is not read-only, schema-allowlisted, and injection-guarded
  (violates P1 SQL-safety); executing it at all before the RFC settles that property.
- Importing a provider SDK or building provider requests outside `internal/gateway`
  (violates P5).
- Symmetric or `none` JWT; accepting a token without a validated `aud`.
- Plumbing vocabulary on the wire or in the UI/error/log surfaces (violates P6).
- Spending effort on a SQLite/embedded store driver (out of scope — D-004).
- Adding a CGo dependency without a decision entry reversing the CGo-free posture (D-005).
- Free-text JSON parsing of model output (§10).
- Silent degradation, dead filters, empty-catch (violates P4).
- Adding a CLI command, endpoint, MCP tool, or config key without a smoke check in the same
  PR.
- Editing a migration after merge.
- Bypassing the pre-commit hook with `--no-verify` outside a documented emergency.

---

## 14. Pre-merge checklist

- [ ] `make drift-audit` passes.
- [ ] `make check-mirror` passes (`AGENTS.md` == `CLAUDE.md`).
- [ ] `make preflight-full` passes (the full sweep — `PREFLIGHT_FULL=1`, every phase's
      smoke; the same gate CI runs. The pre-commit hook's fast `make preflight` runs only
      changed phases, so confirm the full sweep before merge).
- [ ] `go test -race ./...` and `golangci-lint run` are clean.
- [ ] All cross-references (`RFC §X.Y`, bootstrap / request-doc `§X`, `D-NNN`, `brief NN`)
      resolve.
- [ ] Coverage on touched packages ≥ the phase's stated target — `make coverage` passes (a
      new package is added to the coverage config in the same PR).
- [ ] A new CLI command / endpoint / MCP tool / config key has a smoke check in this PR.
- [ ] If a reusable artifact changed: a concurrent-reuse test passes under `-race`.
- [ ] If an access/auth/SQL-safety path changed: the adversarial test obligations (§11)
      still pass.
- [ ] If a cross-subsystem seam was opened or consumed: an integration test against the
      Docker Postgres exists (§17).
- [ ] New vocabulary added to `docs/glossary.md` in this PR.
- [ ] A new architectural decision (or a departure from a brief / the bootstrap doc) is
      filed in `docs/decisions.md`.

---

## 15. When in doubt

The RFC wins; until it exists, the bootstrap doc + `docs/decisions.md` do (and, once the
kickoff produces one, the consumer request). If all are silent, the phase plan decides; if
that too is silent, raise it — do not invent a decision and bury it in code. A new settled
decision is an entry in `docs/decisions.md`; a change to a settled decision is an RFC (or
bootstrap/request-doc) PR plus a superseding decision entry, never a silent edit.

---

## 16. Authoring a phase plan (workflow)

The canonical workflow for any contributor starting a phase. The drift-audit gate enforces
what it can; this workflow covers what it can't.

1. **Read the master plan entry.** Open `docs/plans/README.md`, find the Phase N detail
   block. Note owning subsystem, RFC/bootstrap sections, dependencies, risks.
2. **Read the cited RFC/bootstrap sections.**
3. **Read the relevant briefs** per `docs/research/INDEX.md`. A phase plan that cites no
   informing brief is a drift signal. (The phase-0 predecessor diff brief is mandatory
   before any NLQ/topic-lifecycle phase.)
4. **Read the glossary** for any term you're unsure about; pre-write the entry for any new
   term you introduce.
5. **Read the decisions log** (`docs/decisions.md`) for entries touching this subsystem.
   Settled decisions are not re-litigated silently.
6. **Copy the template:** `cp docs/plans/_template.md docs/plans/phase-NN-slug.md`. Fill
   every section. "Brief findings incorporated" and "Findings I'm departing from" are
   forcing functions — they make inheritance visible.
7. **Author the smoke skeleton:** `cp scripts/smoke/_template.sh scripts/smoke/phase-NN.sh`.
8. **Run `make drift-audit` and `make preflight`** before committing.
9. **Commit only when both pass.** The PR references the RFC/bootstrap section and any
   superseded decision.

---

## 17. End-to-end + integration testing

Per-package unit tests miss two classes of bug: **cross-package wiring gaps** (two phases
each ship their half of a seam, neither connects them) and **cross-subsystem concurrency
interactions**.

A phase ships an integration test whenever its `Deps` name a different subsystem's shipped
phase, or it closes a seam another phase opened, or it introduces a public interface other
phases will build on. Integration tests use **real drivers** on the seam — a real Docker
Postgres for the store, a real token for auth (no mocks at the boundary; the gateway `mock`
driver is the one sanctioned exception, paired with a recorded-fixture test). They prove
identity/scope propagation, cover ≥1 failure mode, and run under `-race`. They live
in-package when the package *is* the wiring boundary, otherwise in `test/integration/`.

At wave boundaries a read-only **checkpoint audit** reviews every shipped phase for wiring
gaps, RFC drift, weak tests, and hygiene regressions, and lands its punch list as one
`chore(checkpoint)` PR. When an integration test surfaces a bug, fix it in the same PR —
even when the root cause is in an earlier phase. A **live-verification gate** (real provider
models via `.env`, never CI-required — D-010) is a standing wave-end check.

---

## 18. Mirroring

`AGENTS.md` and `CLAUDE.md` are kept **verbatim identical**. After any edit:

```bash
diff -q AGENTS.md CLAUDE.md   # expected: no output
```

CI enforces this; the `mirror` job fails the build if they differ.
