# Phase 10 — exec-read (Wave 3)

> **Status:** draft
> **Owner:** orchestrator (Opus-first — difficulty: high)
> **Depends on:** phase-08-sources-core, phase-09-sql-validate-core

The read execution half of `internal/exec`: the single read entry point
`exec.Query(ctx, ValidatedSQL, opts)` over the data-source adapter seam, with
**defense-in-depth** read-only enforcement, server-side statement timeouts wired
to the context deadline, cursor-level row caps that never wrap `LIMIT` by string,
`QueryResult → ResultPreview` shaping, caller idempotency keys, and execution
metrics. Phase 09 built the validation half and the unforgeable `ValidatedSQL`
type; this phase is what turns a `ValidatedSQL` into rows — and nothing else can.

---

## RFC / request sections

- **RFC-001 §9.6** — Execution (read-only, defense-in-depth): the four load-bearing
  guarantees this phase implements (adapter-level read-only session mode,
  server-side statement timeout + context deadline, cursor-level row caps,
  `QueryResult → ResultPreview` shaping) plus idempotency keys.
- **RFC-001 §1.2 P1b** — SQL-safety: generated/submitted SQL executes *only* through
  the §9.6 read-only layer, and only after §9.5 validation. This phase is the second
  half of that gate.
- **RFC-001 §1.2 P1c** — the write split: the `exec` seam exposes **no** write
  operation; warehouse writes live only on `sources.Materializer` (phase 13). This
  phase must keep `internal/exec` structurally write-free.
- **RFC-001 §6.1** — the adapter seam: `Query(ctx, ValidatedSQL, QueryOpts) →
  QueryResult` is the only read path; `ValidatedSQL` is constructible only by the
  validator. This phase consumes that seam, never a raw string.
- **RFC-001 §14** — the `exec` config domain (row caps, timeout, preview rows).
- **RFC-001 §15** — execution metrics (latency, timeouts) exported; audit rows are
  content-free (SQL text lives in the `queries` domain table, never in logs).
- **D-021** — SQL-safety mechanism: `ValidatedSQL` is the only executable type;
  read-only session enforcement at the adapter; server-side timeouts; cursor-level
  caps (never LIMIT-by-wrapping). This phase is D-021's execution half.

---

## Depends on

- **phase-08-sources-core** — supplies the adapter seam (`sources.Adapter`,
  `Query(ctx, ValidatedSQL, QueryOpts) → QueryResult`, `Capabilities()`/`Supports`),
  the `postgres` driver, and the `null`/`mock` drivers. This phase drives execution
  *through* that seam and adds per-driver read-only posture to the `postgres` driver's
  `Query` path; it does not re-open the seam signature.
- **phase-09-sql-validate-core** — supplies the unforgeable `ValidatedSQL` type and
  the typed error vocabulary. `exec.Query` accepts *only* a `ValidatedSQL`, so a caller
  cannot reach execution without having passed validation (the type is the gate).

Both are Wave-3 phases; 08 and 09 run in parallel and this phase closes the seam they
each opened (adapter `Query` shape from 08, `ValidatedSQL` from 09) — so this phase
ships an **integration test** with real drivers (§17).

---

## Informing briefs

Per `docs/research/INDEX.md`, `internal/exec` is informed by briefs **02** and **04**
(primary), with 03/07/08 secondary. This phase draws on:

- **Brief 02** (`02-predecessor-data-and-execution.md`) — the predecessors' plan/run
  execution path, the **single-gate read-only scar** (the validator was the *entire*
  guarantee and callers could reach `execute()` without it), client-side-only timeouts
  (a cancelled query kept consuming warehouse compute), and the LIMIT-wrapping lesson
  (wrapping a query in `SELECT * FROM (…) LIMIT n` silently drops a top-level
  `ORDER BY`). Result-shaping shapes are also from 02.
- **Brief 04** (`04-predecessor-security-tenancy.md`) — the SQL-safety posture and the
  **defense-in-depth recommendation**: the validator is the primary gate; the read-only
  session mode is an independent second gate; both must hold on their own.

## Brief findings incorporated

- **Defense-in-depth, two independent gates (brief 04).** The §9.5 validator is the
  primary guarantee, but this phase adds an *independent* execution-time read-only
  gate at the adapter: even a write statement smuggled past a hypothetically-broken
  validator fails at the engine. Criterion 1 is the mechanical proof — it constructs a
  write via a test-only backdoor that bypasses the validator and asserts the adapter
  rejects it.
