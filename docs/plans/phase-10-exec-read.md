# Phase 10 — exec-read (Wave 3)

> **Status:** draft
> **Owner:** orchestrator (Opus-first — difficulty: high)
> **Depends on:** phase-08-sources-core, phase-09-sql-validate-core

The read execution half of `internal/exec`: the single read entry point
`exec.Query(ctx, ValidatedSQL, opts)` over the data-source adapter seam,
implementing the **layered P1b read posture of D-038** — read-only
credentials/sessions as the **primary** read-only guarantee, the **engine-side
dry-run/EXPLAIN pre-execution step** as the dialect-true validator and
table-grain allowlist gate, server-side statement timeouts wired to the context
deadline, cursor-level row caps that never wrap `LIMIT` by string,
`QueryResult → ResultPreview` shaping, caller idempotency keys, and execution
metrics. Phase 09 built the client-side validation layers and the unforgeable
`ValidatedSQL` type; this phase is what turns a `ValidatedSQL` into rows — and
nothing else can, and never before a completed dry-run.

---

## RFC / request sections

- **RFC-001 §9.6 (amended per D-038)** — Execution: **read-only
  credentials/sessions are the PRIMARY read-only guarantee** (SELECT-only
  provisioning where the engine supports it, reinforced by read-only
  transaction/session mode; the connection test asserts the posture by attempting
  a write and expecting engine denial), plus server-side statement timeout +
  context deadline, cursor-level row caps, `QueryResult → ResultPreview` shaping,
  idempotency keys.
- **RFC-001 §9.5 (amended per D-038), layer 3** — **engine-side dry-run/EXPLAIN,
  every engine, pre-execution**: the candidate SQL is dry-run (BigQuery) or
  EXPLAIN'd (Postgres/MySQL/Snowflake/Databricks; SQL Server showplan/
  `sp_describe_first_result_set`) under the read-only credential; the engine's own
  parser is the dialect-truth syntax check, and the **referenced-table set is
  checked against topic ∩ grants before the real run**. This phase implements the
  layer-3 machinery.
- **RFC-001 §1.2 P1b** — generated/submitted SQL executes only through this layer
  stack; **P1c** — the `exec` seam exposes no write operation (writes are the
  engineering stage's, D-036's Bruin-executed path — structurally unreachable
  from here).
- **RFC-001 §6.1** — the adapter seam: `Query(ctx, ValidatedSQL, QueryOpts)` is the
  only read path; `ValidatedSQL` is constructible only by the validator.
- **RFC-001 §14** — the `exec` config domain (row caps, timeout, preview rows).
- **RFC-001 §15** — execution metrics; content-free audit (SQL text never in logs).
- **D-038** — the layered read-side enforcement this phase realizes (layers 1–2
  are phase 09's; layer 3 + the credential/session posture are this phase's).
- **D-021** — the execution half as amended by D-038: `ValidatedSQL`-only
  execution, server-side timeouts, cursor-level caps, split read/write interfaces.
- **D-036 / D-037** — context only: the write path is Bruin's (never reachable from
  `exec`); the container posture does not change this phase (all adopted parser and
  Postgres drivers are pure Go).

---

## Depends on

- **phase-08-sources-core** — supplies the adapter seam (`sources.Adapter`,
  `Query(ctx, ValidatedSQL, QueryOpts) → QueryResult`, `Capabilities()`/`Supports`),
  the `postgres` driver, the connection test, and the `null`/`mock` drivers. This
  phase **extends the seam with the `DryRun` capability** (sanctioned by the RFC
  §9.5 amendment), adds the read-only posture probe to the connection test, and adds
  the read-only session behavior to the `postgres` driver's `Query` path.
- **phase-09-sql-validate-core** — supplies the unforgeable `ValidatedSQL` type, its
  **validation layer record** (which layers completed), and the typed error
  vocabulary. `exec.Query` accepts only a `ValidatedSQL` and refuses dispatch until
  the layer record carries a completed dry-run.

Both are Wave-3 phases; this phase closes the seam they each opened, so it ships an
**integration test** with real drivers (§17).

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
  **layered-enforcement recommendation**: no single gate is the guarantee; each layer
  holds on its own. D-038 promotes this from "defense-in-depth" to the architecture:
  the engine-level credential is the layer that survives any parser gap.

