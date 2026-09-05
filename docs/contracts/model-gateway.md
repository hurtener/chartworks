# Model gateway: Bifrost SDK, remote inference only

Status: implementation contract, 2026-09-04; owner directive recorded in D-053. Applies to every phase and supersedes any reading of an older plan that permits a second production model driver. Implemented in phase 05 with recorded-wire SDK tests; no paid-provider live support claim is made.

## Boundary

`internal/gateway/bifrost` embeds `github.com/maximhq/bifrost/core` as a Go SDK client. All production completion, structured generation, embedding and reranking calls go through it to configured remote providers. A separate Bifrost HTTP gateway deployment is not required. Bifrost client initialization inside the Go process is not local model inference.

No learned model is loaded, downloaded, fine-tuned or served by Chartworks. Do not ship ONNX inference, llama.cpp/GGUF models, Ollama processes, Python transformer services, local cross-encoders or model weights. Do not add a direct OpenAI-compatible model client alongside Bifrost, even inside the gateway package. Custom supported endpoints remain Bifrost provider configurations, not bypass HTTP implementations. A missing provider capability fails explicitly.

Local deterministic computation remains appropriate: tokenization, SQL parsing, rule-based routing/visual selection, exact aggregation, PostgreSQL/pgvector storage/search and chart rendering. None of those is permission to add a local learned model. The optional SVG renderer is not an inference service.

Fixtures/mocks are explicit test dependencies only. Production cannot select them as a fallback or silently start without required model configuration. Network unavailability is separate from invalid configuration: a failed provider must not disable unrelated frozen execution or retained-result reads.

## Reference implementation and configuration reuse

Use the sibling Go gateways as reviewed references, not imported `internal/` packages and not a wholesale copy of either application. Source observations on 2026-09-04:

| Reference | Observed evidence | Carry into Chartworks |
|---|---|---|
| Soundings `go.mod` | Bifrost core `v1.6.2`, blob `2c4d7317199b49d38097db2283c644326b933676` | Initial SDK pin for phase 05; compile and fixture-test the actual selected API. |
| Soundings `internal/gateway/bifrost/bifrost.go` | Blob `c7f1161826967e3efc127c6662a8bcc63cd41d16` | `core.Init`, SDK embedding/rerank/chat calls, cancellable per-request contexts, configured role targets. |
| Soundings `examples/soundings.render.yaml` | Blob `a46ab568a1020edf2577a92db5823b2aadee13e7` | Native OpenRouter provider, `perplexity/pplx-embed-v1-0.6b` at 1024 dimensions, `cohere/rerank-4-fast`, environment-indirected provider key. |
| Stowage `go.mod` | Core `v1.5.15`, blob `cc951b1ce8c26469abe415a235b4de191741ed58` | Evidence that sibling pins differ; do not mix SDK types or assume identical config schemas. |
| Stowage `internal/gateway/bifrost/driver.go` | Blob `4e76d96338212c5c3223e12a5980e51ca3c683ee` | Independently selectable embedding/rerank providers, batching/cache lifecycle, closed-client behavior and thin client seam for tests. |

The reference configuration is in `examples/chartworks.gateway.json` (a JSON configuration excerpt, also valid YAML syntax). Reuse non-secret model/provider settings; never copy `.env`, keys, access tokens, customer endpoints, authentication modes, local stores or sibling-only role names. The reference files were inspected through the connected repositories; no access to an unmounted local checkout is asserted. Their presence is not a new live quality/cost benchmark.

Do not port a historical custom rerank workaround unless the selected SDK/provider actually needs it. Prefer the native provider path shown by the Soundings reference. Where a supported custom endpoint is necessary, configure it within Bifrost and prove path/auth/response mapping with a recorded-wire test. Stowage's alternate direct-compatible driver is not part of Chartworks' production design.

## Role configuration and lifecycle

The closed role set is `embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`, `profile_summary`, `rerank`, `narrative`, `visual_rank`. Every role resolves its provider, model, endpoint/credential reference, timeout and input/output limits independently. Roles may share one resolved provider client when that full configuration matches; never force embedding or reranking to follow the completion model's provider.

`gateway.driver` is `bifrost` in production. `gateway.bifrost.providers` contains configured provider entries with secret references; `gateway.roles` maps roles to those entries and their model/limits. Optional roles declare `enabled` and an explicit failure policy. Exact input-type/preprocessing options are part of the embedding identity. One canonical config decoder maps the reference excerpt; do not keep competing flat and nested aliases.

No secrets are placed in public configuration inspection, prompts or audit payloads. SDK logs/errors can contain provider-echoed input: return sanitized typed errors, retaining only separately authorized diagnostics. Reuse the existing provider secret seam, not Pengui user-auth tokens. Bifrost provider keys are upstream inference credentials, not Chartworks authorization.

