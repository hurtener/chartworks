# Chartworks consumer contract and product requirements

Current baseline: 2026-09-04, incorporating the owner's reporting, issuer, MCP Apps and Bifrost decisions. The prior request is retained in Git history; current RFCs and the active numbered plans govern implementation.

## Product objective

Deliver Pengui's Go-native structured analytics capability: connect/upload data, propose and review semantic models, explore questions, preserve approved reusable analytics as blocks, compose reports/dashboards, execute/schedule them, retain their results and render those results through API, MCP Apps and authenticated embedded/static views. This is a core differentiated offering, not only a SQL-generation endpoint.

The functional authority is the primary private source implementation; the secondary informs generalized connectivity, semantic generation and setup. Preserve actual behavior, not obsolete code, source-local infrastructure or stubs. Newly committed schemas/examples/tests must use neutral names and synthetic data. No legacy product/client names, private source URLs, prompts, credentials or customer schemas are copied into this repository.

## Integration ownership

Pengui owns authentication, users/groups/service identities, sharing, entitlements, issuer/signing/renewal and access-policy decisions. Chartworks verifies Pengui JWTs and applies signed operation/resource restrictions. No local login, API keys, roles/grants database, service-account provisioning, OAuth server or embed credential issuer. SQL safety, tenant constraints, valid references, publication evidence, certification and retention remain Chartworks business responsibilities.

Harbor/Pengui MCP Apps support is established end to end. Chartworks implements its tools/resources/read viewer against that capability; there is no host qualification/research or protocol upgrade gate. Pengui/client BFF handles iframe authentication and forwards scoped tokens server-side.

All learned-model operations use the embedded Bifrost Go SDK with configured remote providers. No local inference engines, weight downloads, transformer services or alternate direct-compatible production driver. Soundings/Stowage's non-secret gateway patterns can be reused, not their credentials or auth/storage configuration. See [the gateway contract](docs/contracts/model-gateway.md).

## Functional continuity

Preserve semantic topics/entities/joins/versions, source health and rechecks, compact context and explicit metric pins, multilingual question/refinement flows, templates, governed rules, replay/shadow evaluation, feedback/examples and bounded optimization. Externally generated SQL uses the same source/context/safety constraints as internal generation. Uploaded datasets enter the ordinary governed path.

Preserve block identities and aliases, mutable drafts versus immutable publications, exact validation evidence, separate certification/current health, parameter types/period policies, output subsets, protected SQL inspection, one logical result feeding several outputs, bounded narratives and dependency impact. Published query definitions are not regenerated during refresh.

Preserve report review/publication, grid/filter/presentation metadata, block/dynamic/text widgets, replayable versus session-bound query distinctions, partial-failure behavior, exact dashboard page references, private previews and durable artifact retrieval. A later publication never changes an old preview's privacy. A report audience label or creator field grants no access.

Retain real cron/interval/manual schedules for pipelines, reviewed saved queries, explicitly dynamic saved questions, selected block outputs and reports. Resolve occurrence time/window/revisions once; retain them across retries. Fresh execution authority comes from Pengui. Discard event/condition/condition-check/custom-code stubs rather than advertise nonfunctional targets. Catalog delivery is not email; optional outbound notifications use existing Pengui integrations and actual receipts.

## Delivery and operations

Opening retained results makes zero warehouse/model calls. API, MCP viewer, iframe and static rendering consume one versioned result/presentation contract. Go renders tables/KPIs/text; an isolated optional SVG worker supplies actual chart SSR. Client-only chart rendering is not SSR. Keep result data lossless and constrain exports, renderer resources and untrusted content. No standalone drag-and-drop authoring application is required.

Use one core, one PostgreSQL queue and one SDK-backed inference seam. Managed data writes remain separate from query readers, target only registered managed objects and never overwrite customer baseline data. External side effects have attempt/reconciliation/compensation evidence; local transactions do not imply distributed atomicity.

## Implementation handoff

[The master plan](docs/plans/README.md) assigns 34 phases, 224 acceptance criteria and a mapping of 63 source-feature rows plus 41 review gates. Follow each owning phase and COMMON.md; no required feature is silently deferred because it is outside the first demonstration. First useful reporting and complete migration are separate milestones.

Pengui integration scope serialization and unattended execution binding must be implemented against actual platform APIs in the owning integration phases. This document does not assert that newly proposed scope strings or a fresh-authority endpoint are already deployed. Resolve missing platform support on the Pengui side, never by creating local auth. Record exact provider/source versions and runtime evidence before deployment claims.
