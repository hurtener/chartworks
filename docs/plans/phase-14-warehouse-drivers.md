# Phase 14 — warehouse-drivers

Status: shipped. Owner: internal/sources. Hard dependencies: 08, 09, 10.

## Authority and design

RFC-001 §6/9, D-045/D-051/D-067 and [COMMON.md](COMMON.md) apply. Six real driver contracts remain migration scope; unsupported behavior is explicit and prevents a false cohort-complete claim.

## Brief findings incorporated

Briefs 02, 05, 14: multi-warehouse source seams, normalized type categories, native binding/validation and driver-specific read-only controls.

## Findings I'm departing from

No universal EXPLAIN dependency-coverage or CGo-free assertion based only on old pins. A PostgreSQL demo does not prove cloud-driver parity.

## Scope and implementation tasks

1. Add MySQL, SQL Server, BigQuery, Snowflake and Databricks through selective in-process leaf clients from the D-067 minimal Bruin fork. Record its exact immutable commit before qualification; do not import the universal manager or use the stock query CLI. Preserve the native PostgreSQL baseline until forked PostgreSQL parity passes.
2. Run real container fixtures for self-hosted engines plus fork SDK/HTTP fixtures and injected Chartworks Service lifecycle tests for cloud engines; include bind syntax, decimals, paging and cancellation. Keep owner-run live cloud evidence as a separate future cutover qualification.
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

Implement `TestPhase14/AC01` through `TestPhase14/AC06`. Fork leaf-client protocol fixtures and injected Chartworks Service lifecycle tests are separate from owner-run cloud evidence and never called a live pass. Real engine suites cover PostgreSQL/MySQL/SQL Server; cloud gating is explicit per engine. COMMON.md supplies conformance coverage and evidence rules; `scripts/smoke/phase-14.sh` requires all six results for full phase closure.

## Glossary, decisions and deviations

The [capability/support matrix](../contracts/warehouse-drivers.md) records the implemented subset and permanent live-cloud qualification boundary. Under the accepted no-live-cloud boundary, recorded cloud protocol tests establish implementation behavior but do not qualify live cutover/support. Unknown per-engine capabilities deny explicitly. D-051/D-067 apply. Phase 14 shipped at exact head `6883bc2103b2b870595222e623cd4d79bc01bc41` after [qualifying hosted CI run 34182486766](https://github.com/hurtener/chartworks/actions/runs/34182486766); no live-cloud qualification is claimed.

## Current implementation and verification mapping

The phase is shipped after final native CI and review completed at exact head
`6883bc2103b2b870595222e623cd4d79bc01bc41`. The [driver contract](../contracts/warehouse-drivers.md)
fixes the current supported subset and the explicit recorded-only cloud boundary.

| Criterion | Mandatory runtime evidence |
| --- | --- |
| AC01 | `TestPhase14/AC01` native discovery and exact values; injected cloud source lifecycle/type tests and fork leaf-client SDK/HTTP protocol fixtures |
| AC02 | `TestPhase14/AC02` validator/native planning success and rejection; parser dialect tests and injected cloud metadata lifecycle fixtures |
| AC03 | `TestPhase14/AC03` real SELECT-only account and context isolation; injected cloud effective-context negatives |
| AC04 | `TestPhase14/AC04` binding, serialized caps, acknowledged cancellation and rotation; fork owned-cancel/indeterminate protocol tests |
| AC05 | `TestPhase14/AC05` repeatable real fixtures and separate registry identities; SQL Server Developer test-only container; no cloud cutover qualification |
| AC06 | `TestPhase14/AC06` closed/unknown driver behavior and retained metadata without credentials; native shipping builds and documented support limits |

CI runs the actual source test package and pinned fork cloud/owned-session suites,
not a filename inventory. The native Linux amd64 SQL Server fixture is required;
a successful Mac mock or emulated container is not substituted for it.
