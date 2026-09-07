# Phase 14 — warehouse-drivers

Status: planned. Owner: internal/sources. Hard dependencies: 08, 09, 10.

## Authority and design

RFC-001 §6/9, D-045/D-051/D-067 and [COMMON.md](COMMON.md) apply. Six real driver contracts remain migration scope; unsupported behavior is explicit and prevents a false cohort-complete claim.

## Brief findings incorporated

Briefs 02, 05, 14: multi-warehouse source seams, normalized type categories, native binding/validation and driver-specific read-only controls.

## Findings I'm departing from

No universal EXPLAIN dependency-coverage or CGo-free assertion based only on old pins. A PostgreSQL demo does not prove cloud-driver parity.

## Scope and implementation tasks

1. Add MySQL, SQL Server, BigQuery, Snowflake and Databricks through selective in-process leaf clients from the D-067 minimal Bruin fork. Record its exact immutable commit before qualification; do not import the universal manager or use the stock query CLI. Preserve the native PostgreSQL baseline until forked PostgreSQL parity passes.
2. Run real container fixtures for self-hosted engines and recorded plus live suites for cloud engines; include bind syntax, decimals, paging and cancellation.
3. Publish a capability/support matrix separating tested/read-only/snapshot/parameter/managed-write coverage; unknown behavior denies execution.

## Non-goals

No general SQL federation, undisclosed dialect transpilation or unvalidated fallback driver.

## Config and persistence

Per-driver connector/pool/session/cost defaults and pinned module/build constraints; live credentials remain operator-only secrets. The fork seam must expose schema before rows, bounded iteration, typed parameters, native values/types, pre-ack query identity, cancel/status reconciliation and close/rotation. Chartworks retains plan admission, authority, limits, attempts, audit and retry decisions. Persist actual source/context and attempt provenance using the common models, not a bespoke per-driver reporting format.

## Acceptance criteria

1. **AC01** — Each declared driver passes schema discovery/type/result normalization and secret-free error fixtures.
2. **AC02** — Real/native validation proves dependency and function safety rather than assuming EXPLAIN always lists everything.
3. **AC03** — Read-only enforcement, effective context/RLS behavior and unauthorized relation/column access pass per supported engine.
4. **AC04** — Binding, row/byte/time ceilings and cancellation/query-ID reconciliation pass without ordering or numeric loss.
5. **AC05** — Container seeds are repeatable/licensing-clean; cloud live evidence is required before that engine is advertised for migration cutover.
6. **AC06** — Dependency/build tags and support limits are recorded; unavailable or untested features are neither silent fallbacks nor universal parity claims.

## Tests, coverage and smoke

Implement `TestPhase14/AC01` through `TestPhase14/AC06`. Offline recorded evidence is separate from owner-run cloud evidence and never called a live pass. Real engine suites cover PostgreSQL/MySQL/SQL Server; cloud gating is explicit per engine. COMMON.md supplies conformance coverage and evidence rules; `scripts/smoke/phase-14.sh` requires all six results for full phase closure.

## Glossary, decisions and deviations

The capability/support matrix records the tested boundary. Under the accepted no-live-cloud boundary, recorded cloud protocol tests establish implementation behavior but do not qualify live cutover/support. Unknown per-engine capabilities deny explicitly. D-051/D-067 apply. No runtime completion is claimed.
