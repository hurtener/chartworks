# Brief 03 — The predecessors' NLQ-to-SQL pipeline: routing, context engineering, generation, validation, feedback, evaluation

> Status: draft · 2026-07-06 · sources: both predecessors

This brief mines the NLQ-to-SQL pipeline — the heart of the Chartworks migration — from
both Python predecessors' own architecture docs (primarily
`docs/architecture/01-pipeline/*.md`, `docs/architecture/02-learning/*.md`,
`docs/architecture/03-topics/*.md`, `docs/architecture/08-security/*.md`,
`docs/NLQ_CONTEXT_OPTIMIZATION.md`, `docs/TEMPLATE_CONTEXT_CONTRACT.md`,
`docs/adr/ADR-001-router-first-architecture.md`, `docs/adr/ADR-005-learn-positive-db-first.md`,
`docs/VALIDATION_ANALYSIS_SUMMARY.md`, `docs/VALIDATION_ERROR_REFERENCE.md`,
`docs/design/evaluation-quality.md`), plus a direct read of the source trees to verify a
few load-bearing claims (rule token budgeting; the current state of a documented
validation bug). No predecessor code is reproduced; behavior below is prose or
language-neutral pseudocode only.

**Summary.** Both predecessors share one router-first inference core: span extraction →
semantic retrieval → semantic routing → context packing → SQL generation →
validation/auto-fix, with an optional response-assembly layer for agent callers. The
crown-jewel mechanism is **context packing**: topic packs compress into small, schema-like
JSON evidence cards, budgeted by a query-complexity tier (low/medium/high → roughly
1000–1500 / 2000–3000 / 4000–6500 tokens), for a documented (unaudited) 40–70% token
reduction. Three template pathways feed generation — high-confidence **hints** (disable
few-shot), retrieval-based **examples** (enable few-shot), and **edit mode** (reuse a
template as an editable base) — resolved by one precedence rule. Validation is three
stages (pre-parse → parse → post-parse) with a bounded LLM self-curation repair loop on
execution failure. The client predecessor layers a **governed business-rules and
underspecification system** on top — its own token-budgeted injection block, a
proposal→shadow→active lifecycle, proactive clarification — that the generalistic
predecessor **does not carry at all**, the single largest divergence in this pipeline
(brief 05 owns the exhaustive diff). Evaluation exists mostly as a design document, not
confirmed production code, and deserves cautious treatment.

---

## 1. Routing

**Router-first as a foundational contract.** The client predecessor's ADR-001 commits the
whole system to a "router-first" architecture: every API shape (plan/run/self-curate/
debug) shares one inference core (topic routing → retrieval → evidence packaging →
generation → validation), so workerized jobs read the same router state instead of a
bespoke path. This is the strongest single idea to inherit literally: **one core, thin
surfaces** is already P7 in Chartworks' CLAUDE.md, and the predecessors' scar is exactly
what happens without that discipline (§9).

**Span extraction feeds routing, not the LLM.** A span extractor detects time/metric/
entity references via language detection plus lexicon/rule-based extraction and an
NLP-library-backed parser — CPU-bound, no model call — normalized by a span mapper into
one typed shape so retrieval and routing never reinterpret raw output independently.
CPU-heavy parsing offloads to a bounded worker pool with a per-language lock guarding a
shared parser instance — worth inheriting if Chartworks shares an NLP pipeline object
across concurrent requests.

**Semantic retrieval produces typed facet candidates, not raw schema.** A query embedding
plus lexical/semantic triage (and an optional cross-encoder rerank) surfaces candidates
typed as `M` (measure), `D` (dimension), `DKPI` (derived KPI), `QP` (query pattern — a
known SQL skeleton), `REL` (relationship/join path), and `template_facet` (a historical
SQL example for few-shot), with dynamic k-per-type and a per-tenant result cache. These
typed candidates — not tables/columns — are what routing and context packing operate on;
the semantic layer is the unit of retrieval, never the raw warehouse schema.