Build the client once per immutable resolved configuration and shut it down once. Use a fresh cancellable Bifrost request context for every call; never detach a caller deadline or share mutable request state. Bound concurrency, queueing and input bytes. Shutdown must drain/cancel work without leaks. SDK/provider calls are forbidden outside the single adapter by an import/dependency test, not merely naming convention.

## Embeddings

Query and document embeddings must share the recorded embedding-space identity: provider route/model revision, output dimensions, input type, normalization/preprocessing version and applicable configured options. A model change creates a new facet generation; matching dimensions alone are not compatibility. Reindex before activating the new generation. Do not automatically fall back to a different embedding model, even at the same dimension.

Bound batch count/bytes/tokens and preserve input-to-response association. Validate response count against INPUT count, every index exactly once within range, vector dimension, finite values before/after conversion, and the declared normalization contract. Reject duplicate/missing/extra indices, truncated vectors and NaN/Inf rather than manufacturing zero vectors or dropping inputs.

Cache keys include tenant/data-context partition, full embedding identity and normalized input hash. Cache entries expire under bounded memory/retention limits. Do not use a bare model name as the only namespace. Batching must not mix caller authority or cancellation; returned vectors retain their original source IDs. pgvector receives already computed vectors and contains no inference implementation.

## Reranking

Only already authorized retrieved candidates are supplied to the SDK. Reranking may change relevance order, not candidate identity, authority, topic membership, semantic definitions or the question. Ask for all candidates when the contract is a full permutation; perform deliberate top-k reduction in the context owner afterward.

Validate finite scores, index range, uniqueness and complete coverage. Never assign zero to a candidate just because the provider omitted it. Preserve original IDs and stable ordering for ties. A malformed response is a provider-contract failure, not an acceptable partial ranking.

When enabled, the configured policy is either `fail` or `preserve_candidates`. The latter returns the original authorized order with a typed warning and metered failure; it is not local inference and is not silent. Disabled rerank makes zero rerank calls. There is no automatic substitution with an unrelated local model.

## Budgets and reproducibility

Reserve/enforce per-operation and tenant limits before and during calls, including SDK retries and role fallbacks. One attempt budget owns retries; do not multiply gateway and pipeline retries unknowingly. Usage comes from the actual attempted provider route. Unknown token/cost fields stay unknown; do not add a reported total to component costs a second time.

Record role, provider/model revision, prompt/schema version, duration, input/output counts and the selected fallback path without private content. Generated narrative text is retained with its evidence and exact generation provenance. Frozen blocks without narratives call no model service; rendering/reading retained results calls none, regardless of gateway health.

## Implementation proofs

Phase 05 AC01-AC10 cover the SDK boundary, role configuration, schemas, budgets, remote-only policy, embedding/rerank response validation and fixture mapping. Phase 07 covers generation/cache identity; phase 17 covers authorization-before-rerank and visible fallback; phase 28 covers zero-call frozen/artifact paths; phase 25 audits image/package contents and startup without model downloads. G41 maps the cross-cutting requirement. These are runtime obligations, not satisfied by this prose or a config file.

Official SDK documentation checked for implementation syntax: https://docs.getbifrost.ai/quickstart/go-sdk/reranking and https://docs.getbifrost.ai/providers/supported-providers/overview . Adopted version behavior is established by phase-05 tests, not by assuming documentation for a newer release matches a pin.

## Pinned SDK findings adopted in implementation (D-062)

Bifrost core **v1.6.2** has a native OpenRouter rerank method that returns unsupported. The historical sibling configuration row above is an inspected reference, not evidence that its route is callable in this pin. Chartworks therefore routes reranking through **native Cohere**, model `rerank-4-fast`, with an independent environment-indirected key. OpenAI/OpenRouter serve completion and embeddings; unsupported provider-role combinations fail configuration. No custom HTTP workaround is installed.

Successful SDK responses are validated using its internal raw-response observation before accepting typed defaults: a missing/null index or relevance score must not become a fabricated zero. This observation is discarded inside the adapter, never returned, logged or cached. Returned usage counts/costs distinguish absent fields from explicit zero; total cost is never added to its components. Reservations remain charged for unknown failures, and observed overages block further attempts.

The authority/cache key includes tenant, user, session, action, sorted signed scopes, resolved resources, caller context and the full embedding space. Changing model revision, provider/model/endpoint, dimensions or preprocessing changes that space; later published-index consumers must perform fenced reindexing (phase07), not silently replace a same-dimensional model.

SDK retries are disabled; the adapter owns the only 1–4 attempt ceiling. Structured output validation failures are not retried as free text. The first consumer is the fixed-input `/v1/gateway/probes` operator endpoint; domain semantic/NLQ/artifact consumers arrive in their owning phases.