- **Server-side timeouts, not client-side-only (brief 02).** Each adapter sets a
  server-side statement timeout (Postgres `statement_timeout`) *in addition to* the
  Go context deadline, so a cancelled/timed-out query stops consuming warehouse compute
  rather than only abandoning the client read. Criterion 2 verifies the server-side
  kill via `pg_stat_activity`.
- **Never LIMIT-by-string-wrapping (brief 02).** Row capping is a **cursor-level**
  contract — the driver stops reading rows after the ceiling — never a string rewrite
  of the query. This is written as a **standing exec rule** (see Design) with a
  regression guard (criterion 4: an `ORDER BY` query is capped and its order survives).
- **Result shaping from the predecessors' normalized shape (brief 02).** `QueryResult`
  is adapter-agnostic; `ResultPreview` is the ordered, capped, provider-neutral view
  (columns + rows + column metadata) that phase 20 (charts) and the §9.7 envelope consume.
- **Single-gate scar closed by type (brief 02/04).** `exec.Query` takes only a
  `ValidatedSQL`; there is no raw-string execute anywhere in `internal/exec` or on the
  adapter seam — the predecessors' "nothing requires prior validation" path cannot be
  expressed (criterion 6, compile-time).

## Findings I'm departing from

- **No regex "injection heuristics" at execution time (brief 02/04).** The predecessors
  layered string-level injection checks around execute(); the RFC (§9.5) makes the AST
  allowlist *the* guardrail and forbids regex heuristics presented as controls. This
  phase adds **no** string-level SQL inspection at execution; the only execution-time
  gate is the engine-level read-only session mode. This is deliberate, per RFC §9.5.
- **No result pagination or cross-request result cache (RFC §9.6/§19).** The
  predecessors carried result windowing; V1 explicitly does not (a per-grant result
  cache is correctness-hazardous — §19). Idempotency keys cover retries; that is the
  only cross-request state this phase adds.
- **Execution-time self-repair is deferred, not implemented here.** RFC §9.6 places a
  bounded (≤1) gateway repair loop behind `query.execute`, but it depends on the
  generation stack (`sqlfix` role, gateway) that lands in **phase 18**. This phase ships
  the read-execution primitive only; the repair loop wraps it in phase 18. The
  `exec` config domain's `self_repair` flag is therefore *not* added by this phase (a
  config key ⇒ a smoke check ⇒ a wired reader; phase 18 adds it when it wires it).

---

## Scope

Delivered in `internal/exec` (execution half) and the `postgres` adapter's read path:

1. **`exec.Query(ctx, ValidatedSQL, QueryOpts) → (QueryResult, error)`** — the single
   read execution entry point on the read path. No other exported execution function;
   no write function anywhere in the package (P1c).
2. **Per-driver read-only enforcement.** The `postgres` driver opens each query in a
   read-only transaction (`BEGIN … READ ONLY` / `pgx` `AccessMode: ReadOnly`) so a DML/DDL
   statement fails at the engine independent of validation. A documented **per-driver
   read-only posture table** records the mechanism (and its guarantee strength) for each
   V1 engine, with the enforcement stub each future driver (phase 14) must satisfy.
3. **Timeout interplay.** `QueryOpts.Timeout` (clamped to `exec.statement_timeout`) sets
   *both* the Postgres server-side `statement_timeout` (via `SET LOCAL` inside the
   read-only txn) *and* a `context.WithTimeout` deadline; whichever fires first cancels,
   and the server-side setting guarantees the warehouse stops computing.
4. **Cursor-level row capping.** Rows are read through the driver cursor and reading
   stops once `min(caller_max_rows, exec.max_row_cap)` rows are materialized; a
   `truncated` flag is set on `QueryResult`. **No LIMIT string-wrapping, ever** (standing
   exec rule). The clamp holds regardless of caller input (a caller asking for 10× the
   ceiling gets the ceiling).
5. **`QueryResult → ResultPreview` shaping.** `QueryResult` (adapter-agnostic: ordered
   columns, typed rows, row count, truncated flag, elapsed) projects to `ResultPreview`
   (columns + rows capped to `exec.preview_rows`, plus `ColumnMetadata` for phase 20),
   preserving column and row order.
6. **Idempotency keys.** `QueryOpts.IdempotencyKey` scopes a run
   `(operation, tenant, principal, key)` against the `idempotency_cache` table (store
   seam, phase 02): a repeat within TTL returns the recorded response **without
   re-executing**. First-write-wins under concurrency.
