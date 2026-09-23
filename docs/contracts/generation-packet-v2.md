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

Broader dimension-only and multi-topic physical projections remain separate work.
Recorded wire and synthetic fits do not establish live language quality, vendor
capacity calibration, cloud-dialect parity or performance. Frozen report refresh
adds no model work.
