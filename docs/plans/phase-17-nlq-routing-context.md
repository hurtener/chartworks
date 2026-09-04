# Phase 17 — nlq-routing-context

Status: planned. Owner: internal/nlq. Hard dependencies: 05, 07, 15, 16.

## Authority and design

RFC-001 §8/9, D-045/D-049/D-053 and [COMMON.md](COMMON.md) apply. One ContextAssembler owns pruning. Signed reach is applied before retrieval or any model call. Learned embeddings/reranking use [the Bifrost gateway](../contracts/model-gateway.md); deterministic span hints/tokenization need no learned model.

## Brief findings incorporated

Briefs 03, 05, 08 and 14: typed facets, calibrated routing, compact budgets, provenance, pinned metrics and confirmed relationship evidence. Reuse the remote embedding/rerank provider configuration, not source-local NLP/cross-encoder runtimes.

## Findings I'm departing from

Similarity is not calibrated confidence. No local learned models, duplicate character-based budgets, hidden mandatory-constraint truncation or blanket language/multi-topic deferral. Missing rerank scores are not zero; provider failure cannot widen candidates or silently switch embedding spaces.

## Scope and implementation tasks

1. Implement light deterministic span hints, Bifrost-computed query embeddings, signed-authorized batched facet retrieval, per-kind limits, confidence calibration and typed routing outcomes.
2. Assemble cards, rules, examples, pinned metrics and provenance under one tokenizer-backed tier budget without shared mutation.
3. Preserve confirmed same-source multi-topic eligibility. When enabled, submit only already authorized candidates to Bifrost reranking, validate the permutation, then let the context owner apply top-k/budget reduction.
4. Cache under exact facet/embedding/context/authority semantics. On explicit `preserve_candidates`, keep the original authorized order and emit a typed warning; on `fail`, stop. Disabled rerank makes zero calls. Neither path starts a local model.

## Non-goals

No federation engine, semantic publication during routing, local embedding/cross-encoder service or provider-specific inference outside the gateway.

## Config and persistence

Context tiers, confidence bands, k-per-kind, max_examples=7 and advisory-rule budgets remain domain settings. Gateway owns remote model/provider/timeout/batch settings; routing only selects optional rerank and its declared failure behavior. Cache by full context/facet/embedding identity rather than tenant/question alone. Record reductions, confidence, priors, excluded advisory evidence, stage timing and fallback attribution independently.

## Acceptance criteria

1. **AC01** — Only published/healthy/signed-authorized topics/facets reach retrieval, reranking or generation; hints cannot expand reach and denied data causes no model call.
2. **AC02** — Batched origins/per-kind limits are correct; Bifrost reranking preserves permitted IDs and validates complete scores; disabled and explicit failure modes have the documented calls, order and warnings.
3. **AC03** — Tiers 1500/3000/6500 and example/rule bounds use one tokenizer currency, not a character estimate or locally inferred substitute.
4. **AC04** — Pinned metrics and mandatory constraints survive; impossible budgets yield typed insufficiency rather than a changed question.
5. **AC05** — Multi-topic choices require confirmed same-source joins and all-resource reach; ambiguous grain/cardinality triggers clarification.
6. **AC06** — English/Spanish, source-copy immutability, full-space/context cache isolation and concurrent-user fixtures pass with stage and remote-call attribution.

## Tests, coverage and smoke

Implement `TestPhase17/AC01` through `TestPhase17/AC06` with real facet retrieval and the real Bifrost adapter over recorded response fixtures. Spy on candidate payloads to prove authorization happened before external transmission. Cover missing/duplicate rerank indices, same-dimension embedding mismatch, timeout, visible fallback and immutable caller context. COMMON.md sets coverage; `scripts/smoke/phase-17.sh` requires all six results. G41 includes this consumer, not only gateway unit tests.

## Glossary, decisions and deviations

Calibrated confidence, retrieval similarity, deterministic fallback and remote inference are distinct. D-049/D-053 apply. No runtime completion is claimed.