**Routing aggregates evidence into a decision, not just a score.** A semantic evidence
router combines span evidence and retrieval scores, applies template priors (§2.5) and a
multi-topic coherence check, and emits a `routing_decision` (`single_topic`,
`multi_topic`, `llm_fallback`) plus a confidence level and a primary/secondary topic set.
Routing is explicitly **request-safe under a shared singleton**: no per-request mutable
state on the router instance, cross-topic lookups tenant-scoped and cached by
`(tenant_id, topic_id)`, `tenant_id` passed explicitly through every helper — directly
relevant to Chartworks' P3 tenant-isolation-under-concurrency requirement and its "shared
instances are immutable after construction" rule (CLAUDE.md §5).

**Cost profile.** Embedding + retrieval is the primary CPU cost driver upstream of
generation; routing is nearly free once caches are warm.

---

## 2. Context engineering (the semantic layer) — the deepest section

This is the mechanism the kickoff singled out as the thing to keep and enhance: the topic
pack (governed semantic model), the packer (bounded evidence card), and generator-side
pruning (query-complexity-adaptive). A fourth, client-predecessor-only layer — governed
business rules — injects a parallel, independently budgeted block into the same prompt
(§2.6).

### 2.1 The topic pack and its facet index

A **topic** is the predecessors' name for what Chartworks' CLAUDE.md calls a semantic
model: a versioned, governed bundle of measures, dimensions, derived KPIs, query patterns,
join paths/relationships, and (client predecessor only) business rules and
underspecification config. Lifecycle: schema discovery pulls raw metadata from a warehouse
adapter → a draft version is LLM-enhanced (derived KPIs, business definitions/synonyms,
query patterns, example NLQs, inferred join paths) → human review/edits → promotion
(activates the version, refreshes the router cache) → reindex. Enhancement/discovery are
async and LLM-cost-dominated; promotion/cache refresh are cheap and synchronous — expensive
LLM work never sits on a query's hot path.

The enhanced pack is *not* embedded wholesale: a facet builder decomposes it into the same
typed units retrieval matches against (measure, dimension, derived-KPI, query-pattern, and
template facets), each embedded under a template/topic-scoped vector namespace, reindexed
async on promotion or template update. This is what makes retrieval (§1) return governed
semantic units instead of a free-text schema search — the retrieval target is engineered
at index time, not guessed at query time.

A parallel **dataset mode** (client predecessor; not confirmed in the generalistic fork,
§9) lets a caller upload a spreadsheet, profiles/sanitizes it, and synthesizes a topic pack
from the inferred schema so uploaded data flows through the *same* pipeline as a governed
warehouse topic rather than a bespoke second path. "One topic-shaped contract regardless of
source" is worth keeping regardless of what Chartworks' RFC names its data-engineering
stage.

### 2.2 The packer — evidence-card construction and hard caps

Context packing converts routing+retrieval output into one or more per-topic **evidence
cards**: it dedups measures/dimensions across cards resolving to the same field, prunes
schema fields for low/medium complexity (§2.3 gives exact budgets), merges retrieval-
selected template examples into the topic's query-pattern list (capped combined at 5), and
emits a merge report for session lineage. The packer itself is a small, single-purpose
node (single-topic / multi-topic / fallback) — notably thin, with essentially all real
pruning logic living one hop downstream in the generator (§2.3); the predecessors' own
docs flag this packer/generator pruning split as a never-completed "future enhancement"
(move empty-metadata pruning into the packer), a duplication trap worth avoiding.

