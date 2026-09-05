# Phase 02 — store-migrations

Status: shipped. Owner: internal/store. Hard dependencies: 01.

## Authority and design

RFC-001, RFC-002, the Pengui authority contract, D-044–D-052 and [COMMON.md](COMMON.md) apply. D-057 records the first concrete store consumer. Shipped means implemented and verified in this PR, not a deployment or permission to expose unprotected business routes.

## Brief findings incorporated

Briefs 02, 05, 14: tenant-composite durable references, immutable revision/CAS semantics, fresh/upgrade database proofs, transactional effects and real-driver recovery. Historical plans are not competing instructions.

## Findings I'm departing from

No local IAM or preallocation of every later analytics table. `store.Scope` is an internal validated tenant/actor isolation coordinate, not proof of authentication or a public request parameter. Phases 03/04 will project verified Pengui authority into this boundary. No HTTP/CLI tenant-override maintenance endpoint bypasses them. Local transaction success does not imply distributed exactly-once execution.

## Scope and implementation tasks

1. Delivered narrow store interfaces, real pgx pool/transactions, cancellable statement/lock timeouts, compiled forward-only migrations and disposable PostgreSQL conformance fixtures.
2. Delivered tenant-composite keys/FKs, immutable operational revisions, CAS pointers with transactional audit, unique operation keys/manifests and monotonic lease fences.
3. Delivered the internal retention service as the first real consumer; its settings are business/retention configuration, not identity or sharing policy. Operator backup/restore is real PostgreSQL tooling, not an in-memory serialization demonstration.

## Non-goals

No local users, API keys, roles, grants, memberships, service-account provisioning, issuer secrets, SQLite, raw public SQL port, reporting engine or scheduler dispatcher. Later domain phases add their necessary schema through forward migrations.

## Config and persistence

The [configuration reference](../configuration.md) defines secret-reference DSN, pool/connect/transaction timeouts and apply/check migration policy. Only five necessary relations ship: `schema_migrations`, `policy_revisions`, `policies`, `audit_events`, `operations`. Every tenant-owned repository operation takes a nonzero isolation coordinate; database predicates and composite references enforce its partition independently.

Migration history includes ordered names/checksums. Concurrent fresh boot applies each once, and failed DDL cannot commit partial history. Check/startup also verifies required relations exist. This does not claim protection against an operator with database-superuser privileges deliberately disabling constraints.

## Acceptance criteria

1. **AC01** — Fresh PostgreSQL boot and upgrade fixtures apply migrations exactly once; rollback is operational recovery, not editing a merged migration.
2. **AC02** — All tenant-owned keys/FKs and repository methods reject absent/mismatched tenant scope, including references and cascades.
3. **AC03** — Competing pointer changes/CAS have one winner; losing writes cannot publish audit or partially changed state.
4. **AC04** — Schema inspection finds no local IAM or issuer-secret state; source-secret custody is classified separately.
5. **AC05** — Run/idempotency/lease constraints survive rollback and restart; helpers have real first consumers and no memory-only integration shortcut.
6. **AC06** — Backup/restore and retention primitives preserve required references and report errors; store conformance runs under race detection.

## Tests, coverage and smoke

`test/acceptance/phase02_test.go` implements all six `TestPhase02/ACxx` children with real PostgreSQL. Tests cover concurrent migration/CAS/key reservation, injected audit/DDL failures, composite-reference escape, stale-worker fencing, mid-transaction expiry, changed retention policy, restart replay, actual private `pg_dump`/`pg_restore`, and refusal to restore over a nonempty target. A separate schema-health regression prevents valid history from hiding a missing relation.

`scripts/smoke/phase-02.sh` requires all actual named results. `make coverage` instruments these real integration callers and enforces 85% on both store packages. Missing database/client tooling fails, never skips. [Operator instructions](../../GETTING-STARTED.md), [self-review](../reviews/phase-01-02-adversarial.md) and [verification record](../reviews/phase-01-02-verification.md) accompany the code.

## Glossary, decisions and deviations

D-057: storage scope is not authentication; retention is the first consumer; expired keys retain tombstones. D-058: cross-package coverage and trusted-operator recovery. `DeletedOperations` counts compacted expired operation payloads, not reusable deletion of their keys. Required revision/audit references are retained rather than cascaded destructively; tests verify scoped deletion cannot affect another tenant or orphan dependencies.

All six acceptance criteria and the actual backup/restore round-trip passed under race detection before this status changed. Store coverage, vet and lint passed. Final read-only CI rechecks the exact PR tree. Full scheduler/reporting/product release remains in its owning later phases.
