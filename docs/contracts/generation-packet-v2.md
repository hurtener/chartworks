# Generation packet v2: selected physical context and strategy fit

Status: implemented for review in PR #62; this contract covers AP-02 and its
scoped extensions. Exact CI results are maintained in the PR, not inferred from
this document. This extends phases 17/18 and SQL-03/04/05 without changing identity
authority, SQL approval, or the frozen reporting execution path.

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
certify a join to the selected graph. Unrelated catalog growth therefore does not
spend the selected query's prompt budget.

The original single-topic metric renderer retains the marker
`physical_projection:selected-closure-v1`. Unselected/legacy inputs still retain
their full rendering. AP-02C below extends scoped dimension-only and multi-topic
inputs; it does not rewrite stored SQL or introduce a client authorization subset.
Neither renderer certifies complete free-text intent or SQL semantics. Broader
paraphrase and continuation interpretation remain separately qualified.

## AP-02C: topic-scoped dimension and multi-topic rendering

The marker `physical_projection:topic-selected-closure-v2` identifies the extended
rendering. Independently selected dimension/column roots can use their mandatory
column closure without inventing a pinned metric. Mandatory grouping/filter and
clarification dependencies remain included even when they are not output columns.
Projection does not create an analytical metric receipt for a dimension-only query.

Multi-topic inputs resolve each metric namespace and semantic-dependency content
ID against the admitted topic set. The existing `catalog-selection-v1` producer's
`selection-` and `semantic-` JSON/SHA256 IDs are used for ownership, not as authority
or cryptographic authentication. Definitions, selection digests and persisted wire
schemas are unchanged. Unknown, mixed or ambiguous ownership fails instead of
borrowing another topic's column mapping. Each selected topic's column dependencies
are validated against that topic's own reviewed relations, even when a dataset is
shared. Conflicting physical mappings for one semantic column are rejected. Work
is bounded by the existing relation/topic limits, 128 roots in a selection and at
most 4,096 combined dependencies.

Shared datasets retain their full per-topic reviewed columns. The existing
multi-topic admission contract independently confirms the same relationship in
each topic, so its endpoints are shared datasets; the assembler does not receive
those join choices. Keeping shared relations prevents removal of confirmed join
keys outside the selected dimension/metric closure. Columns are never merged
across topic boundaries, and sharing a dataset does not itself approve a join.
This conservative envelope is not minimal join-aware projection or fan-out proof.
Nonshared unneeded relations and columns can be omitted from the rendered schema.

A topic without a selected column closure, an opaque legacy metric, or a dataset-only
selection keeps its full original physical rendering. The same is true when all
roots originate only from clarification, required rules or interpreted values/time:
a known filter does not establish the requested detail output schema. Only existing
catalog/explicit root reasons make a topic eligible; unknown reasons stay conservative.
An entirely legacy unscoped selection keeps the previous behavior; it is not guessed
into a new namespace. Once scoped mappings are present, missing/foreign columns do not
trigger a permissive fallback. Returned projected and conservatively retained column
slices are detached. Optional evidence cannot alter the mandatory projection.

Generation and provider-envelope refitting reuse the context owner's renderer and
retain the full `Relations` value. No request fields, SQL records, native scopes,
semantic selection versions, analytical receipts or migration versions change.
Terminal replay/frozen operations add no model work. New recorded-provider tests
cover EN/ES dimension planning, private predicate grounding, filter-only detail
context, durable scope, saved-result privacy and zero-work terminal replay.
Producer/assembler tests cover shared join keys, topic ownership, conservative
fallback and unrelated catalog growth. See the [AP-02C adversarial review](../reviews/sql-scoped-projection.md).
These are software regressions, not live model or cross-dialect qualification.

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

## Effective provider envelope (AP-02B)

The real Bifrost adapter now exposes immutable, local generation-envelope preparation.
NLQ refits a sealed packet against that envelope before dispatch, including the
native-dialect/context suffix. Only the context owner removes optional groups;
the complete reviewed relation scope, required semantics and essential instructions
stay unchanged. Optional evidence/advisory groups yield first, then the lowest-ranked
demonstrations. The original default instructions remain available as an in-process
fallback when every example is removed. Required overflow returns typed insufficiency
without a generation call. The fitted initial packet and actual example-use evidence
are persisted; validation repair resolves its own sqlfix model/envelope. Retained JSON
cannot reconstruct a seal or acquire a new interpretation through this method.

`gateway.model_windows` is an optional bounded list of exact provider-route/model
pairs, each with `context_tokens` and `protocol_reserve_tokens`. When supplied, it
must cover every enabled chat role. A reviewed runtime model override must have its
own exact entry; it cannot inherit a different model's window. Unknown overrides
fail before dispatch. Entries are operator-owned configuration, not query inputs or
a model-discovery service. No real model capacities are inferred from names.

The versioned `utf8-json-byte-bound-v1` admission policy counts the normalized JSON
chat payload's UTF-8 bytes, including escaped system/user messages, model ID, strict
response schema and output/store parameters, plus the configured protocol reserve.
It then reserves the complete role output ceiling. This is deliberately conservative
for byte-based remote tokenizers and is NOT a claim of exact provider tokenization.
Operators must qualify their tokenizer/adapter framing and reserve; the accepted
remote routes use the existing OpenAI/OpenRouter adapter. This does not replace the
pinned cl100k semantic-tier budget or create a second semantic pruner.

If no model-window policy is configured, normalized request-byte and operation limits
still apply and optional content is refitted, but model capacity remains explicitly
unknown. No guessed context size is inserted for legacy installations. The default
protocol reservation for this legacy mode remains 1,024 units. With explicit windows,
output + input-byte bound + protocol reserve must not exceed the selected window.
The adapter reconstructs and checks the same effective envelope immediately before
reservation/dispatch; a preparation object never grants permission to execute.
Runtime model override slices are copied when installed to prevent configuration
changes between fitting and dispatch.

Each actual attempt may carry `Usage.envelope` (`prompt-envelope-v1`): policy,
effective-input/policy digest, normalized bytes, input upper bound, protocol/output
reserves and configured context limit. It contains no raw system/prompt/schema or
private scalar values. Estimated admission counts remain distinct from nullable
provider-reported `input_tokens`/`output_tokens`. Zero context limit means unconfigured,
not unlimited verified capacity. Old receipts without envelope evidence stay unknown.
Retries reserve the same complete envelope per attempt. Errors and preparation logs
remain redacted; failure before dispatch creates no fabricated model-attempt receipt.

Minimal join-aware projection and actual model-window/tokenizer qualification remain
separate work. Recorded wire and synthetic fits do not establish live language
quality, vendor capacity calibration, cloud-dialect parity or performance. Frozen
report refresh adds no model work.
