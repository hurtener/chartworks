# Phase 04 — access-grants

Status: shipped. Owner: internal/access. Hard dependencies: 02, 03.

## Authority and design

The existing filename is retained for stable links; the implementation is signed-scope enforcement, not local grants. RFC-001 §5, the Pengui authority contract and D-045 control it. [COMMON.md](COMMON.md) supplies shared mechanics.

## Brief findings incorporated

Briefs 04, 05, 14: deny by default, tenant predicates in the data path, independent SQL inspection/execution scopes and nondisclosing denial.

## Findings I'm departing from

Remove local roles, memberships, grant CRUD, principal management, ownership shortcuts and an implicit admin sentinel. Pengui makes those policy decisions and signs the result.

## Scope and implementation tasks

1. Keep the phase number/path for continuity, but replace grant resolution with exact enforcement of Pengui-signed operation/resource scopes.
2. Build Require/Constrain helpers and scoped query inputs; resolve resource/dependency identity without role/membership/ownership-derived permission expansion.
3. Register route/tool scope and audit requirements centrally; expose signed-operator diagnostics of actual checks, not a local principal management system.

## Non-goals

No local policy engine, user/grant/role tables, sharing inference or token issuance.

## Config and persistence

Use the single provider scope grammar and common bounds; no configurable alternate authorization mode. Business resource lookups verify references/tenant/context only. Explicit wildcard scopes follow the contract and never erase tenant predicates.

## Acceptance criteria

1. **AC01** — Operation and resource reach are both necessary; exact/wildcard semantics match the contract and bare admin/creator/agent names confer no access.
2. **AC02** — Empty/foreign reach prevents source access; store and source predicates enforce tenant/resource restrictions before reading data.
3. **AC03** — Report dependencies, execution contexts and artifact partitions are checked from signed reach, not tenant-only reuse or report audience labels.
4. **AC04** — Private preview, SQL view, publication, certification, export and schedule operations remain distinct permissions.
5. **AC05** — Diagnostics require signed operator scope and disclose no inaccessible names; there are no grants/roles/principals mutation APIs.
6. **AC06** — Mechanically enumerated endpoint/tool negatives and concurrent actor/context tests prove isolation; no local policy/role tables are consulted.

## Tests, coverage and smoke

Implement `TestPhase04/AC01` through `TestPhase04/AC06`. Assert no source query on denial, not just an empty filtered response. Use actual registered operations and store boundaries, test malformed/foreign scopes and reuse across concurrent callers. COMMON.md requires 85% access coverage. `scripts/smoke/phase-04.sh` requires all six results.

## Glossary, decisions and deviations

Signed reach and execution context are defined in the provider contract. D-044/D-045 supersede the former mechanism, not deny-by-default safety. Runtime acceptance and integration boundaries are recorded below.

## Implementation record — 2026-09-05

The six named acceptance criteria have real Go assertions and first consumers. See [provider handoff](../contracts/pengui-provider-registration.md) and [adversarial review](../reviews/phase-03-04-adversarial.md). No production platform credential or deployed session was used; issuer-shaped synthetic signing fixtures feed the actual verifier and PostgreSQL API/SDK consumer. No local policy database or issuer was added.

`internal/access` supplies Require/Constrain and execution/artifact partition gates; `internal/securityapi` applies them before the existing real metadata/maintenance consumer and again in-process. All six protected routes have SDK coverage and a checked operation manifest. Reporting/source domains must still supply complete server-resolved dependency manifests in their owning phases; no unimplemented reporting API is advertised.
