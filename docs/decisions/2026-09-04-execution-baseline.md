# Decisions appended after the reporting review

Date: 2026-09-04. Source: owner feedback after merging PR #2 plus the resulting implementation-plan reconciliation. These entries extend the original `docs/decisions.md` without altering its historical text. D-043 is reserved for the separately recorded provider-role follow-up; this amendment starts at D-044 to avoid collision.

### D-044 — Pengui is the sole issuer and identity/access-policy owner

Accepted owner directive. Supersedes the self-issue portions of D-006/D-030 and all planned local key/bootstrap/token-exchange flows. Chartworks validates configured Pengui JWTs and never signs identity, API, embed or renewed execution tokens. No local passwords, API-key registry, users or service-account provisioning. Warehouse connector credential custody is not user authentication and remains a separate source concern.

### D-045 — Enforce signed scopes; do not compute local grants

Accepted owner directive. Supersedes the local grants/roles/resolver mechanism of D-020, while preserving deny-by-default enforcement. All action and resource authority comes from Pengui's signed token. Chartworks checks exact scopes/restrictions and data-context integrity, without membership, role, sharing or ownership-derived permission expansion. No local IAM tables/routes, policy epochs or online revocation database. Expiring JWT validation provides bounded authorization freshness, not immediate offline revocation. Provider scope bindings are documented in the consumer contract.

### D-046 — MCP Apps support is established

Accepted owner fact: Harbor and Pengui support MCP Apps end to end. Removes the host-compatibility qualification gate from the first reporting proposal and the old framework re-evaluation checkpoint. Implement Chartworks tools/resources/viewer and ordinary tests for new code; do not reopen host support or require a protocol migration. Earlier unverified release-date/stateless-core claims are not implementation requirements.

### D-047 — Governed reporting is required product scope

Ratifies the merged reporting analysis as implementation design, subject to D-044–D-046. Supersedes chart-only/no-rendering constraints from D-013/D-026 and corresponding non-goals. Preserve blocks, immutable revisions, validation, certification, typed parameters, multiple outputs, narratives, reports, hybrid widgets, dashboards, retained artifacts, discovery, private previews and migration behavior. Frozen runs do not invoke NLQ or chart selection. Thin visual delivery is not a builder UI.

### D-048 — Functional scheduling, fresh delegated authority, no stubs

Carry cron/interval/manual test runs, reviewed and dynamic saved-query targets, direct block/output targets, report targets, service attribution, exact periods/revisions, retries/pause/resume/retire and catalog delivery. Persist occurrence identity and resolve targets once; retries reuse it. Obtain fresh JWTs from Pengui for unattended work using a durable opaque execution binding. Chartworks does not create service accounts or store the user's token for later replay. Remove event/condition triggers and condition-check/custom-code stubs from public capability enums; maintenance remains bounded internal work. Catalog recipients are not proof of outbound delivery.

### D-049 — Preserve useful pipeline intelligence without runtime drift

Retain per-role provider configurability and optional reorder-only reranking; add bounded narrative and optional exploratory visualization ranking through the same gateway. Preserve source rule replay/shadow comparisons, follow-ups, same-source confirmed multi-topic queries, learned examples and prompt evaluation/optimization as actionable parity work. Do not defer implemented source behavior merely because the earlier plan called it later. Pin dependencies during implementation without claiming historical pins or POC performance are current measurements.

### D-050 — Replace conflicting active plans; preserve historical notes

The active RFCs, contributor rules, consumer request and all 26 original phase plans are reconciled; eight reporting/onboarding/migration phases are added. The earlier detailed plans and reporting proposal are retained byte-for-byte under `docs/archive/`, not left as competing active instructions. Phase numbers identify workstreams, not chronological order. Early transport shells are consumed by each feature phase. The ledger maps all 63 source/continuity rows and 40 review gates to acceptance IDs or explicit removal.

### D-051 — Honest execution and staged external effects

Supersedes cross-system all-or-nothing/revert wording in D-039/D-041. Local pointer/evidence publication is transactional; external queries/DDL/delivery can have indeterminate outcomes. Use leases/fencing, durable step results, reconciliation and dependency-aware compensation. No universal exactly-once warehouse/delivery promise. Correct source metadata and read-only credentials are required before validation planning can be considered safe; EXPLAIN alone is not a security proof.

### D-052 — Resumable onboarding and bounded expansion

Guided connection/configuration -> profiling -> semantic proposal -> review/publication -> example/draft-block flow is implementation scope. It has explicit checkpoints, budgets, evidence and unresolved semantics; no environment/credential manufacture by an LLM. Direct-source operation remains first-class. L2 reviewed engineering and drift amendments are retained; L3 and a new internal analyst orchestrator are post-cutover extensions rather than parity blockers. The signed context-bundle token from D-034 is replaced by an opaque stored reference plus normal JWT authorization, eliminating an unnecessary local signing subsystem.
