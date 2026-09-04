# Chartworks — Contributor and Agent Normatives

Binding for human and automated contributors. This file and CLAUDE.md are byte-identical. The current task is a doc-driven Go implementation; a written plan or green planning check is not a shipped runtime capability.

## 1. Product

Chartworks is Pengui's structured analytics and governed publishing service: sources/uploads, profiles, reviewed semantic topics, NLQ/BYO query execution, reusable approved blocks, reports/dashboards, scheduled runs, retained results and portable rendering. API-first means no required standalone authoring application; a read viewer and true static rendering are in scope.

Pengui alone owns authentication, the issuer, identity, service accounts and access-policy decisions. Chartworks verifies Pengui JWTs and enforces their signed action/resource scopes. Do not recreate users, memberships, roles, grants, API keys, login/OAuth, token issuance, bootstrap admin or embed credentials. Warehouse connector secrets are a separate source concern, not permission to duplicate Pengui integration credentials.

Harbor/Pengui MCP Apps support is established end to end by the owner. Build/test Chartworks tools/resources/viewer; do not add host qualification, framework re-evaluation or unrelated protocol-upgrade gates.

## 2. Authority and orientation

Read, in order: RFC-001 (shared architecture/security), RFC-002 (reporting), the owning active phase plan and COMMON.md, the master plan/registry, this file, the consumer request, informing research. Read the Pengui authority contract for any protected operation. Decisions are append-only across docs/decisions.md and docs/decisions/*.md; an accepted superseding entry requires corresponding current RFC/plan updates.

Current entry point: docs/plans/README.md. There are 34 planned workstreams; IDs are not execution order. Phases 21–23 provide early thin shells; domain phases add concrete operations. Phase 25 is the final release gate. Phase-registry status and the coverage map are bookkeeping, never runtime evidence.

Anything under docs/archive is historical reference, not active instruction. Prior source research remains evidence at its stated date/depth; the latest owner decisions supersede recommendations that assigned authentication or host qualification to Chartworks.

## 3. Confidential source hygiene

Mine source behavior only through neutral research/contract descriptions. No client/product identifiers, source repository URLs, source code/prompts, confidential schemas/data or credentials are copied into new implementation/docs/tests. Authorized private source checkouts and mappings remain outside the repository. Existing historical references stay historical. Newly committed examples and goldens are synthetic.

Use ordinary domain names: source, dataset, topic, block, report, dashboard, run, artifact, rendition, schedule, parameter. Protocol-standard field names and technical evidence metadata are legitimate; do not distort MCP/OpenAPI or artifact contracts to satisfy an overbroad vocabulary grep. A lexical naming check is hygiene, not a security control.

## 4. Structural invariants

P1: deny when signed authority or data-safety proof is insufficient; tenant/resource restrictions apply before data access. P2: the verified envelope is the identity source, never request headers/body fields. P3: tenant boundaries exist in storage/source interfaces, not optional post-filtering. P4: typed observable failure, no silent authority widening. P5: one model gateway. P6: clear domain vocabulary. P7: one core per capability with thin surfaces.

Read execution accepts only a nonzero validator-issued plan bound to source/context/semantics/parameters. SQL parsing or native planning alone is not a safety proof. Use read-only source credentials/session controls plus positive dependency/function/statement enforcement. Managed engineering writes use a separate interface/process and registered owned destinations, never a read/write flag.

Published definitions are immutable. Publication, certification, current health and current signed authority are different facts. An approved block refresh does not interpret questions, generate/correct SQL or select charts. Optional narratives operate on bounded retained evidence only. Existing artifact reads/renders do not execute warehouse/model work. Private previews remain private after later publication.

## 5. Identity and durable authority

Implement only the decoder/enforcer in docs/contracts/pengui-authority.md. Require configured issuer/audiences, asymmetric algorithm/key binding, expiration/temporal and claim-size checks. JWKS addresses are trusted configuration, not token-selected URLs. No admin/creator/agent name expands permissions. Do not invent alternative issuer profiles.

A JWT is a bounded authority snapshot, not instantaneous offline revocation. Pengui owns renewal/revocation decisions. New requests validate current supplied bearers; long-lived jobs/schedules obtain fresh Pengui authority using an authorized opaque execution binding through the existing platform broker adapter. Never persist a user's token for later replay or sign a replacement locally. Do not invent a broker endpoint when its actual contract must be read and wired.

The actual source execution context defines its data partition. A client label cannot narrow a broad result. Reuse and reading require signed target/context reach and persisted privacy. No tenant-only cache or browser-side security filter.

## 6. Architecture and Go conventions

Use the existing Go toolchain/module baseline, gofmt/goimports, context-first cancellable I/O, wrapped typed errors and log/slog. Shared compiled dependencies are immutable and race-safe; request state lives in the call context. Bound goroutines/channels, join them on shutdown and avoid mutable package globals except explicit registries/metrics.

Use seams with first real consumers; no speculative interface hierarchy. Keep exec/source dependencies acyclic by defining read interfaces/validated plans in exec and injecting concrete source drivers. Thin HTTP/MCP/SDK/CLI callers must not repeat domain logic or bypass the verified envelope.

The container is the reference deployment unit. CGo-free core builds remain preferred, but accepted per-dependency exceptions are explicit rather than contradicted by old unconditional claims. The pipeline/render subprocesses are bounded, pinned and supervised; no generic workflow engine or extra message broker is required.

## 7. Secrets and logging

Never log tokens, signing/warehouse/provider credentials, raw prompts, SQL or result rows by default. Domain SQL/evidence storage is separately protected and retained. Source list/read shapes structurally exclude secret bytes. Secret rotation invalidates relevant pools/contexts. No shell interpolation, path traversal or plaintext long-lived credentials in generated pipeline/render artifacts.

Audit contains actor/resource/operation IDs, reason/outcome and version/effect evidence. Metrics use bounded labels; high-cardinality identity belongs in protected traces/audit. Unknown costs/outcomes stay unknown, not fabricated zero/success.

## 8. Persistence and concurrency

PostgreSQL/pgx with forward-only migrations and tenant-composite identity/reference constraints. Add domain tables with their consumers. No API-key/grant/role/membership/service-identity/issuer-key tables. Domain operational settings do not grant identity authority.

CAS and transaction boundaries protect draft edits, publication pointers, operation keys and accepted manifests. One leased queue/occurrence engine supplies attempts/fences/retries. Local atomic commit does not guarantee external exactly-once execution or cross-warehouse rollback; persist staged effects, reconcile and compensate explicitly.

## 9. Intelligence and source continuity

Every model/embedding/rerank/narrative call uses the gateway, independently configured per role. Structured output is validated, budgets reserved/enforced before and during work, and retries counted. Frozen operations remain useful with irrelevant model services unavailable.

Keep compact semantic contracts, one tokenizer budget, pinned metrics/hard constraints, English/Spanish fixtures, confirmed same-source multi-topic relationships, templates/refinement, rules/replay/shadow, feedback/examples and bounded optimization. Do not defer actual source features under a new internal-analyst label. L2 reviewed engineering stays included; L3/new internal analyst are later extensions.

## 10. Scheduling and rendering

Only actual cron/interval/manual behavior and functional targets are advertised. Discard event/condition/condition-check/custom-code stubs; bounded maintenance remains internal. Preserve accepted due time/window/revisions through retries. Catalog delivery is not email; optional notification effects use existing Pengui integration receipts.

MCP uses the established Apps bridge. Iframe auth uses a Pengui/client BFF forwarding scoped tokens server-side. Chartworks creates no embed session/bootstrap code. Static chart SSR must render actual content without client chart JavaScript. Renderer input is sealed typed data/spec, not arbitrary URLs or scripts; no network/source/model credentials, bounded resource usage and sanitized output.

## 11. Testing

Follow docs/plans/COMMON.md. Each phase owns TestPhaseNN/ACxx acceptance subtests with real assertions. Missing/skipped tests are not passing implementation. Use real PostgreSQL and applicable source/renderer boundaries; recorded model fixtures are not live provider measurements. Fuzz parse surfaces and require cross-tenant/same-tenant-different-context/concurrent-reuse negatives.

Coverage defaults: 85% store/auth/identity/access/exec/vindex/conformance, 80% other internal code, 70% CLI/evaluation. Benchmark environment/raw values and distinguish source/model latency from service overhead. Do not claim a screenshot, fixture inventory or a lexical scan proves security, performance or parity.

## 12. Build and preflight

make planning-check validates documents, dependency graph, references and mapped criteria. make preflight-full runs planning plus implemented-phase runtime acceptance; explicitly allowed planned SKIPs are reported as unimplemented. make release-check requires all actual phase tests, no skips and reviewed shipped status. Never use a planned status to evade tests for new implementation.

Go race tests require the race-capable build environment, even if the shipping core is built with CGO_ENABLED=0. Build tags/dependency exceptions must be documented consistently in CI and the reference image. The documentation phase does not claim a Go build ran when there is no Go source.

## 13. API/SDK changes

Every new operation registers schema, scope/resource loader, errors and audit/side-effect classification. Generate parity coverage from actual registration. Domain phases update clients and examples with their concrete operations. No success-returning placeholder capabilities. Explicit cancellation of durable work differs from disconnecting an HTTP client.

## 14. Documentation coherence

An ownership/non-goal change updates current RFCs, affected phases, the master/registry, consumer request, glossary and both mirrored rule files in the same change. Preserve historical decisions; add superseding entries with unique IDs across the log and annexes. Do not leave contradictory active instructions and ask the next agent to reconcile them.

## 15. Review and delivery

Work on a branch and open a scoped PR. No force push/main merge/tag/deployment without explicit authorization. State exactly what was checked, which tests were run and which evidence is still missing. A documentation change may be complete while the runtime remains unimplemented. Do not describe planned checks as passing tests.

## 16. Authoring a phase plan

Read the master, RFCs, informing briefs, glossary and decision history. Fill docs/plans/_template.md with owning packages, hard dependencies, concrete tasks/config/persistence, non-goals and individually numbered observable acceptance criteria. Pair scripts/smoke/phase-NN.sh and registry/coverage updates. Run the planning checker and applicable preflight. Use COMMON.md for repeated mechanics instead of copying pages of generic guidance into each phase.

A new required capability needs an owner, actual consumer, migration disposition and tests; do not solve it only with another research paragraph. Any reasonable implementation deviation records preserved/equivalent behavior and evidence in the phase before closure.

## 17. Integration and migration

Runtime/service data is accessed through public seams, never another service's private DB. Pengui/Harbor retain references to authoritative Chartworks artifacts, not a sole divergent copy. Import/export uses neutral mappings, private source-controlled comparisons and explicit lifecycle/authority revalidation. Cutover deduplicates scheduled occurrences and retains an honest rollback path without claiming to undo irreversible external effects.

## 18. Mirror and final check

AGENTS.md and CLAUDE.md must be byte-identical. Confirm the active plan DAG, all feature/gate references, named test coverage, supported source/renderer matrix and secret/name hygiene before approval. Archive content never overrides these rules. Full migration/release requires phase 34 then phase 25; the first useful slice is not complete parity.