## Brief findings incorporated

- **The engine is the authority; credentials are the primary gate (brief 04 → D-038).**
  The predecessors' single client-side gate is inverted: read-only
  **credentials/sessions are the primary P1b read-only guarantee** — SELECT-only
  provisioning per engine where supported, reinforced by `BEGIN … READ ONLY`, with the
  connection test *asserting* the posture (attempt a write, expect engine denial —
  criterion 1). The client-side validator (phase 09) adds depth; it is never the sole
  guarantee. Criterion 2 is the mechanical proof: a write forged past a
  hypothetically-broken validator still fails at the engine.
- **Dialect-truth validation at the engine (D-038 layer 3, closing brief 02's
  validator-coverage gap).** Before real execution, every candidate SQL is
  dry-run/EXPLAIN'd under the read-only credential: syntax errors become typed
  failures from the engine's own parser (the only authority on its dialect), and the
  **referenced-table set is extracted and checked against topic ∩ grants** before any
  row is read — table-grain allowlisting guaranteed on every engine regardless of
  client-parser coverage.
- **Server-side timeouts, not client-side-only (brief 02).** Each adapter sets a
  server-side statement timeout (Postgres `statement_timeout`) *in addition to* the Go
  context deadline, so a cancelled/timed-out query stops consuming warehouse compute.
  Criterion 4 verifies the server-side kill via `pg_stat_activity`.
- **Never LIMIT-by-string-wrapping (brief 02).** Row capping is a **cursor-level**
  contract — the driver stops reading rows after the ceiling — never a string rewrite.
  A standing exec rule (see Design) with a regression guard (criterion 7: an
  `ORDER BY` query is capped and its order survives).
- **Result shaping from the predecessors' normalized shape (brief 02).** `QueryResult`
  is adapter-agnostic; `ResultPreview` is the ordered, capped, provider-neutral view
  that phase 20 (charts) and the §9.7 envelope consume.
- **Single-gate scar closed by type (brief 02/04).** `exec.Query` takes only a
  `ValidatedSQL`; no raw-string execute exists anywhere in `internal/exec` or on the
  adapter seam (criterion 10, compile-time) — and dispatch additionally requires the
  completed dry-run stamp (criterion 8), so even a `ValidatedSQL` cannot skip layer 3.

## Findings I'm departing from

- **No regex "injection heuristics" at execution time (brief 02/04).** The RFC (§9.5)
  makes the layered allowlist + statement blocking *the* guardrail and forbids regex
  heuristics presented as controls. This phase adds **no** string-level SQL inspection;
  its execution-time gates are the engine-level credential/session posture and the
  engine's own parser via dry-run/EXPLAIN. Deliberate, per RFC §9.5.
- **No result pagination or cross-request result cache (RFC §9.6/§19).** The
  predecessors carried result windowing; V1 explicitly does not. Idempotency keys cover
  retries; that is the only cross-request state this phase adds.
- **Execution-time self-repair is deferred, not implemented here.** RFC §9.6 places a
  bounded (≤1) gateway repair loop behind `query.execute`, but it depends on the
  generation stack (`sqlfix` role, gateway) that lands in **phase 18**. This phase ships
  the read-execution primitive only; the `exec.self_repair` config key is therefore not
  added here (a config key ⇒ a smoke check ⇒ a wired reader; phase 18 adds it).

---

## Scope

Delivered in `internal/exec` (execution half) and `internal/sources` (seam + postgres
driver additions):

1. **`exec.Query(ctx, ValidatedSQL, QueryOpts) → (QueryResult, error)`** — the single
   read execution entry point. No other exported execution function; no write function
   anywhere in the package (P1c).
2. **Read-only credential/session posture — the primary gate (D-038 layer 1).** The
   `postgres` driver documents SELECT-only provisioning (a provisioning recipe per
   engine: a role with `SELECT` on granted schemas only), opens each query in a
   read-only transaction (pgx `TxOptions{AccessMode: ReadOnly}`), and the **connection
   test gains the posture probe**: it attempts a trivial write against a scratch target
   and asserts **engine denial**; a connection whose credential permits the write is
   marked with a typed posture error (P4 — loud, never a silent `connected`). A
   documented **per-driver posture table** records provisioning + session mechanism per
   engine, with the stub each phase-14 driver must satisfy.
