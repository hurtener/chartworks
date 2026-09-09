# Chartworks — Decisions Log

> Append-only log of settled architectural decisions. Each entry is `D-NNN`, a status,
> the decision, and its rationale. **Do not re-litigate a settled decision silently** — a
> change is a new superseding entry (and, once the RFC exists, an RFC PR), never an
> in-place edit. Grep here before reopening a question.
>
> Status vocabulary: **accepted** (binding) · **superseded-by-D-NNN** · **proposed**
> (recorded, not yet binding).
>
> **Bootstrap note.** Chartworks is a new repo. The seed entries D-001…D-010 were
> inherited from the sibling Soundings build as *proposed*; the **kickoff interview
> (2026-07-06)** flipped them to *accepted* (D-007 superseded by D-011) and filed the
> interview-born decisions D-011…D-018 below. Product-specific decisions continue from
> there as the RFC and phases land.

---

### D-001 — Clean-room Go rewrite, two predecessors, never copied · *accepted (kickoff interview, 2026-07-06)*

Chartworks is a clean-room redesign of two Python predecessors — **the client
predecessor** (`_ref/original_wayfinder/`, the client-tailored original) and
**the generalistic predecessor** (`_ref/forked_wayfinder_explorer/`, the "Explorer" fork)
— rethought Go-native, the way Harbor and Soundings were rethinks rather than
translations. **No code or files are copied or vendored** from either; `_ref/` is
gitignored and never read across into the tree. The predecessors are referred to only as
"the client predecessor" and "the generalistic predecessor." Ideas are inherited through
`docs/research/` briefs, and a dedicated **diff brief** (client fixes vs. the fork,
especially the topic lifecycle) is a mandatory phase-0 artifact.

**Why:** the Soundings build proved a clean-room rewrite keeps the good bones (here, the
router-first semantic core and the topic lifecycle) and sheds the monolith scars a port
would carry. Two predecessors sharing most functionality make the diff brief the single
cheapest way to inherit the *right* half of each.

---

### D-002 — Product name & family seat · *accepted (kickoff interview, 2026-07-06)*

The product is **Chartworks** (repo `chartworks`, module
`github.com/hurtener/chartworks` — a **placeholder pending the kickoff interview**), the
ecosystem's **Explorer seat**: structured-data analytics (engineer → model → NLQ-to-SQL →
charts), a peer to Portico / Harbor / Dockyard / Stowage / Soundings. "Wayfinder" and
"Explorer" were the predecessors' names; in the family, the seat is **Chartworks**.

**Why:** the family names products by nautical/structural codename, not by function;
consistency keeps the ecosystem legible. The module path mirrors the sibling convention
but is not yet load-bearing, so it stays a placeholder the kickoff confirms.

---

### D-003 — One intelligence seam, Bifrost driver · *accepted (kickoff interview, 2026-07-06)*

Every embedding / LLM / rerank / SQL-generation call goes through the `internal/gateway`
seam (interface + factory + driver). V1 drivers: **`bifrost`** (wrapping
`github.com/maximhq/bifrost/core`, as Stowage and Soundings do) and **`mock`** (tests). No
package outside `internal/gateway` may import a provider SDK or build a provider HTTP
request. Structured outputs (routing, generated SQL, chart specs) are schema-constrained;
free-text JSON parsing of model output is forbidden.

**Why:** the ecosystem standardises LLM access on Bifrost; a single seam keeps the binary
model-free at every other layer and makes provider swaps a driver change. The predecessors
scattered model calls across services (DSPy, module overrides) — the exact sprawl the seam
prevents.

---

### D-004 — Postgres-first for Chartworks' OWN store; SQLite dropped; customer data sources are separate · *accepted (kickoff interview, 2026-07-06)*

Durable Chartworks state goes through a `Store` seam. **V1 ships exactly one store driver —
`postgres` (pgx/v5).** A **SQLite / embedded store is explicitly out of scope**; do not
spend effort on it. The seam exists for a *future* backend, not a second V1 driver. Local
development and CI run against a **Docker Postgres** (`make pg-up`); integration tests hit
the real store, with no in-memory shortcut.

**New concern the RFC must distinguish (absent in Soundings):** the **customer data
sources** Chartworks *queries* to answer NLQ (Databricks / Postgres / BigQuery / Snowflake
in the predecessors) are **not** the same as Chartworks' own `Store`. They are reached
through read-only, access-scoped **data-source adapters** behind their own seam; the RFC
pins the boundary between "Chartworks' state" and "the warehouse it reads." A
vector-search backend, if the RFC's routing needs one, ships behind its own seam with a
single V1 driver — not carried speculatively.

**Why:** full coverage of one production-grade store beats a half-covered portable one, as
the Soundings build found. The store-vs-warehouse split is genuinely new here and, left
implicit, is the most likely source of a tenant-isolation or credential leak — so it is
called out at bootstrap rather than discovered in a phase.

---

### D-005 — CGo-free posture (deliberately reversing the Soundings exception) · *accepted (kickoff interview, 2026-07-06)*

Chartworks is **CGo-free**: `CGO_ENABLED=0`, a single static binary, no C toolchain
prerequisite — the strict posture of Portico / Harbor / Dockyard / Stowage. This
**deliberately reverses** Soundings' D-005 CGo exception, which existed only because that
product's extraction library required it; Chartworks has no equivalent dependency. Any
future CGo need is a **new decision entry** that reverses this posture — never a silent
`CGO_ENABLED=1`.

