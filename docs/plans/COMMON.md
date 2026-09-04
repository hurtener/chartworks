# Shared phase implementation and completion contract

This is part of every active phase plan, factored once to avoid repetitive boilerplate. It complements RFC-001/RFC-002 and does not weaken their invariants. Historical plans in docs/archive are reference only.

## Before implementation

Read the active RFC sections, owning phase, authority contract, relevant briefs and the original plus appended decision log. Preserve the source-feature IDs in coverage.json. Work on a branch; no direct main/automatic release. Use the established Go conventions and secret-safe synthetic fixtures. An interface is delivered with its first real consumer, not a promise that someone might implement it later.

When precise deployment/provider syntax is needed, consult the actual supported implementation and record the selected contract/dependency version. This is ordinary integration work, not permission to reopen owner-confirmed MCP Apps support. New provider resource scopes must be supplied by Pengui's existing signing seam; no alternative issuer or guessed broker endpoint is implemented here.

## Service and data design

Implement domain services once and pass verified immutable envelopes to all I/O. Separate operation authorization from business validation. Signed scopes are applied, never widened by local role/ownership/membership policy. Source read contexts and managed-object ownership are verified against real connector configuration. Do not introduce a general policy language.

Use bounded contexts/channels/workers, explicit cancellation and deterministic clock seams. Source readers consume only validator-issued plans; reject zero/mismatched plans. Go package dependencies remain acyclic. Do not retain user tokens in operations; the existing Pengui client obtains fresh transient authority for durable work.

Ship migrations with the consuming domain, including tenant-composite keys/foreign keys, CAS/publication/idempotency constraints and retention indexes. Test fresh database and upgrade behavior. Do not overwrite published definitions, historical evidence or applied migrations. External work uses durable attempts/staged effects; local transaction atomicity is not distributed atomicity.

## Surface deliverables

Every feature phase adds actual HTTP/API schemas and SDK operations, typed errors, audit/usage attribution and scope/resource registration in the same change. Add the relevant existing-core MCP operations or reporting tools where assigned. Early shell tests enumerate implemented registration; every later feature reruns them. Missing feature endpoints cannot be advertised as ready.

Use one route action naming convention and generate OpenAPI/SDK shape checks. Closed write schemas and bounded unions reject unknown/malformed authority-bearing fields. CLI has injectable I/O and never accepts an actor override as authority. Examples use synthetic names and secret references. Frontend code is the read viewer, not a mandatory builder UI.

## Tests and acceptance naming

Each numbered phase criterion owns a named Go acceptance subtest under `test/acceptance`: parent `TestPhaseNN`, children `AC01` ... declared count. Helpers exercise the real service/driver; a parent test that passes with zero children is not acceptance. More detailed child cases may exist underneath each AC. The strict runner requires all expected cases to have pass events and rejects failure/skip events, including nested cases.

Use pure unit/property tests for normalization, time arithmetic, state transitions and bindings. Add fuzz seeds for JWT/scope/input/SQL/output decoders. Shared objects need concurrent reuse tests under race detection; identity/resource changes need cross-tenant and same-tenant/different-context adversarial fixtures.

Store/warehouse semantics need real boundary drivers. PostgreSQL/pgvector and self-hostable engines run in containers; cloud support needs recorded fixtures plus separately captured live evidence before its support/cutover claim. Gateway test fixtures are permitted but are not live model-quality evidence. A browser/component or real renderer test can be invoked by a Go acceptance subtest; its process failure or missing dependency fails that acceptance, never skips into success.

Test forbidden behavior directly: zero source calls on denial; no inference in frozen refresh; zero SQL/model calls on artifact view; no baseline writes; no mutable publication; no data from broader contexts; no hidden preview publication. A lexical check can catch documentation drift or API accidents but is not an authorization/security proof.

## Coverage and measurement

Coverage defaults: 85% auth/identity/access/store/exec/vindex/conformance code; 80% other new internal packages; 70% CLI/evaluation tooling. Add exact touched-package entries to scripts/coverage-bands.conf during implementation. Exceptions require a reviewed deviation with rationale; do not achieve a percentage through no-op wrappers.

Record benchmark environment, data, warm/cold behavior, source/model time versus service overhead and raw measurements. No inherited POC speedup or promised startup/latency is a measurement. Budget assertions include retries and work accepted before a crash. Unknown cost remains unknown rather than a fabricated zero.

## Configuration and documentation

Each changed setting needs an exact typed key, units, default, valid bounds, secret classification, example and a positive/negative test. Reject retired auth-mode/signing/bootstrap settings rather than treating them as aliases. Update source/runner/renderer support matrices, getting-started instructions and operational limits in the same feature change. Artifact retention and preview policy are explicit and cannot be inferred from a current pointer.

## Smoke and status semantics

`phase-registry.json` status is planned, in_progress or shipped. It is an evidence label, not a feature toggle. All phases start planned in this planning change. Once implementation is submitted for phase acceptance, supply the named tests and change status accordingly; leaving implemented code marked planned to evade tests is not allowed.

`python3 scripts/planning_check.py` validates document metadata, DAG, reference/coverage completeness, mirrored contributor rules and current decision references. Its success is planning coherence only.

`python3 scripts/run_phase_acceptance.py --phase NN` runs the actual Go acceptance parent with race detection and uncached results, then checks every expected child event. Missing Go code/tests/dependencies or a skipped child fails. `--all` covers the registry. Planned phases may explicitly report SKIP only with `CHARTWORKS_ALLOW_PLANNED_SKIP=1`; this is the documented development/preflight mode, never a passing phase. `--release` disallows all skips and requires every phase to be shipped after evidence review.

The runtime harness should arrange deterministic local fixtures automatically where practical. Expensive live provider/warehouse evidence has its own recorded execution reference, configuration/commit/data fingerprints and owner-run command. Release acceptance checks that evidence is applicable to the current support claim; a filename or manually checked status is not proof.

## Phase completion and deviations

Each owning phase registers its actual tests, surfaces, config and migrations; run its acceptance plus affected prior suites. Attach evidence IDs to feature/gate rows or the corresponding release evidence file as implementation proceeds. Shipped status requires the named results plus the relevant consumer/real-driver obligations. Cumulative tests can extend a previously delivered core without creating reverse package dependencies.

Add new domain vocabulary to docs/glossary.md. Append new decisions without rewriting the original log or annex history; IDs remain unique across both. Record reasonable implementation deviations in the phase's final section, including the replaced criterion, equivalent behavior and proof. A required source feature cannot disappear by editing a row to deferred without the owner's explicit scope change.
