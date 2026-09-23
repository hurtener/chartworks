# Generation packet v2: selected physical context and strategy fit

Status: implemented for review in PR #62; this contract covers the AP-02 initial
slice. Exact CI results are maintained in the PR, not inferred from this document.
This extends phases 17/18 and SQL-03/04/05 without changing identity authority,
SQL approval, or the frozen reporting execution path.

## Two distinct projections

`AssembledContext.Relations` remains the complete reviewed physical projection.
The route, current-source admission, persistence, repair and replay continue to
compare it exactly; the native validator still narrows against reviewed scope.

For a single-topic route with selected-semantic evidence and pinned metrics, the
rendered physical schema is derived from mandatory column dependencies, including
selected grouping/filter/time definitions and connecting joins. Every rendered
column must exist in the corresponding full reviewed relation. Missing/foreign
coordinates are an error, not a fallback to an incomplete prompt. Rich native
column types, nullability and business meaning remain in mandatory dependencies.
Optional retrieved text does not establish the mandatory physical projection.
A newly hydrated semantic candidate carries its own exact reviewed physical
mappings with its dependency definitions in `semantic-evidence-v2`; both are
retained or omitted atomically. Retained v1 evidence is not rewritten. This avoids
orphaning an optional dimension or metric when deterministic selection covered only
part of a question. A candidate remains optional and does not activate rules or
certify a join to the selected graph. Unrelated
catalog growth therefore does not spend the selected query's prompt budget.

Unselected/legacy contexts, dimension-only contexts without metrics, and multi-topic
contexts retain the full physical rendering in this first version. There is no
new authorization subset supplied by a client. No persisted SQL is rewritten.
The marker `physical_projection:selected-closure-v1` distinguishes the renderer;
this is not a certificate that free-text intent or SQL semantics are complete.
Broader paraphrase and continuation interpretation remain separately qualified.

## One example lane and declared order

`edit_base > hints > examples > default` is decided before final fitting.
Instruction order is preserved; stable IDs do not replace ranked example order.
Context examples are merged after explicitly ranked instruction examples, with
identical entries deduplicated and conflicting definitions rejected. All included
demonstrations live in the instruction lane, at most seven. Edit/hint packets have
no typed example demonstrations in their base context. Learned-example repository
selection and reranking are skipped when edits or hints win. This does not skip
normal admission, route retrieval or any applicable authority check.

Required context and selected edit/hint/default instructions must fit. Examples
and optional evidence/advisory groups are then admitted whole under the same
pinned tokenizer. Optional overflow records an omission, rather than blocking a
valid edit. When no demonstration fits, default guidance is the explicit fallback.
The detached retained context is reassembled/resealed by the existing context owner.
The original route input and its assembly audit are not mutated.

`GenerationContext.fit`, when present, uses `generation-fit-v2`; it records bounded
IDs and omission reasons with an uncapped total, not discarded source text. It
covers the final fit in addition to the original route's omission audit.
`ExampleSelectionEvidence.usage`, when present, uses `example-prompt-usage-v1` and
records actually rendered learned examples with final positions, separately from
candidate ranking. Absent fields on retained legacy records mean unknown, not zero.
These additive protected evidence fields do not grant authority or require rewriting
saved SQL. Usage describes the initial generation packet; repair is distinct.

Validation repair does not nest optional demonstrations in its edit packet.
Original essential edit/hint/default instructions and mandatory semantic context
remain; failed unbound SQL and parameter kinds remain available, never private
bound scalar values. Existing one-repair and native validation boundaries remain.

## Effective provider input and remaining qualification

The actual Bifrost adapter rechecks the byte ceiling after applying server-owned
runtime system instructions, before reservation or provider dispatch. Recorded
TLS fixtures inspect the effective model, full system and user messages, response
schema, configuration receipt and configured output ceiling on the serialized
provider request. Rejected oversized runtime additions spend zero provider calls.

This is not yet a model-specific combined context-window fitter. Context tier
counts still cover the assembled user packet, while the gateway applies its byte
and pessimistic attempt/output reservation bounds. Model-aware system/schema/wire
and output-token allocation with optional packet refitting across that full envelope
remains AP-02 work. No live-model correctness, latency or cloud-dialect claim follows
from recorded wire tests. Frozen report refresh adds no generation calls.
