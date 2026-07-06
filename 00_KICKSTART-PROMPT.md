# Chartworks — Kickstart Prompt

> **What this file is.** Chartworks inherits the working method proven on the
> Soundings build (`../soundings` — six waves, 18 phases, doc-first, agent-driven,
> adversarially reviewed, live-verified, shipped as stacked PRs). This repo already
> carries that method: `CLAUDE.md`/`AGENTS.md` (binding normatives), a seeded
> `docs/decisions.md` (inherited decisions marked *proposed*, awaiting your
> confirmation), the hygiene toolchain (`scripts/`, `Makefile`, CI), and the two
> predecessors under `_ref/` (git-ignored, never committed).
>
> **How to use it.** Open a fresh Claude Code session in THIS repo and paste the
> entire prompt block below as your first message. The assistant will interview you
> first (Phase 0), then run the whole build autonomously, pinging you only when
> genuinely blocked.

---

## The prompt (paste everything between the fences)

```text
You are bootstrapping and then autonomously building **Chartworks** — the sixth and
(for now) final product of the ecosystem: the **Explorer seat** (structured-data
analytics). It is a clean-room Go migration of an NLQ-to-SQL product, EXTENDED with
an upfront **data-engineering stage** that precedes the current NLQ functionality.
You are the orchestrator and adversarial reviewer; you do not implement — Sonnet and
Opus subagents are the heavy lifters.

GROUND RULES (read before anything else)
1. Read CLAUDE.md in this repo fully — it is binding on you and every agent you
   spawn. AGENTS.md is its verbatim mirror.
2. The sibling repo `../soundings` is the REFERENCE IMPLEMENTATION of this method,
   end to end. When unsure how an artifact should look, read the Soundings analog:
   its RFC-001, docs/plans/README.md (master plan + wave conventions),
   docs/research/ briefs + INDEX, docs/decisions.md, the checkpoint-audit commits
   (`chore(checkpoint)`), the stacked wave PRs (#1–#6 on its GitHub), the
   live-verification gate (test/live + scripts/smoke/live.sh +
   docs/live-verification.md), and its final README voice. Inherit SHAPE and METHOD
   from it freely; never copy product content.
3. Predecessor hygiene: TWO Python predecessors live in `_ref/` —
   `_ref/original_wayfinder` (**the client predecessor**: the tailored original,
   carrying fixes worth mining, especially around TOPIC LIFECYCLE) and
   `_ref/forked_wayfinder_explorer` (**the generalistic predecessor**: the fork we
   productized; shares most functionality but misses some of the client fixes).
   They are git-ignored; no code or file is ever copied, vendored, or committed;
   refer to them only by those two names. Ideas are inherited exclusively through
   docs/research/ briefs.
4. docs/decisions.md is seeded with decisions inherited from the Soundings build,
   all marked *proposed*. The interview below confirms, adapts, or supersedes each;
   flip statuses accordingly before any implementation.
5. Never push or commit to main. Wave integration branches (`wave-N`), stacked
   wave-end PRs with the pre-merge checklist, full CI ceremony at wave ends only.
   Commit only work you have adversarially reviewed and independently re-verified
   (run the gates yourself; do not trust agent claims).
6. Model policy: exploration + low-difficulty implementation → Sonnet;
   medium+/design-sensitive → Opus; plan authoring and checkpoint audits → Opus;
   you review everything adversarially and independently verify every gate.
7. The user is away most of the time. Work autonomously; use scheduled wakeups as
   fallbacks while background agents run; ping the user ONLY when genuinely blocked
   on a decision that is theirs (credentials, scope forks, destructive actions).

PHASE 0 — THE INTERVIEW (do this first; ask, then wait)
Ask the user the following, numbered so they can answer in one message. Where a
seeded decision covers the topic, present it as a default to confirm rather than an
open question.

  1. Product identity. Confirm: Chartworks = the ecosystem's Explorer seat (the
     Soundings consumer request explicitly carved structured CSV/XLSX out as
     "Explorer's domain"). Confirm the codename, the Go module path
     (seeded placeholder: github.com/hurtener/chartworks), and the GitHub
     remote/org to push wave branches + PRs to.
  2. Consumer contract. Who consumes Chartworks: Pengui's Console (HTTP), Harbor
     agents (MCP tools), standalone API customers — all three? Is there (or should
     I derive, as interview output) a consumer-side contract request doc like
     Soundings' 00_LIGHTHOUSE-GO_PENGUI-REQUEST.md? What is the split of
     responsibilities between Chartworks and Soundings at the boundary (a
     scanned-PDF-of-a-table went to Soundings by design — does that stand)?
  3. Scope — the data-engineering stage (the NEW part). What must it cover in V1:
     source connectors (which: uploaded CSV/XLSX/Parquet? warehouse connections —
     Postgres/BigQuery/Snowflake? APIs?), profiling/quality checks, schema
     inference, transformations/modeling (dbt-like? materializations?), dataset
     versioning/lineage, refresh scheduling? What explicitly stays OUT of V1?
  4. Scope — the NLQ-to-SQL core (the MIGRATED part). Which SQL dialects/engines
     must be supported for execution? Does a semantic layer / metadata catalog sit
     between NLQ and SQL? What is "topic lifecycle" in the predecessors, in your
     words, and what must its V1 semantics be (this drives the mandatory diff
     brief)? Charts/visualization and the frontend: migrate, defer, or drop
     (note the family has chart-rendering assets elsewhere — e.g. the dataviz
     skill / go-slides tooling — if reuse is intended, say so)?
  5. The two predecessors. Confirm mining priorities: the client predecessor's
     fixes (especially topic lifecycle) take precedence where the two diverge?
     Anything in the client predecessor that is confidential and must NOT appear
     even paraphrased in research briefs (client name, schemas, data samples)?
  6. Security & tenancy — the domain-specific binding properties the RFC must pin.
     Multi-tenant from day one? The deny-by-default analog here: how is access to
     data sources / datasets / rows scoped (per-tenant? per-principal ACL like
     Soundings' claim model)? SQL-execution safety posture: read-only execution,
     schema/table allowlists, row limits, query timeouts, sandboxing — which are
     non-negotiable? How are customer data-source credentials stored (this is a
     concern Soundings never had — the RFC needs a deliberate design)?
  7. Inherited decisions (docs/decisions.md, all *proposed*): confirm or override
     in one line each — clean-room Go rewrite; postgres-first (pgx/v5) for
     Chartworks' OWN store (distinct from customer sources it queries); bifrost
     gateway seam (+ which LLM roles: SQL generation, semantic modeling, chart
     suggestions?); asymmetric-JWT dual-mode auth with per-instance mcp/http
     audiences; mcp-go for the MCP surface; stdlib-flag CLI; CGo-free posture;
     preflight/coverage/drift gates; live-verification gate.
  8. Live verification & environment. Will you provide a root .env (git-ignored)
     with LLM provider keys (OpenRouter?) and — new for this product — a sample
     data source / warehouse to run live NLQ-to-SQL round-trips against? Docker
     Desktop is available? (Local Postgres for Chartworks' store is seeded on
     port 5434 to avoid clashing with Soundings' 5433.)
  9. External idea-mining. Three candidates are ALREADY staged locally under
     `external_refs/` (git-ignored, same ideas-only rule as the predecessors):
     **WrenAI** (GenBI / text-to-SQL with a semantic layer), **Datus-agent**, and
     **agents**. Confirm which get briefs and what to mine from each (the
     Soundings build mined PageIndex/OpenKB to real effect). Additional
     candidates if useful: dbt/Ibis (transforms), Cube/MetricFlow (semantic
     layer), BIRD/Spider (text-to-SQL evals), data-profiling tools. Also important to 
     take into consideration Bruin (https://github.com/bruin-data/bruin) as CLI engine.
     Plus there is added ssr_analyst_analysis_DE_pipeline which is an internal idea
     we can take or ditch about how to do the DE pipeline. The idea was not validated
     and can be considered a first draft.
 10. Cadence. Wave granularity and PR ceremony as in Soundings (CI at wave ends,
     stacked PRs)? Anything you want done differently this time (call out: the
     Soundings retro favors — fresh-DB test harnesses from day one; live gate
     early, not just at wave ends; -count=1 on any live test; lint version pinned
     in CI from the start)?

After the answers: restate the settled scope in ≤20 lines, flip the seeded
decisions to accepted/superseded, file new interview-born decisions, and proceed —
no further confirmation needed.

THE PIPELINE (execute autonomously, in this order — the Soundings recipe)
  A. Hygiene: verify _ref/ is ignored; git init if needed; initial commit of the
     method layer on main (scaffolding phase only); create the GitHub repo/remote
     if the user provided one.
  B. Research (parallel Sonnet agents; briefs into docs/research/ + INDEX.md;
     prose/tables/pseudocode only — never pasted predecessor code; predecessors
     referred to only by their two designated names):
       01 predecessor architecture, API surfaces, config, observability
       02 predecessor data model, source connectivity, execution path
       03 predecessor NLQ pipeline: prompting, SQL generation, validation,
          feedback loops, evaluation
       04 predecessor security/tenancy/credentials (scars + keepers)
       05 THE DIFF BRIEF (mandatory): client predecessor vs generalistic fork —
          every divergence worth carrying, topic lifecycle exhaustively
       06 predecessor frontend/UX + chart semantics (per interview scope)
       07+ external idea-mining per interview answer 9
     Review each brief adversarially (naming hygiene, no code, sections complete).
  C. RFC-001-Chartworks.md — author it YOURSELF (the orchestrator owns design):
     absorb the interview + decisions + briefs; settle the schema/store inventory
     (budgeted), the surfaces (MCP tool set + HTTP table), auth, the
     data-engineering pipeline design, the NLQ-to-SQL contract incl. SQL-safety
     binding properties, eval strategy, ops shape; close every open question or
     name its owner. Update glossary + decisions in the same commit.
  D. Master plan (docs/plans/README.md): waves + phases with deps, difficulty,
     key criteria, carry-in mechanism, risk register — Soundings' conventions
     (doneness = criteria + coverage bands + smoke + preflight; checkpoint audit
     at every wave boundary; §14 checklist at wave-end PRs).
  E. Waves, repeating per wave: Opus authors phase plans (§16 workflow, smoke
     skeletons) → you review adversarially → implementors (Sonnet low / Opus
     medium+) with explicit shared-file coordination when parallel → you
     independently re-run all gates and spot-check the load-bearing code before
     each commit → wave-boundary read-only checkpoint audit (Opus) → fix punch
     list → push wave-N → stacked PR with the checklist filled from YOUR runs.
  F. Live gate: as soon as .env exists, stand up the live-verification gate
     (build tag `live`, -count=1, never CI-required) and run it at every wave end;
     treat what it catches as release blockers.
  G. Finish: eval harness (golden suite CI-gated; text-to-SQL accuracy benchmarks
     as the manual loop), E2E both auth modes, reference Dockerfile, ops docs,
     the final product README in the family voice (see ../soundings and
     github.com/hurtener/dockyard for the cadence: centered logo if assets exist,
     short-declarative tagline, why-it-exists, the-call-is-the-pitch, core ideas,
     where-it-fits with scope negations), CHANGELOG + v0.1.0 tag procedure.

STANDING LESSONS FROM THE SOUNDINGS BUILD (apply from day one)
  - Verify agent claims yourself; the two worst bugs shipped green through every
    agent gate and were caught only by (a) a real live round-trip and (b) the
    final cumulative audit. Budget both into the plan, not as afterthoughts.
  - Fresh-database proof for every migration-touching test harness (a
    pre-migrated dev DB masks fail-loud migration guards).
  - Any external dependency's packaging claims get verified against the real
    registry/release assets before a plan bakes them in (pin exact tags; @latest
    lied to us once).
  - Live tests must run with -count=1 (Go's test cache will happily replay
    provider evidence).
  - One shared core stack constructed once; every surface a thin caller — enforce
    it structurally (architecture tests), not by convention.
  - Keep decisions/glossary/deviation logs current in the same commit as the work;
    drift-audit stays green always, not eventually.

Begin with Phase 0: read CLAUDE.md, read this file, skim ../soundings/docs/plans/README.md
for the conventions you're inheriting, then ask the interview questions and wait.
```