3. **Engine-side dry-run/EXPLAIN — the pre-execution step (D-038 layer 3).** The
   adapter seam gains `DryRun(ctx, ValidatedSQL) → DryRunReport` (capability-gated,
   `Supports(CapDryRun)`; **every real driver must support it** — `null` fails loud,
   `mock` is scriptable). For `postgres`: `EXPLAIN (FORMAT JSON)` under the read-only
   credential; the report carries dialect-true syntax validation (engine parse error ⇒
   typed `parse.engine_rejected`-class error) and the **referenced-table set** extracted
   from the plan. `exec` checks that set against the allowed-table set (topic ∩ caller
   grants, carried on `QueryOpts.Scope`) **before** real execution: an out-of-allowset
   table ⇒ typed `table.not_granted` / `table.not_in_topic`, zero rows read. Per-engine
   mechanism documented in the posture table (BigQuery dryRun — returns referenced
   tables explicitly; Snowflake/Databricks/MySQL EXPLAIN; SQL Server showplan/
   `sp_describe_first_result_set` — phase 14 fills those rows).
4. **Layer-record enforcement.** `ValidatedSQL` (phase 09) carries a validation **layer
   record**; `exec.Query` performs the dry-run, stamps `dry_run: completed` with the
   checked table set, and only the stamped record can reach the (unexported) dispatch
   path. A `ValidatedSQL` without a completed dry-run stamp is structurally
   undispatchable — a typed `exec.dry_run_required` error, never a skip-to-execute.
5. **Timeout interplay.** `QueryOpts.Timeout` (clamped to `exec.statement_timeout`) sets
   *both* the Postgres server-side `statement_timeout` (via `SET LOCAL` inside the
   read-only txn) *and* a `context.WithTimeout` deadline; whichever fires first cancels,
   and the server-side setting guarantees the warehouse stops computing. The dry-run
   runs under the same budget.
6. **Cursor-level row capping.** Reading stops once
   `min(caller_max_rows, exec.max_row_cap)` rows are materialized; `Truncated` is set.
   **No LIMIT string-wrapping, ever** (standing exec rule); the clamp holds regardless
   of caller input.
7. **`QueryResult → ResultPreview` shaping.** Adapter-agnostic `QueryResult` (ordered
   columns, typed rows, row count, truncated flag, elapsed) projects to `ResultPreview`
   (columns + rows capped to `exec.preview_rows`, plus `ColumnMetadata` for phase 20),
   preserving column and row order.
8. **Idempotency keys.** `QueryOpts.IdempotencyKey` scopes a run
   `(operation, tenant, principal, key)` against the `idempotency_cache` table: a repeat
   within TTL returns the recorded response **without re-executing** (and without
   re-dry-running). First-write-wins under concurrency.
9. **Execution metrics.** `exec_query_duration_seconds`, `exec_query_rows_returned`,
   `exec_query_truncated_total`, `exec_query_timeouts_total{reason=server|context}`, and
   `exec_dry_run_total{outcome=ok|syntax|table_blocked}` registered and exported
   (telemetry conformance, phase 01). Content-free execution audit stamp (ids + outcome
   + row count + duration — never SQL text, never rows).

Packages touched: `internal/exec` (execution files, same package as phase 09's
validation half), `internal/sources` (the `DryRun` seam method + capability, the
postgres driver's read-only `Query` path, EXPLAIN-based `DryRun`, and the
connection-test posture probe), `internal/telemetry` (metric registration).

## Non-goals

- **Client-side SQL validation (layers 1–2)** — phase 09's; this phase consumes
  `ValidatedSQL` and adds only the engine-side layer 3.
- **The write path / materialization** — the engineering stage's, executed through
  Bruin behind the `PipelineRunner` seam (D-036, phase 13). `internal/exec` stays
  write-free by construction; NLQ reads never route through Bruin (D-036).
- **Execution-time self-repair (`sqlfix` loop)** — phase 18.
- **BigQuery / Snowflake / Databricks / SQL Server / MySQL posture + dry-run
  implementations** — phase 14; this phase documents the per-driver posture/dry-run
  table and implements `postgres` (+ `mock`/`null`).
