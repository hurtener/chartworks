# Phase 17 — nlq-routing-context

> **Status:** draft
> **Owner:** orchestrator (Wave 5, critical path)
> **Depends on:** phase-05-gateway, phase-07-vindex, phase-15-topics-lifecycle, phase-16-rules-clarification

The read side of the NLQ pipeline up to (but not including) generation: turn a
natural-language question into a **routing decision** and an assembled, budgeted
**query context**. Generation, validation, and execution are phase 18 — this phase
stops at the stable envelope handed to a generator (mode a) or published as a
context bundle (mode b, phase 19). The context-engineering machinery here is the
kickoff's named crown jewel (D-013), kept and enhanced.

---

## RFC / request sections

- **RFC §9.1 (Routing)** — span hints → facet retrieval → evidence aggregation →
  `routing_decision` + calibrated confidence; published ∩ health-eligible ∩
  grant-visible eligibility; shared-singleton router.
- **RFC §9.2 (Context assembly)** — the `ContextAssembler` producing the query
  context; the stable envelope with a `strategy` discriminator.
- **RFC §8.3 (Context engineering — the crown jewel)** — facet decomposition,
  two-layer compression under one owner, one tokenizer-backed budget currency,
  never-mutate-source, governed-rules lane, provenance, **one confidence
  primitive** (routing confidence is *the* calibrated quantity; example weight is a
  documented prior that feeds it — the relationship is pinned here per §8.3, closing
  brief 03 Q2).
- Cross-refs consumed, not implemented here: RFC §8.4 (rules/clarification lanes —
  phase 16), §9.3–9.6 (generation/validation/execution — phase 18), §9.4 (BYO
  bundle — phase 19), §5 (grants resolver — phase 04), §13/§14 (gateway/config).
- Decisions relied on: **D-028** (lexicon-light span hints, no in-process NLP
  library), **D-029** (`vindex` facet vectors), **D-020** (grants + the one
  resolver), **D-003** (gateway seam, schema-constrained), **D-027** (governed
  rules + clarification are V1 scope), **D-013** (context engineering is the crown
  jewel), **D-043** (per-role provider config; `rerank` as the eighth gateway role,
  consumed here as an optional config-gated retrieval stage — amends RFC §13).
  Downstream: **D-021** (validation), **D-022/D-014** (BYO handoff).

## Depends on

- **phase-05-gateway** — the `embedding` role (question + facet vectors) and the
  optional `rerank` role (D-043) flow through the gateway seam; the `mock` driver
  backs every routing test. Routing itself makes **no generative LLM call** (span
  hints are model-free; retrieval uses only the pinned embedding role plus, when
  enabled, the metered rerank role).
- **phase-07-vindex** — facet retrieval reads `(tenant, topic, version)`-scoped
  facet vectors via the `vindex` seam.
- **phase-15-topics-lifecycle** — supplies published, health-stamped topic packs,
  the **capability contract** with structural card caps already baked at publish
  time, and the facet vectors themselves. Routing eligibility reads the lifecycle's
  published/health state.
- **phase-16-rules-clarification** — supplies the governed-rules lane (with its own
  independent token budget) and topic-scoped clarification slots that the assembler
  splices into the query context. This phase provides the *hook*; phase 16 owns the
  rule/slot content and the `nlq.rules_lane_budget` config key.

## Informing briefs

- **`docs/research/03-predecessor-nlq-pipeline.md`** — the backbone (§1 routing, §2
  context engineering, §2.4 the strategy-discriminated envelope, §2.5 precedence,
  §7 clarification, Q2/Q3/Q5 the confidence/pruning-location/tokenizer questions).
- **`docs/research/05-predecessor-diff.md`** — the fork's governance-version filter
  before routing, and source-table health as a first-class routing gate.
- **`docs/research/08-datus-agent-ideas.md`** — metrics-first routing (prefer a
  pre-vetted metric/query-pattern shape before ad-hoc generation).
- Secondary: **`docs/research/02-predecessor-data-and-execution.md`** for the
  tenant-scoped cache scope and its `GLOBAL_CACHE_TENANT` fallback bucket pattern.