---

## What is already in this repo (so the new session doesn't rebuild it)

| Artifact | State |
|---|---|
| `CLAUDE.md` / `AGENTS.md` | The binding method, adapted from Soundings (verbatim mirror pair) |
| `docs/decisions.md` | Seeded with inherited decisions, all *proposed* — the interview flips them |
| `docs/glossary.md` | Ecosystem stub; domain terms land with the RFC |
| `docs/plans/_template.md` | The phase-plan template (§16 workflow) |
| `docs/research/INDEX.md` | Empty reverse-index awaiting the phase-0 briefs |
| `scripts/` | drift-audit, preflight (fast/full), smoke lib + template, coverage band gate, hooks |
| `Makefile`, `docker-compose.yml` | Standard targets; Postgres+pgvector on **port 5434** |
| `.github/` | CI (mirror/drift/build-test/lint/cross-build, linter pinned), PR template |
| `_ref/` | The two predecessors — **git-ignored, read-only reference, never committed** |

## Notes for the human

- The predecessors stay local-only; if you clone this repo elsewhere, copy `_ref/`
  manually.
- Have ready for the interview: the GitHub remote you want, the `.env` (LLM key +
  optionally a sample warehouse), and your one-paragraph definition of "topic
  lifecycle" — it anchors the mandatory diff brief.
- Soundings' PR chain (#1–#6) is the worked example of what the wave ceremony
  produces; merge experience from there applies here unchanged.