**Why:** structured-data analytics has no in-process native dependency forcing CGo, so the
default reclaims the single-static-binary guarantee the family prefers. Recording the
reversal here stops a contributor from assuming Soundings' exception transfers.

**Kickoff nuance (2026-07-06):** CGo-free is the *preferred default*, not dogma — if a
dependency materially better on performance or development simplicity requires CGo, the
reversal is on the table; it still requires its own decision entry, never a silent flip.

---

### D-006 — Asymmetric-JWT dual-mode auth · *accepted (kickoff interview, 2026-07-06)*

Auth uses **asymmetric JWT only** (RS/ES `256|384|512`); `HS*`/`none` are rejected at the
parser before any business logic. Two modes ship from one binary and are combinable:
**`self_issue`** (Chartworks mints its own tokens for API-key/standalone callers) and
**`external_issuer`** (it validates Pengui-issued tokens as an ecosystem citizen). `aud` is
mandatory and **per-instance**, with **dual audiences** (a distinct MCP audience and HTTP
audience) so a UI token cannot be replayed as an agent token. JWKS past `jwks_max_stale`
**fails closed**. The validated claim is read once into a **frozen per-request identity
envelope** and never mutated. **If Chartworks carries an ACL-style access claim** (as
Soundings does), it lives in that same claim contract — but whether access is claim-carried,
grant-scoped by data source/dataset, or both is **RFC-owned** (see CLAUDE.md P1).

**Why:** this is the ecosystem's inherited auth invariant; the Soundings build validated
the dual-mode, dual-audience, fail-closed-JWKS, frozen-envelope shape end to end. It
transfers wholesale; only the *access* half is product-specific and deferred to the RFC.

---

### D-007 — MCP surface on `mark3labs/mcp-go` unless Dockyard becomes consumable · *superseded-by-D-011 (kickoff interview, 2026-07-06)*

