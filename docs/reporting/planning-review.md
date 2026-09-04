# Re-evaluation after owner feedback

Date: 2026-09-04. PR #2 is merged. This review corrects the earlier proposal and makes its scope actionable in the active phase documents.

## Corrections to my earlier proposal

The MCP Apps compatibility qualification was unnecessary: the owner confirms Harbor/Pengui support end to end. Remove the gate and the old framework re-evaluation checkpoint. Test new Chartworks resources/tools/viewer, not the established hosts. The earlier release-date/stateless-core statement was not needed for the design and is not carried as an implementation requirement.

Authentication correction is wider than deleting a mint endpoint. Local grant/role resolution, service-account provisioning, embed grants/bootstrap codes and signed bundle handles would all have introduced another authority mechanism. They are removed. Pengui signs identity/action/resource authority; Chartworks verifies/enforces it. It still validates SQL, business state, dependencies, budgets and actual execution-context restrictions. Those checks do not decide user membership or sharing.

A valid signed JWT cannot support a claim of instantaneous offline revocation. The design now states token-lifetime-bounded freshness, uses new tokens on new requests and fresh Pengui delegation for queued work. No local revocation database or policy-epoch subsystem is invented.

The rendering exclusion, deferred source reporting features and late all-at-once surface phase conflicted with the desired offering. Active RFCs/phase plans are now reconciled, transport shells land early, and every domain phase ships its own concrete operations. Historical detailed notes are preserved under `docs/archive/`, not left as competing specifications.

## Functional scope retained and made explicit

Blocks/revisions/validation/certification; multiple outputs/parameters/narratives; reports/hybrid widgets/dashboards; private previews and retained artifacts; typed scopes/SQL-read separation; cron/interval/manual runs and real saved-query/block/report targets; current delegated service authority; source adapters/uploads; topic/context/rules/refinement/replay/learning; onboarding; MCP viewer; static rendering/BFF iframe; and migration/cutover all have owning phases and named acceptance IDs.

Event/condition/condition-check and unrestricted custom-job stubs are intentionally discarded, not left as public enums returning success. Bounded cleanup remains implemented maintenance. Recipient metadata stays catalog/notification intent rather than an email delivery claim.

## Other design problems corrected

Native planning is not itself a safety proof. Read interfaces and validation must avoid a Go import cycle and reject zero/unbound executable plans. Topic publication must not expose new meaning before matching facets are ready. Sampling is not proof that no full scan occurred. Model outages must not break healthy frozen/artifact reads. Exact values survive rendering/export; hidden partial results cannot look complete. Logical idempotency and local transactions do not imply universal exactly-once remote execution or atomic cross-engine revert.

L2 reviewed engineering remains in scope. L3 and a new internal analyst are explicit extensions rather than blockers. Existing same-source multi-topic/rule replay/learning features are not swept into that deferral.

## Verification boundary

This change is planning/documentation plus planning-gate tooling, not production implementation. The source review in brief 14 remains selected-source evidence, not a claim that every file or deployed warehouse was tested. The new matrix makes missing runtime evidence visible and fail-closed at release instead of repeating a claim of complete parity.
