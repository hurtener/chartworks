# Brief 07 — WrenAI (Canner): context-layer / MDL / text-to-SQL — ideas for Chartworks

> Status: draft · 2026-07-06 · source: external_refs/WrenAI-main (license: multi —
> **Apache-2.0** for `core/**`, `sdk/**`, `skills/**`, `examples/**`, and root files;
> **CC BY 4.0** for `docs/**`; an `LICENSE-AGPL-3.0` text is pre-staged for **future**
> AGPL modules — none exist in this snapshot). Ideas only — no code, file, or string is
> copied into Chartworks; WrenAI is named directly since it is OSS. Where the Python
> predecessors come up, they are called only "the client predecessor" / "the
> generalistic predecessor"; this brief does not read `_ref/`.

**Scoping caveat.** This staged copy is **post-pivot WrenAI** (its own
`.claude/CLAUDE.md` calls it "the Open Context Engine"). The *previous* WrenAI product —
a Docker-based, chat-first GenBI app with a Python `ai-service` running NLQ pipelines
(retrieval → routing → SQL generation → chart render) — moved to a `legacy/v1` branch
per the README and **is not present here**. What's mined below is the current
architecture: a Rust semantic engine (`wren-core`, DataFusion-based) exposed through a
Python CLI/SDK (`wrenai`), thin per-framework agent toolkits (`wren-langchain`,
`wren-pydantic`), and a browser dashboard builder (`wren-core-wasm`, "GenBI"). **There is
no in-repo pipeline that goes from question to SQL to chart spec** — WrenAI inverted
that onto a BYO agent (§2–3). Studying the old pipeline would need the `legacy/v1`
branch — flagged as a gap, not chased here.

---

## Summary

WrenAI's current architecture is the closest precedent yet to Chartworks' "dual
SQL-generation modes incl. BYO-agent context handoff": it separates a durable, versioned,
file-based semantic contract (MDL) from a BYO-agent execution model, where WrenAI
supplies context + validation primitives (`dry-plan`, `dry-run`, `query`, `memory
fetch/recall/store`) and the calling agent decides how to write SQL. Its OSS tier ships
*only* the handoff mode — its own SQL-writing pipeline is gone. Five ideas stand out:
(1) a first-class **cube** (pre-aggregation) object that collapses the
GROUP BY/DATE_TRUNC/metric-arithmetic hallucination class; (2) a **three-tier query
safety ladder** (plan-only → validate-without-executing → execute) that maps almost
exactly onto Chartworks' RFC-pending SQL-safety property; (3) a **phase-aware,
retry-vs-propagate structured error taxonomy** with secret redaction and a byte cap,
ready-made for P4; (4) a **markdown-source-of-truth, disposable-index** discipline (an
embedding index is a rebuildable cache, never the durable record); (5) a **size-gated
hybrid retrieval heuristic** that avoids retrieval infra for small schemas. The weakest
fit is chart generation: "GenBI" ships whole browser app source trees via a WASM engine,
not a typed chart spec — far from Chartworks' "outputs are first-class, normalized
shapes" rule. Its access story is a trap, not a template: RLAC/CLAC are declared in MDL
and open-source, but *binding* a rule to a real per-user identity is Commercial-only —
the opposite of Chartworks' P1–P3, where deny-by-default access is inherited-binding,
never an upsell.

---

## 1. The semantic layer — MDL

**Model** — a logical dataset via `table_reference` (physical table) or `ref_sql` (a
SELECT), mutually exclusive. **Column exposure is selective**: an undeclared column is
physically invisible to every client — no SQL can reference it, it never appears in
introspection. That's WrenAI's column-control primitive in its plainest form: omission,
not a runtime filter. A column can be a plain mapping, a `relationship` (join-handle,
e.g. `orders.customer.first_name` resolves automatically), or `is_calculated` +
`expression`. An optional `column_level_access_control`: `operator` + typed `threshold`
+ `required_properties` (named session properties) — declarative, not yet bound to a
real identity in OSS (see §6).

**Relationship** — two models, a `join_type`, and a `condition` — **equality only**.
`TO_MANY` calculated columns must aggregate; the engine wraps the join in an aggregate
subquery to prevent row-multiplication fan-out — an engine-enforced guardrail against a
specific, common correctness bug.

**View** — a named SQL SELECT behaving like a stable virtual table; views can nest.

