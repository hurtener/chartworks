# 09 — Idea mining: `astronomer/agents`

> Status: draft · 2026-07-06 · source: external_refs/agents-main (license: Apache-2.0)

## Summary

`external_refs/agents-main` is **not** a Chartworks predecessor — it's a snapshot of
[`astronomer/agents`](https://github.com/astronomer/agents), Astronomer's public,
Apache-2.0-licensed Claude Code / Cursor plugin for **Airflow + data-engineering
workflows**. It bundles an Airflow REST MCP server (`astro-airflow-mcp`) plus ~25 Claude
Code "skills" (DAG authoring, dbt/Cosmos, lineage, deployment, migration). Most of the
repo is Airflow/DAG-orchestration tooling Chartworks has no analogue for and is
irrelevant. The genuinely useful slice is narrow but concrete: the `analyzing-data` /
`warehouse-init` / `profiling-tables` / `checking-freshness` skills implement a
schema-discovery-plus-caching workflow for NL-to-SQL analysis, and `astro-airflow-mcp`
carries a couple of well-tested MCP-tool-safety patterns worth adapting. One finding is
an explicit **anti-pattern**: this repo's SQL execution model (an agent-driven Jupyter
kernel running arbitrary SQL/Python with the user's own warehouse credentials, no
read-only enforcement, no schema allowlist) is the mirror image of Chartworks' P1
SQL-safety property and should be named as what NOT to do, not adapted.

## What it is

- **Repo:** `astronomer/agents` (public GitHub, Astronomer, Inc.), Apache License 2.0
  (`LICENSE` at repo root; `astro-airflow-mcp/LICENSE` likely mirrors it — not separately
  checked, assume Apache 2.0 throughout).
- **Shape:** a Claude Code / Cursor **plugin** (`.claude-plugin/`, `skills/`) plus one
  standalone Python MCP server + CLI (`astro-airflow-mcp/`, published to PyPI, runs via
  `uvx`).
- **Audience:** individual data engineers/analysts working locally against their own
  Airflow deployment (OSS or Astro-managed) and their own warehouse credentials, inside
  an agentic coding tool. This is a **single-user, single-tenant, developer-trusted**
  context — a materially different trust boundary from Chartworks' multi-tenant,
  deny-by-default service.
- **Two halves:**
  1. `astro-airflow-mcp/` — a Python MCP server + `af` CLI wrapping the Airflow REST API
     (DAG management, triggering, task logs, health). Has a real test suite (adapters,
     scopes, read-only mode, tool annotations, telemetry).
  2. `skills/` — markdown "skill" files (SKILL.md + optional scripts) that teach Claude
     Code specific workflows: DAG authoring/testing/debugging, dbt-via-Cosmos, lineage
     tracing, Astro deployment, Airflow 2→3 migration, and — the relevant cluster —
     warehouse schema discovery + SQL-based data analysis.
- **Not relevant to Chartworks:** the DAG lifecycle skills (`authoring-dags`,
  `testing-dags`, `debugging-dags`, `deploying-airflow`, `airflow-hitl`,
  `migrating-airflow-2-to-3`), the dbt/Cosmos skills, the lineage skills
  (OpenLineage-specific), `blueprint`/`dag-factory` (YAML→DAG generation), and the
  Astro-product skills (`setting-up-astro-project`, `managing-astro-local-env`,
  `managing-astro-deployments`, `troubleshooting-astro-deployments`,
  `delegating-to-otto`). Chartworks has no orchestration surface, no DAGs, and no
  Astro-product integration — none of this maps to engineer/model/NLQ/charts.

## Relevant findings

### 1. Concept cache + pattern cache (schema/query memoization)

