# Phase 24 — eval

Status: in_progress. Owner: internal/evaluation, internal/nlq. Hard dependencies: 18, 19, 20, 29. Current contract: [evaluation v1](../contracts/evaluation-v1.md).

## Authority and design

RFC-001 §13/16, D-049/D-050 and [COMMON.md](COMMON.md) apply. Quality evaluation is distinct from execution success and from planning-document validation.

## Brief findings incorporated

Briefs 03, 08, 12, 14: grounded correctness, replay/shadow, feedback-seeded cases, schema/context goldens, red-team testing and evidence-aware optimization.

## Findings I'm departing from

Do not drop source prompt/evaluation workflows solely because the implementation language changes. Do not claim live results from fixtures/skips or automatic semantic promotion from a higher score.

## Scope and implementation tasks

1. Build routing/context/SQL/validation/chart/reporting golden and adversarial suites with real-driver boundaries and separate recorded/live model paths.
2. Preserve replay/shadow comparisons, feedback-seeded cases and bounded prompt-pack optimization/evaluation with held-out data and human promotion.
3. Track correctness, safety, token/call cost, latency and version provenance; score result semantics rather than SQL text alone.

## Non-goals

No broad research benchmark competition, autonomous prompt/semantic release, unrestricted optimizer calls or production testing on undeclared customer data.

## Config and persistence

Eval per-suite thresholds, case/call/token/time ceilings and explicit fixture/live mode; optimizer off unless authorized/configured. Store exact input/version/result/evaluation provenance with protected content retention. Thresholds and accepted equivalence rules belong in versioned suite definitions, not unreviewed magic numbers. Live gateway configuration and pessimistic maximum-attempt cost are stored in an immutable runtime-pack record and require an explicit, distinct authenticated review over visible model, role, digest and cost values before any provider attempt.

## Acceptance criteria

1. **AC01** — Seeded regressions make the gate fail; security criticals have zero tolerance and quality thresholds are explicit per suite.
2. **AC02** — Routing/generation/context/chart tests pin inputs/versions and allow documented equivalent results without hiding semantic drift.
3. **AC03** — Adversarial suites cover identity/data scope, injection, dialect escapes, resource exhaustion, BYO and frozen-report invariants.
4. **AC04** — Replay/shadow and prompt-pack optimization respect signed reach, bounded calls and held-out cases; no optimizer auto-publishes semantics/prompts.
5. **AC05** — Live engine/model results are reported separately from fixtures/skips; no public benchmark equivalence or inherited timing is fabricated.
6. **AC06** — Evaluation history and feedback-case export are reproducible, content-protected and usable by the cutover ledger.

## Tests, coverage and smoke

Implement `TestPhase24/AC01` through `TestPhase24/AC06`, including deliberately failing cases that prove the gate can fail. Run documented live suites separately with their version/context evidence. COMMON.md sets coverage and evidence rules; `scripts/smoke/phase-24.sh` requires all six results.

## Glossary, decisions and deviations

Quality score, execution success and security correctness are separate outcomes. D-049 and D-087 apply. The deterministic runtime, protected persistence, CLI and six named acceptance tests are implemented for review. Live calibrated owner workloads and cross-system comparison remain Phase 34/25 evidence, not fixture-derived claims.

## CW-07 evaluation input

Routes now retain `evidence-v1` topic scores, facet-kind coverage, optional rerank
position, policy floor/margin and `semantic-interpretation-v1` digests. These are
reproducible inputs for AC02 and later differential calibration. Focused fixtures
prove policy behavior and fail-closed ambiguity; they do not establish live query
correctness calibration, which remains an explicit phase-24/final-gap measurement.

## CW-08 evaluation substrate

The runtime records bounded positive/negative evidence, posterior score,
uncertainty and immutable selection provenance, and exports a protected neutral
case bundle without promoting it. Phase 24 still owns held-out calibration,
time-decay policy, live quality measurement and optimizer decisions; none may
auto-publish an example or alter a frozen query.

## EXP-05 calibration boundary

Deterministic rule selection, conflict, semantic-edit, replay and authority
regressions are owned by phases 15/16/17 and are executable without a model.
Phase 24 still owns held-out and representative-user evaluation of conflict
explanation comprehension and live-model behavior. Those measurements must pin
the exact topic, ruleset, source/context and model revisions; synthetic contract
fixtures alone cannot close that quality boundary.

## EXP-01 continuity cases

The deterministic evaluation corpus now names bilingual add-dimension,
replace-filter, remove-filter, change-metric, clarification-correction and
semantic-republication cases. The real PostgreSQL service journey inspects the
persisted canonical route and fail-closed session/context/publication outcomes.
This closes the fixture/runtime disposition; live calibrated comparison remains
Phase 34/25 evidence.