- **The semantics-aware allowset computation** — the topic ∩ grants *set* is computed
  by the caller stack (phase 18 wires topic packs; phase 04 supplies grants); this
  phase implements the **check mechanism** against the allowset carried on
  `QueryOpts.Scope` and enforces its presence (an absent allowset is a typed error,
  never allow-all).
- **Result pagination, cross-request result cache** — explicit V1 non-goals (§19).
- **Query history persistence (`queries` table writes)** — phase 18's plan/run service.

## Design

### The one read entry point, layered

```
exec.Query(ctx, vsql ValidatedSQL, opts QueryOpts)
   │ 1. idempotency check ── hit ⇒ recorded response, zero adapter calls
   │ 2. DryRun(ctx, vsql) under the read-only credential   (D-038 layer 3)
   │      engine parse error  ⇒ typed error (dialect-truth)
   │      referenced tables ⊄ opts.Scope.AllowedTables
   │                         ⇒ typed table.not_granted / table.not_in_topic
   │ 3. stamp vsql layer record: dry_run completed (+ table set)
   │ 4. dispatch (unexported; requires the stamped record)
   │      read-only txn (BEGIN … READ ONLY) + SET LOCAL statement_timeout
   │      cursor-level cap → QueryResult{…, Truncated}
   ▼
QueryResult ── project ──▶ ResultPreview (+ ColumnMetadata)
```

`ValidatedSQL` (phase 09) is opaque and constructible only inside the validator; its
**layer record** (tokenizer / AST-where-covered / dry-run) is append-only and stamped by
the owning layer. The dispatch path is unexported and takes a stamped-record type, so a
`ValidatedSQL` that has not completed the dry-run **cannot reach the wire** — the
enforcement is structural, proven by criterion 8's test backdoor. An architecture test
(criterion 9) asserts the package's exported surface contains no write-shaped symbol and
imports neither `sources.Materializer` nor the `PipelineRunner` seam (P1c proof).

`QueryOpts` carries: `Scope` (non-optional — tenant + `AllowedTables`, the
topic ∩ grants set derived from `access.EffectiveAccess` by the caller stack; P1a/P3),
`MaxRows` (clamped), `Timeout` (clamped), `IdempotencyKey` (optional), `Operation`
(`run|refine|submit`). A `QueryOpts` without scope cannot be constructed; an empty
`AllowedTables` short-circuits to a typed denial **before** the dry-run — no query, no
EXPLAIN, consistent with P1a's empty-set rule.

### Read-only posture — the primary gate (per driver)

| Driver | SELECT-only provisioning | Session reinforcement | Posture probe (connection test) | Dry-run mechanism |
|---|---|---|---|---|
| `postgres` (this phase) | Documented role recipe: `SELECT` on granted schemas only | `BEGIN … READ ONLY` (pgx `AccessMode: ReadOnly`) | Attempted write ⇒ expects engine denial; a permitted write ⇒ typed posture error | `EXPLAIN (FORMAT JSON)` under the read-only credential; referenced tables from plan relation nodes |
| `mock` (tests) | n/a | Records requested access mode | Scriptable outcome | Scriptable `DryRunReport` (drives the table-extraction tests) |
| `null` | n/a | n/a | Typed "unavailable" | Typed "unavailable" — fails loud |
| `bigquery` | *phase 14* | *phase 14* | *phase 14* | `dryRun: true` — referenced tables returned explicitly |
| `snowflake` / `databricks` / `mysql` | *phase 14* | *phase 14* | *phase 14* | `EXPLAIN` |
| SQL Server (post-V1 driver) | *with driver* | *with driver* | *with driver* | showplan / `sp_describe_first_result_set` |

The **credential is the layer that survives any parser gap** (D-038): the engine
enforces read-only in its own dialect against any SQL whatsoever. The session mode
reinforces it; the client validator adds depth. Criterion 2 proves the engine gate holds
independently: a test-only unexported constructor forges a `ValidatedSQL` around a write
statement *and* a forged dry-run stamp (simulating both client layers broken) and asserts
the adapter still rejects it at the engine.

### Dry-run/EXPLAIN — dialect-truth validation + table-grain allowlisting

`sources.Adapter` gains:

```go
DryRun(ctx context.Context, vsql ValidatedSQL) (DryRunReport, error)
// DryRunReport { ReferencedTables []TableRef; Elapsed time.Duration; … }
```