7. **Execution metrics.** `exec_query_duration_seconds`,
   `exec_query_rows_returned`, `exec_query_truncated_total`, and
   `exec_query_timeouts_total{reason=server|context}` registered and exported (telemetry
   conformance, phase 01). Content-free execution audit stamp (ids + outcome + row count
   + duration — never SQL text, never rows).

Packages touched: `internal/exec` (new execution files, same package as phase 09's
validation half), `internal/sources` (postgres driver read-only `Query` path — the
adapter contract is unchanged; the driver gains the read-only session behavior), and
`internal/telemetry` registration of the four exec metrics.

## Non-goals

- **SQL validation** — owned by phase 09; this phase consumes `ValidatedSQL`, never
  re-validates.
- **The write path / materialization** — `sources.Materializer` and all warehouse
  writes are phase 13 (P1c). `internal/exec` stays write-free by construction.
- **Execution-time self-repair (`sqlfix` loop)** — phase 18; needs the gateway
  generation stack. The `exec.self_repair` config key is not added here.
- **BigQuery / Snowflake / Databricks read-only posture** — phase 14; this phase
  documents the per-driver posture *table* and the stub each driver fills, and
  implements only `postgres` (+ `mock`/`null`).
- **Result pagination, cross-request result cache** — explicit V1 non-goals (§19).
- **The NLQ envelope / chart spec assembly** — phases 18/20 consume `ResultPreview`.
- **Query history persistence (`queries` table writes)** — the plan/run service (phase
  18) owns writing the `queries` row; this phase returns the `QueryResult`.

## Design

### The one read entry point

```
exec.Query(ctx, vsql ValidatedSQL, opts QueryOpts) (QueryResult, error)
```

`ValidatedSQL` (phase 09) is opaque and constructible only inside the validator, so
`Query`'s signature is itself the P1b gate: no code path reaches execution with an
unvalidated string. `internal/exec` exports exactly this one execution function on the
read path and **no** write function — an architecture test (criterion 5) asserts the
package's exported surface contains no `Insert`/`Update`/`Delete`/`Materialize`/`Exec`/
`Write`-shaped symbol and imports no `sources.Materializer` (P1c proof).

`QueryOpts` carries: `Scope` (the `access.EffectiveAccess`-derived tenant/grant scope —
non-optional, P1a/P3), `MaxRows` (caller request, clamped), `Timeout` (clamped),
`IdempotencyKey` (optional), `Operation` (`run|refine|submit`), and `Dialect`/adapter
handle resolved by the caller. Scope is non-optional: a `QueryOpts` without scope cannot
be constructed (a required field on a constructor, not a settable zero) — the
"no unscoped query API" rule (CLAUDE.md §6).

### Defense-in-depth read-only enforcement (per driver)

The adapter's `Query` runs the statement inside an engine read-only unit of work:

| Driver | Read-only mechanism | Guarantee |
|---|---|---|
| `postgres` (V1, this phase) | `BEGIN … READ ONLY` (pgx `TxOptions{AccessMode: ReadOnly}`) | Engine rejects any INSERT/UPDATE/DELETE/DDL with a typed error, independent of the validator |
| `mock` (tests) | Records the requested access mode; asserts read-only was requested | Test-observable proof the caller asked for read-only |
| `null` (fail-loud placeholder) | Any operation returns a typed "unavailable" error | Never silently succeeds |
| `bigquery` / `snowflake` / `databricks` | *phase 14* — documented per-engine equivalent (job read-only / session `READ ONLY` / statement type restriction) | phase 14 fills this row + a live-gated probe |

The read-only session is **independent** of the validator: the validator is the primary
gate; the session mode is the second, standalone gate (brief 04). Criterion 1 proves the
second gate holds *even if the first is hypothetically broken* — a test-only unexported
constructor forges a `ValidatedSQL` around a write statement (simulating a validator
bug) and asserts the adapter still rejects it at the engine. This is the headline
defense-in-depth proof; it lives in-package because `internal/exec` is the wiring boundary.

### Timeout + context-deadline interplay

`opts.Timeout` is clamped to `exec.statement_timeout` (ceiling). For `postgres`:

1. `ctx, cancel := context.WithTimeout(ctx, effectiveTimeout)` — the client deadline.
2. Inside the read-only txn: `SET LOCAL statement_timeout = <effectiveTimeout_ms>` — the
   **server-side** kill, so an over-budget query is terminated by Postgres and stops
   consuming warehouse compute (brief 02's scar: client-side-only cancellation left the
   server churning).