**Card-level hard caps** (independent of §2.3's complexity-tier budgets): a topic's
compressed card view keeps only the top **5** measures and top **5** dimensions, top **3**
join relationships, and top **2** query patterns; business-definition strings truncate to
~200 characters. If the pre-built compressed view is unavailable, the generator falls back
to a smaller ad-hoc "top N" slice rather than the full topic pack. This is the single
biggest lever in the pipeline: it is *always* applied before complexity-tier pruning below
narrows the already-small card further — tiers never start from the full topic pack.

### 2.3 Generator-side adaptive pruning — the token-budget machinery

The generator applies a second, complexity-driven pruning pass whose target is an
explicit token budget, not just "smaller":

| Complexity | Routing-confidence band | Token budget | What's pruned |
|---|---|---|---|
| Low | < 0.70 | ~1000–1500 | Minimal schema (only selected columns), 2–3 few-shot examples, business definitions truncated to ~50 chars, top 2 join paths, empty metadata arrays stripped entirely |
| Medium | 0.70–0.85 | ~2000–3000 | Selected columns + related filter candidates, 3–4 examples, ~100-char business definitions, top 3 join paths |
| High | ≥ 0.85 | ~4000–6500 | No pruning — full context, all examples (documented ceiling of 7), full business definitions, all join paths |

Reported (self-measured, unaudited) reductions: ~68% at low complexity, ~45% at medium,
0% (by design) at high. Every call logs `complexity`, `original_tokens`, `pruned_tokens`,
`token_reduction_pct`, and few-shot count; the *unpruned* full context stays in result
metadata even when a pruned version was sent — never mutate the source evidence, only a
per-call copy, and treat "what got pruned and why" as a structured artifact, not something
to reconstruct from logs. Few-shot *selection* (not just count) is itself scored by
query-pattern shape, CTE-usage relevance, and domain-keyword overlap — re-ranked for
shape-fit rather than taking the first N retrieved.

### 2.4 The wire contract — what actually reaches the model

Three inputs reach a schema-constrained generation call: a short **business-context**
string (primary topic, join strategy, router confidence, domain, time window, template
hints, a complexity directive like "CTEs allowed, avoid window functions"), a
**structured-context** JSON blob (the pruned evidence-card payload), and the NLQ itself.
The JSON is documented, versioned, and **strategy-tagged** — a `template_context` object
always carries the same four keys (`strategy`, `edit_base`, `few_shot_examples`,
`hint_templates`) regardless of which strategy is active, so a consumer branches only on
the `strategy` tag, never on which fields exist. This "stable envelope, variant payload
gated by one discriminator field" is worth carrying into Chartworks' own context contract,
documented as strategy-agnostic and multilingual-safe.

Per-topic filters carry `name`/`operator`/`value` plus nullable `confidence` and
`source` — exposed to the model, not hidden. Generalizable idea: **when a filter's
provenance is uncertain, say so in the context itself**, rather than presenting it at the
same confidence as an explicit one.

### 2.5 Template resolution — four pathways, one precedence rule

Templates enter generation four ways, resolved by strict precedence: (1) **edit mode**
(highest, single-topic only) — requires `single_topic` routing, a high-confidence tier,
and a template clearing two thresholds (weight ≥ 0.9, similarity ≥ 0.8) plus a topic-match
check; **all few-shot demos disable** and the prompt gets a "start from this base SQL,
minimal edits" block; (2) **hints** (routing-derived known-good priors) — also disable
demos; (3) **examples** (retrieval-derived few-shot) — ranked by a hybrid score
(`similarity × (1-α) + w_template × α`), deduplicated by AST hash, MMR-diversified — become
ordinary demos only if neither higher path fired; (4) **fallback** — hardcoded/prompt-pack
demos. The precedence (`edit_base → clear; elif hints → clear; elif examples → inject;
else → default`) is documented *twice* (prose and a diagram) because engineers kept being
surprised demos "disappear." **Worth inheriting as one explicit, testable resolution
function**, not scattered conditionals.

### 2.6 Business-rule injection — a parallel, independently budgeted lane (client predecessor only)

The client predecessor's governed business-rules system injects its *own* block into the
same structured context, with its own independent token budget (default 300 tokens, a
char-count/4 estimator) — deliberately decoupled from §2.3's pruning budgets so governance
never crowds out semantic evidence or vice versa. Rules are selected per-query by
tenant/topic/measure/dimension/template scope, ordered by precedence (category:
computation < semantic < structural; specificity: template < compound < topic-wide; then
priority; then recency), and trimmed to budget — with *dropped* rules and *contradictions*
surfaced as named output fields, never silently discarded. Rule categories, targets, and
inline wording are structurally validated before injection, not passed as free text. This
"structured, scoped, budgeted, exclusions visible" design is the shape a governed
constraint layer should have; that it's entirely absent from the generalistic fork is the
headline divergence for this brief (§9).

---

## 3. SQL generation

Generation is schema-constrained (a typed input/output signature, not free-text
completion), driven by the three inputs in §2.4 through a DSPy-style predictor with an LLM
response cache to avoid re-paying for repeated identical calls. Concurrency is explicit:
because the generator instance is shared across concurrent requests, temporarily swapping
its few-shot demo set (§2.5) is guarded by an internal async lock so two requests can never
cross-contaminate demonstrations — a concrete instance of Chartworks' "reusable artifact
must be safe under concurrent use" rule (CLAUDE.md §5), worth an explicit design
requirement, not an afterthought.

**Prompt packs** are the tenant-tunable configuration surface for generation: base
instructions, demo sets, and template-selection parameters (source policy, max examples,
minimum confidence, ranking alpha, diversity lambda). A genetic/Pareto prompt-pack
optimizer (the published "GEPA" technique) proposes candidate packs; an autopilot worker
applies a guarded promotion policy, and the generator reloads its active pack on init or
explicit refresh — a prompt change is explicit and versioned, never silent drift,
generalizing Chartworks' P5 "a model change is explicit, never silent" rule to prompt
packs.

**Dialect targeting** is explicit per template (a `canonical_dialect` field on every
template/hint/edit-base record, §2.4), and dialect-aware parsing is what validation uses
downstream (§4). Nothing in the documented flow suggests cross-dialect transpilation at
generation time; a template authored for one dialect is presented as a hint/example for
that dialect, not rewritten. **Complexity hints double as a guardrail:** the same
low/medium/high tier driving token pruning (§2.3) also emits an explicit natural-language
constraint ("avoid window functions" at low, CTEs allowed at medium, anything at high) —
confidence-derived guardrails handed to the model as an instruction, not left implicit.

---

## 4. Validation

Three sequential stages, each with a distinct responsibility and distinct error-code
namespace:

1. **Pre-parse (structural).** Balanced parentheses/quotes, a FROM-clause presence check,
   dialect-specific date-function syntax checks (e.g. rejecting `DATEADD` mixed with an
   `INTERVAL` keyword, or a malformed `DATE_TRUNC` missing its comma) — before the SQL
   reaches a real parser. **A documented, now-resolved bug** is instructive: an earlier
   rule flatly rejected any query not starting with `SELECT`, silently blocking *every*
   CTE query even though the parser/validator had comprehensive CTE support that was
   simply unreachable — an estimated 15–25 point acceptance-rate cost while live. Current
   source no longer has this rule, but that a dedicated root-cause document existed for a
   bug this simple, shipped despite validator-level CTE tests, is the lesson: **a
   pre-parse stage duplicating a decision the real parser already makes correctly is a
   latent trap.**
2. **Parse.** Dialect-aware AST parsing via a SQL-transpiler library, cached to stay
   CPU-light — this is where "is this even valid SQL" gets a real answer.
3. **Post-parse (schema + policy).** Every table/column reference is checked against the
   topic pack's own schema — not the warehouse's full schema — producing named, typed
   error codes (`table.out_of_scope`, `table.topic_mismatch`, `column.not_in_table`,
   `column.undefined`, `join.unreachable_tables`) plus governance codes
   (`governance.table_blocked`, `governance.disallowed_statement`,
   `governance.injection_pattern`). CTE-scoped columns get warning-level codes rather
   than hard-fail, since a CTE's local scope can legitimately shadow a "real" name.

**Guardrails enforced at this layer:** DDL/DML blocking, multi-statement rejection,
row-cap enforcement, and — client predecessor only — an injection-pattern check and
sensitive-value screening tied to the rules lifecycle. Results cache per
tenant/dialect/schema-version. **None of this executes against a live warehouse by
default** — it validates a candidate SQL string; Chartworks' P1 "read-only,
schema-allowlisted, injection-guarded" property maps almost one-to-one onto this
three-stage design, modulo the CTE-blocking scar above.

**Auto-fix.** A minor, mechanically-fixable issue gets a safe automated transformation and
re-validation before returning, with a fix summary attached — never a silent swap.

**Self-curation (execution-time retry, distinct and narrower).** Only on an explicit
"run with self-curation" request: already-validated SQL executes against the warehouse; on
an execution error (a live warehouse error, not a validation error), a single LLM repair
attempt runs using the error context, is re-validated, and re-executed once. The result —
success or final failure — returns best-effort; there is no unbounded retry (contrast the
sibling KnowledgeProvider brief's far more expensive, loosely-bounded recompose-loop scar —
this one-attempt bound is the better pattern to inherit).

---

## 5. Feedback loops

A feedback event (a user/system verdict on previously generated SQL) is canonicalized and
persisted; a shared feedback service updates a canonical-SQL template record and enqueues
an async "learn-positive" job — feedback ingest is CPU-light and non-blocking.

**The learn-positive worker (client predecessor; DB-first per ADR-005).** On a positive
verdict, the worker re-embeds and re-paraphrases the template, persists deduplicated
paraphrases in a samples table keyed by `(tenant, template, sample_hash)`, rebuilds three
deterministic facet kinds, and recomputes routing weight via a blend of a Wilson-score
confidence interval, recency, and log-growth of supporting evidence — persisted back to
the template's own row, so the *database*, not an in-memory cache, is the durable source of
truth. Router-facing priors read from this persisted weight plus a short-lived cache, so a
positive event durably improves future routing without a cron job. ADR-005 requires
orchestration failures here to surface as structured, traceable error payloads — an early
instance of Chartworks' own P4 "fail loud" rule being learned the hard way.

**Rule learning (client predecessor only).** A positive *correction* (a supplied fix, not
just a thumbs-up) can propose a new governed rule through the same proposed→shadow→active
lifecycle as manually-authored rules (§2.6); best-effort, never blocking template learning.

**Template lifecycle:** seed → candidate (staging, pending evidence) → active (promoted
once weight/support thresholds are met — eligible for hints/edit-mode, §2.5, not just the
lower-precedence examples pathway) → deprecated/retired, with every transition audited.

**Rule lifecycle (client predecessor only).** `proposed → shadow → active`, or any state
`→ rejected`/`retired`; shadow evaluation compares effect before promotion; sensitive
literals must be marked `sensitive` and an unmarked one is rejected outright (fail-loud,
not a silent skip).

**Replay (client predecessor only).** Before activating a rule/underspecification change,
a replay service re-runs it against historical queries through the *same* production
pipeline (carrying a shadow/excluded rule id set) rather than a parallel simulator —
expensive but structurally clean.

---

## 6. Evaluation

**Caveat up front:** the primary source here (`docs/design/evaluation-quality.md`) reads as
a **design document with example code**, not confirmed shipped infrastructure — treat this
section as intended design, not a verified in-repo capability, until brief 05 or direct
source verification says otherwise.

**Golden datasets.** A named, versioned collection of NLQ + expected-SQL examples (plus
alternative-acceptable SQLs, an expected-result hash, a category, and a difficulty tier),
with a coverage report by topic/category/difficulty; examples can seed directly from
*correct*-verdict feedback events (§5), closing the loop without hand-authoring every case.

**Comparison strategy.** SQL comparison normalizes (case, whitespace, quotes) then checks
exact match against the expected SQL or an alternative; failing that, token-Jaccard
similarity ≥ 0.9 counts as `partial` rather than `fail`. An expected-result hash match
overrides a SQL-text mismatch — "did we get the right answer" outranks "did we write it
the same way."

**Red-team suite.** A fixed catalog of adversarial NLQs across six categories (prompt/SQL
injection, data exfiltration, privilege escalation, resource exhaustion, semantic
confusion), each with an expected safe behavior; the runner checks guardrail blocking
first, then inspects for category-specific safety signals only if the guardrail passed.
Reasonable starting checklist for Chartworks' own adversarial test obligations
(CLAUDE.md §11), independent of whether this exact harness ships.

**Gating (as designed, not confirmed live).** CI runs both suites on PRs and nightly,
fails on a pass-rate threshold (0.85 documented) or any critical red-team failure, and
diffs against a main-branch baseline.

---

## 7. Failure & clarification flows

**Ambiguity, assumptions, and risk are first-class output, not side effects.** An optional
response-assembly layer (for agent-facing callers) collects a weighted per-stage
confidence breakdown, an ambiguity assessment (score-gap between competing topics/
templates, ambiguous time/metric), an assumptions list (e.g. a defaulted time range), and a
risk assessment (validation issues, PII-pattern hints) — then recommends execute-vs-clarify
and, when clarifying, suggests specific questions. It never executes anything itself, and
resolves internal ids to human labels before returning — never leaking an id as a
user-facing label (the same "never fabricate an id-as-title" principle the sibling
KnowledgeProvider brief flags as worth keeping).

**Underspecification detection (client predecessor only) is topic-scoped and proactive.**
Rather than only reacting to a low routing-confidence score, a dedicated detector runs
topic-specific ambiguity *patterns* (LLM-generated at topic-enhancement time, mergeable
with manual patterns, cached per topic) against the NLQ, producing explicit clarification
slots — "which region," "which time grain" — *before* generation runs, rather than
generating a guessed SQL and hoping it was right. The most direct answer to "what should
Chartworks do when a query is genuinely ambiguous": detect it against a governed,
topic-specific pattern set, and ask.

**Failure-mode summary:**

| Failure | Documented behavior |
|---|---|
| No retrieval candidates | Routing falls back; context packing proceeds with minimal evidence |
| Missing/unavailable prompt pack | Falls back to hardcoded demos and default template config |
| Invalid SQL | One auto-fix attempt, then error propagation |
| Vector-store search-batch mismatch | Falls back to non-batched search, or errors |
| Warehouse execution error (self-curate only) | One LLM repair attempt, then best-effort result |

Two of these ("no candidates," "missing prompt pack") degrade gracefully rather than
failing outright — each is exactly the kind of "empty result that reads as nothing found"
case Chartworks' P4 asks to distinguish from a genuine denial or validation failure. A
phase plan should decide, per fallback, whether degrade-and-continue or Chartworks'
stricter typed-error-plus-metric default is correct, not assume they match.

---

## 8. Keepers

- **One router-first core, three thin API shapes over it** (ADR-001) — directly validates
  Chartworks' own P7.
- **Two-layer context-card compression**: hard structural caps at the card level (top-5
  measures/dimensions, top-3 joins, top-2 patterns) applied *before* a second,
  complexity-tier token budget narrows further; **prune, log the reduction, keep the
  unpruned original in metadata** — never mutate the source evidence.
- **One explicit, testable template-precedence rule** (edit-base > hints > examples >
  default) rather than scattered conditionals — documented twice because it kept
  surprising engineers.
- **A stable envelope with one strategy discriminator field** for the template wire
  contract — callers branch on one field, never on which keys exist.
- **Per-filter provenance exposed to the model** (`confidence`, `source`, nullable) instead
  of presenting an inferred filter at the same confidence as an explicit one.
- **A governed constraint layer gets its own independent token budget**, with dropped items
  and contradictions surfaced as named output fields rather than silently discarded.
- **DB-first, not cache-first, for learned state** (ADR-005): template weight lives in the
  database with a short-lived cache read-through, so a restart never regresses quality.
- **Bounded, single-attempt execution-time repair** (self-curate) over an unbounded
  recompose loop.
- **A proactive, topic-scoped ambiguity detector before generation**, producing named
  clarification slots, instead of relying solely on a post-hoc confidence score.
- **Golden-dataset import directly from positive feedback events** — closes the loop from
  production correction into the regression suite without hand-authored duplication.

---

## 9. Scars

- **A pre-parse structural check duplicated a decision the real parser already made
  correctly, and duplicated it wrong** — blocking 100% of CTE queries for an estimated
  15–25 point acceptance-rate regression, caught only by after-the-fact root-cause
  analysis despite existing validator-level CTE tests, implying no standing regression
  test caught it where it was introduced. Current source has since dropped the offending
  check. Lesson: a pre-parse stage should do only what a real parser cannot do more
  correctly, and the process gap that let a stage duplicate another's already-correct
  logic matters more than the one-line fix.
- **The governed business-rules/underspecification layer and cache-attribution telemetry
  exist in the client predecessor and are absent from the generalistic predecessor's own
  docs and source tree** (confirmed by direct diff) — the single largest capability gap
  between the two codebases in this pipeline. The RFC needs to decide deliberately whether
  a governed-rules layer is V1 scope. Brief 05 owns the full inventory; this brief flags it
  as load-bearing context for §2.6.
- **The generalistic predecessor's own tenancy docs state its enforcement middleware is
  "not yet wired"** for several tenant-sensitive routes, versus the client predecessor's
  more developed authorization story — a "cleaned-up" fork regressing a security property
  the client-specific version had hardened under production pressure. Any pattern
  inherited from the generalistic fork needs its own tenancy audit.
- **Two independently-tuned confidence scores** (routing confidence, driving both routing
  and complexity tiering; template weight, driving promotion and edit-mode eligibility) are
  never reconciled into one calibrated notion — a query can be "high confidence" for
  token-budget purposes while its template is barely past the edit-mode threshold. Decide
  up front whether Chartworks wants one confidence primitive or several, explicitly.
- **A char-count/4 token estimator governs the rules-injection budget** while topic-pack
  pruning budgets are tuned against real measured token counts — two budget currencies
  feeding one prompt, which can drift out of proportion as prompts evolve.
- **Self-measured, unaudited token-reduction and acceptance-rate numbers** (the 40–70%
  figures, the 15–25-point CTE-fix estimate) are the *only* quantitative evidence for the
  context-optimization strategy — treat as directional, not a validated baseline.
- **Evaluation/quality gating is documented at design-doc fidelity only** — unlike the
  sibling KnowledgeProvider predecessor's actual (if imperfect) in-repo eval harness, this
  pipeline's evaluation story reads as aspirational; do not assume §6 runs today.

---

## 10. Open questions for the RFC

1. **Does Chartworks inherit a governed business-rules/underspecification layer**, and at
   what phase — it is the largest capability gap between the two predecessors, and
   CLAUDE.md's P1 SQL-safety property (RFC-pending) could naturally live partly in such a
   layer (schema/table blocking, injection-pattern rejection are already rule-adjacent in
   the client predecessor) — but it needs a deliberate decision, not silent inheritance.
2. **What is Chartworks' one calibrated confidence primitive** (if any) replacing the two
   independently-tuned scores in §9 — do routing confidence, template weight, and any
   future SQL-safety confidence collapse into one quantity, or stay separate with
   documented relationships?
3. **Where does the two-layer evidence-compression idea (§2.1–2.3) live in Chartworks'
   package layout** — card capping as a `semantics` concern and token budgeting as an `nlq`
   concern, or one component, avoiding the predecessors' "pruning split across two layers"
   scar?
4. **Does Chartworks adopt the three-way template-precedence model**, a subset, or
   something else — and if kept, is it one unit-tested resolution function from day one?
5. **What replaces the char-count/4 token estimator** — one real tokenizer-backed budget
   currency for every context lane, so nothing drifts out of proportion as prompts evolve?
6. **Is the pre-parse/parse/post-parse validation split kept as-is**, and what guardrail
   (a golden CTE fixture; a "pre-parse must not duplicate parser-level judgment" review
   rule) prevents reintroducing the CTE-blocking regression in §4?
7. **Does self-curation ship in V1**, given CLAUDE.md's hard gate against executing
   generated SQL before SQL-safety is settled — self-curation by definition executes SQL,
   so it cannot land before that property is pinned.
8. **What does Chartworks' evaluation harness inherit from §6**, given its design-doc-only
   status — is golden-dataset-plus-red-team still the right shape, and does gating run in
   CI from day one?
9. **Does Chartworks keep a proactive, topic-scoped ambiguity detector** before generation
   (§7), or fold ambiguity handling into routing-confidence thresholds alone — the
   predecessors' docs suggest the proactive approach catches cases a pure threshold misses.
10. **What would a BYO-agent context-handoff mode need from this layer?** If an external
    agent (not Chartworks' own generator) receives the assembled context and returns SQL
    for Chartworks to validate: (a) the handoff contract is almost exactly §2.4's
    `structured_context`/`business_context` split, but must become a **published,
    versioned, stable schema**, not an internal call signature — every field justified
    today as "helps our predictor" must instead be justified as "helps an arbitrary
    external agent produce compliant SQL," a stricter bar; (b) the three-stage validator
    (§4) becomes the *only* trust boundary — it cannot assume the SQL came from a
    cooperative internal generator, so every guardrail must hold against an adversarial or
    careless external agent; (c) rules/governance and underspecification (§2.6, §7), if
    kept, must run *before* handoff and have their constraints **restated inside the
    handed-off context** rather than assumed enforced by a cooperating generator, since an
    external agent has no reason to know a governed rule exists unless told; and (d) the
    template-precedence pathways (§2.5) mostly stop applying once generation is
    externalized — "hints disable demos" has no meaning for an agent not running the same
    predictor — but the underlying idea generalizes: attach known-good prior SQL to the
    handoff payload as *optional, provenance-and-confidence-labeled* guidance, letting the
    external agent decide how to weigh it rather than silently constraining generation.