capability-gated by `Supports(CapDryRun)`; every real driver must declare it (an adapter
without it cannot serve the NLQ read path — typed refusal at registration, not a silent
skip). For `postgres`, referenced tables are extracted from the EXPLAIN JSON plan's
relation nodes (schema-qualified, deduplicated). `exec` then requires
`report.ReferencedTables ⊆ opts.Scope.AllowedTables` — the engine's own parser thereby
guarantees table-grain allowlisting on every engine, regardless of client-parser dialect
coverage (D-038 layer 3). Failures are typed (`parse.engine_rejected`,
`table.not_granted`, `table.not_in_topic`) and metered
(`exec_dry_run_total{outcome}`); a dry-run that cannot run (adapter error) is a typed
failure, **never** a skip-to-execute (P4).

### Timeout + context-deadline interplay

`opts.Timeout` is clamped to `exec.statement_timeout` (ceiling). For `postgres`:
(1) `context.WithTimeout` — the client deadline; (2) `SET LOCAL statement_timeout` inside
the read-only txn — the **server-side** kill, so an over-budget query is terminated by
Postgres and stops consuming warehouse compute. Whichever fires first wins; both are
always set. Server-side termination ⇒ typed `exec.timeout` +
`exec_query_timeouts_total{reason="server"}`; context cancel ⇒ `reason="context"`.
Criterion 4 asserts the server-side path via `pg_stat_activity`.

### Cursor-level capping (standing exec rule)

