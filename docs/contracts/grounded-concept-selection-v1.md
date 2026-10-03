# Grounded concept selection v1

S2 implementation. This contract separates structural grounding, model intent,
source authorization, analytical proof and held-out quality qualification. It is
not a claim that a model's interpretation or quoted evidence is always correct.

## Explicit policy and bounded model work

`concept_policy: grounded-v1` on the ordinary Route/Preflight/Plan question enables
one bounded semantic-choice operation through the existing `clarify` gateway role.
Without it the historical deterministic path and cost profile remain unchanged.
Explicit references or metric IDs override the model policy and make no concept
call. The same opt-in field is carried by Refine, replayable saved selections and
the SDK. A disabled role, exceeded budget or unsupported candidate count does not
silently fall back to an ungrounded executable result.

Every topic/source/dataset/context is admitted before the choice. The candidate
set consists only of currently reviewed measures, KPIs and dimensions from those
publications: at most 64, at most 128 KiB total JSON. It is not a top similarity
slice presented as a complete catalog. Exceeding the bound requires a narrower
authorized topic/explicit references. Cards include compact labels, descriptions,
aggregation/KPI inputs and coordinates, but no governed-value lists or filter
literals. The ID pins the complete topic version/digest and exact reference.

The gateway sees the known-value-redacted question and strict response schema.
Current sensitive answer spellings, defaults, governed canonical values and
aliases are removed locally before model dispatch. Catalog text is data, never a
system instruction. No embeddings/rerank scores are interpreted as confidence.
This adds at most one bounded operation (two provider attempts), not a new model
role or a second SQL generation/correction allowance. The adapter's normal full
request envelope and permission checks apply. Early routing work is not free.

## Closed decisions and evidence

`select` requires 1–8 distinct candidate IDs and a unique, exact, non-overlapping
question excerpt for each. Excerpts cannot consume even part of a redaction marker.
The persisted proof stores hashes, IDs and byte spans, not a second copy of private
text. This proves membership and attribution only—not semantic truth. A selected
KPI's ingredients become required dependencies, not extra independent outputs.

`clarify` requires 2–8 distinct supplied alternatives and no selected IDs. Labels
and exact topic/reference coordinates are reconstructed from reviewed candidates,
not supplied by the model. Clients may submit those references through a fresh
ordinary Plan. `no_match` has neither selections nor alternatives. Both outcomes
return a non-executable clarification result: no SQL plan or warehouse call, and
no SQL repair intended to force an answer. General durable model-question state
remains S12; opaque option IDs are not bearer authority or form answer tokens.

Selection runs before the existing rules/answer-dependent fixed point. Selected
roots use the explicit `grounded_model` reason; the ordinary closure produces the
full nested KPI, typed-column, filter and relationship context for generation.
The policy does not infer a scalar predicate or a grouping merely from choosing a
dimension. Analytical intent and projection retain their independent conservative
rules; unsupported joins, grains or formulas do not become supported by this mode.

## Protected persistence, answers and replay

The choice evidence is a response/snapshot field, never a client question input.
Native execution and terminal replay reconstruct current authorized candidates
and verify exact input/catalog/choice hashes without a model call. Missing,
changed or unknown policy/proof metadata cannot fall back to reselection. A model
root requires its policy and evidence even in a query without private predicates.
Historical deterministic records are not backfilled or reinterpreted.

A reviewed rule preflight may retain a selected concept while awaiting its typed
scalar answers. A same-session authorized Plan submission replays that protected
original before using request-local custody of its choice. The incoming question
and catalog must still match; changing the question through stateless submission
cannot borrow the old origin. Answer evaluation and private predicate binding are
unchanged. Refine retains prior independent model roots as explicit reviewed IDs,
not by rerunning the old paraphrase or copying SQL authority; deliberate typed
replacement/removal uses the existing APIs. Ambiguous/no-match model questions
are resolved via fresh reviewed inputs, not the pending-rule shortcut.

No database migration, execution permission, result exposure, live model credential
or frozen-refresh interpretation is introduced. SDK projections remain transport,
not proof verification. Counterfactual tampering is checked by the authoritative
service, not by trusting the public evidence's hashes as an authentication token.

## Qualification

Required tests cover EN/ES model-proposed paraphrases, nested KPI closure, denied
sources, malformed/foreign/overlapping evidence, explicit and legacy no-call
paths, exact-source replay, private defaults/aliases, schema bounds, saved policy,
SDK choices and actual PostgreSQL results. Recorded provider responses exercise
integration and rejection, not model accuracy. Q1/Q2/Q3 still require the paired
original/replacement cohort, real deployment models and held-out ambiguity and
selection calibration. Opt-in availability is not a blanket default cutover.
