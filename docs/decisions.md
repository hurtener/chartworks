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

*RFC-001-Chartworks.md is the next artifact: it settles what D-011…D-018 defer (the P1
access primitive and SQL-safety mechanism, the pipeline shape, the surface set, the
store inventory). Product decisions land here as phases ship, numbered D-019+.*
