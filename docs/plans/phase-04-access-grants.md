# Phase 04 — access-grants

Status: shipped. Owner: internal/access. Hard dependencies: 02, 03.

## Authority and design

The filename is retained for stable links; the implementation is signed-scope enforcement, not local grants. RFC-001 §5, [the Pengui authority contract](../contracts/pengui-authority.md), D-045 and D-059–D-061 control it. [COMMON.md](COMMON.md) supplies shared mechanics.

## Brief findings incorporated

Briefs 04, 05, 14: deny by default, tenant predicates in the data path, independent SQL inspection/execution scopes and nondisclosing denial.

## Findings I'm departing from

No local roles, memberships, grant CRUD, principal management, ownership shortcuts or implicit admin sentinel. Pengui makes policy decisions and signs the result. The implementation does not expose unbuilt reporting/source APIs to claim their future integration is already complete.

## Scope and implementation tasks

1. Exact enforcement of Pengui-signed operation and addressed-resource scopes with tenant-bound whole-ID wildcard semantics.
2. Require/Constrain helpers, expiry-bound immutable query selections, and execution/dependency/artifact-context checks over server-resolved metadata.
3. A central actual [operation registry](../contracts/chartworks-operations.json), protected diagnostics, and the first real PostgreSQL retention/audit/maintenance consumers with matching public SDK methods.

## Non-goals

No local policy engine, user/grant/role tables, sharing inference or token issuance. Full reporting, warehouse and MCP transport consumers stay in their owning phases. The selection/manifest contracts are mandatory inputs to those consumers, not a claim to infer omitted dependencies automatically.

## Config and persistence

One provider-scope grammar and common issuer bounds; no configurable alternate authorization mode. Business resource metadata establishes tenant/reference/context integrity, not access. Explicit wildcard scopes never remove tenant predicates. No new schema migration or local permission-policy table is required by this phase.

## Acceptance criteria

1. **AC01** — Operation and resource reach are both necessary; exact/wildcard semantics match the contract and bare admin/creator/agent names confer no access.
2. **AC02** — Empty/foreign reach prevents source access; store and source predicates enforce tenant/resource restrictions before reading data.
3. **AC03** — Report dependencies, execution contexts and artifact partitions are checked from signed reach, not tenant-only reuse or report audience labels.
4. **AC04** — Private preview, SQL view, publication, certification, export and schedule operations remain distinct permissions.
5. **AC05** — Diagnostics require signed operator scope and disclose no inaccessible names; there are no grants/roles/principals mutation APIs.
6. **AC06** — Mechanically enumerated endpoint/tool negatives and concurrent actor/context tests prove isolation; no local policy/role tables are consulted.

## Tests, coverage and smoke

`TestPhase04/AC01` through `AC06` cover the actual access helpers and registered operational consumers. A counted wrapper around the real PostgreSQL repository proves denial before I/O, not broad retrieval followed by filtering. Concurrent SDK requests use distinct tenants, service attributions and operation keys. Every registered endpoint has missing-bearer/action-only/reach-only negatives. Direct in-process service calls independently enforce the same policy. Execution/artifact fixtures cover complete reference sets, private preview status, context revision mismatches and distinct operation permissions; later domain adapters must supply complete manifests and extend the same tests at their source boundary.

`TestCompiledAuthorityLifecycle` exercises the actual binary, ephemeral TLS JWKS, PostgreSQL, all six SDK operations and graceful shutdown. `TestProviderRegistrationManifest` prevents documented operation/scopes from drifting from the registry. `TestSelectionExpiresWithVerifiedEnvelope` rejects stale pre-query selections. The existing 85% access threshold remains. `scripts/smoke/phase-04.sh` requires every named criterion with no skips.

## Glossary, decisions and deviations

The [adversarial review](../reviews/phase-03-04-adversarial.md) and [verification record](../reviews/phase-03-04-verification.md) distinguish implemented evidence from later domain work. Actual execution-context revision IDs, not caller labels, define policy partitions. Reading retained values does not imply permission to execute a new query, and exact-run read does not bypass private preview requirements. Operational metrics is separately issued deployment-operator authority. Pengui remains the sole policy owner; Chartworks merely verifies/enforces the signed result.