Whichever fires first wins; both are set so neither is the sole guarantee. A server-side
termination surfaces as a typed `exec.timeout` error and increments
`exec_query_timeouts_total{reason="server"}`; a context-deadline/caller-cancel surfaces
as `reason="context"`. Criterion 2 asserts the server-side path via `pg_stat_activity`
(the query is gone from the activity view after the timeout, not merely detached).

### Cursor-level capping (standing exec rule)

**Standing exec rule (binding on this and every future driver): row limiting is applied
at the cursor / result-read layer — the driver stops fetching after the ceiling — never
by wrapping the validated SQL in an outer `LIMIT`/subselect.** String-wrapping a query in
`SELECT * FROM (<vsql>) LIMIT n` silently drops a top-level `ORDER BY` (Postgres does not
guarantee ordering of a subselect), which is the fork's documented data-corruption lesson
(brief 02). The cap is `min(opts.MaxRows, exec.max_row_cap)` with `exec.default_row_cap`
when the caller omits `MaxRows`; the clamp is unconditional (a caller cannot exceed the
ceiling). When the cursor is stopped early, `QueryResult.Truncated = true`.

Criterion 4 is the regression guard: a `SELECT … ORDER BY … ` returning more than the cap
is executed, and the returned (capped) rows are asserted to be the first N *in the query's
declared order* — proving order survived capping (which it cannot if the cap were a
subselect wrap).

### Result shaping

```
QueryResult { Columns []ColumnDescriptor; Rows [][]Value; RowCount int;
              Truncated bool; Elapsed time.Duration }
        │  (adapter-agnostic; ordered)
        ▼
ResultPreview { Columns []ColumnMetadata; Rows [][]Value (≤ exec.preview_rows);
                Truncated bool; TotalRows int }
```

`ColumnMetadata` is the phase-20 chart-spec input (name, display name, data type,
inferred semantic role — derived from the executed shape, no extra I/O). Column order and
row order are preserved through the projection (deterministic — golden-tested).

### Idempotency

