# Phase 17 — nlq-routing-context

Status: shipped. Owner: internal/nlq. Hard dependencies: 05, 07, 15, 16. Current cumulative evidence: [phases 15–18 and 21](../reviews/phase-15-18-current-evidence.md).

Proposed PR #11 delivery; this status becomes effective after every required hosted
check passes and the PR merges.

The current slice combines the deterministic context seam with a first routing
consumer: current topic/source contract admission, reviewed rule and slot
evaluation, Bifrost query embeddings, authorized batched facet retrieval,
optional candidate reranking, calibrated outcomes, and a typed same-source
one-to-one multi-topic join check. The pinned `cl100k_base` tokenizer,
1500/3000/6500 tiers, mandatory-lane preservation or typed insufficiency,
bounded omission audit, detached English and Spanish inputs, and immutable
caller boundaries remain owned by `internal/nlq`. Phase 17 closes against its
named real PostgreSQL/Bifrost AC01–AC06 routing/context fixtures; phase 18
separately verifies the downstream generation/execution consumers.

## Authority and design

RFC-001 §8/9, D-045/D-049/D-053 and [COMMON.md](COMMON.md) apply. One ContextAssembler owns pruning. Signed reach is applied before retrieval or any model call. Learned embeddings/reranking use [the Bifrost gateway](../contracts/model-gateway.md); deterministic span hints/tokenization need no learned model.

## Brief findings incorporated

Briefs 03, 05, 08 and 14: typed facets, calibrated routing, compact budgets, provenance, pinned metrics and confirmed relationship evidence. Reuse the remote embedding/rerank provider configuration, not source-local NLP/cross-encoder runtimes.

## Findings I'm departing from

Similarity is not calibrated confidence. No local learned models, duplicate character-based budgets, hidden mandatory-constraint truncation or blanket language/multi-topic deferral. Missing rerank scores are not zero; provider failure cannot widen candidates or silently switch embedding spaces.

## Scope and implementation tasks

1. The routing service performs deterministic bounded question admission, Bifrost query embeddings, signed-authorized batched facet retrieval, per-kind limits, confidence calibration, and typed routing outcomes.
2. It assembles retrieved evidence, reviewed advisory rules, mandatory constraint results, examples, pinned metrics, and provenance under the one tokenizer-backed tier budget without shared mutation.
3. Confirmed same-source one-to-one multi-topic joins are admitted only when every selected published definition independently confirms the same normalized relationship. The sealed, budgeted model header carries the complete ordered topic/version set; its singular topic fields remain a compatible first-topic projection and cannot disagree with that set. Unqualified metric IDs that match multiple selected topics return a typed invalid request instead of choosing one topic by order. When reranking is enabled, only already authorized candidates enter Bifrost; the complete permutation is validated before the context owner applies budget reduction.
4. The vector service remains an explicit no-evidence-cache boundary. Bifrost cache identity includes the full authority call and embedding space. Disabled rerank makes zero calls; provider preserve/fail behavior remains owned by gateway configuration. Neither path starts a local model.

## Non-goals

No federation engine, semantic publication during routing, local embedding/cross-encoder service or provider-specific inference outside the gateway.

## Config and persistence

Context tiers, confidence bands, k-per-kind, max_examples=7 and advisory-rule budgets remain domain settings. Gateway owns remote model/provider/timeout/batch settings; routing only selects optional rerank and its declared failure behavior. This slice has no vector evidence cache; if a later consumer adds one, its key must include the full authority, context, facet and embedding identity. Record reductions, confidence, priors, excluded advisory evidence, stage timing and fallback attribution independently.

## Acceptance criteria

1. **AC01** — Only published/healthy/signed-authorized topics/facets reach retrieval, reranking or generation; hints cannot expand reach and denied data causes no model call.
2. **AC02** — Batched origins/per-kind limits are correct; Bifrost reranking preserves permitted IDs and validates complete scores; disabled and explicit failure modes have the documented calls, order and warnings.
3. **AC03** — Tiers 1500/3000/6500 and example/rule bounds use one tokenizer currency, not a character estimate or locally inferred substitute.
4. **AC04** — Pinned metrics and mandatory constraints survive; impossible budgets yield typed insufficiency rather than a changed question.
5. **AC05** — Multi-topic choices require the same confirmed same-source join and all-resource reach; unrelated same-source joins and ambiguous grain/cardinality trigger clarification, exact ordered topic versions reach the model context, and duplicate unqualified metric IDs fail before Bifrost.
6. **AC06** — English/Spanish, source-copy immutability, full-space/context cache isolation and concurrent-user fixtures pass with stage and remote-call attribution.

## Tests, coverage and smoke

`TestPhase17/AC01` through `TestPhase17/AC06` exercise real facet retrieval and the real Bifrost adapter over recorded response fixtures. Candidate-payload spies prove authorization happens before external transmission. The suite covers missing/duplicate rerank indices, same-dimension embedding mismatch, timeout, visible fallback and immutable caller context. The HTTP/SDK acceptance uses the registered `POST /v1/nlq/routes` operation and `RouteNLQ`, including runtime OpenAPI composition. COMMON.md sets coverage; `scripts/smoke/phase-17.sh` requires all six results. G41 includes this consumer, not only gateway unit tests.

## Glossary, decisions and deviations

Calibrated confidence, retrieval similarity, deterministic fallback and remote inference are distinct. D-049/D-053 apply. The reviewed runtime candidate is recorded in [phase 17 routing evidence](../reviews/phase-17-routing.md). Coverage, lint, preflight and the local combined acceptance have passed. PR #11 records this phase as shipped conditionally; that status becomes effective after every required hosted check passes and the PR merges. Recorded Bifrost fixtures establish reproducible behavior; live semantic quality remains unmeasured.