## Brief findings incorporated

- **Router-first, one shared core (brief 03 §1, keeper).** A single router computes
  the routing decision every surface reads; per-request state rides `ctx`, the
  router singleton is immutable after construction (P7, CLAUDE.md §5).
- **Span hints feed routing, not the LLM (brief 03 §1).** A lexicon-light,
  CPU-bound extractor produces typed **time / metric / entity** hints with **zero
  gateway calls** — D-028's "no in-process NLP library, no bilingual model
  downloads" is honoured: the lexicon is a small in-repo phrase/pattern table
  (relative-date words, comparison/aggregation verbs, known canonical-entity
  synonyms from the pack's registry), not a language model.
- **Typed facet candidates, not raw schema (brief 03 §1, brief 05 §"router").**
  Retrieval returns candidates typed measure / dimension / derived-KPI /
  query-pattern / example with **dynamic k-per-type** and a **tenant-scoped**
  result cache — the unit of retrieval is the governed semantic facet, never the
  warehouse schema.
- **Evidence aggregation → a decision, not just a score (brief 03 §1).** The router
  combines span + retrieval evidence, applies a multi-topic coherence check, and
  emits `routing_decision ∈ {single_topic, multi_topic, clarify, no_route}` + one
  calibrated confidence + primary/secondary topics.
- **Governance-version + health filter before routing (brief 05 §"router", §230).**
  Only **published** versions are eligible (the fork's `_governed_version_ids`
  keeper); **source health** gates further (`is_available`/`health_status` — the top
  client-predecessor carry) so routing never selects an unavailable table; and the
  §5 **grant** visibility intersects on top. Eligibility = published ∩ healthy ∩
  granted.
- **Two-layer compression, one owner (brief 03 §2.2/§2.3, Q3).** Structural card
  caps (top-5 measures / top-5 dimensions / top-3 joins / top-2 patterns; truncated
  definitions) are baked into the capability contract at publish (phase 15); the
  **complexity-tier token budget** prunes further at query time. **All runtime
  pruning lives in one component** — `ContextAssembler` — never split across a
  packer and a generator (the predecessors' never-completed "move pruning into the
  packer" trap).
- **One tokenizer-backed budget currency (brief 03 §2.6, Q5, scar §9).** Every lane
  — evidence, rules, examples — is measured by the same `TokenCounter`. No
  char-count/4 estimator anywhere; the two-currencies-feeding-one-prompt drift is
  the named counterexample.
- **Never mutate source evidence (brief 03 §2.3, keeper).** Pruning operates on a
  per-call copy; the **unpruned** contract stays in result metadata; every assembly
  logs `original_tokens / pruned_tokens / reduction_pct` as a structured artifact,
  not something reconstructed from logs.
- **Stable envelope, one strategy discriminator (brief 03 §2.4, keeper).** The query
  context is one documented envelope: consumers branch on a single `strategy` tag,
  never on which keys exist; all lanes are always present (possibly empty).
- **Provenance on everything (brief 03 §2.4).** Per-filter and per-prior
  `source` + `confidence` (nullable) are exposed in the context itself — an inferred
  filter is never presented at the same confidence as an explicit one.
- **One confidence primitive (brief 03 Q2, RFC §8.3).** Routing confidence is the
  single calibrated quantity; **example weight** (learned, §9.8, phase 18) is a
  documented *prior* that feeds routing confidence via the aggregation function —
  it is never surfaced as a second competing score. The aggregation contract is
  unit-tested here.
- **Metrics-first routing (brief 08 §106).** Query-pattern (`QP`) and derived-KPI
  facet hits are weighted as pre-vetted "metrics-first" evidence and recorded in the
  routing evidence, so generation (phase 18) can prefer a known metric shape over
  ad-hoc SQL. (The *direct metric-execution path* is not built here — see non-goals.)
- **Optional rerank, gateway-based (brief 03 §1, D-043).** The predecessors' optional
  cross-encoder rerank pattern is carried in its API-based form: a **config-gated**
  stage between retrieval and evidence aggregation, through the new gateway `rerank`
  role (metered like every role, P5). Off (default) ⇒ the retrieval order stands
  byte-identical; on ⇒ candidates rerank through the gateway; a gateway error or
  hard timeout **falls back to the original order loudly** — logged + metriced,
  never silent (P4).
- **Tenant-scoped cache + the flagged GLOBAL fallback (brief 02 §252).** The
  retrieval cache key carries `(tenant, topic, version)`. Chartworks' tenant is
  mandatory (P3), so the predecessors' `GLOBAL_CACHE_TENANT` bucket is adopted only
  as a **fail-loud regression guard**: if a lookup ever resolves no tenant, it never
  serves a cross-tenant entry and increments a metric — the exact unscoped-cache
  leak P3 exists to prevent.

## Findings I'm departing from

- **The predecessors' `llm_fallback` routing outcome** is renamed to the RFC's
  `clarify` / `no_route` split (RFC §9.1): a genuinely ambiguous question is a
  *typed* `clarify` decision with clarification slots, and an unroutable one is a
  *typed* `no_route` — never an empty result that reads as "nothing found" (P4).
  Routing does not silently fall through to an LLM.
- **Graceful-degrade fallbacks (brief 03 §7 "no candidates → proceed with minimal
  evidence", "missing prompt pack → hardcoded demos").** Chartworks does *not* carry
  the silent degrade path: no candidates ⇒ `no_route` (typed, metered); there is no
  prompt-pack concept to fall back from (prompt packs / GEPA autopilot are out of
  V1 scope). This is P4 over the predecessors' degrade-and-continue.
- **`ContextAssembler` package placement — a flagged RFC reconciliation.** RFC §8.3
  labels the pruning owner `semantics.ContextAssembler`, but the authoritative
  package table (RFC §3.2, which "settles §3 of CLAUDE.md") assigns *context
  assembly (both modes)* to `internal/nlq`. This plan places the assembler in
  **`internal/nlq`**, consuming the capability contract that `internal/semantics`
  bakes at publish; §3.2 governs placement. The "one owner" invariant is preserved
  (all *runtime* pruning is in `nlq.ContextAssembler`; `semantics` only bakes the
  static card caps). Filed as a follow-up to align §8.3's label with §3.2 — flagged,
  not silently ignored (CLAUDE.md §2/§15).
- **The in-process cross-encoder rerank (brief 03 §1) and the DSPy response cache
  (§3).** The torch-based in-process cross-encoder is not carried (D-028); its
  **API-based replacement ships here** as the optional, config-gated gateway
  `rerank` role (D-043) — see "Brief findings incorporated." The free-text-cached
  predictor is not carried; the retrieval cache above covers the hot path.

## Scope

Delivers in **`internal/nlq`** (new package, created this phase):

- **Span hints** (`nlq/spans`): lexicon-light, model-free extraction of typed
  time / metric / entity hints; one normalized `SpanHints` shape; a per-language
  lexicon table (no NLP library, D-028); safe for concurrent use.
- **Facet retrieval** (`nlq/retrieval`): embeds the question via the gateway
  `embedding` role, queries `vindex` under `(tenant, topic, version)`, returns typed
  `FacetCandidate`s with per-type k caps; the tenant-scoped retrieval cache with the
  flagged GLOBAL-fallback guard; the **optional config-gated rerank stage** via the
  gateway `rerank` role with loud timeout-fallback (D-043).
- **Router** (`nlq/routing`): the shared, immutable-after-construction router;
  eligibility filter (published ∩ healthy ∩ granted); evidence aggregation → typed
  `RoutingDecision` + one calibrated confidence + primary/secondary topics; the
  aggregation function that folds example weight in as a prior.
- **Context assembler** (`nlq/context`): the single runtime pruning owner —
  complexity-tier selection from confidence bands, the `TokenCounter`-backed budget
  applied over per-call copies of the (already card-capped) capability contract,
  reduction logging, the rules-lane and clarification-slot hooks (phase 16), the
  example lane (phase 18 feeds it), and the stable `QueryContext` envelope with the
  `strategy` discriminator and per-filter provenance.
- Config: the `nlq` domain keys below.
- Telemetry: the NLQ funnel counters for the routed/clarified segment, retrieval
  cache hit/miss + the GLOBAL-fallback guard counter, the rerank timeout-fallback
  counter (P4), and per-tier reduction gauges.

## Non-goals

- **SQL generation, precedence resolution, validation, execution** — phase 18. This
  phase produces the *input* to generation, not SQL. The example precedence rule
  (`edit_base > hints > examples > default`) is phase 18's; this phase only assembles
  the example lane as candidates.
- **The BYO context bundle** (`get_query_context`, `bundle_version`) — phase 19; it
  is a published projection *of* the query context assembled here.
- **The direct metric-to-SQL execution path** (brief 08) — routing merely *weights*
  metric/query-pattern evidence; executing a pre-vetted metric shape is generation/
  exec territory (phase 18).
- **Learn-positive weight recomputation** (§9.8) — phase 18. This phase *reads* a
  stored example weight as a routing prior; it does not compute or update it.
- **Prompt packs / GEPA autopilot, cross-encoder rerank, shadow rule evaluation** —
  out of V1 scope (RFC §8.4 defers shadow eval; the rest are not carried).
- **HTTP/MCP surfacing** — phases 21/22 expose `preflight`/`plan`; this phase ships
  the core they call.

## Design

**Data flow (read side):**

```
question + envelope(tenant,principal,session,grants)
  │
  ├─▶ spans.Extract        (model-free; typed time/metric/entity hints)
  │
  ├─▶ retrieval.Candidates (gateway embedding → vindex(tenant,topic,version)
  │                          → typed FacetCandidate[], k-per-type, tenant-cached)
  │
  ├─▶ routing.Route        (eligibility = published ∩ healthy ∩ granted;
  │                          aggregate evidence + example-weight prior + metrics-first
  │                          weighting → RoutingDecision{decision, confidence,
  │                          primary/secondary}; coherence check)
  │
  └─▶ context.Assemble     (tier ← confidence bands; TokenCounter budget over a
                             per-call copy of the capped capability contract;
                             + rules lane (§16) + clarification slots (§16)
                             + example lane; reduction log; unpruned kept in metadata)
                            → QueryContext{strategy, business_ctx, evidence,
                              rules_lane, example_lane, clarification_slots, provenance}
```

**Span hints (D-028).** A `spans.Extractor` holds an immutable lexicon (relative-date
vocabulary, aggregation/comparison verbs, canonical-entity synonyms sourced from the
pack registry at construction). `Extract(ctx, question) SpanHints` is pure and
allocation-light — **zero gateway calls, no `spacy`/NLP-library import** (asserted by
an architecture test). Hints are advisory evidence to routing and retrieval, never a
hard filter, so a lexicon miss degrades gracefully to embedding-only routing (not a
`no_route`).

**Facet retrieval.** `retrieval.Candidates(ctx, q, eligibleVersions)` embeds `q`
through the gateway `embedding` role (metered; with rerank off — the default — the
only model call on the read path), then does a per-type `vindex` search scoped `(tenant, topic, version)` for
each eligible version, capping at `nlq.retrieval.k_per_type` per facet kind. Results
are typed `FacetCandidate{kind, topicVersion, facetID, score, payload}`. A
`(tenant, topic, version, q-hash)` cache (TTL `nlq.retrieval.cache_ttl`) fronts it;
the cache scope is tenant-mandatory, and a `GLOBAL` bucket exists only as a fail-loud
guard (never served, metered if hit) per brief 02.

**Optional rerank (D-043).** When `nlq.retrieval.rerank_enabled` is on, the typed
candidates pass through the gateway **`rerank`** role (the API-based replacement for
the predecessors' in-process cross-encoder — brief 03 §1; the torch version stays
excluded by D-028) **before evidence aggregation**. Semantics: off (default) ⇒ the
retrieval order stands unchanged — the rerank code path is not entered and results
are byte-identical to baseline; on ⇒ candidates rerank through the gateway, metered
like every role (P5), under a hard timeout (`nlq.retrieval.rerank_timeout`, plus the
request's context deadline); on timeout or gateway error the stage **falls back to
the original retrieval order** with a structured log + a dedicated metric — never a
silent degrade (P4). Rerank reorders; it never adds, drops, or mutates candidates
(the k-per-type caps and typed shapes are unchanged), and it runs after the cache
(cached entries store the pre-rerank order, so toggling the flag never poisons the
cache).

**Routing + the one confidence primitive.** `routing.Route` runs the eligibility
filter first — **published** (lifecycle stage, phase 15) ∩ **healthy**
(`is_available`/`health_status`, phase 15) ∩ **granted** (the §5 resolver's effective
access on the envelope, `topic` grain with `query` permission). An empty eligible set
short-circuits to `no_route` (P1a/P4 — no retrieval, no query). Evidence aggregation
folds span hints, retrieval scores, the multi-topic coherence check, the
metrics-first weighting of `QP`/`derived-KPI` hits, and — as a **documented prior** —
each candidate example's stored routing weight, into **one** calibrated
`confidence ∈ [0,1]`. The aggregation function is a single, unit-tested pure function
`aggregate(evidence, priors) → (decision, confidence)`; example weight enters only
through it and is never returned as a second score (RFC §8.3, brief 03 Q2). The router
is a shared singleton: lexicon, config, and seam handles are set at construction and
immutable; all per-request state (candidates, scratch scores) lives in locals/`ctx`.

**Context assembler — the single pruning owner.** `context.Assemble(ctx, decision,
contract, rules, slots, examples) QueryContext`:

1. Selects the **complexity tier** from `nlq.confidence_bands`
   (`< medium → low`, `[medium, high) → medium`, `≥ high → high`) and the matching
   budget from `nlq.complexity_budgets` (defaults 1500 / 3000 / 6500).
2. Works on a **deep per-call copy** of the capability contract (already card-capped
   at publish: ≤ top-5 measures / 5 dimensions / 3 joins / 2 patterns). It asserts
   the caps hold on its output and prunes *from the capped card*, never the full
   pack — tiers never re-expand.
3. Measures every lane with **one `TokenCounter`** — a pure-Go BPE token counter
   (tiktoken-family encoding, `nlq.tokenizer.encoding`, pinned and verified against a
   fixture per master-plan convention 8; CGo-free, D-005). No `len/4`. The
   evidence/example lanes prune to the tier budget; the **rules lane** is measured by
   the same counter but budgeted independently by phase 16's `nlq.rules_lane_budget`
   (governance never crowds out evidence, brief 03 §2.6) — dropped rules and
   contradictions arrive as *named fields* from phase 16 and are passed through, never
   silently discarded.
4. Emits the stable `QueryContext` envelope: a `strategy` discriminator (all lanes
   present, possibly empty — consumers branch on `strategy` only), `business_ctx`
   (topic, join strategy, confidence, time window, dialect + complexity directive),
   the pruned evidence, the rules lane, the example lane, clarification slots, and
   **per-filter/per-prior provenance** (`source` + nullable `confidence`).
5. Records `original_tokens / pruned_tokens / reduction_pct` (structured, per lane)
   and stashes the **unpruned** contract in result metadata — never mutating the
   source (immutability test).

**P1–P7 upholding.** P1a: eligibility intersects grants inside the eligible-version
set before any retrieval; empty ⇒ typed `no_route`, no query. P3: every `vindex` and
store read carries the mandatory tenant predicate; the retrieval cache is
tenant-keyed with the fail-loud GLOBAL guard. P4: `clarify`/`no_route` are typed
decisions + metrics, never empty results; a lexicon/retrieval miss is a typed outcome.
P5: the only model calls are the metered `embedding` role and (when enabled) the
metered `rerank` role, both through the gateway seam; routing aggregation and
assembly are model-free. P6: the envelope carries domain nouns only (no
"embedding"/"index"/"cache" on the wire). P7: one router, one assembler, one confidence
primitive — no parallel paths; the same assembled context feeds mode-a generation
(phase 18) and the mode-b bundle (phase 19).

## Config keys added

`nlq` domain (RFC §14). `nlq.rules_lane_budget` is **consumed** here but **owned by
phase 16** (not re-declared). Card caps (top-5/5/3/2) are owned by phase 15's
capability-contract build (not re-declared here). The `rerank` role's per-role
provider block (`gateway` domain, `{provider, model, credential, endpoint?, params}`
— D-043) is owned by phase 05's config surface; this phase adds only the `nlq`-side
gate below.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `nlq.complexity_budgets.low` | int (tokens) | 1500 | no | Low-tier context budget (RFC §14). |
| `nlq.complexity_budgets.medium` | int (tokens) | 3000 | no | Medium-tier budget. |
| `nlq.complexity_budgets.high` | int (tokens) | 6500 | no | High-tier ceiling (no pruning at/above it). |
| `nlq.confidence_bands.medium` | float | 0.70 | no | low→medium confidence threshold (RFC §14). |
| `nlq.confidence_bands.high` | float | 0.85 | no | medium→high confidence threshold. |
| `nlq.example_caps.max` | int | 7 | no | Few-shot example-lane ceiling (RFC §14; brief 03 §2.3 documented ceiling of 7). |
| `nlq.retrieval.k_per_type` | int | 5 | no | Max facet candidates per type from `vindex`. |
| `nlq.retrieval.cache_ttl` | duration | 5m | no | TTL of the `(tenant,topic,version,q-hash)` retrieval cache. |
| `nlq.retrieval.rerank_enabled` | bool | false | no | Config-gated rerank of facet candidates via the gateway `rerank` role (D-043). Off ⇒ retrieval order stands unchanged. |
| `nlq.retrieval.rerank_timeout` | duration | 2s | no | Hard timeout on the rerank gateway call; on expiry, loud fallback to the original order (logged + metriced, P4). |
| `nlq.tokenizer.encoding` | string | `cl100k_base` | no | Token-counter encoding (tiktoken-family); pinned + fixture-verified (convention 8), CGo-free (D-005). |

Documented in the plan (here), the example config (§14 config file, this PR), and
smoke-checked (a bad `nlq.*` value ⇒ typed fail-loud boot error, exercised by the
config validator; see smoke check 12).

## Acceptance criteria

Numbered, mechanically checkable; each maps to a smoke assertion below.

1. **Eligibility = published ∩ healthy ∩ granted (table-driven).** Over the full
   `(published?, healthy?, granted?)` truth table, a topic version is routable iff
   all three hold; every other combination is excluded from candidates and, when it
   empties the set, yields a typed `no_route` with no retrieval/store query issued.
2. **Span hints are model-free (D-028).** The span extractor makes **zero** gateway
   calls (mock call-count assertion) and imports no in-process NLP library
   (architecture test); it returns typed time/metric/entity hints for the fixture
   corpus.
3. **Typed, k-capped, tenant-scoped retrieval.** Retrieval returns `FacetCandidate`s
   typed per kind with ≤ `k_per_type` per kind; a cross-tenant probe returns nothing
   (adversarial); the retrieval cache key carries `(tenant, topic, version)` and the
   GLOBAL fallback bucket is never served and increments its fail-loud metric if
   reached.
4. **One calibrated confidence.** Routing returns exactly one `confidence ∈ [0,1]`;
   the aggregation function folds a candidate's example weight as a prior and never
   surfaces it as a second score (unit test on `aggregate`).
5. **Per-tier token budgets hold (golden).** For each tier (low/medium/high) the
   assembled context's `TokenCounter` total stays within the configured band; the
   golden token counts are pinned and a regression trips the test.
6. **Card caps applied before tier pruning.** The assembled context keeps
   ≤ top-5 measures / ≤5 dimensions / ≤3 joins / ≤2 query patterns at every tier, and
   tier pruning demonstrably starts from the capped card, never the full pack.
7. **Pruning never mutates source.** The input capability contract is byte-identical
   before and after `Assemble`; the unpruned contract is present in result metadata;
   each assembly logs `original/pruned/reduction` per lane (immutability + reduction
   log test).
8. **One tokenizer currency.** Every lane (evidence, rules, examples) is measured by
   the same `TokenCounter`; no `len(x)/4`-style estimator exists in the package
   (architecture/grep test); the rules lane respects its independent budget.
9. **Router safe under concurrent reuse (`-race`).** The shared router singleton
   carries no per-request mutable receiver state; concurrent routing of distinct
   tenants under `-race` shows no cross-contamination of candidates or decisions.
10. **`no_route` and `clarify` are typed (P4).** Both are typed decisions with
    structured payloads (reason / clarification slots), never an empty result read as
    "nothing found"; a metric increments per outcome.
11. **Stable envelope + provenance (golden).** The `QueryContext` carries a
    `strategy` discriminator with all lanes always present, and per-filter/per-prior
    `source` + nullable `confidence`; a golden envelope test asserts consumers branch
    on `strategy` only.
12. **Config fail-loud.** A malformed `nlq.*` value (e.g. `complexity_budgets.high <
    medium`, or a non-tiktoken `tokenizer.encoding`) is rejected at boot with a typed
    error; the shipped example config validates.
13. **Config-gated rerank (D-043).** With `rerank_enabled` off (default), retrieval
    results are **byte-identical** to the no-rerank baseline (no gateway `rerank`
    call is made — mock call-count zero); with it on against the gateway `mock`
    driver, the reordering is applied to the candidates (order differs per the mock's
    fixture, membership/typing/k-caps unchanged); a simulated gateway timeout falls
    back to the original retrieval order **loudly** — the fallback metric increments
    and a structured log is emitted, never a silent degrade.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven — the eligibility truth table (1), the `aggregate`
  confidence function (4), tier selection from confidence bands (5), card-cap
  enforcement (6), the `TokenCounter` against pinned fixtures (5/8), envelope shape
  (11), config validators (12), the rerank gate's three-way semantics — off /
  on-with-mock / timeout-fallback — against the gateway `mock` driver (13; paired,
  per §10, with the `rerank` role's recorded-fixture test owned by phase 05).
- **Integration:** required — this phase consumes phase-05 (gateway `embedding` +
  `rerank`),
  phase-07 (`vindex`), phase-15 (capability contracts + health), phase-16 (rules/
  slots) and opens the `ContextAssembler`/`RoutingDecision` interfaces phase 18/19
  build on. An integration test wires the real `vindex` (Docker Postgres + pgvector,
  `make pg-up`) and the gateway **`mock`** driver (the one sanctioned boundary mock,
  paired with a recorded-fixture embedding test) and proves tenant/grant propagation
  end-to-end from question to assembled `QueryContext`. Lives in `internal/nlq`
  (the package *is* the wiring boundary) with a slice in `test/integration/`.
- **Adversarial:** required (routing reads the grants/access path). Cross-tenant
  retrieval probe (3), empty-access-set short-circuit ⇒ typed `no_route` with a
  store call-count of zero (1/10), forged-envelope attempt has no effect (the frozen
  envelope is read-only), and a fetch-then-filter regression guard (grants intersect
  *before* retrieval, not after). Derived from the mechanically-built registry, not a
  hand list.
- **Fuzz:** `FuzzSpanExtract` (arbitrary/multilingual question bytes never panic and
  never emit a hint referencing an out-of-lexicon span) and `FuzzTokenCounter`
  (arbitrary bytes never panic; count is monotone under concatenation) — the two
  parse/estimate surfaces on this hot path.
- **Bench:** `BenchmarkRoute` and `BenchmarkAssemble` (both hot, reusable, shared
  singletons) — baselines, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 defaults; `internal/nlq` is a new `internal/` package.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/nlq` | 80% | New `internal/` package (default). Added to `scripts/coverage-bands.conf` in this PR. |

## Smoke checks

`scripts/smoke/phase-17.sh` SKIPs cleanly until `internal/nlq` exists, then runs the
package tests via `run_group` (one `go test` process). The `-race` group runs
criterion 9 under the race detector.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestRoutingEligibilityMatrix` (published ∩ healthy ∩ granted truth table) |
| 2 | `TestSpanHintsModelFree` (zero gateway calls + no NLP import) |
| 3 | `TestFacetRetrievalTypedTenantScoped` (k-per-type, cross-tenant nothing, GLOBAL guard) |
| 4 | `TestOneCalibratedConfidence` (single confidence; example weight as prior) |
| 5 | `TestTierTokenBudgetsGolden` (per-tier golden token counts) |
| 6 | `TestCardCapsBeforeTierPruning` (top-5/5/3/2 at every tier) |
| 7 | `TestPruneNeverMutatesSource` (input identical; unpruned in metadata; reduction logged) |
| 8 | `TestOneTokenizerCurrency` (one `TokenCounter`; no char/4; rules-lane budget) |
| 9 | `TestRouterConcurrentReuse` (run under `-race`) |
| 10 | `TestNoRouteClarifyTyped` (typed decisions + metrics, never empty) |
| 11 | `TestQueryContextEnvelopeStrategy` (golden envelope; strategy discriminator; provenance) |
| 12 | `TestNLQConfigFailLoud` (malformed `nlq.*` ⇒ typed boot error; example config valid) |
| 13 | `TestRerankConfigGated` (off ⇒ byte-identical + zero rerank calls; on-with-mock ⇒ reordered; timeout ⇒ loud fallback to original order) |

## Glossary additions

Pre-written for `docs/glossary.md` (Domain / Internals, this PR) — only terms not
already present:

- **Span hint** — a lexicon-light, model-free typed reference (time / metric /
  entity) extracted from a question to feed routing and retrieval; advisory, never a
  hard filter (D-028).
- **Facet candidate** — a typed semantic unit (measure / dimension / derived-KPI /
  query-pattern / example) returned by facet retrieval, capped k-per-type; the unit
  routing operates on, never raw schema.
- **Routing decision** — the router's typed verdict `single_topic | multi_topic |
  clarify | no_route`, with one calibrated confidence and a primary/secondary topic
  set.
- **Routing confidence** — the single calibrated quantity in [0,1] driving
  eligibility gating and complexity-tier selection; example weight is a documented
  prior that feeds it, never a second competing score (RFC §8.3).
- **Complexity tier** — low / medium / high, chosen from routing-confidence bands,
  setting the context token budget.
- **Context assembler** — the single runtime pruning owner: applies the complexity-
  tier token budget over a per-call copy of the (already card-capped) capability
  contract, splices the rules and clarification lanes, emits the query context, and
  logs the reduction — never mutating source evidence.
- **Query context** — the assembled, stable envelope handed to generation (or
  projected into a BYO bundle): business context + pruned evidence + rules lane +
  example lane + clarification slots, gated by one `strategy` discriminator.
- **Token counter** — the single tokenizer-backed budget currency measuring every
  context lane (evidence / rules / examples); no char/4 estimator (RFC §8.3, brief
  03 Q5).

## Decisions filed

Files **no new decision entry** (per authoring scope). Relies on existing: **D-028**
(lexicon-light span hints), **D-029** (`vindex` facets), **D-020** (grants + one
resolver), **D-003** (gateway seam), **D-027** (rules/clarification V1 scope),
**D-013** (context engineering is the crown jewel), **D-005** (CGo-free tokenizer),
**D-043** (the `rerank` gateway role, consumed here config-gated; RFC §13 as
amended).
One **flagged reconciliation** (not a decision): RFC §8.3 labels the pruning owner
`semantics.ContextAssembler` while §3.2 places context assembly in `internal/nlq`;
this plan follows §3.2 and files a follow-up to align §8.3's label — see "Findings
I'm departing from."

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring. -->