`analyzing-data` maintains three small JSON caches at `~/.astro/ai/cache/`:
**concepts** (business term → table + key column + date column, e.g. "customers" →
`HQ.MODEL.ORGS`, key `ACCT_ID`), **patterns** (a named query strategy for a recurring
question shape, with recorded success/failure counts), and **table schemas** (column
lists + row counts, TTL'd). A query first checks pattern cache, then concept cache,
falling back to `INFORMATION_SCHEMA` discovery only on a miss, and **always writes back**
before presenting results.

→ **Chartworks landing:** this is a lightweight, self-improving version of exactly what
the semantic model / topic-pack layer (`internal/semantics`, TBD-by-RFC) is meant to be
formal about. It's a useful *bootstrapping* pattern to consider for topic-pack authoring
tooling (e.g., a `chartworks topic suggest` command that proposes concept→table mappings
from `INFORMATION_SCHEMA` + observed query success), but it is explicitly **not** a
substitute for a validated, versioned topic pack — the cache here has no access scoping,
no schema-allowlist enforcement, and silently degrades on a stale mapping (a P4
violation if ported as-is).

### 2. Data-layer hierarchy convention ("query downstream first")

`warehouse-init` generates a `Data Layer Hierarchy` table ranking schemas
`reporting > mart_* > metric_* > model_* > raw (IN_*)` and instructs the agent to prefer
the most downstream layer available. Large tables (>100M rows) are flagged with a
standing warning to always filter by date.

→ **Chartworks landing:** a good, cheap heuristic for the data-engineering stage
(`internal/engineering`, TBD-by-RFC) to encode as metadata on a dataset/table — a
"preferred layer" or "materialization tier" attribute the semantic model or SQL
generator can prefer, and a "large table" flag that forces a date-bound predicate before
generated SQL is considered valid. Complements, doesn't replace, the RFC's schema
allowlist.

### 3. Categorical value-family discovery

Before filtering on a categorical column, the skill runs a `GROUP BY` distinct-value
scan and groups results into "families" by common prefix/suffix (e.g. `Export*` for
`ExportCSV`, `ExportJSON`, `ExportParquet`) so the agent doesn't miss variants when a
user says "exports."

→ **Chartworks landing:** directly useful for the semantic model's dimension metadata —
storing known value families (or at minimum sample distinct values) per
low-cardinality dimension column avoids a class of NLQ-to-SQL filter bugs (partial-match
misses). Worth a line in the eventual semantics phase plan.

### 4. Table profiling as a structured, scored artifact

`profiling-tables` produces a fixed-shape profile: schema + null%/distinct stats,
row count/date range, a 4-axis quality score (completeness/uniqueness/freshness/
validity), a prose "potential issues" list, and 3-5 "recommended queries."

→ **Chartworks landing:** a reasonable target shape for a dataset-profile output of the
engineering stage — normalized, provider-agnostic (per CLAUDE.md §6 "outputs are
first-class normalized shapes"), and something the chart-spec stage could consume
directly (e.g., to pick sensible axis types). The "recommended queries" idea is a nice
seed for a "suggested question" feature once the semantic model exists.

### 5. Freshness check with escalation to the owning pipeline

`checking-freshness` computes an age-based status (Fresh / Stale / Very Stale /
Unknown) from a `MAX(timestamp)` scan, then — if stale — escalates to checking the
owning Airflow DAG's status and offers to hand off to a debugging skill.

→ **Chartworks landing:** the freshness-classification scale (age bucket → status
string) is directly reusable as a normalized "freshness" field on a dataset descriptor.
The escalation-to-owning-pipeline step doesn't apply (Chartworks doesn't own or
introspect customer ETL/orchestration) — the honest read is that Chartworks' freshness
signal should stop at "stale, source unknown" rather than reach into a system it has no
contract with.

### 6. Connector interface + registry (adapter seam precedent)

`connectors.py` defines an abstract `DatabaseConnector` (per-connector `validate`,
`get_required_packages`, `get_env_vars_for_kernel`, `to_python_prelude`) with a
decorator-based registry (`@register_connector`) covering Snowflake/Postgres/BigQuery/
generic SQLAlchemy (25+ dialects via URL sniffing).

→ **Chartworks landing:** a concrete (if Python-shaped) precedent for the
data-source/warehouse adapter seam CLAUDE.md §4.4 already mandates as an
interface+factory+driver pattern. The useful bit isn't the code, it's the **minimal
method set** a driver needs: validate config, declare dependencies, supply
connection/auth material, and produce a query-execution entry point. Confirms the RFC's
adapter seam is the right shape; nothing new to add beyond what §4.4 already commits to.

### 7. MCP tool safety annotations, exhaustively tested against an allowlist

`test_tool_annotations.py` defines a `WRITE_TOOLS` dict of every write tool with its
expected `(destructiveHint, idempotentHint)`, then asserts **every tool not in that dict
is read-only** by listing all registered tools and checking annotations — a new tool
that's accidentally writable (or accidentally missing an annotation) fails CI by
default-deny, not by someone remembering to update a test.

→ **Chartworks landing:** directly portable pattern for `internal/mcpserver`. A single
test that enumerates all registered MCP tools and asserts each carries an explicit
read-only/write annotation (defaulting new tools to "must justify write") is a cheap,
high-value CI gate, and fits P4 (fail loud) — a missing annotation is a build failure,
not a silent gap. Same idea for the `af` CLI's global `AF_READ_ONLY` env guard
(`_assert_writable`, called once at the adapter boundary, tested with parametrized
true/false-ish values): the pattern of "one guard function at the one seam, tested
exhaustively for truthy/falsy env-string variants" is worth mirroring for whatever
Chartworks' own execution-mode toggle ends up being (though Chartworks' P1 SQL-safety
posture should make read-only the *only* mode pre-RFC, not a togglable env var).

### 8. Consolidated, partial-failure-tolerant tools

`explore_dag` (an MCP tool) bundles three underlying API calls (dag info, task list,
source) into one response and degrades gracefully when one sub-call fails, returning
partial data with per-field error markers rather than failing the whole tool call.

