# Phase 05 — gateway

Status: shipped. Owner: internal/gateway. Hard dependencies: 01.

## Authority and design

RFC-001 §13, D-049/D-053 and [COMMON.md](COMMON.md) apply. [The model gateway contract](../contracts/model-gateway.md) is binding: the embedded Bifrost Go SDK is the only production inference path. All learned-model operations use configured remote providers; there are no local models or direct-compatible fallback clients.

## Brief findings incorporated

Briefs 03 and 14: lean structured context, independent role configuration, metering and bounded fallback. Soundings' Bifrost v1.6.2 adapter and non-secret remote embedding/rerank configuration supply the initial reference. Stowage contributes per-concern routing, batching/cache and cancellation/lifecycle patterns. The exact inspected blobs and differences are recorded in the model gateway contract.

## Findings I'm departing from

Do not copy sibling authentication, stores, credentials or all gateway alternatives. Stowage's older SDK pin/custom rerank path is not assumed identical to Soundings' native provider path. Do not copy incomplete response validation, echo provider error input, double-count cost components, silently switch embedding models or make artifact reads depend on model availability.

## Scope and implementation tasks

1. Build `internal/gateway/bifrost` around `github.com/maximhq/bifrost/core v1.6.2`, with a narrow typed SDK seam for tests. Prove actual API usage against recorded HTTP fixtures through the real SDK. No separately operated Bifrost gateway is required.
2. Implement `embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`, `profile_summary`, `rerank`, `narrative` and `visual_rank`. Resolve provider/model/endpoint/credential and request limits independently per role. Only explicitly enabled optional roles are callable.
3. Consume the nested provider/role configuration excerpt. Reuse clients only when their complete immutable configuration matches. Pass fresh cancellable SDK request contexts, join workers and close each client exactly once.
4. Reserve/account call/token/time budgets before and during attempts. Coordinate SDK and domain retry ceilings instead of multiplying them. Validate structured output and redact provider errors.
5. Validate embedding response count/index/dimensions/finite values and complete rerank permutations. Preserve source association, tenant/context partition and generation identity through batching and caches.
6. Document configuration defaults, provider capabilities, reference pin, safe customization and failure behavior. Test fixtures cannot be selected by production config or automatic fallback.

## Non-goals

No local learned models/weight downloads/ONNX/Ollama/transformer services, alternate direct model HTTP driver, separate inference platform, new billing/entitlement service or narrative query tools. Local deterministic tokenization and pgvector search remain allowed. No automatic embedding model fallback.

## Config and persistence

Production `gateway.driver=bifrost`; `gateway.bifrost.providers` resolves remote credentials/endpoints; `gateway.roles.<role>` holds provider/model/timeout/output limits and role-specific embedding/rerank settings. The initial excerpt is [chartworks.gateway.json](../../examples/chartworks.gateway.json). Embeddings use `perplexity/pplx-embed-v1-0.6b`, 1024 dimensions in that reference; rerank uses `cohere/rerank-4-fast`. These are replaceable remote settings, not hard-coded semantics or live benchmark claims.

Reference bounds: embedding batch 64 items/256 KiB, rerank 64 candidates/10s, `max_attempts_per_call=2`. Further input token limits follow actual model capability. Narrative and visual ranking are disabled until explicitly enabled. Rerank failure is `fail` or visible `preserve_candidates`. Usage/prompt/model/version records are data, never authority. Credentials use the existing secret seam; no key values enter configuration inspection.

## Acceptance criteria

1. **AC01** — Every production model call reaches the real Bifrost SDK adapter; independent role/provider/endpoint/credential routing is tested and no direct provider SDK/HTTP path exists outside it.
2. **AC02** — Schema-constrained output is independently validated or fails; no guessed free-text JSON, unvalidated partial output or provider-echoed sensitive error is returned.
3. **AC03** — Call/token/time admission bounds concurrent/retried work, including SDK attempts; usage follows the actual route and unknown/aggregate cost is not fabricated or double-counted.
4. **AC04** — Embedding-space identity/dimensions cannot mix published generations; frozen no-narrative execution and retained-result reads work with remote inference unavailable.
5. **AC05** — Optional reranking/visual ranking only orders authorized candidates; disabled roles make zero calls and enabled failure policies are explicit. Narratives/visual ranking acquire no query/write tools.
6. **AC06** — Every enabled role has a recorded-wire SDK fixture and cancellation/timeout/error/lifecycle/race tests; client shutdown is idempotent and does not leak workers.
7. **AC07** — Production config, dependency graph and reference image contain no local inference engine, model weights or model-download/bootstrap step; test fixtures cannot become a production fallback.
8. **AC08** — Embedding fixtures reject missing/duplicate/extra/out-of-range indices, input/response count mismatch, wrong dimensions and nonfinite/overflow values; valid reordered results map to the original inputs.
9. **AC09** — Rerank fixtures require complete unique indices and finite scores, preserve IDs/tie order, and reject omissions instead of inventing zero scores; visible fallback returns only the original authorized set.
10. **AC10** — The reference excerpt round-trips through the actual typed config and Bifrost provider setup; embedding/rerank routes can differ from completion, caches include full space/context identity, and a model change requires a fenced reindex rather than same-dimension substitution.

## Tests, coverage and smoke

Implement `TestPhase05/AC01` through `TestPhase05/AC10`. Exercise the real SDK with synthetic or secret-scrubbed recorded provider responses; retain one separately authorized live smoke per enabled provider operation before deployment support claims. Do not call paid services in ordinary CI. Include negative response-shape cases, cross-context cache cases, concurrent budget admission, cancellation during batching and provider error redaction. COMMON.md sets coverage; `scripts/smoke/phase-05.sh` requires all ten runtime results. G41 traces the remote-only requirement across consumers.

## Glossary, decisions and deviations

D-053 narrows the existing gateway seam to one production SDK driver. A local SDK client is not local inference; pgvector is storage/search, not an embedding model. Configuration reuse does not mean copying secrets or claiming sibling test execution. Implementation and recorded-wire/real-store acceptance are provided in this change; no production deployment or paid-provider live acceptance is claimed.

## Implemented evidence and limits

See [phase 05–06 review](../reviews/phase-05-06-adversarial.md), [operator setup](../../GETTING-STARTED.md), and [execution authority v1](../contracts/execution-authority-v1.md). Named criteria execute against the pinned SDK and PostgreSQL 17, not mocked production drivers. D-062/D-063 record provider corrections, the exact Pengui-owned companion and scope boundaries. Broader analytics, pipeline, report and delivery targets remain owned by their subsequent phases.
