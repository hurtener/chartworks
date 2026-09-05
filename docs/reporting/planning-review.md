# Re-evaluation and completed planning corrections

Reviewed 2026-09-04 after merged PR #2 and the owner's subsequent corrections. This records changes to the implementation baseline, not completed Go features.

## Ownership corrections

Pengui exclusively owns identity/access policy and token issuance. Removed the local issuer/API-key/bootstrap/roles/grants/service-account/embed-credential design from active plans, RFCs and contributor rules. Chartworks validates supplied JWTs and applies signed scopes while retaining SQL safety, business evidence/lifecycle and tenant/data-context constraints. Expiry-bounded validation is not instantaneous offline revocation.

Harbor/Pengui MCP Apps is an established dependency. Removed the host qualification/framework-selection/protocol migration detour. New Chartworks resource/tool/viewer behavior still needs ordinary functional tests.

All production learned-model calls now use the embedded Bifrost Go SDK and remote providers. The single gateway contract covers completions, structured output, embeddings, rerank, narratives and model-assisted optimization. No local model/weight download/server/cross-encoder or alternate direct-compatible production client. Local deterministic tokenization, SQL parsing, rules, pgvector and rendering remain allowed.

## Product and correctness corrections

Frozen refresh cannot regenerate SQL, reroute a question or select charts. Existing artifact reads/renders make zero model/source calls and remain available during unrelated provider failures. Certification, publication, health and data authority are independent. Private previews, lossless numeric data, exact occurrence periods/revisions, bounded partial results and actual source context partitions are required across surfaces.

Preserve blocks/revisions/parameters/output subsets/narratives, reports/hybrid widgets/dashboards, retained results and functional schedules. Retain source semantic/NLQ/templates/refinement/rules/replay/feedback/learning and guided setup behavior through explicit phase ownership. Event/condition/condition-check/custom-code stubs are removed rather than counted as delivered features. Catalog recipient metadata is not email evidence.

Separate local transactional publication from external side effects; use fenced attempts/reconciliation/compensation without universal exactly-once or distributed rollback claims. Managed writes require actual ownership/credential proof, not a schema-name prefix.

## New-pass findings

The prior rewritten plans were only in an unattached tree; recovered them onto the branch. Mirrored contributor rules, root kickoff/request/README/glossary, active counts, smoke wrappers and missing checker/acceptance tooling are now part of the change. The old empty fast-preflight ownership map is replaced with conservative cumulative checking. Shipping CGo-free and race-test CGo settings are distinguished.

Phase31 no longer waits for scheduling30. Phase06 now delivers the actual Pengui fresh-authority adapter and first durable consumer, avoiding an undeclared late dependency for early profiling/semantic/report jobs;30 only extends reporting targets. The exact new execution-binding API is not claimed deployed: read/reuse the actual platform contract or make a Pengui-owned extension in06.

Do not blindly copy sibling inference adapters. Validate embedding INPUT/response counts, unique/full indices, dimensions and finite values; full embedding identity includes provider/model revision and preprocessing, not dimensions alone. Rerank requires complete unique valid indices/finite scores; missing results cannot become zero scores. Optional failure preserves original authorized order visibly or fails. Cache and batching preserve caller/context/input association. SDK and domain retries share a budget.

## Verification boundary

The 34 active phases assign224 acceptance criteria, all63 source-feature rows and41 review gates. Those mappings are checked mechanically but not proof of business correctness. The strict runner requires actual named Go child-test results, rejects missing/skipped/empty tests and forbids development skips at release. Tooling has21 regression tests; these are tests of the checker/runner, not the future Chartworks service.

No live warehouse/model call, source-suite execution, runtime benchmark, browser report or actual migration is claimed in this documentation pass. Applicable runtime evidence belongs to the phase; missing engine/feature support prevents that cohort's cutover. Archived material remains historical evidence, not an alternative authority chain.
