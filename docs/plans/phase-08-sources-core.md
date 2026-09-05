# Phase 08 — sources-core

Status: shipped. Owner: internal/sources. Hard dependencies: 04, 09.

## Authority and design

RFC-001 §6, the Pengui authority contract and [COMMON.md](COMMON.md) apply. The exec package owns the read adapter contract; concrete source drivers implement it, avoiding an import cycle.

## Brief findings incorporated

Briefs 02, 04, 05: source registry, dialect/type normalization, secret-free reads, rotation and independent SQL-safety controls.

## Findings I'm departing from

Warehouse connection credentials are not user authentication. Retain appropriate connector custody without copying Pengui identity tokens or creating another integration vault. A source name or request context label cannot establish data restrictions.

## Scope and implementation tasks

1. Implement source registry/discovery and PostgreSQL adapter using the read interface defined by exec; avoid an exec-to-concrete-source import cycle.
2. Retain safe connector credential custody/secret-reference integration, independent read/write credential references, rotation and non-secret API models.
3. Register execution contexts binding source, credential/RLS/secure-view behavior and effective versions; use signed scopes to select them.

## Non-goals

No authentication provider, local grants, raw SQL execution escape or ungoverned write operation on a read adapter.

## Config and persistence

Sources connection limits, credential provider/key-ring references and test interval; execution context versioning. Secret-reference and custody behavior are documented separately from JWT verification. Store source locator/config/status and actual context version without secrets in public read shapes.

## Acceptance criteria

1. **AC01** — Source create/read/test/rotate honors signed reach and tenant scope; read/list types cannot include secret bytes.
2. **AC02** — Source secret rotation/pool refresh is tested without storing Pengui identity tokens or creating an integration vault.
3. **AC03** — Discovery classifies numeric/temporal/text/boolean/structured/binary/unknown types consistently, including money-like types.
4. **AC04** — Adapters reject invalid/zero/unvalidated plans and do not expose a raw-string execution escape.
5. **AC05** — Execution-context versions reflect actual data restrictions; request labels cannot turn broad credentials into a narrow partition.
6. **AC06** — Real PostgreSQL discovery/status/credential failures and concurrent reuse pass; unsupported adapter operations fail explicitly.

## Tests, coverage and smoke

Implement `TestPhase08/AC01` through `TestPhase08/AC06` using real PostgreSQL and secret rotation/pool fixtures. Check forbidden metadata/token leaks and typed unsupported paths. COMMON.md sets coverage; `scripts/smoke/phase-08.sh` requires all six results.

## Glossary, decisions and deviations

Source execution context is connector configuration, not an IAM role. D-044/D-045/D-051 apply. Runtime implementation and named acceptance are supplied here; no production deployment is claimed.

## Implemented evidence and qualified scope

D-064 and [the vector/source/read contract](../contracts/vector-sources-validation.md) pin the qualified scope. See the [adversarial review](../reviews/phase-07-08-adversarial.md). Existing hard dependencies and all six numbered criteria remain unchanged. Real PostgreSQL/pgvector and native-parser fixtures provide runtime evidence; final exact-head CI must pass before merge. The full execution product remains phase 10, additional engines phase 14 and the semantic generation consumer phase 15.
