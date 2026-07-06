# Chartworks — Decisions Log

> Append-only log of settled architectural decisions. Each entry is `D-NNN`, a status,
> the decision, and its rationale. **Do not re-litigate a settled decision silently** — a
> change is a new superseding entry (and, once the RFC exists, an RFC PR), never an
> in-place edit. Grep here before reopening a question.
>
> Status vocabulary: **accepted** (binding) · **superseded-by-D-NNN** · **proposed**
> (recorded, not yet binding).
>
> **Bootstrap note.** Chartworks is a new repo. The seed entries below are **proposed
> (inherited from the sibling Soundings build)** — they restate transferable ecosystem
> decisions for this product so the method layer is coherent from day one. The **kickoff
> interview** flips each to *accepted* or *superseded*; product-specific decisions
> (pipeline shape, semantic model, SQL-safety mechanism, surface set) start **after** the
> kickoff and the RFC, numbered from where these seeds leave off.

---

### D-001 — Clean-room Go rewrite, two predecessors, never copied · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-002 — Product name & family seat · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

The product is **Chartworks** (repo `chartworks`, module
`github.com/hurtener/chartworks` — a **placeholder pending the kickoff interview**), the
ecosystem's **Explorer seat**: structured-data analytics (engineer → model → NLQ-to-SQL →
charts), a peer to Portico / Harbor / Dockyard / Stowage / Soundings. "Wayfinder" and
"Explorer" were the predecessors' names; in the family, the seat is **Chartworks**.

**Why:** the family names products by nautical/structural codename, not by function;
consistency keeps the ecosystem legible. The module path mirrors the sibling convention
but is not yet load-bearing, so it stays a placeholder the kickoff confirms.

---

### D-003 — One intelligence seam, Bifrost driver · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-004 — Postgres-first for Chartworks' OWN store; SQLite dropped; customer data sources are separate · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-005 — CGo-free posture (deliberately reversing the Soundings exception) · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

Chartworks is **CGo-free**: `CGO_ENABLED=0`, a single static binary, no C toolchain
prerequisite — the strict posture of Portico / Harbor / Dockyard / Stowage. This
**deliberately reverses** Soundings' D-005 CGo exception, which existed only because that
product's extraction library required it; Chartworks has no equivalent dependency. Any
future CGo need is a **new decision entry** that reverses this posture — never a silent
`CGO_ENABLED=1`.

**Why:** structured-data analytics has no in-process native dependency forcing CGo, so the
default reclaims the single-static-binary guarantee the family prefers. Recording the
reversal here stops a contributor from assuming Soundings' exception transfers.

---

### D-006 — Asymmetric-JWT dual-mode auth · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-007 — MCP surface on `mark3labs/mcp-go` unless Dockyard becomes consumable · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-008 — CLI on stdlib `flag`; cobra rejected · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

The `chartworks` CLI (including any `admin` command family) uses stdlib `flag` with
hand-rolled subcommand dispatch, not cobra.

**Why:** the command surface is small and closed and the `run(args, stdout, stderr) int`
dispatch is already testable; the family's minimal-dependency posture does not justify
cobra's tree. The Soundings build reached the same conclusion at its breadth point.

---

### D-009 — Preflight fast/full + coverage-band + drift-audit as the standing quality machinery · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

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

### D-010 — Live-verification gate as a standing wave-end check · *proposed (inherited from the Soundings build; confirm or supersede at kickoff)*

A **live-verification gate** runs the real pipeline against real provider models (via a
local `.env`, e.g. an OpenRouter key through the `bifrost` driver) at each wave boundary.
It is **never CI-required** (no secrets in CI) but is a standing pre-wave-close check.

**Why:** on the Soundings build this gate caught a silent-degrade the entire mock-backed CI
suite passed over (the real engine could not run with the inputs the fake one accepted) —
the exact "every gate green, production broken" failure P4 exists to prevent. For
Chartworks, whose SQL-generation and routing are model-driven, a live check against real
models is even more load-bearing.

---

*The RFC-001-Chartworks.md is pending; the kickoff interview flips these seed entries to
**accepted** or **superseded** and settles the product-specific concerns each defers to
the RFC (P1 access model, SQL-safety mechanism, pipeline shape, surface set). Product
decisions land here as phases ship, numbered from where these seeds leave off (D-011+).*
