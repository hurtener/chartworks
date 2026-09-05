# Phase 02 — store-migrations

Status: in_progress. Owner: internal/store. Hard dependencies: 01.

## Authority and design

RFC-001, RFC-002, the Pengui authority contract and D-044–D-052 apply. [COMMON.md](COMMON.md) supplies the binding implementation, testing, coverage and smoke workflow. Register concrete operations with the early transport/SDK shells in the same feature change.

## Brief findings incorporated

Briefs 02, 05, 14; retain tenant-safe durable state and real-driver conformance. Historical detailed notes in `docs/archive/phase0-plans/` are background, not competing instructions.

## Findings I'm departing from

Remove speculative local IAM and preallocation of every domain table before its consumer exists. Pengui issues all authority.

## Scope and implementation tasks

1. Implement narrow domain store interfaces, pgx transactions, forward-only migrations and a fresh-database conformance harness.
2. Provide tenant-composite keys/FKs, immutable revision/CAS helpers, operation-key and lease primitives. Domain owners add their tables with the first consuming feature.
3. Remove planned API-key/grant/role/membership/service-identity tables. Retain business/configuration metadata without granting access from it.

## Non-goals

No local IAM, token issuance, SQLite driver or unrelated subsystem implementation.

## Config and persistence

Store DSN via secret reference, pool/transaction timeouts and migration policy; no local identity bootstrap configuration. Domain schema changes ship with their first consumer. All domain methods carry verified tenant/authority context.

## Acceptance criteria

1. **AC01** — Fresh PostgreSQL boot and upgrade fixtures apply migrations exactly once; rollback is operational recovery, not editing a merged migration.
2. **AC02** — All tenant-owned keys/FKs and repository methods reject absent/mismatched tenant scope, including references and cascades.
3. **AC03** — Competing pointer changes/CAS have one winner; losing writes cannot publish audit or partially changed state.
4. **AC04** — Schema inspection finds no local IAM or issuer-secret state; source-secret custody is classified separately.
5. **AC05** — Run/idempotency/lease constraints survive rollback and restart; helpers have real first consumers and no memory-only integration shortcut.
6. **AC06** — Backup/restore and retention primitives preserve required references and report errors; store conformance runs under race detection.

## Tests, coverage and smoke

Implement `TestPhase02/AC01` through `TestPhase02/AC06` against real PostgreSQL transactions; COMMON.md sets the workflow and 85% store coverage. `scripts/smoke/phase-02.sh` requires every named acceptance result. Missing/skipped runtime tests are not passes. Feature/gate ownership is in `coverage.json`.

## Glossary, decisions and deviations

Update the shared glossary. D-044–D-052 govern this revision. No runtime completion is claimed; record implementation findings and equivalent behavior before closure.

## Implementation record — 2026-09-05

The criterion-to-test mapping is implemented in `test/acceptance/phase02_test.go`, with shared adversarial cases and real PostgreSQL fixtures. The first foundation is intentionally loopback health-only before phases 03/04; storage scopes are isolation coordinates, not authentication. See D-056, D-057 and D-058, [operator instructions](../../GETTING-STARTED.md), [configuration reference](../configuration.md) and [self-review](../reviews/phase-01-02-adversarial.md). Package coverage uses full-suite cross-package instrumentation at unchanged thresholds. All six criteria must pass without skips before this phase is marked shipped.