**Cube** — a pre-aggregated object: `base_object` + `measures` (aggregation
expressions) + `dimensions` + `time_dimensions` (granularity applied at query time) +
`hierarchies` (drill-down levels). Queried **structurally**
(`wren cube query --cube revenue --measures total --time-dimension order_date:month`),
never via hand-written `GROUP BY`/`DATE_TRUNC`. This is the one MDL object with no
obvious topic-pack analogue — a narrow, purpose-built anti-hallucination device, not a
general modeling primitive. WrenAI calls it "the highest-leverage correctness primitive
for smaller models."

**Row-level access control (RLAC)** — a named rule: `required_properties` + a SQL
boolean `condition`.

**Versioning** — a `schema_version` (currently 5); YAML source (snake_case) compiles via
`wren context build` into `target/mdl.json` (camelCase). Upgrades are forward-only and
idempotent, dry-runnable.

**→ Chartworks landing (`internal/semantics`):** adopt **selective column exposure**
directly (default-deny undeclared columns, a free complement to the RFC-pending P1
model); adopt a **narrow metric/aggregation object** distinct from a generic topic — a
measures/dimensions/time_dimensions object with a *structured* query API has no
SQL-injection surface at all, plausibly the easiest class of "generated SQL" to make
provably safe, and a strong candidate answer to the RFC-pending SQL-safety property for
the aggregation query class specifically; consider the **equality-only join condition**
constraint to narrow correctness/injection surface at the join graph. **Adapt with
caution:** an OSI (Open Semantic Interchange) adapter lets WrenAI build its manifest
from an externally-owned semantic-model YAML without forking it, with a vendor
extension block for gaps — worth an RFC mention as a V2 idea if customers already run
an external semantic layer, not a V1 must (parsing a third-party schema is untrusted
input under the same P1/P4 discipline as any ingest).

---

## 2. The text-to-SQL pipeline — retrieval, prompting, correction, dialect, validation

No SQL-generation pipeline remains in-repo; instead, primitives a BYO agent orchestrates:

```text
wren memory recall -q "..."   → similar confirmed NL→SQL pairs (few-shot)
wren memory fetch  -q "..."   → relevant models/columns/relationships/rules
[agent writes SQL against MDL object names, never physical tables]
wren dry-plan  --sql ...      → expand to target-dialect SQL, no DB hit
wren dry-run   --sql ...      → validate against the live DB, no rows returned
wren query     --sql ...      → execute, capped at 1000 rows
wren memory store             → persist the confirmed pair
```

**Retrieval — size-gated hybrid heuristic.** `wren memory fetch` measures the
**character length** of the full plain-text schema description: under ~30,000 chars
(≈8K tokens), return full text (small schemas do better with complete context than
fragments); above it, fall back to embedding search over indexed schema items. Character
length is free to compute and conservative for CJK content (compresses less, so it flips
to search sooner). No LLM call spent deciding.

**Retrieval — driver seam.** The source of truth is markdown (`knowledge/rules/*.md`,
`knowledge/sql/*.md`), never a vector index. An optional install-time extra swaps a
dependency-free grep backend (token/substring match, works day one) for a LanceDB
embedding backend — mirrors Chartworks' own extensibility-seam discipline (interface +
factory + driver, one V1 driver is fine as long as the seam exists), except WrenAI ships
two drivers day one specifically so a small project pays zero infra cost.

**Prompting — two fixed templates, no silent default.** `wren ask "<question>"`
requires `--guided` (strict ordered task flow, for weaker models) or `--direct`
(minimal wrapping, for stronger models) — explicitly no default, since silently changing
one would alter agent behavior across an upgrade.