`opts.IdempotencyKey` (present only for `run`/`refine`/`submit`) keys
`idempotency_cache` on `(tenant_id, operation, principal, client_key)`. On a hit within
TTL the recorded response is returned and **no query is issued** (asserted by an adapter
call-count of zero — the same short-circuit discipline as P1a's empty-access set). First
write wins under `-race`; a concurrent duplicate observes the first result, never a
double execution.

### How it upholds P1–P7

- **P1b** — execution only via `ValidatedSQL` through the read-only layer; validation
  failure never reaches here (it is a phase-09 typed error).
- **P1c** — `internal/exec` exposes no write; the write split is structural (criterion 5).
- **P3** — `QueryOpts.Scope` is non-optional; the tenant predicate rides every warehouse
  query; no unscoped execution API exists.
- **P4** — a timeout, a read-only violation, or an adapter error is a **typed error +
  metric**, never an empty result read as "nothing found," never a silent skip-to-return.
- **P7** — one execution core; BYO `submit_sql` (phase 19) and internal `run_query`
  (phase 18) both converge on this one `exec.Query` — no parallel execution path.

---

## Config keys added

Under the `exec` config domain (RFC §14). Documented here, in the example config, and
smoke-checked (a bad value ⇒ refused boot, fail-loud validator — phase 01).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `exec.default_row_cap` | int | `10000` | no | Rows returned when the caller omits `MaxRows`. Must be `>0` and `≤ exec.max_row_cap`. |
| `exec.max_row_cap` | int | `100000` | no | Hard ceiling; caller `MaxRows` clamps to this regardless of request. Must be `>0`. |
| `exec.statement_timeout` | duration | `60s` | no | Ceiling for `QueryOpts.Timeout`; applied server-side (`statement_timeout`) **and** as the context deadline. Must be `>0`. |
| `exec.preview_rows` | int | `200` | no | Row ceiling for `ResultPreview`. Must be `>0` and `≤ exec.max_row_cap`. |

> `exec.self_repair` (RFC §14) is intentionally **not** added by this phase — the
> repair loop it gates lands in phase 18 with the gateway generation stack; adding the
> key here would create a smoke obligation for an unwired reader.

---

## Acceptance criteria

1. **Defense-in-depth read-only proof.** A write statement (`INSERT`/`UPDATE`/`DELETE`/
   DDL) forged into a `ValidatedSQL` via a test-only backdoor (simulating a broken
   validator) and passed to `exec.Query` against the `postgres` adapter is **rejected at
   the engine** (read-only transaction), returning a typed error and issuing zero row
   reads — proving the execution-time gate holds independently of validation.
2. **Server-side timeout kill.** A query exceeding `exec.statement_timeout` executed
   against Docker Postgres is terminated **server-side**: after the timeout the query is
   absent from `pg_stat_activity` (not merely client-detached), `exec.Query` returns a
   typed `exec.timeout` error, and `exec_query_timeouts_total{reason="server"}`
   increments.
3. **Row cap clamps to ceiling.** With `exec.max_row_cap = N`, a caller requesting
   `MaxRows > N` (and a source returning `> N` rows) receives exactly `N` rows and
   `QueryResult.Truncated == true` — the clamp holds regardless of caller input.
4. **`ORDER BY` preserved under capping (regression).** A `SELECT … ORDER BY … `
   returning more rows than the cap returns the first-N rows **in the query's declared
   order** — proving capping is cursor-level, not a `LIMIT`/subselect wrap that would drop
   the ordering.
5. **`internal/exec` is structurally write-free (P1c).** An architecture test asserts the
   package exports no write-shaped execution symbol and does not import
   `sources.Materializer`; `exec.Query` is the only exported read-execution function.
6. **Execution requires `ValidatedSQL` (compile-time).** `exec.Query` accepts only a
   `ValidatedSQL`; a raw `string` cannot be passed, and `ValidatedSQL` is unconstructible
   outside the validator package (compile-fail fixture under `go vet`/build — reused from
   phase 09's guarantee, exercised here).
7. **Context-deadline cancellation stops execution.** A caller-cancelled context aborts an
   in-flight `exec.Query` promptly with a typed error and
   `exec_query_timeouts_total{reason="context"}` increments (the context half of the
   interplay).
8. **Idempotency short-circuit.** A second `exec.Query` with the same
   `(operation, tenant, principal, IdempotencyKey)` within TTL returns the recorded
   response and issues **zero** adapter queries (call-count assertion); concurrent
   duplicates under `-race` execute exactly once.
9. **`QueryResult → ResultPreview` shaping is order-preserving and capped.** A golden test
   asserts the projection preserves column and row order and caps rows to
   `exec.preview_rows`, with `ColumnMetadata` populated for every column.
10. **Exec metrics exported.** `exec_query_duration_seconds`, `exec_query_rows_returned`,
    `exec_query_truncated_total`, and `exec_query_timeouts_total` all appear on `/metrics`
    after a query (telemetry conformance — phase 01's dead-counter guard).
11. **Config validators fail loud.** A non-positive `exec.default_row_cap`,
    `exec.max_row_cap`, `exec.statement_timeout`, or `exec.preview_rows`, or a
    `default_row_cap`/`preview_rows` exceeding `max_row_cap`, is a refused boot with a
    typed error (not a clamp-to-default).

## Test obligations

Per CLAUDE.md §11:

- **Unit (table-driven, `-race`):** cap clamping across `{omitted, under, at, over}`
  caller `MaxRows`; timeout clamping; `QueryResult → ResultPreview` projection goldens
  (order + preview cap + `ColumnMetadata`); config-validator matrix (criterion 11); the
  metrics-registration conformance slice (criterion 10).
- **Integration (real drivers, Docker Postgres via `make pg-up`, `-race`):** this phase
  closes the seam phases 08 (adapter `Query`) and 09 (`ValidatedSQL`) opened, so an
  integration test drives a real `ValidatedSQL` through the real `postgres` adapter:
  criteria 1 (read-only engine rejection), 2 (`pg_stat_activity` server-side kill), 3
  (row cap), 4 (`ORDER BY` regression), 7 (context cancel), 8 (idempotency). Identity/
  scope propagation (P3) asserted end-to-end; ≥1 failure mode (the timeout) covered.
- **Adversarial (SQL-safety path — standing obligation, §11 / master-plan convention 5):**
  the **write/DDL-injection** and **schema-escape** probes become live once exec ships.
  Criterion 1 is the write-injection defense-in-depth guard; a schema-escape probe
  (a statement referencing an out-of-scope table forged past the validator) asserts the
  read-only session still refuses to widen access at execution. Cross-tenant execution
  probe: a `QueryOpts.Scope` for tenant A cannot read tenant B rows through the adapter.
- **Fuzz:** n/a — the parse/decode fuzz surface (`FuzzValidate`) is phase 09's; this phase
  consumes already-typed `ValidatedSQL` and has no untrusted-bytes decode surface of its
  own. (Idempotency-key handling is a bounded opaque string, not a parser.)
- **Bench:** `BenchmarkQueryReadOnlyPath` — the read-only txn + cursor-cap hot path is a
  reusable per-request artifact; a baseline (not a CI gate) proving the read-only wrapper
  and cursor cap add no per-row allocation surprise.

## Coverage targets

`internal/exec` carries the **85% `exec` band** (master-plan convention 4). The band
entry `internal/exec 85` is created by phase 09 (the package's first phase); phase 10 adds
execution files to the same package and **must not regress** that band — no new
`scripts/coverage-bands.conf` entry is added by this phase (the existing exec entry
already covers it). The defense-in-depth and cap/timeout paths are the highest-value
lines and are directly exercised by the integration + adversarial tests above.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/exec` | 85% | The `exec` band (convention 4) — a SQL-safety-critical, conformance-adjacent subsystem. Band entry already present from phase 09; not re-added. |

## Smoke checks

Each criterion maps to a `go test -run` assertion in `scripts/smoke/phase-10.sh`
(sourcing `scripts/smoke/lib.bash`, driven through `run_group`). Integration-tier
criteria `t.Skip` cleanly when the Docker Postgres URL is unset, surfacing as SKIP.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 Read-only defense-in-depth | `TestExec_ReadOnly_RejectsForgedWrite` (integration) |
| 2 Server-side timeout kill | `TestExec_Timeout_ServerSide_PgStatActivity` (integration) |
| 3 Row cap clamps to ceiling | `TestExec_RowCap_ClampsToCeiling` |
| 4 `ORDER BY` preserved under cap | `TestExec_Cap_PreservesOrderBy` (integration) |
| 5 Package is write-free (P1c) | `TestExec_NoWriteSurface_Architecture` |
| 6 Requires `ValidatedSQL` | `TestExec_RequiresValidatedSQL` (build/compile fixture) |
| 7 Context cancel stops exec | `TestExec_ContextCancel_Stops` (integration) |
| 8 Idempotency short-circuit | `TestExec_Idempotency_NoReExecute` |
| 9 `ResultPreview` shaping golden | `TestExec_ResultPreview_OrderAndCap` |
| 10 Exec metrics exported | `TestExec_Metrics_Exported` |
| 11 Config validators fail loud | `TestExec_Config_FailLoud` |

## Glossary additions

Only terms this phase introduces that are not already in `docs/glossary.md`:

- **Query result / result preview** — `QueryResult` is the adapter-agnostic executed
  shape (ordered columns + rows + row count + truncated flag + elapsed); `ResultPreview`
  is its capped, ordered, provider-neutral projection (columns + rows ≤ `preview_rows` +
  `ColumnMetadata`) consumed by the chart spec (§10) and the answer envelope (§9.7).
- **Cursor-level row cap** — the standing exec rule that a row limit is applied at the
  driver cursor (stop reading after the ceiling), **never** by wrapping the validated SQL
  in an outer `LIMIT`/subselect (which silently drops a top-level `ORDER BY`).
- **Read-only session enforcement** — the execution-time, engine-level second gate
  (Postgres `BEGIN … READ ONLY`) that rejects any write independent of the validator; the
  defense-in-depth complement to §9.5 validation.
- **Idempotency key** — a caller-supplied key scoping a run `(operation, tenant,
  principal, key)` so a retried request returns the recorded response without
  re-executing.

*(`ValidatedSQL`, `data source`, `dataset`, `freshness`, `chart spec`, `scope-debug`
are already in the glossary — not re-added.)*

## Decisions filed

No new decision. This phase **implements** existing decisions:

- **D-021** — its execution half (read-only session enforcement, server-side timeouts,
  cursor-level caps, `ValidatedSQL`-only execution, read/write split).
- **D-020** — `QueryOpts.Scope` non-optional (P1a/P3 at the execution boundary).
- **D-004** — the customer data source (queried here) is distinct from the `store` seam;
  `idempotency_cache` lives in the `store`, the executed data does not.

A per-driver read-only posture that turns out materially weaker than Postgres's (a phase-14
discovery) would be a *new* decision entry then — flagged here as the risk-register
mitigation (master-plan risk "read-only session enforcement differs per engine").

## Deviation log

<!-- Filled DURING implementation. Every reasonable deviation from this plan
     (CLAUDE.md §4.3): what changed, why, and confirmation this file was updated in
     the same PR. Empty at authoring time. -->

_None yet — authoring._