**Standing exec rule (binding on this and every future driver): row limiting is applied
at the cursor / result-read layer — the driver stops fetching after the ceiling — never
by wrapping the validated SQL in an outer `LIMIT`/subselect.** String-wrapping silently
drops a top-level `ORDER BY` (brief 02's documented lesson). The cap is
`min(opts.MaxRows, exec.max_row_cap)` with `exec.default_row_cap` when the caller omits
`MaxRows`; the clamp is unconditional. Early stop ⇒ `QueryResult.Truncated = true`.
Criterion 7 is the regression guard: capped rows of an `ORDER BY` query are the first-N
*in the query's declared order*.

### Result shaping

```
QueryResult { Columns []ColumnDescriptor; Rows [][]Value; RowCount int;
              Truncated bool; Elapsed time.Duration }
        ▼ (order-preserving)
ResultPreview { Columns []ColumnMetadata; Rows [][]Value (≤ exec.preview_rows);
                Truncated bool; TotalRows int }
```

`ColumnMetadata` is the phase-20 chart-spec input (derived from the executed shape, no
extra I/O). Column and row order are preserved (golden-tested).

### Idempotency

`opts.IdempotencyKey` keys `idempotency_cache` on
`(tenant_id, operation, principal, client_key)`. A hit within TTL returns the recorded
response and issues **zero** adapter calls (neither dry-run nor query). First write wins
under `-race`.

### How it upholds P1–P7

- **P1b (as layered by D-038)** — the credential/session posture is the primary gate;
  the engine dry-run is the dialect-truth validator and table-grain allowlist; the
  client validator (phase 09) adds depth; execution requires all stamps.
- **P1c** — `internal/exec` exposes no write; the write split is structural
  (criterion 9); the Bruin write path (D-036) is a different seam entirely.
- **P3** — `QueryOpts.Scope` is non-optional; the tenant predicate rides every
  warehouse query; no unscoped execution API exists.
- **P4** — a posture failure, dry-run failure, timeout, or adapter error is a **typed
  error + metric**, never an empty result, never a skip-to-execute.
- **P7** — one execution core; internal `run_query` (phase 18) and BYO `submit_sql`
  (phase 19) both converge on this one `exec.Query`.

---

## Config keys added

Under the `exec` config domain (RFC §14). Documented here, in the example config, and
smoke-checked (a bad value ⇒ refused boot — phase 01's fail-loud validators).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `exec.default_row_cap` | int | `10000` | no | Rows returned when the caller omits `MaxRows`. Must be `>0` and `≤ exec.max_row_cap`. |
| `exec.max_row_cap` | int | `100000` | no | Hard ceiling; caller `MaxRows` clamps to this regardless of request. Must be `>0`. |
| `exec.statement_timeout` | duration | `60s` | no | Ceiling for `QueryOpts.Timeout`; applied server-side (`statement_timeout`) **and** as the context deadline; covers the dry-run too. Must be `>0`. |
| `exec.preview_rows` | int | `200` | no | Row ceiling for `ResultPreview`. Must be `>0` and `≤ exec.max_row_cap`. |

> The dry-run step is **not** config-gated — D-038 makes it a mandatory layer; a toggle
> would be a silent-widening switch (P4). `exec.self_repair` is intentionally deferred
> to phase 18 (see Departures).

---

## Acceptance criteria

1. **Read-only posture probe (primary gate asserted).** The `postgres` connection test
   attempts a write under the connection's credential and **expects engine denial**;
   against a correctly provisioned (SELECT-only / read-only-session) Docker Postgres
   source the probe passes, and against a writable credential the source is marked with
   a typed posture error — never silently `connected`.
2. **Primary read-only guarantee holds with both client layers broken.** A write
   statement forged into a `ValidatedSQL` *with a forged dry-run stamp* (test-only
   backdoor simulating a broken validator and a broken layer record) and passed to
   `exec.Query` against the `postgres` adapter is **rejected at the engine** (read-only
   session), returning a typed error with zero rows read.
3. **Dry-run is dialect-truth syntax validation (Docker Postgres).** A syntactically
   invalid (but client-layer-plausible) statement fails the `postgres` `DryRun`
   (`EXPLAIN` under the read-only credential) with a typed engine-parse error **before**
   any real execution; a valid statement's `DryRunReport` carries the correct
   schema-qualified referenced-table set.
4. **Server-side timeout kill.** A query exceeding `exec.statement_timeout` executed
   against Docker Postgres is terminated **server-side**: after the timeout the query is
   absent from `pg_stat_activity` (not merely client-detached), `exec.Query` returns a
   typed `exec.timeout` error, and `exec_query_timeouts_total{reason="server"}`
   increments.
5. **Dry-run table set gates execution (mock-adapter extraction tests).** With the
   `mock` adapter scripted to report referenced tables, a report containing a table
   outside `opts.Scope.AllowedTables` yields a typed `table.not_granted` /
   `table.not_in_topic` error and **zero** execution calls (adapter call-count
   assertion); a fully-allowed set proceeds to dispatch. An empty `AllowedTables`
   short-circuits before the dry-run (P1a).
6. **Row cap clamps to ceiling.** With `exec.max_row_cap = N`, a caller requesting
   `MaxRows > N` (and a source returning `> N` rows) receives exactly `N` rows and
   `QueryResult.Truncated == true`.
7. **`ORDER BY` preserved under capping (regression).** A `SELECT … ORDER BY …`
   returning more rows than the cap returns the first-N rows **in the query's declared
   order** — proving capping is cursor-level, not a `LIMIT`/subselect wrap.
8. **Layer-record enforcement.** A `ValidatedSQL` whose layer record lacks a completed
   dry-run cannot be dispatched: the internal dispatch path requires the stamped record
   (typed `exec.dry_run_required` if reached without it), proven by a test-only probe of
   the dispatch boundary.
9. **`internal/exec` is structurally write-free (P1c).** An architecture test asserts
   the package exports no write-shaped execution symbol and imports neither
   `sources.Materializer` nor the pipeline-runner seam; `exec.Query` is the only
   exported read-execution function.
10. **Execution requires `ValidatedSQL` (compile-time).** `exec.Query` accepts only a
    `ValidatedSQL`; a raw `string` cannot be passed, and `ValidatedSQL` is
    unconstructible outside the validator package (compile-fail fixture).
11. **Context-deadline cancellation stops execution.** A caller-cancelled context aborts
    an in-flight `exec.Query` promptly with a typed error and
    `exec_query_timeouts_total{reason="context"}` increments.
12. **Idempotency short-circuit.** A second `exec.Query` with the same
    `(operation, tenant, principal, IdempotencyKey)` within TTL returns the recorded
    response and issues **zero** adapter calls (no dry-run, no query); concurrent
    duplicates under `-race` execute exactly once.
13. **`QueryResult → ResultPreview` shaping is order-preserving and capped.** A golden
    test asserts the projection preserves column and row order and caps rows to
    `exec.preview_rows`, with `ColumnMetadata` populated for every column.
14. **Exec metrics exported.** `exec_query_duration_seconds`,
    `exec_query_rows_returned`, `exec_query_truncated_total`,
    `exec_query_timeouts_total`, and `exec_dry_run_total` all appear on `/metrics`
    after exercise (telemetry conformance — phase 01's dead-counter guard).
15. **Config validators fail loud.** A non-positive `exec.default_row_cap`,
    `exec.max_row_cap`, `exec.statement_timeout`, or `exec.preview_rows`, or a
    `default_row_cap`/`preview_rows` exceeding `max_row_cap`, is a refused boot with a
    typed error (not a clamp-to-default).

## Test obligations

Per CLAUDE.md §11:

- **Unit (table-driven, `-race`):** cap clamping across `{omitted, under, at, over}`;
  timeout clamping; dry-run table-set gating matrix (allowed / partial / disjoint /
  empty allowset — criterion 5); layer-record enforcement (criterion 8);
  `QueryResult → ResultPreview` goldens (criterion 13); config-validator matrix
  (criterion 15); metrics conformance slice (criterion 14).
- **Integration (real drivers, Docker Postgres via `make pg-up`, `-race`):** this phase
  closes the seams phases 08/09 opened, so integration drives a real `ValidatedSQL`
  through the real `postgres` adapter: criteria 1 (posture probe), 2 (engine rejection
  with both client layers forged), 3 (EXPLAIN dialect-truth + table extraction),
  4 (`pg_stat_activity`), 7 (`ORDER BY` regression), 11 (context cancel),
  12 (idempotency). Identity/scope propagation (P3) asserted end-to-end; ≥1 failure
  mode (timeout) covered.
- **Adversarial (SQL-safety path — standing obligation):** the **write/DDL-injection**
  probe is criterion 2 (now proving the *primary* gate); the **schema-escape** probe is
  criterion 5's disjoint-table case run end-to-end (a statement referencing an
  out-of-allowset table is stopped by the dry-run check at execution, independent of
  client validation). Cross-tenant execution probe: a `QueryOpts.Scope` for tenant A
  cannot read tenant B rows through the adapter.
- **Fuzz:** n/a — the untrusted-bytes parse surface is phase 09's (`FuzzValidate`); the
  EXPLAIN-plan JSON decode is engine-produced (not caller-controlled) and is covered by
  table-driven fixtures per plan shape rather than fuzz.
- **Bench:** `BenchmarkQueryReadOnlyPath` — the dry-run + read-only txn + cursor-cap hot
  path; a baseline (not a CI gate) recording the per-query overhead the dry-run adds.

## Coverage targets

`internal/exec` carries the **85% `exec` band** (master-plan convention 4). The band
entry `internal/exec 85` is created by phase 09 (the package's first phase); phase 10
adds execution files to the same package and **must not regress** it — no new
`scripts/coverage-bands.conf` entry for `exec`. The `DryRun`/posture additions to
`internal/sources` fall under phase 08's existing sources band.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/exec` | 85% | The `exec` band (convention 4) — SQL-safety-critical. Band entry already present from phase 09; not re-added. |
| `internal/sources` | 85% | Existing band from phase 08 (conformance-tested subsystem); this phase's `DryRun` + posture-probe additions must hold it. |

## Smoke checks

Each criterion maps to a `go test -run` assertion in `scripts/smoke/phase-10.sh`
(sourcing `scripts/smoke/lib.bash`, driven through `run_group`). Integration-tier
criteria `t.Skip` cleanly when the Docker Postgres URL is unset, surfacing as SKIP.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 Read-only posture probe | `TestExec_ConnectionTest_PostureProbe` (integration) |
| 2 Engine rejects forged write (primary gate) | `TestExec_ReadOnly_RejectsForgedWrite` (integration) |
| 3 Dry-run dialect-truth + table extraction | `TestExec_DryRun_ExplainPostgres` (integration) |
| 4 Server-side timeout kill | `TestExec_Timeout_ServerSide_PgStatActivity` (integration) |
| 5 Dry-run table set gates execution | `TestExec_DryRun_TableAllowset` (mock adapter) |
| 6 Row cap clamps to ceiling | `TestExec_RowCap_ClampsToCeiling` |
| 7 `ORDER BY` preserved under cap | `TestExec_Cap_PreservesOrderBy` (integration) |
| 8 Layer-record enforcement | `TestExec_DryRunStamp_RequiredForDispatch` |
| 9 Package is write-free (P1c) | `TestExec_NoWriteSurface_Architecture` |
| 10 Requires `ValidatedSQL` | `TestExec_RequiresValidatedSQL` (build/compile fixture) |
| 11 Context cancel stops exec | `TestExec_ContextCancel_Stops` (integration) |
| 12 Idempotency short-circuit | `TestExec_Idempotency_NoReExecute` |
| 13 `ResultPreview` shaping golden | `TestExec_ResultPreview_OrderAndCap` |
| 14 Exec metrics exported | `TestExec_Metrics_Exported` |
| 15 Config validators fail loud | `TestExec_Config_FailLoud` |

## Glossary additions

Only terms this phase introduces that are not already in `docs/glossary.md`:

- **Read-only posture probe** — the connection-test step asserting D-038's primary
  read-only guarantee: an attempted write under the connection's credential must be
  denied by the engine; a permitted write is a typed posture failure, never a silent
  `connected`.
- **Dry-run (pre-execution check)** — the engine-side, pre-execution validation of a
  candidate SQL under the read-only credential (BigQuery dryRun; EXPLAIN on
  Postgres/MySQL/Snowflake/Databricks; SQL Server showplan): the engine's own parser is
  the dialect-truth syntax check, and the extracted referenced-table set is checked
  against topic ∩ grants before the real run (D-038 layer 3).
- **Validation layer record** — the append-only record on a `ValidatedSQL` of which
  D-038 layers completed (tokenizer / client AST where covered / dry-run); execution
  dispatch requires a completed dry-run stamp.
- **Query result / result preview** — `QueryResult` is the adapter-agnostic executed
  shape (ordered columns + rows + row count + truncated flag + elapsed); `ResultPreview`
  is its capped, ordered, provider-neutral projection consumed by the chart spec (§10)
  and the answer envelope (§9.7).
- **Cursor-level row cap** — the standing exec rule that a row limit is applied at the
  driver cursor (stop reading after the ceiling), **never** by wrapping the validated
  SQL in an outer `LIMIT`/subselect (which silently drops a top-level `ORDER BY`).
- **Idempotency key** — a caller-supplied key scoping a run `(operation, tenant,
  principal, key)` so a retried request returns the recorded response without
  re-executing.

*(`ValidatedSQL`, `data source`, `dataset`, `freshness`, `chart spec`, `scope-debug`
are already in the glossary — not re-added.)*

## Decisions filed

No new decision. This phase **implements** existing decisions:

- **D-038** — its execution-side layers: read-only credentials/sessions as the primary
  P1b guarantee (posture probe included), the engine dry-run/EXPLAIN layer with
  table-grain allowlisting, layer-record discipline.
- **D-021 (as amended by D-038)** — `ValidatedSQL`-only execution, server-side
  timeouts, cursor-level caps, split read/write interfaces.
- **D-020** — `QueryOpts.Scope` non-optional (P1a/P3 at the execution boundary).
- **D-036** — respected boundary: NLQ reads never route through Bruin; `exec` cannot
  reach the `PipelineRunner` seam (asserted by criterion 9).
- **D-004** — `idempotency_cache` lives in the `store`; the executed data does not.

A per-engine dry-run mechanism that proves materially weaker than Postgres's (a phase-14
discovery — e.g. an EXPLAIN variant that does not surface all referenced relations)
would be a *new* decision entry then, per the master-plan risk-register mitigation.

## Deviation log

<!-- Filled DURING implementation. Every reasonable deviation from this plan
     (CLAUDE.md §4.3): what changed, why, and confirmation this file was updated in
     the same PR. Empty at authoring time. -->

_None yet — authoring._

**Authoring-time revision (pre-implementation, coordinator-directed):** revised for the
D-036/D-037/D-038 amendments to RFC §9.5/§9.6 — read-only credentials promoted to the
primary P1b guarantee (posture-probe criterion added), the engine-side dry-run/EXPLAIN
pre-execution step added as a deliverable (seam `DryRun` capability + table-set
allowlist gate + layer-record enforcement, criteria 3/5/8), and the defense-in-depth
framing inverted accordingly. Timeouts, caps, `ORDER BY` regression, shaping, and
idempotency stand unchanged.