**Correction loop — phase-aware, not free-text.** A `WrenError` carries an `ErrorCode`
and an `ErrorPhase` (`SQL_PARSING`, `SQL_PLANNING`, `SQL_TRANSPILE`, `SQL_DRY_RUN`,
`SQL_EXECUTION`, `METADATA_FETCHING`, `MDL_EXTRACTION`, `VALIDATION`). The Pydantic-AI
adapter splits errors into **propagate** (connection/config/filesystem — retry can't
fix it, bubbles to the caller) vs. **retry** (SQL/lookup/validation the model can
plausibly self-correct), converting the latter into a phase-specific hint (e.g. "SQL
planning error: {msg}. Check model/column names and retry."). Two details worth lifting
independently: **recursive secret redaction** (keys matching password/secret/token/
credential replaced before the message is built) and a **hard byte cap** with a
truncation marker, not a silent drop or an unbounded blob.

**Dialect handling.** sqlglot parses/qualifies/transpiles; a CTE rewriter identifies
referenced MDL objects and injects their expanded SQL; `wren-core` (Rust/DataFusion) is
the source of truth for MDL-semantics-to-SQL expansion. Staged: parse → qualify →
identify MDL objects → expand → inject CTEs → policy checks → transpile to target
dialect.

**Validation — the three-tier ladder.** `dry-plan` (transpile only, **no DB connection
required** — usable offline/in CI), `dry-run` (live DB plans the query, no rows
returned — catches things dry-plan can't, e.g. a table missing in that environment),
`query` (execute). Each is independently callable, not one monolithic step.

**→ Chartworks landing (`internal/nlq` + `internal/exec`):** the plan → dry-run →
execute ladder is a strong, concrete shape for "read-only execution, schema
allowlisting, and injection guardrails" — `dry-plan`'s "no DB connection needed"
property maps well onto a mandatory pre-execution allowlist gate. The phase-aware error
taxonomy is a near-direct answer to P4's "typed error, never a raised stack trace" —
adopt the retry-vs-propagate split and the redaction/byte-cap discipline specifically.
The size-gated retrieval heuristic and grep/embedding driver seam are directly reusable
for whatever retrieval `internal/nlq` builds over the topic-pack.

---

## 3. Chart generation — from result to chart spec

**Weakest match — WrenAI does not generate a chart spec from a query result.** "GenBI"
produces a whole deployable browser-side app: the CLI (`wren genbi build`) prints a
build instruction (pinned wasm version, model inventory, data-mode guidance) but writes
no files; a **coding agent** authors the app under `apps/<name>/` by following it,
choosing charts/layout itself; `wren genbi verify` runs a deterministic preflight
(required files, `mdl.json` parses, snapshot apps ship a data asset, a **default-deny
secret scan** flagging inlined credentials — "best-effort defense-in-depth, not a
guarantee"); `wren genbi deploy` ships to the caller's own Vercel/Cloudflare account
using a token from environment/`.env`, never a CLI flag. Two data modes: **snapshot**
(data frozen in at build time, queried client-side via `wren-core-wasm`, DataFusion
compiled to ~72MB of WASM) or **live** (callback to the warehouse at view time, CORS
required, no inlined credentials). The CLI/agent split is disciplined: a deterministic
index file (`.wren/apps.yml`) only the CLI ever writes, verified against whatever
freeform code the agent produced.

**→ Chartworks landing (`internal/charts`):** the shipped-app mechanism doesn't map onto
"outputs are first-class, normalized shapes" (CLAUDE.md §6) — a deployed app source tree
is the opposite of a normalized, provider-agnostic chart spec. Keep two narrower ideas:
the **snapshot-vs-live toggle** if chart output ever needs to distinguish frozen result
data from live requery, and the **default-deny secret scan on any generated artifact**
if Chartworks ever ships an embeddable/exportable bundle. Skip the WASM-engine-in-the-
browser approach itself (§6).

---

## 4. API / tool surface for agents

WrenAI OSS ships **no MCP server** — its own OSS-vs-Commercial comparison table lists
"MCP server and hosted REST API" as **Commercial-only**. OSS ships a CLI plus two thin
per-framework toolkits (`wren-langchain`, `wren-pydantic`), built identically:
`WrenToolkit.from_project(path)` attaches to a CLI-prepared project (profile + compiled
MDL + optional memory index) as a pure adapter. **3 runtime tools always present:**
`wren_query` (capped at 1000 rows), `wren_dry_plan`, `wren_list_models` (avoids an agent
guessing physical table names — a documented failure mode traces directly to skipping
this call). **0/2/3 memory tools, auto-detected** from whether `.wren/memory/` exists;
`include_memory_write=False` drops the persistence tool for shared/curated projects, and
the prompt/instructions builder is handed the *same* tool list so it never tells the
agent to call a tool that was just removed. Framework specifics are thin: LangChain gets
a system prompt string; Pydantic AI gets typed `output_type=` structured output and a
`takes_ctx=True` interop knob. Both are documented **sync-only**, with the scaling
ceiling for many-concurrent-user servers flagged openly.

**→ Chartworks landing (`internal/mcpserver` + `sdk/chartworks`):** the load-bearing
finding is negative but important — the nearest comparable OSS project gates real MCP
exposure and identity-bound access behind a paid tier, meaning Chartworks' bootstrap
posture (MCP + HTTP dual surface, deny-by-default access, open from day one per P1–P3)
is *stronger* than this precedent gives away for free — worth citing in the RFC as
validation. The **auto-detected optional tool + tool-list-passed-to-prompt-builder**
pattern is directly reusable for Chartworks' own conditional MCP tool registration
(e.g. execute vs. dry-run-only mode per caller).

---

## 5. Eval — how they measure accuracy

The one eval artifact, `evals/spodbtify_ab/`, is small, manual-graded, and
**agent-agnostic**: it compares `schema_only` vs. `dbt_integrated` (richer) context by
having *any* coding agent answer 20 fixed questions against a real, not-checked-in
dataset. The spec fixes workflows/controls/questions; `run_eval.py` validates the spec,
generates prompts, drives an arbitrary agent CLI via a command template, and summarizes
score files. Agent output is schema-validated (`question_id`, `workflow`, `agent`,
`selected_tables`, `sql`, `answer`, `notes`) but **grading is manual** — a human/external
grader assigns three 0/1/2 scores per question. WrenAI's own architecture doc lists
"Eval" as a correctness pillar still **"in development"** — this is a promising shape,
not a mature harness.

**→ Chartworks landing (`eval/`):** steal the **context-richness A/B axis** — run the
same fixed question set through "topic-pack absent/minimal" vs. "topic-pack + rules +
confirmed examples" and diff the scores — a cheap, direct way to justify the
engineering-stage (engineer → model) investment. The **schema-validated structured
output contract** is a reasonable starting shape for a golden-eval record, consistent
with P5's schema-constrained-output discipline. Caution: manual grading doesn't satisfy
CLAUDE.md §11's mechanical coverage-gate discipline — Chartworks needs an automated
judge or exact/fuzzy SQL-match layered on top, not a copy of the human-grading step.

---

## 6. Architecture — services, steal vs. over-engineered

Four layers: agent skills (markdown guides served by the CLI on demand, never cached in
the agent's own directory, so content can't drift from the installed version) → project
context (MDL + `knowledge/` + memory, all files) → planning engine (sqlglot + CTE
rewrite + `wren-core`) → connectors. A Cargo workspace (`wren-core`, shared manifest
types, PyO3 bindings, wasm build) sits under a thin Python CLI/SDK ("the CLI is a thin
Typer wrapper over the SDK; both share the orchestration code").

**Steal:**

- **Markdown-source-of-truth, disposable derived index** — `knowledge/*.md` is durable
  and committed; the LanceDB index is gitignored and rebuilt from markdown any time,
  with a drift-check command (`wren memory check`) and an explicit "reset preserves the
  source" guarantee. Mirrors D-004's Store-vs-seam discipline; a strong precedent for
  treating any Chartworks retrieval index as a cache over the durable record, never the
  record itself.
- **Cube as a narrow, structurally-validated aggregation API** (§1) — highest-confidence
  anti-hallucination primitive in the source.
- **Phase-aware structured errors, retry/propagate split, redaction, byte cap** (§2).
- **The plan → dry-run → execute three-tier ladder** (§2).
- **Skill content served by the running binary, not cached in the agent's tree** —
  WrenAI's own migration note explains why: version drift between installed CLI and
  cached content, no way to gate what's loaded. Applies directly to any future
  Chartworks agent-facing workflow guide.
- **A CI guard scanning every documented CLI invocation against the real command tree**
  — cheap insurance against a stale workflow guide; portable to Chartworks' smoke-check
  discipline (§4.2).

**Skip / over-engineered for the one-binary, CGo-free Go posture (D-005, P7):** the
**Rust-to-WASM browser dashboard path** (a ~72MB binary and a whole client-side query
engine — a typed chart spec rendered by an existing JS charting library is the
right-sized answer if embeddable charts are ever needed); **the polyglot service tree**
(Rust workspace + PyO3 + two Python SDKs — validates the *pattern* P7 already mandates,
not the *implementation*; no Go analogue beyond "evaluate an existing Go SQL parser/
transpiler as a research spike" if dialect transpilation becomes a real need); **the
legacy chat-first product** (`ai-service`/`wren-ui`/`wren-launcher` — not in this
snapshot, don't assume it represents current WrenAI practice).

**Adapt with real caution — access control:** RLAC/CLAC are declared in MDL and are
genuinely open-source, but the OSS/Commercial table is explicit that **"RLS/CLS per
user, session properties, audit log"** — binding a declared rule to a real authenticated
identity at query time — is **Commercial-only**. WrenAI OSS lets you *write* a row-filter
condition keyed on a named "session property" without wiring it to a real per-user
session. Borrow the **shape** (declarative condition + named required properties, and
column-omission for column control) but not the license-gating: for Chartworks,
deny-by-default access bound to the frozen per-request identity envelope is core
(P1–P3), never an upsell.

---

## Steal / Adapt / Skip

| Idea | Verdict | Where it would land |
|---|---|---|
| Cube (measures/dimensions/time-dims/hierarchies) as a structured aggregation object | **Steal** | `internal/semantics` |
| Plan → dry-run → execute three-tier validation ladder | **Steal** | `internal/exec` — candidate for RFC-pending SQL-safety property |
| Phase-aware error taxonomy, retry-vs-propagate split, redaction + byte cap | **Steal** | `internal/nlq`, `internal/exec` — P4 |
| Selective column exposure (undeclared = invisible) | **Steal** | `internal/semantics` — composes with P1 |
| Equality-only join conditions | **Steal (candidate constraint)** | `internal/semantics` |
| Size-gated hybrid retrieval (full text vs. embedding by char threshold) | **Steal** | `internal/nlq` retrieval |
| Markdown-source-of-truth + disposable, rebuildable index with drift-check | **Steal** | `internal/semantics` / eval knowledge |
| Skill/workflow content served live from the binary | **Steal** | any future agent-facing workflow guide |
| CI guard: docs vs. real command tree | **Steal** | `scripts/`, CI |
| Auto-detected optional tool + tool-list-passed-to-prompt-builder | **Steal** | `internal/mcpserver` |
| Context-richness A/B eval axis over a fixed question set | **Steal (idea, not manual grading)** | `eval/` |
| Schema-validated structured eval-output contract | **Adapt** | `eval/` — needs automated scoring |
| OSI-style "read an external semantic model, don't fork it" adapter | **Adapt (V2 idea)** | `internal/semantics` |
| Snapshot-vs-live toggle + default-deny secret scan on generated artifacts | **Adapt** | `internal/charts`, if embeddable output ships |
| Declarative RLAC/CLAC shape (condition + required properties) | **Adapt — shape only, never the license gate** | `internal/semantics`, bound to P1–P3 as core |
| GenBI: ship a whole deployable browser app (WASM + Vercel/Cloudflare) | **Skip** | conflicts with normalized-shapes rule and D-005 |
| Rust-to-WASM in-browser query engine | **Skip** | out of scope for a Go, one-binary product |
| Polyglot service tree as an implementation to imitate | **Skip** | validates the pattern (P7), not the stack |
| The legacy chat-first product | **Skip (not present, don't assume it)** | flagged as a gap |

---

## Open questions

1. Should Chartworks' topic-pack include a first-class metric/aggregation object
   (WrenAI's cube) to make the aggregation-hallucination class provably safe via a
   structured, non-SQL input?
2. Is plan → dry-run → execute the right shape for the RFC's still-open SQL-safety
   property, with "dry-plan needs no live DB" as a mandatory allowlist-only gate?
3. Should the `ErrorCode`/`ErrorPhase` taxonomy seed Chartworks' canonical typed-error
   enum for generated SQL (P4), including the retry-vs-propagate split?
4. Does "BYO-agent context handoff" imply Go-side thin toolkit adapters for popular
   agent frameworks, or does the MCP tool surface alone satisfy that handoff? WrenAI
   treats these as separate investments (Python toolkits ship in OSS; MCP does not).
5. Should `eval/` include a context-richness A/B axis alongside the golden-SQL tests
   §11 already mandates, and what automated scoring replaces WrenAI's manual grading?
6. Is an OSI-style external-semantic-model ingestion path in scope for any phase, or
   explicitly V2/out-of-scope pending the RFC naming which external formats Chartworks
   customers bring?
7. Should WrenAI's declarative RLAC/CLAC rule shape (condition + required named
   properties) seed how P1's still-undefined access primitive expresses row-level
   rules, independent of WrenAI's own license-gating of the binding?
