# Phase 17 — nlq-routing-context

Status: planned. Owner: internal/nlq. Hard dependencies: 05, 07, 15, 16.

## Authority and design

RFC-001 §8/9, D-045/D-049 and [COMMON.md](COMMON.md) apply. One ContextAssembler owns runtime pruning; signed reach is applied before retrieval/routing, not after broad context reaches a model.

## Brief findings incorporated

Briefs 03, 05, 08, 14: typed facets, calibrated routing, compact token budgets, source provenance, explicit metric pins and multi-topic relationship evidence.

## Findings I'm departing from

Do not confuse retrieval similarity with calibrated routing confidence. No local NLP-model downloads, duplicate char-based budget, hidden constraint truncation or blanket language/multi-topic deferral.

## Scope and implementation tasks

1. Implement light span hints, authorized batched semantic retrieval, explicit per-kind limits, confidence calibration and routing outcomes.
2. Use one ContextAssembler for compact cards, tokenizer-backed tier budgets, pinned metrics, constraints, examples and provenance; no shared-state mutation.
3. Retain confirmed same-source multi-topic eligibility and optional reorder-only rerank, with explicit non-route/clarify/fallback results.

## Non-goals

No cross-warehouse federation, semantic publication during routing, or provider-specific context path outside the gateway.

## Config and persistence

NLQ context tiers, confidence bands, k-per-kind, max_examples=7, advisory-rule budget, optional rerank and locale behavior. Cache only under full topic/facet/context/authority semantics; do not cache merely by tenant/question. Record confidence, priors, reductions, excluded advisory evidence and stage timings separately.

## Acceptance criteria

1. **AC01** — Only published/healthy/signed-authorized topics/facets route; untrusted hints cannot expand the set.
2. **AC02** — Batched retrieval preserves origin and per-kind limits; confidence/prior weighting and rerank fallbacks are independently visible.
3. **AC03** — Context tiers 1500/3000/6500 and example/rule bounds are obeyed using one tokenizer currency, not a second character estimate.
4. **AC04** — Pinned metrics and mandatory constraints survive; impossible budgets yield typed insufficiency instead of silently changing the question.
5. **AC05** — Multi-topic choices require confirmed same-source joins and all-resource reach; ambiguous cardinality/grain triggers clarification.
6. **AC06** — English/Spanish, source-copy immutability, cache-partitioning and concurrent-user fixtures pass with stage timing attribution.

## Tests, coverage and smoke

Implement `TestPhase17/AC01` through `TestPhase17/AC06` with real facet retrieval, token-count goldens, multi-user concurrency and ambiguous join/grain fixtures. Optional rerank fixtures must prove reorder-only behavior. COMMON.md sets coverage; `scripts/smoke/phase-17.sh` requires all six results.

## Glossary, decisions and deviations

Calibrated confidence, example prior and execution restriction are not interchangeable. D-049 applies. No runtime completion is claimed.