→ **Chartworks landing:** a useful shape for read-aggregation tools (e.g. a
`describe_topic` tool bundling measures/dimensions/sample-values), but flag the tension
with P4: partial-success-with-markers is fine for *availability* aggregation across
independent read calls, but must never apply to an **access decision** — a denied scope
inside a bundled call must still fail the whole tool loudly, never silently omit that
one field. Any adoption needs this distinction made explicit in the tool's contract.

### 9. No formal SQL-quality eval harness

Despite being an NL→SQL-adjacent tool, there's no golden-test or offline eval harness
for query quality here — the only quality signal is the live `pattern record --success/
--failure` cache, an implicit, unaudited feedback loop. Chartworks' planned `eval/`
harness (`chartworks eval`) is already a stronger design than what this repo does; no
adoption needed, but the contrast is worth naming as validation that CLAUDE.md §11's
golden-test requirement is the right call.

### 10. Anti-pattern: unguarded SQL execution (name, do not adopt)

`analyzing-data`'s `run_sql`/`run_sql_many` execute **arbitrary, agent-authored SQL**
directly against the real warehouse via the user's own credentials in a persistent
Jupyter kernel — no read-only transaction mode, no schema allowlist, no
injection/parameterization guard at the SQL layer (the `AF_READ_ONLY` guard in
`astro-airflow-mcp` only gates Airflow *API* writes, a different subsystem entirely).
This is a defensible design for a single-developer local tool using their own
already-scoped credentials, but it is the precise shape of what CLAUDE.md §1/§6/§13
forbid for Chartworks: generated SQL must be read-only, schema-allowlisted, and
injection-guarded before it is ever executed, and access must be computed *inside* the
query, never left to warehouse-level ambient credentials. Worth citing in the eventual
SQL-safety RFC section as a concrete "here's the model we are deliberately not
building."

## Steal / Adapt / Skip

| Idea | Verdict | Why |
|---|---|---|
| Concept/pattern cache (business term → table, question → strategy) | **Adapt** | Bootstrapping aid for topic-pack authoring tooling; must gain access-scoping and versioning before use — not a substitute for the semantic model |
| Data-layer hierarchy ("prefer mart/metric over raw", large-table warning) | **Adapt** | Cheap dataset-metadata heuristic for the engineering stage |
| Categorical value-family discovery | **Adapt** | Improves dimension metadata quality for NLQ filter generation |
| Structured table-profile output (schema + quality score + recommended queries) | **Adapt** | Good normalized-shape precedent for an engineering-stage dataset profile |
| Freshness status scale (age bucket → Fresh/Stale/Very Stale/Unknown) | **Steal** | Directly reusable as a normalized dataset-freshness field; drop the DAG-escalation step (out of contract) |
| Connector interface method set (validate/deps/env/prelude) | **Steal (as precedent, not code)** | Confirms the shape of the §4.4 data-source adapter seam; nothing net-new |
| Exhaustive MCP tool-annotation allowlist test (fail-closed on unclassified tools) | **Steal** | Cheap CI gate for `internal/mcpserver`, fits P4 |
| Single-guard-function write gate, parametrized truthy/falsy env tests | **Adapt** | Pattern is good; Chartworks' pre-RFC posture should make read-only non-optional, not env-togglable |
| Consolidated, partial-failure-tolerant read tools | **Adapt with caveat** | Fine for availability aggregation; must never mask an access denial |
| Live pattern-cache success/failure as the only quality signal (no eval harness) | **Skip** | Chartworks' planned `eval/` harness is already the stronger design |
| Unguarded agent-driven SQL execution via Jupyter kernel + user credentials | **Skip (anti-pattern, name explicitly)** | Directly contradicts P1 SQL-safety; defensible only in a single-tenant, developer-trusted context Chartworks doesn't have |
| All DAG/orchestration, dbt/Cosmos, lineage, Astro-deployment, migration skills | **Skip** | No Chartworks analogue — no orchestration surface, no pipeline ownership |
| Layered CLI config (`~/.astro/config.yaml` + project `.astro/config.yaml` + local override, scope enum) | **Skip** | Local-CLI config-precedence concern; Chartworks is a server with typed config (§4/§5), not a local dev tool |

## Open questions

1. Does the RFC's semantic model (topic packs) want a lightweight, agent-writable
   "concept cache" layer in front of the curated model — e.g. for draft/candidate
   mappings pending topic-pack authoring — or is that scope creep the RFC should
   explicitly reject in favor of one authoritative model (P7)?
2. Should the engineering stage's dataset descriptor carry a "preferred layer" /
   materialization-tier field (idea #2), and if so, is it RFC-authored metadata or
   inferred at ingest time?
3. Is a freshness field (idea #5) in scope for V1's dataset descriptor, or deferred
   until a later phase — and if in scope, what's the data source for "last updated"
   across heterogeneous warehouse adapters (a per-adapter capability, likely)?
4. Worth an explicit decision entry (`docs/decisions.md`) citing this brief's finding
   #10 as the recorded rationale for *why* Chartworks' SQL-safety property forbids
   ambient-credential, unguarded execution, so a future contributor doesn't reach for
   the "just exec what the agent wrote" shortcut this repo takes?
