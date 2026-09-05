# Shared phase implementation and completion contract

Binding with every active phase. RFC-001/RFC-002, [Pengui authority](../contracts/pengui-authority.md), and [Bifrost-only remote inference](../contracts/model-gateway.md) apply. Archived plans are historical reference, not competing instructions. D-053 narrows any generic gateway wording to one production SDK driver.

## Before implementation

Read the owning phase, master, relevant research and append-only decisions. Work on a branch and retain all source-feature IDs in coverage.json. Use synthetic fixtures; private source mappings, code, prompts, customer schemas and credentials do not enter the repository.

A seam lands with its first real consumer. Resolve actual platform/provider API syntax from the supported implementation; do not invent a broker endpoint or claim new scope serialization is already deployed. Required Pengui extensions remain Pengui-owned. This is ordinary integration work, not permission to reopen owner-confirmed MCP Apps support.

## Service design and authority

One core service implements each capability. All protected I/O receives an immutable verified envelope. Signed scopes are applied, not widened by creator/role/membership policy. Source execution contexts and managed-object ownership reflect actual credentials and database boundaries. No local IAM or general policy-language subsystem.

Use bounded cancellable work and deterministic clock seams. Readers take nonzero validator-issued plans. Keep package dependencies acyclic and migrations paired with consumers. Tenant-composite keys, CAS, publication and idempotency constraints are database-enforced as appropriate. Published definitions and applied migrations are immutable. External side effects use attempts, reconciliation and compensation; local commit is not distributed atomicity.

Tokens remain transient. Durable work stores an authorized opaque binding and obtains fresh Pengui authority at dispatch/retry or required checkpoints. Expiry-bounded validation is not instantaneous offline revocation. The concrete binding/broker adapter is an integration deliverable, not a local token issuer.

## Model access

Every production learned-model call uses Bifrost core SDK through internal/gateway/bifrost and remote providers. There is no alternate compatible HTTP client, local model, weight download, ONNX runtime or local cross-encoder. Config/fixtures from Soundings/Stowage guide the adapter; their local stores/auth and credentials are not copied. Deterministic tokenization, rules, SQL parsing, pgvector and renderers are allowed normal computation.

Check role-specific provider/capability/config, full embedding-space identity, complete response index coverage, finite values and SDK attempt budgets. Rerank may only order already authorized candidates. Disabled optional roles make zero calls; failure behavior is explicit. No automatic embedding-model substitution. Provider failures must not disable frozen no-narrative execution or retained-result reads. Bifrost is an embedded client library, not a required separately deployed proxy.

## Surface deliverables

Domain phases add actual HTTP schemas and SDK operations, signed-scope/resource checks, errors, audit/usage and side-effect classification with their first consumer. MCP operations are added where assigned; early registration/parity suites expand with every implemented feature. An unbuilt endpoint is absent, not a success-returning stub. Explicit cancellation of durable work is different from disconnecting a client.

Use closed write schemas and bounded unions, one route action convention and generated OpenAPI/SDK checks. The browser surface is a read viewer, not a mandatory builder. Its resources contain no bearer/provider/source credentials or authoritative duplicate report state. Iframe auth remains in the BFF.

## Acceptance naming and evidence

Each numbered criterion has a named Go acceptance subtest under `test/acceptance`: `TestPhaseNN/AC01` through its declared count. Helpers exercise real behavior; a parent that passes without children is not acceptance. More detailed cases may appear beneath an AC. The strict runner checks actual Go JSON pass events and rejects missing/duplicate/unexpected criteria, failure, skip, nonzero exit and timed-out execution, including nested skipped cases.

Pure unit/property tests cover normalization, time arithmetic, bindings and transitions. Fuzz parse/decode surfaces. Shared objects require concurrent-reuse tests under race detection. Identity-sensitive changes require cross-tenant and same-tenant/different-context negatives. Assert zero source calls on denial, zero model calls on frozen/no-narrative work, zero source/model calls on artifact views, no baseline writes, no mutable publication and no preview leakage.

Storage/warehouse guarantees use real drivers. Self-hostable engines use container fixtures; cloud support needs recorded fixtures plus applicable live evidence before a support/cutover claim. The actual Bifrost SDK can run against recorded provider responses in ordinary CI; those fixtures are not live quality measurements. Real viewer/renderer tests can be invoked by a Go AC; missing runtime dependencies or process failure fail acceptance rather than skipping to success.

## Configuration, coverage and measurements

Each changed setting names its typed key, units, default/bounds, secret status and positive/negative tests. Reject retired auth/signing/bootstrap settings. Keep the reference gateway excerpt compatible with the actual typed decoder. Update setup/source/renderer/runner matrices and operational limits with implementation.

Coverage defaults: 85% auth/identity/access/store/exec/vindex/conformance, 80% other internal packages, 70% CLI/evaluation tooling. Register touched packages in coverage-bands.conf. Exceptions need reviewed evidence; no no-op wrappers to manufacture coverage.

Record benchmark environment, data and warm/cold behavior; separate model/warehouse latency from service overhead. Unknown cost/outcome stays unknown. Test pre-call admission and in-flight/retry usage, not only post-hoc accounting. A lexical check, row count or screenshot is not a security/performance proof.

## Planning, smoke and release

`phase-registry.json` statuses are planned, in_progress or shipped. All start planned here; they are evidence labels, not feature flags. Implementation submitted for acceptance changes status and supplies actual named tests. Leaving code planned to evade testing is prohibited.

`make planning-check` runs the standard-library checker and its unit tests: graph, plan metadata, criteria, links, feature/gate mappings, mirrored rules and Bifrost configuration policy. It proves planning coherence only.

`python3 scripts/run_phase_acceptance.py --phase NN` executes real Go acceptance with race detection and uncached results. Missing code/tests/dependencies fails. Explicit `CHARTWORKS_ALLOW_PLANNED_SKIP=1` permits only a clearly labeled unimplemented planned phase to skip in development; `make preflight-full` uses that mode. It still fails implemented-phase errors. Phase wrappers use this runner, not independent success counters.

`make release-check` runs every phase in dependency order, requires all statuses shipped and disallows all skips regardless of that environment flag. A status or existing filename alone is insufficient; actual child test events are required. Applicable live provider/source evidence must also match the code/config/data/support claim through the phase25/34 assertions.

Go race testing requires a race-capable build environment with CGo enabled even when the shipping core is built CGo-free. Build and race-test settings are separated in Makefile. The documentation stage does not claim a Go build/test occurred when source is absent.

## Completion and deviations

Attach actual evidence to each implemented feature/gate. Cumulative consumer tests extend earlier services without reverse package dependencies. Keep the registry/counts, phase headers, master, scope contracts, examples and mirrored rules coherent. Preserve old decisions and append explicit superseding ones with unique IDs. A required source feature cannot be removed by silently editing its disposition; approved equivalent behavior needs an explicit migration rule and proof.