The MCP tool surface (`internal/mcpserver`) is built on **`github.com/mark3labs/mcp-go`**,
which supplies streamable-HTTP, stdio, in-process sessions, and the context/middleware
seams a one-binary dual surface needs. Dockyard (the ecosystem's MCP Apps framework) is the
preferred choice **if it becomes consumable as a Go dependency**; at the Soundings build it
was not (absent from the module graph; its tooling scaffolds standalone UI-embedded
servers, architecturally mismatched with an in-process goroutine-group MCP server).

**Why:** an unavailable sibling framework cannot gate the contract — the contract is the
tool set + typed error results + JWT validation + access, not the framework. Re-check
Dockyard's availability at kickoff; if it now ships as a library, this decision is
superseded.

---

### D-008 — CLI on stdlib `flag`; cobra rejected · *accepted (kickoff interview, 2026-07-06)*

The `chartworks` CLI (including any `admin` command family) uses stdlib `flag` with
hand-rolled subcommand dispatch, not cobra.

**Why:** the command surface is small and closed and the `run(args, stdout, stderr) int`
dispatch is already testable; the family's minimal-dependency posture does not justify
cobra's tree. The Soundings build reached the same conclusion at its breadth point.

---

### D-009 — Preflight fast/full + coverage-band + drift-audit as the standing quality machinery · *accepted (kickoff interview, 2026-07-06)*

The standing quality gates are: a **preflight** gate in two modes (fast — changed phases
only, for the pre-commit hook; full — every phase's smoke, for CI and pre-merge); a
**mechanical coverage-band** gate (80% new packages / 85% store, auth, access &
conformance-tested subsystems / 70% CLI-tooling, a regression or unbanded package failing
the build); and a **drift-audit** (RFC/plan/brief cross-references, the AGENTS↔CLAUDE
mirror, forbidden-name scan). Each phase ships a smoke script; a new CLI command / endpoint
/ MCP tool / config key ships its smoke check in the same PR.

**Why:** this machinery carried the Soundings build across eighteen phases without silent
drift; it is product-agnostic and transfers directly. It is the cheapest insurance against
the "green CI, broken product" failure D-010 guards.

---

### D-010 — Live-verification gate as a standing wave-end check · *accepted (kickoff interview, 2026-07-06)*

A **live-verification gate** runs the real pipeline against real provider models (via a
local `.env`, e.g. an OpenRouter key through the `bifrost` driver) at each wave boundary.
It is **never CI-required** (no secrets in CI) but is a standing pre-wave-close check.

**Why:** on the Soundings build this gate caught a silent-degrade the entire mock-backed CI
suite passed over (the real engine could not run with the inputs the fake one accepted) —
the exact "every gate green, production broken" failure P4 exists to prevent. For
Chartworks, whose SQL-generation and routing are model-driven, a live check against real
models is even more load-bearing.

---

### D-011 — Dockyard is consumable as a Go library; MCP-surface library choice is RFC-owned · *accepted (kickoff interview, 2026-07-06)*

Supersedes D-007's premise: at kickoff the user confirmed **Dockyard was and is
consumable as a Go dependency** (its rejection on the Soundings build was the prior
agent's call, not a hard constraint). The Chartworks MCP surface library —
`mark3labs/mcp-go` (the Soundings-proven path) vs. Dockyard — is therefore **re-opened
and decided in the RFC**, after a dedicated research brief evaluates Dockyard's fit for
an in-process, one-binary dual-surface server. The contract is unchanged either way:
the tool set + typed error results + JWT validation + access (P7 thin-surface rule).

**Why:** the D-007 rationale ("an unavailable sibling framework cannot gate the
contract") no longer applies; using the family's own MCP framework has ecosystem value
if its architecture fits, and that is an evidence question for a brief, not a default.

---

### D-012 — Three consumer classes; a derived consumer-request doc · *accepted (kickoff interview, 2026-07-06)*

Chartworks is consumed exactly as Soundings is: **Pengui Console (HTTP)**, **Harbor
agents (MCP)**, and **standalone API customers** (self-issue mode). A **consumer-request
doc** (the analog of Soundings' Pengui request doc) is derived at bootstrap as an
interview output and fills slot 5 of the CLAUDE.md §2 authority chain. The
Soundings/Chartworks boundary stands as drawn — unstructured documents (including a
scanned PDF of a table) are Soundings' domain; structured data, **including full
warehouse consumption** (not just CSV/XLSX upload), is Chartworks' domain.

**Why:** the dual-surface, three-consumer posture is the ecosystem shape; deriving the
request doc now gives phases a contract anchor before Pengui's side exists.

---

### D-013 — V1 scope: full data-engineering stage + migrated NLQ core; charts/frontend deferred · *accepted (kickoff interview, 2026-07-06)*

V1 includes the **complete data-engineering stage** — source connectors (uploaded
CSV/XLSX/Parquet **and** warehouse connections), profiling/quality checks, schema
inference, transformations/modeling with materializations, dataset versioning/lineage,
and refresh scheduling — plus the **migrated NLQ-to-SQL core** with the topic-pack
semantic layer **kept and enhanced** (its lean, budget-friendly context-card engineering
is a deliberate crown jewel). **Charts and the frontend are deferred from V1** — the
priority is migrating everything else; a future session may re-create or adopt a
frontend. Normalized output shapes still reserve a chart-spec slot so deferral is not a
redesign. Depth is favored over delivery speed ("nobody is waiting for this to be mega
fast"). The competitive capability target: exceed Teramot-class GenBI offerings on top
of predecessor capabilities.

**Why:** the DE stage is the product's reason to exist beyond the predecessors; cutting
it would rebuild the predecessor instead of the successor. Charts are the family's most
replaceable layer (dataviz assets exist elsewhere) and the cheapest deferral.

---

### D-014 — Dual SQL-generation modes: gateway-generated and BYO-agent, one validation/execution core · *accepted (kickoff interview, 2026-07-06)*

The NLQ pipeline supports two generation modes: **(a) internal** — Chartworks generates
SQL through the `gateway` seam (the predecessors' mode), and **(b) BYO-agent** — the
calling agent (e.g. a ChatGPT- or Claude-backed agent outside our subscription) receives
the semantic context (topic pack / context cards / schema slice) from Chartworks,
generates the SQL itself, and submits it back; Chartworks **validates and executes** it
under exactly the same safety gates. Both modes converge on **one** validation +
execution core (P7 — no parallel paths); mode (b) never bypasses a check mode (a) runs.

**Why:** the predecessors force BYOK on every team wanting agents outside their
subscription — a real adoption blocker the user called out. Teramot proves the
context-handoff pattern; the safety property makes it viable: generated SQL is untrusted
regardless of who generated it, so the same validator serves both.

---

### D-015 — Access model: per-principal (and per-agent) deny-by-default ACL; concrete primitive RFC-owned · *accepted (kickoff interview, 2026-07-06)*

Multi-tenant from day one, as Soundings. The access primitive moves to **per-principal
ACL — and likely per-agent principals** — replacing the predecessors' model where the
*application* held the privileges and the only restriction lived at the key/agent level
("if you have access to the topic, the server has access to the data"). The DE stage
makes that model untenable: engineered datasets and write paths need finer grants than
topic visibility. The concrete primitive (claim-carried ACL like Soundings, data-source/
dataset grants, or both) is **RFC-owned** (P1).

**Why:** greenfield chance to fix the predecessors' coarsest security scar; deferring
the mechanism (not the principle) to the RFC keeps the decision evidence-based on the
briefs.

---

### D-016 — Customer data-source credentials: encrypted at rest in the Store (MVP) · *accepted (kickoff interview, 2026-07-06)*

Warehouse/data-source credentials are stored in Chartworks' own store **encrypted at
rest** (MVP posture); never logged, never echoed into errors or results (CLAUDE.md §7).
The RFC pins the mechanism (envelope encryption, key source, rotation) and keeps an
external secret-manager reference as a future seam, not a V1 driver.

**Why:** a concern Soundings never had; encrypted-at-rest is the proportionate MVP for a
single-binary product, with the seam leaving room for vault-class backends later.

---

### D-017 — Write-posture split: NLQ strictly read-only; the DE stage writes through a distinct governed path · *accepted (kickoff interview, 2026-07-06)*

Chartworks **does write into client warehouses** — that is the point of the
data-engineering stage (materializations, engineered datasets). The P1 SQL-safety
property therefore splits: the **NLQ path remains strictly read-only** (no DDL/DML ever,
schema-allowlisted, injection-guarded), while the **engineering stage owns a separate,
explicitly governed write path** — declared destinations, scoped credentials/grants,
audited operations — that NLQ-generated or BYO-agent SQL can never reach. The RFC
defines both halves as binding properties.

**Why:** "read-only everywhere" would amputate the product's new stage; "writes anywhere"
would gut P1. Splitting the posture by pipeline stage keeps both invariants honest and
reviewable.

---

### D-018 — Bruin is a candidate embedded engine, adopt-or-ditch in the RFC · *accepted (kickoff interview, 2026-07-06)*

**Bruin** (`github.com/bruin-data/bruin`, staged under `external_refs/bruin-cli`) is
evaluated as a candidate **engine** — not just an idea source — for both query execution
against customer warehouses and DE pipeline running (which could absorb significant
development). Unlike the predecessors, code-level dependency on Bruin is permissible if
adopted (it is OSS, not confidential); the brief evaluates library consumability,
license, connector coverage, CGo implications (D-005), and architectural fit. The RFC
makes the adopt/ditch call. The `ssr_analyst_analysis_DE_pipeline` internal draft is
mined the same pass as a non-validated first draft — take or ditch freely.

**Why:** a proven multi-warehouse execution/pipeline engine could collapse the largest
new subsystem's cost; but an engine that fights the seam architecture or the CGo/binary
posture would cost more than it saves. Evidence first, decision in the RFC.

---

### D-019 — MCP surface on `mcp-go` for V1; Dockyard re-evaluated at Wave 5 · *accepted (RFC-001 §11.1, 2026-07-06)*

Resolves the question D-011 re-opened, on brief 13's evidence. V1 builds
`internal/mcpserver` on **`mark3labs/mcp-go`**, pinned at the current upstream line
(v0.55.x; Soundings' v0.43.2 seams are shape-identical). Deciding factor: mcp-go's
global `WithToolHandlerMiddleware` makes the scope/grant gate **structural** — every
tool passes it; a missed wrap cannot exist — which is load-bearing for P1a. Dockyard's
runtime is confirmed importable and otherwise fit, but applies cross-cutting gates
per-registration by convention. Chartworks keeps its tool contracts Dockyard-portable
(Go structs as source of truth, one registration list, one package) and **re-evaluates
Dockyard at the Wave-5 boundary**.

**Why:** a structurally guaranteed access gate beats a conventioned one on the product's
most security-sensitive surface; the proven path also avoids being Dockyard's first
headless production consumer while shipping a new DE stage and dual SQL modes.

---

### D-020 — The access primitive: one grants relation, three grains, one resolver · *accepted (RFC-001 §5, 2026-07-06)*

Resolves D-015's mechanism. Access = `(tenant, principal, grain ∈ {source, topic,
dataset}, resource, permission ∈ {read, query, manage})`, deny-by-default, with tenant
roles (admin/member/viewer) as defaults and **agents as first-class principals**
(`agent:<id>` holds its own grants). Capability scopes ride the token and gate
operation families; grants live in Chartworks' store and gate resources; both must
pass. One resolver computes the caller's effective access into the frozen envelope
once per request; every store/warehouse query takes non-optional scope parameters;
empty set short-circuits. Per-decision metrics + the admin-only scope-debug diagnostic.

**Why:** dataset-grain is what the DE stage demands (a materialization grantable
independently of its topic); the shape synthesizes the client predecessor's grant data
model with the fork's unified resolver (brief 05) and closes the "topic access ⇒ data
access" scar (brief 04).

---

### D-021 — SQL-safety mechanism: three-stage AST validation ∩ grants, defense-in-depth read-only execution, split read/write interfaces · *accepted (RFC-001 §9.5–9.6, 2026-07-06)*

Concretizes P1b/P1c. Validation: minimal pre-parse (never duplicating parser judgment —
the CTE-regression lesson, brief 03) → dialect-aware AST parse → whole-tree statement
blocking + single-statement + allowlisting against **topic pack ∩ caller grants** +
join-graph reachability, all as typed error codes. Execution: `ValidatedSQL` is the
only executable type (an adapter cannot run a raw string); read-only
transaction/session enforcement at the adapter where the engine supports it;
server-side statement timeouts; cursor-level row caps (never LIMIT-by-wrapping). The
write path (`sources.Materializer`) is a distinct interface on declared destinations —
no shared entry point with a read/write flag. No regex injection heuristics presented
as controls.

**Why:** the predecessors' validator was strong but was the *entire* guarantee, and
callers could reach `execute()` without it (brief 02/04); defense-in-depth and
type-level unbypassability close that class.

---

### D-022 — BYO-agent mode: a published, versioned context bundle + `submit_sql` through the identical core · *accepted (RFC-001 §9.4, 2026-07-06)*

Mechanism for D-014. `get_query_context` returns a **published, versioned** bundle
(routing result, capability-contract slice, governance constraints *restated
explicitly*, dialect + SQL requirements, clarification slots, provenance-labeled prior
SQL as optional guidance). `submit_sql` runs the identical validation/execution core as
internal generation — the validator assumes an adversarial submitter; provenance
(`internal | byo`) is recorded end-to-end; context-read and submit are distinct
capability scopes. A standing parity test proves mode (b) cannot bypass a mode-(a)
check.

**Why:** brief 03's Q10 analysis — the bundle is a public contract the moment it ships,
and external agents can't be assumed to know unspoken governance; P7 forbids a second
validation path.

---

### D-023 — Bruin: mine ideas only; no engine dependency · *accepted (RFC-001 §7, 2026-07-06; resolves D-018)*

Bruin is not adopted as a library or CLI subprocess: its SQL-parsing core requires
CGo + a Rust FFI (or an embedded Python runtime), and ingestion delegates to a Python
tool — direct collisions with D-005's single-static-binary posture (brief 10). Its
pipeline/quality/lineage model is mined as design input for `internal/engineering`.
The standalone, pure-Go **`semantic-engine`** submodule remains a candidate for a
narrow post-V1 spike, as a separate decision.

---

### D-024 — Uploads are first-class sources via a managed Postgres upload workspace · *accepted (RFC-001 §7.4, 2026-07-06)*

CSV/XLSX/Parquet uploads load into a tenant-scoped, Chartworks-managed Postgres
database (the *upload workspace*) and register as datasets queried through the
standard `postgres` **data-source adapter** — same identity path, same grants, same
execution route as any warehouse. No DuckDB (its Go driver requires CGo — D-005
holds); no parallel "spreadsheet mode" (the predecessors' weak-auth parallel path is
the named scar, briefs 01/02/04). The workspace is customer-data territory reached via
the adapter seam, never the `store` seam (D-004 boundary preserved).

---

### D-025 — One generic leased job queue; no per-concern worker classes · *accepted (RFC-001 §3.3, 2026-07-06)*

All background work (profiling, topic generation, publishing, pipeline runs, refresh)
runs as typed handlers on one Postgres-leased queue (`FOR UPDATE SKIP LOCKED`, lease +
heartbeat + reclaim); the schedule dispatcher enqueues into the same queue. The
predecessors' 13 worker classes are the P7 counterexample (brief 01).

---

### D-026 — Charts V1: declarative spec + deterministic selector; no renderer, no LLM ranker · *accepted (RFC-001 §10, 2026-07-06)*

V1 emits `ColumnMetadata[]` + `ChartRecipe` + provenance envelope (brief 06's
recommended contract) and selects via the ported rules engine (slot binding + weighted
suitability + adaptive alternatives — pure Go, no model call). ECharts-style option
inflation never happens in Go; the LLM ranker is post-V1 and gateway-schema-constrained
when it comes (the predecessors' free-text JSON parse is the named anti-pattern).

---

### D-027 — Governed rules + proactive clarification are V1 scope (scoped down) · *accepted (RFC-001 §8.4, 2026-07-06)*

The client predecessor's governed business-rules and underspecification layers — the
largest capability gap vs the fork (briefs 03/05) — ship in V1 as: rule authoring with
structural validation and lifecycle (`proposed → active → retired`), a budgeted
injection lane with dropped/contradiction visibility, and topic-scoped clarification
patterns running before generation. Shadow evaluation and historical replay defer to a
late wave.

---

### D-028 — No in-process NLP library; no bundled language models · *accepted (RFC-001 §9.1, 2026-07-06)*

Span hints are lexicon-light Go; embedding retrieval and gateway models carry semantic
weight. The predecessors' spaCy EN/ES dependency (brief 01) does not transfer — it
would break D-005 and the single-binary posture for marginal routing gain.

---

### D-029 — `vindex` seam confirmed: pgvector, single driver, facet vectors only · *accepted (RFC-001 §3.2/§12, 2026-07-06)*

Routing needs vector retrieval over topic facets, so the conditional seam in CLAUDE.md
§4.4 is exercised: `internal/vindex` with one V1 driver (`pgvector`), scoped
`(tenant, topic, version)`, embedding model + dims pinned per index and validated at
boot.

---

### D-030 — No local user management; self-issue = API keys; admin bootstrap via CLI · *accepted (RFC-001 §4.3–4.4, 2026-07-06)*

Chartworks ships no password login, signup, invites, or header-exchange endpoints.
Ecosystem users arrive as Pengui tokens; standalone callers are API keys exchanged for
short-lived self-issued JWTs; first-admin provisioning is a local operator CLI action.
This deletes the predecessors' highest-severity scar (header-trust identity minting,
brief 04) by removing the surface entirely.

---

### D-031 — Eval strategy: golden + red-team CI gates, grounded-accuracy manual loop, live gate · *accepted (RFC-001 §16, 2026-07-06)*

Five golden suites (routing, SQL generation, validation incl. the standing CTE
fixture, chart selection, context budgets) and a red-team suite (injection,
schema-escape, cross-tenant, resource exhaustion, adversarial BYO submissions) gate CI
at a 0.85 pass threshold + zero criticals, on the mock/fixture path.
BIRD/Spider-informed accuracy benchmarking against the sample warehouse is a manual
loop scored as *grounded* generation; the live gate (D-010) blocks wave closes.

---

### D-032 — V1 warehouse driver set expanded: + `mysql`, `sqlserver`; dockerized-engine validation for the self-hostable class · *accepted (kickoff follow-up, 2026-07-06)*

User directive during planning: the V1 data-source driver set is **postgres, mysql,
sqlserver, bigquery, snowflake, databricks** — SQL Server and Postgres per the fork's
shipped adapters (brief 05), MySQL added. Both additions have official pure-Go drivers
(`go-sql-driver/mysql`, `microsoft/go-mssqldb`), so D-005 holds. **Validation strategy
by engine class:** the self-hostable engines (Postgres, MySQL, SQL Server) run the full
adapter conformance suite against **dockerized instances loaded with public datasets**
(Kaggle-class) locally and at wave ends — real-engine validation without cloud
credentials; the cloud warehouses (BigQuery, Snowflake, Databricks) validate
hermetically via recorded fixtures and fully via the live gate (D-010). Amends RFC-001
§6.1 (amended in the same planning PR).

**Why:** the predecessors' production coverage is the floor, not the ceiling; and the
self-hostable trio turns most of phase 14's risk (per-engine read-only posture, capping,
dialect fixtures) into cheap, repeatable local proof instead of live-gate-only evidence.

---

### D-033 — SQL parser: `cockroachdb/cockroachdb-parser` v0.25.2; fail-closed dialect posture; generation-side dialect obligation · *accepted (phase-09 planning, 2026-07-06)*

The validation core parses with **`github.com/cockroachdb/cockroachdb-parser` v0.25.2**
(pure-Go, D-005-clean; transitive `gosigar` pinned v0.14.4 for the darwin dev build) —
selected against a real six-dialect fixture corpus; vitess/tidb (MySQL-only) and
sqlglot (Python) rejected. No pure-Go multi-dialect parser exists, so the posture is
**fail-closed**: dialect-specific surface syntax the base grammar cannot represent is a
typed `parse.unsupported`, never a silent pass, plus a per-dialect escape blocklist.
**The generation-side obligation this creates (phase 18):** internal generation and the
BYO context bundle's SQL requirements target a **validated ANSI-conservative subset**
per engine; any mechanical post-validation surface rendering (quoting/limit forms) is
derived from the parsed AST, never string patching; the dockerized MySQL/SQL Server
engines (D-032) are the proving ground. If a construct cannot be expressed in the
subset for an engine, that is a typed limitation — never a validation bypass.

**Why:** the predecessors' multi-dialect validator was Python `sqlglot`; porting its
posture wholesale is impossible without breaking D-005. Fail-closed + a constrained
generation subset preserves P1b at the cost of dialect breadth — the right trade for a
security property, with the gap made visible and testable instead of implicit.

---

### D-034 — BYO `bundle_ref`: a stateless, signed, TTL-bounded context handle that pins context, not capability · *accepted (phase-19 planning, 2026-07-06)*

`get_query_context` returns a `bundle_ref` that is **stateless** (no store table — §12
budget preserved), **asymmetrically signed**, **TTL-bounded** (`nlq.bundle_ttl`,
default 15m), **principal-bound**, and **reusable within its TTL** (iterate-then-submit
is legitimate). Critically it pins **context, not capability**: grants, topic health,
and publication state are re-resolved live at every `submit_sql` — a revoked grant
fails loud regardless of a valid bundle. Answers brief 08's URL-path-scope
anti-pattern directly.

---

### D-035 — Warehouse-driver and upload-parser version pins (convention-8 verified) · *accepted (phase-11/14 planning, 2026-07-06)*

Pinned against real release assets: `go-sql-driver/mysql` v1.10.0 ·
`microsoft/go-mssqldb` v1.10.0 · `cloud.google.com/go/bigquery` v1.77.0 ·
`snowflakedb/gosnowflake` v1.19.1 — **CGo-free only with `-tags minicore_disabled`,
which is therefore a mandatory build tag** (a silent default build links a CGo probe —
the D-005 hazard made explicit) · `databricks/databricks-sql-go` v1.13.0 ·
`xuri/excelize/v2` v2.11.0 · `parquet-go/parquet-go` v0.30.1 (uploads; CSV is stdlib).
A version bump re-runs the conformance suite; a driver that loses its pure-Go property
at a bump is a D-005 event requiring its own decision.

---

### D-036 — Bruin adopted as the DE write-path executor (CLI subprocess, narrowed scope) · *accepted (planning review with user, 2026-07-06; narrows-and-supersedes D-023)*

The data-engineering **write path** executes through **Bruin** (`bruin-data/bruin`,
Apache-2.0, pinned **v0.11.666**) as a **CLI subprocess behind a `PipelineRunner`
seam** — not a library import. Chartworks' declarative pipeline definitions render to
Bruin's pipeline format at run time; Bruin executes transformations, materializations
(create+replace, delete+insert, append, **merge/incremental**, time_interval,
scd2 — per-engine support varies; Databricks lacks merge), and quality checks;
Chartworks consumes `bruin validate -o json` and `bruin lineage -o json`
(machine-readable, spike-verified) and wraps runs with its own audit/metering.
**Hard V1 rules:** SQL-only assets — never Python/R assets and never `ingestr`
ingestion (FSL-1.1 license + Python runtime; spike-verified that SQL-only runs need no
Python); credentials injected via `.bruin.yml` `${ENV}` interpolation or a
custody-rendered tmpfs config — plaintext never lands on persistent disk; Bruin's
telemetry is disabled, and **confirming the real disable mechanism is an
implementation blocker** for phase 13; the prebuilt binary is glibc-dynamic — the
reference image uses a glibc base (debian-slim class), never bare musl/Alpine.
NLQ **reads never route through Bruin** — they stay on Chartworks' native adapters
(D-038). Brief 11's orchestration keepers (blind planner, whitelist composition,
contract-before-materialize) remain Chartworks-owned; Bruin is the executor, not the
designer. Daily upstream release cadence ⇒ strict pin + conformance re-run per bump
(D-035 discipline).

**Why:** D-023 rejected *engine adoption* (library import: CGo/Rust build, Python
ingestion, plaintext config) — every one of those objections dissolves in the narrowed
shape, verified against real release artifacts. It buys the most expensive halves of
the DE stage (incremental/merge strategies, quality checks, column lineage, write-side
engine breadth) at the cost of one pinned subprocess boundary.

---

### D-037 — CGo-free posture relaxed: the container is the deployment unit · *accepted (user directive, 2026-07-06; supersedes D-005 per its own reversal clause)*

The primary deployment target is a **container Chartworks controls** ("this will be
mostly dockerized"). Consequences: (a) the deployment unit is the **image**, which may
carry additional pinned binaries (Bruin, D-036) and dynamic linkage; the
single-static-binary property is demoted from invariant to *preference for the
`chartworks` binary itself*; (b) **CGo is permitted** in the `chartworks` binary when a
dependency genuinely earns it — recorded per-dependency in a decision entry, never a
silent flip. As of this entry the binary remains CGo-free (no current dependency needs
it: the adopted parser drivers are pure Go, D-038). Bare-metal/standalone deployment
remains supported: the binary runs everywhere; DE pipeline execution requires `bruin`
on PATH and degrades to a typed "pipeline execution unavailable" otherwise (P4 — loud,
never silent).

---

### D-038 — Read-side SQL validation: layered engine-side enforcement + a parser seam; native-dialect generation · *accepted (planning review with user, 2026-07-06; supersedes D-033's generation-subset obligation)*

P1b on the NLQ read path is enforced in layers, none of which depends on an immature
parser:

1. **Read-only credentials/sessions are the primary read-only mechanism.** Each
   data-source connection is provisioned SELECT-only where the engine supports it
   (documented per driver); the connection test asserts the posture (attempts a write,
   expects engine denial). The engine enforces read-only in its own dialect against
   any SQL whatsoever.
2. **Engine dry-run/EXPLAIN is the dialect-true validator.** Before execution, the
   candidate SQL is dry-run (BigQuery dryRun — returns referenced tables explicitly)
   or EXPLAIN'd (Postgres/MySQL/Snowflake/Databricks; SQL Server showplan/
   `sp_describe_first_result_set`) under the read-only credential; syntax errors are
   typed failures, and the **referenced-table set is checked against topic ∩ caller
   grants** before the real run. Table-grain allowlisting is thereby guaranteed on
   every engine by the engine's own parser.
3. **Client-side AST validation rides a parser seam** (interface + factory + driver,
   §4.4): driver `crdb` = `cockroachdb-parser` v0.25.2 (hardened, postgres-family —
   covers postgres sources and every upload workspace; D-033's pin retained); driver
   `sqlglotgo` = **`jonathan-fulton/sqlglot-go` v0.4.0** (MIT, pure Go, all six
   dialects, walkable AST + statement classification + transpile; the only genuine Go
   sqlglot port — `tensafe/sqlglot-go` was evaluated and rejected: fingerprinting
   tool, parser unimplemented). sqlglot-go adoption is **per-dialect and
   evidence-gated**: a dialect flips to it only after its conformance claims are
   independently reproduced against the phase-09 fixture corpus; it is 4 weeks old
   with bus factor 1, so a fork under our org is the anticipated endgame if it proves
   out. *(Phase-09 verification addendum, same day: its go.mod declares a hyphen-less
   module path (`jonathanfulton/...`) that resolves to no repo — consumption requires
   a verified `replace` directive, or the fork fixes the path; live-parse of all six
   dialects' distinctive syntax confirmed.)* Where a parser driver covers the dialect, column-grain allowlisting and the
   whole-tree statement blocklist apply client-side as an additional layer.
4. **Tokenizer-level screens everywhere**: byte/encoding caps, dialect-aware
   single-statement enforcement, comment/quote hygiene.

**Generation targets the native dialect** of the source — no ANSI-conservative subset
constraint (D-033's obligation is dropped; it would have broken ordinary questions on
4 of 6 engines). The BYO context bundle states the dialect and the SQL requirements;
validation is the trust boundary, exactly as before.

**Why:** the engine's own parser is the only authority on its dialect that will ever
exist; credentials are the only read-only guarantee that survives any parser gap. The
client parser layer then *adds* depth without gating capability — and improves
per-dialect as the sqlglot-go driver matures, with zero redesign (the seam is the
contract).

---

### D-039 — Agentic data engineering: the autonomy ladder; L2 is the V1 target, L3 ships policy-gated · *accepted (user directive, 2026-07-06)*

The DE stage's agentic posture is an explicit **autonomy ladder** (RFC §7.8):

- **L0 manual** and **L1 assisted drafting** (agent drafts, human publishes) — the
  floor, already planned.
- **L2 goal-driven proposal — the V1 target.** A business goal in ⇒ the demand-driven
  engine (brief 11's keepers, now promoted from ideas to design: **blind planner** so
  gaps are detectable, retrieve→verify→confirm matching against the canonical
  registry, top-down-match/bottom-up-build) ⇒ one reviewable, atomic **proposal**
  (changeset): pipelines + datasets + topic deltas + schedules, every choice carrying
  a **decision record** (goal served, matches found vs built, alternatives rejected,
  evidence consulted, agent/model provenance). A human approves the *proposal*, not
  each piece; approval publishes through the existing gates (write-shape validation,
  quality checks, D-040 destinations). Rejection and partial-edit-then-approve are
  first-class. Proposals and decision records are budgeted store tables — the
  reasoning trail is queryable audit state, not log archaeology.
- **L3 policy-scoped auto-apply — designed in V1, enabled per tenant.** A tenant
  `autonomy_policy` declares what may auto-publish without a human gate: risk classes
  (e.g. non-destructive strategies only), destination scopes (D-040 managed schemas
  only — always), quality-gate requirements, cost/row ceilings, schedule bounds.
  Anything outside policy degrades to L2 review — never silently proceeds (P4).
  Every auto-applied change is a decision-recorded, revertible proposal post-hoc.
- **The evolution loop** closes at L2/L3: schema drift or failed freshness/quality
  produces a *proposed amendment* (same proposal machinery) rather than only a flag —
  the medallion evolves under audit instead of decaying.

Teramot's "agent fleet" claim is marketing-opaque (brief 12); this ladder is built on
our own validated primitives instead: the ssr draft's engine (brief 11), the
publication gates, Bruin execution (D-036), grants (D-020). New phase 26
(`engineering-autopilot`) owns L2/L3 on top of phases 12/13/15/16.

**Why:** the DE stage is the product's reason to exist beyond the predecessors and its
only un-battle-tested part; the ladder gives Teramot-class automation with a
reviewable, auditable, revertible unit at every level — capability without betting
the warehouse on an opaque loop.

---

### D-040 — The write boundary: Chartworks-managed schemas only; client baseline data is read-only, forever · *accepted (user directive, 2026-07-06)*

Materializations write **only into Chartworks-managed schemas** — namespaces
Chartworks creates and owns inside the customer warehouse (default prefix
`chartworks_`, configurable per source at declaration time). **Client baseline
tables/views — anything not created by Chartworks — are structurally read-only
forever**: they may appear only as *inputs*; the write path rejects, at definition
validation AND at render time, any output that resolves outside a managed schema, and
the medallion (bronze/silver/gold) is Chartworks tables/views exclusively. Destination
declaration (D-036) therefore means: which source + which **managed** schema — never
an existing client schema. Enforcement is layered like everything else: definition
validation → render-gate on the Bruin asset outputs → and, where the engine supports
it, the write credential is scoped to the managed schemas only (mirror of D-038's
credential-primary posture, applied to writes). This strengthens P1c: NLQ can't
write; pipelines can't touch baseline.

**Why:** "we never overwrite clients' raw data" is the single most important trust
property an enterprise DE product has; making it structural (namespace + credential +
double gate) rather than behavioral means no agent decision at any autonomy level can
violate it even in principle.

---

### D-041 — Autopilot governance: two new scopes; no autonomous topic publication; revert drops managed-schema artifacts · *accepted (phase-26 planning review, 2026-07-06)*

Ratifies the phase-26 plan's proposals: (a) two capability scopes join the §5.3 set —
**`autonomy.propose`** (submit/read goals and proposals) and **`autonomy.apply`**
(approve/reject/revert — additionally requiring D-020 `manage` grants on each affected
resource); policy CRUD reuses `admin`. (b) **Autonomous topic publication is barred at
every autonomy level**: an L3 policy can auto-apply data plumbing into managed
schemas, but topic deltas always stop at draft/review — a published topic shapes what
every NLQ caller sees, so meaning changes always get a human. This is a standing
guardrail, not a policy option. (c) **Revert semantics**: per object class —
pipelines unpublish (versions retained), managed-schema materializations **drop by
default** (reproducible by construction; `archive` configurable), datasets
unregister, draft topic versions discard, schedules detach; atomic per proposal,
blocked loudly on cross-proposal dependents; the drop itself rides D-040's
managed-schema write gates, so revert can no more touch baseline than apply can.

---

### D-042 — Investigations: the read-side autonomy analog is the designated V1.1 wave · *accepted (user directive, 2026-07-06)*

The read path's power ceiling is raised the same way the write path's was (D-039), as
a committed **V1.1 wave** (not V1 scope), with three components:

1. **The internal investigation orchestrator** — an analytical goal → gateway-driven
   decomposition into sub-questions → each through the *unchanged* routing/context/
   validation/execution primitives (P7 — a new caller in a loop, no core rework) →
   synthesized, evidence-backed findings. Governance is D-039's pattern
   re-instantiated: investigation = read-side proposal; per-sub-query decision
   records; budgets (token, query-count, row, wall-clock) as the policy; since reads
   are safe by construction, auto-run-within-budget needs no human gate.
2. **Cross-topic queries, promoted from post-V1**: confirmed cross-topic
   relationships (the predecessors' machinery, brief 05) extend the join graph and
   the allowlist so validated queries may span topics the caller is granted.
3. **The analysis scratchpad**: ephemeral intermediate views/tables under managed
   `chartworks_` schemas, created and dropped through the same D-040 gates —
   the read path borrows the write path's governed muscle; scratchpad artifacts are
   session-scoped, erased on investigation close and by tenant erasure.

**V1 reserves only what is cheap now:** the `investigation` vocabulary (glossary +
RFC §2), session linkage on `queries` rows (already present), and BYO-bundle
documentation positioning **multi-step external-agent analysis as a supported
pattern today** — a Harbor/Claude/ChatGPT agent looping `get_query_context` →
`submit_sql` under the same gates *is* the analyst surface at V1 ship, every step
validated and audited. No V1 phase gains scope; the V1.1 wave is planned after the
Wave-7 checkpoint proves the D-039/D-040 machinery in production.

**Why:** competing with Teramot-class "AI analysts" is a sequencing choice, not a
redesign risk — the seams are already right; writing the commitment down keeps the
power ceiling a plan instead of a hope, without letting it creep into V1.

---

*RFC-001-Chartworks.md v1.0 (2026-07-06) settles D-019…D-031; D-032…D-042 were filed
during the planning review. Further product decisions land here as phases ship,
numbered D-043+.*

Read execution continuation: [bounded plan-only reads and attempt uncertainty](decisions/2026-09-06-read-execution.md).

Warehouse read substrate continuation: [D-067 pinned minimal Bruin leaf-client fork](decisions/2026-09-07-bruin-read-adoption.md).

Canonical registry continuation: [D-068 reviewed publication approves exact canonical meaning](decisions/2026-09-08-canonical-registry.md).

Output specification continuation: [D-069 bounded provider-neutral output specifications and executable HTTP registration](decisions/2026-09-08-chart-specifications.md).
