# Phase 09 — `sql-validate-core`

> **Status:** draft
> **Owner:** orchestrator-assigned (Opus-first — high difficulty, security-critical)
> **Depends on:** phase-01-binary-config-telemetry

Copy per CLAUDE.md §16. This plan owns the **validation half** of `internal/exec`
(P1b, D-021, RFC §9.5). The read-execution half is phase 10; the topic-pack ∩ grants
allowlist walk is phase 18 (it needs `internal/semantics`, which does not exist yet).

---

## RFC / request sections

- **RFC §1.2 (P1b)** — SQL is untrusted regardless of origin; it executes only after
  the three-stage validation, and validation failure is a typed error, never a
  skip-to-execute.
- **RFC §9.5** — the three validation stages (pre-parse → dialect-aware AST parse →
  post-parse whole-tree statement blocking + single-statement + typed error
  vocabulary). This phase implements **stages 1–2 in full** and the
  **semantics-independent half of stage 3** (statement-family / blocked-node / single
  statement / dialect escapes). The `topic pack ∩ caller grants` allowlist and
  join-reachability portions of stage 3 are phase 18.
- **RFC §6.1** — the adapter seam owns `ValidatedSQL` as an opaque type
  constructible **only** by this validator; the `ansi` dialect sentinel covers
  unknown-dialect handling.
- **RFC §7.6 (P1c)** — the write-shape variant this phase produces for phase 13's
  `sources.Materializer` (single statement, declared output, declared inputs).
- **D-021** — three-stage AST validation, `ValidatedSQL` as the only executable type,
  split read/write interfaces, no regex injection heuristics.
- **D-032** — the V1 driver/dialect set is **six**: `postgres`, `mysql`, `sqlserver`
  (tsql), `bigquery`, `snowflake`, `databricks`, plus the `ansi` sentinel.
- **D-005** — CGo-free; the parser must be pure Go.
- **D-017** — the read/write posture split this validator enforces at the type level.

## Depends on

- **phase-01-binary-config-telemetry** — typed errors + the telemetry/metric
  primitives the typed validation-error vocabulary emits against, and the module/go.mod
  the parser dependency is pinned into. No store, no auth, no gateway: validation is a
  pure, deterministic, dependency-light library (this is deliberate — it keeps the most
  security-sensitive gate testable without a DB or a live model).

## Informing briefs

Per `docs/research/INDEX.md` (`internal/exec` row): primary
`docs/research/02-predecessor-data-and-execution.md`,
`docs/research/04-predecessor-security-tenancy.md`; secondary
`docs/research/03-predecessor-nlq-pipeline.md`,
`docs/research/07-wrenai-ideas.md`, `docs/research/08-datus-agent-ideas.md`.

## Brief findings incorporated

- **Brief 02 (headline scar): AST allowlist, not regex.** The predecessors' validator
  parsed with an AST library and allowlisted only `Select`/`With`/`Union`/`Intersect`/
  `Except` as the top-level node, with a base DDL/DML blocked-node list plus
  dialect-specific additions (e.g. DuckDB `Attach`/`Copy`/`Pragma`). This plan carries
  the **shape** — a walkable typed AST, a top-level SELECT-family gate, a blocked-node
  class list, and per-dialect escape additions — but re-derives the error codes fresh
  (brief 02 §"reusable shapes … not the codes").
- **Brief 02: the single-gate scar is the thing we are correcting.** The predecessors'
  read-only guarantee lived *entirely* in the validator, and callers could reach
  `execute()` without it. This phase's answer is the **unforgeable `ValidatedSQL`
  type** (an adapter cannot execute a raw string); the *independent* execution-time
  read-only enforcement is phase 10. Both must hold; this phase ships the first half.
- **Brief 02/04: no regex "injection heuristic" presented as a control.** The AST
  allowlist *is* the injection guardrail; identifiers are never string-assembled from
  model output. A source-level architecture test forbids a regex statement-classifier
  in the validate path (brief 02 §"dead-weight code that looks like security").
- **Brief 03 §4 (the CTE-regression lesson): the pre-parse stage must not duplicate
  parser judgment.** The predecessors once shipped a pre-parse rule that flatly
  rejected any query not starting with `SELECT`, silently blocking *every* CTE query
  (an estimated 15–25pt acceptance-rate cost) even though the parser had full CTE
  support. This plan makes it a **standing review rule + a golden CTE fixture guard**:
  pre-parse does *only* what a parser cannot (byte/length/encoding caps), never a
  keyword or statement-shape decision the parser makes correctly.
- **Brief 04 (D-017 requirement 2): the two allowlist sets are intersected, not
  conflated.** This phase does not yet do the allowlist (phase 18), but it **reserves
  the split** in the error vocabulary (`table.not_in_topic` vs `table.not_granted` as
  distinct, documented, semantics-gated codes) so phase 18 cannot collapse them.
- **Brief 07 (WrenAI): the validate-without-executing rung.** WrenAI's `dry-plan`
  (transpile/validate, no DB hit) → `dry-run` → `query` ladder maps onto Chartworks'
  validate (this phase, no DB) → execute (phase 10). The typed retry-vs-propagate
  error taxonomy (its `ErrorPhase`) informs our closed, exported error-code enum: a
  parse/validation code is a *caller/model-fixable* class (phase 18 bounded repair
  keys off it), distinct from an execution/connection class (phase 10).
- **Brief 08 (Datus): several small independently-testable gates, not one
  "validate SQL" function.** Statement-count, statement-type/family, node-class, and
  (later) allowlist are separate gates that each fail closed and each are unit-testable
  — the `SqlPolicyEnforcer`-seam shape, adopted as internal structure of the validator.

## Findings I'm departing from

- **Brief 02's `sqlglot`-family multi-dialect parser is not portable to Go.** `sqlglot`
  is Python and multi-dialect; adopting it means a subprocess or an embedded runtime —
  a direct D-005 / single-static-binary violation. No pure-Go multi-dialect parser
  exists (evaluated below). We therefore depart from "one parser understands every
  dialect" and adopt a **Postgres-grammar pure-Go base + fail-closed rejection of
  unparseable dialect syntax + a per-dialect escape blocklist** — the risk-register
  mitigation, made concrete.
- **Brief 03's dialect-specific pre-parse syntax checks (e.g. `DATEADD`/`INTERVAL`
  mixing, `DATE_TRUNC` comma checks) are NOT carried into pre-parse.** They are exactly
  the "pre-parse duplicating parser judgment" trap the CTE regression warns against; if
  a construct is malformed, the AST parse stage rejects it as `parse.error`. Pre-parse
  stays byte-level only.
- **Brief 08's `read_only` as a runtime flag on a shared path is rejected** in favor of
  the D-017/P1c type split (a separate `ValidatedWriteSQL` type + a separate
  materializer interface). No flag ever selects read vs write on a shared entry point.

## Scope

Delivers, in `internal/exec` (validation half only):

1. **Parser selection + pin** — `github.com/cockroachdb/cockroachdb-parser`
   **v0.25.2** (see Design), added to `go.mod` and its dev-build caveat pinned.
2. **The three-stage validator** — `Validate(ctx, raw string, dialect Dialect) →
   (ValidatedSQL, error)`:
   - Stage 1 **pre-parse**: byte-length cap, statement-length cap, UTF-8/encoding
     sanity, NUL/control-byte rejection. Nothing keyword- or shape-aware.
   - Stage 2 **parse**: dialect-aware AST parse via the pinned parser; unparseable ⇒
     typed `parse.error`; a construct the base grammar cannot represent ⇒ typed
     `parse.unsupported` (fail-closed, never a silent pass).
   - Stage 3 (semantics-independent half) **whole-tree statement blocking**: top-level
     node ∈ SELECT-family (`SELECT`/`WITH`/`UNION`/`INTERSECT`/`EXCEPT`) only; blocked
     node classes rejected **anywhere in the tree**; single-statement enforcement;
     dialect-escape blocklist. CTE-local names are resolved scope-aware (never a
     hard-fail on legitimate CTE shadowing).
3. **The typed validation-error vocabulary** — a closed, exported enum of error codes,
   normative for both surfaces; a golden test pins the set. Semantics-gated codes
   (`table.not_in_topic`, `table.not_granted`, `column.unknown`, `join.unreachable`)
   are declared here as reserved constants so phase 18 lands against a fixed contract,
   but are not *emitted* by this phase.
4. **The unforgeable `ValidatedSQL` type** — an opaque struct in `internal/exec` with
   no exported fields and no exported constructor; the only path to a value is a
   successful `Validate`. A compile-time proof (external `_test` package) shows it is
   unconstructible outside the package. This is the type the phase-08 adapter `Query`
   signature already requires (RFC §6.1).
5. **The write-shape variant** — `ValidateWriteShape(ctx, raw string, dialect Dialect,
   spec WriteShapeSpec) → (ValidatedWriteSQL, error)` for phase 13's `Materializer`:
   a single statement whose *only* write target is the one declared output, whose read
   references are ⊆ the declared inputs, and whose body otherwise passes the same
   node-class blocking (no nested DDL/DML, no second write). Returns a **distinct**
   `ValidatedWriteSQL` type — the read `exec.Query` path does not accept it and the
   materializer does not accept a read `ValidatedSQL` (P1c type-split, no flag).

## Non-goals

- **No execution.** No adapter call, no DB connection, no read-only session, no
  timeouts/row caps — all phase 10. Validation is pure and DB-free.
- **No topic-pack ∩ grants allowlist, no join-reachability walk.** Needs
  `internal/semantics` — phase 18. The error codes and the intersection *contract* are
  reserved here; the walk is not implemented.
- **No bounded repair / self-curation** — phase 18/§9.6.
- **No cross-dialect transpilation.** A template authored for one dialect is validated
  against that dialect; we never rewrite it (RFC §9.3).
- **No new surface** (no CLI command, endpoint, MCP tool, config key). This is an
  internal library other phases consume.

## Design

### Parser selection (this plan's first design duty — convention 8)

The choice was made against a **real per-dialect fixture corpus** exercised through
each candidate's actual API (a throwaway module, `go run`, against the live module
proxy — not from memory). Findings:

| Candidate (verified on proxy) | Grammar | Pure Go / CGo-free | Verdict |
| --- | --- | --- | --- |
| `github.com/cockroachdb/cockroachdb-parser` **v0.25.2** | PostgreSQL + ANSI | yes | **Selected** |
| `vitess.io/vitess/go/vt/sqlparser` v0.24.2 | MySQL | yes | Rejected — single MySQL dialect (wrong for 5 of 6) |
| `github.com/pingcap/tidb/pkg/parser` | MySQL | yes | Rejected — single MySQL dialect |
| `github.com/auxten/postgresql-parser` v1.0.1 (2022) | PostgreSQL | yes | Rejected — stale, older CRDB extraction, same grammar with less maintenance |
| `sqlglot` (predecessors' choice) | multi-dialect | **no (Python)** | Rejected — violates D-005 / single static binary |

**Why cockroachdb-parser v0.25.2.** It is the only actively-maintained, pure-Go parser
with a full Postgres-grade grammar and a **walkable typed AST**
(`tree.Statement`, `tree.WalkStmt` + a `Visitor` for whole-tree traversal). Verified
behaviour against the six-dialect corpus:

- **ANSI-common SELECT / CTE (`WITH`) / set-ops parse as a single `*tree.Select`** —
  across all six dialects for the shared read core. Postgres is the *exact* grammar
  (and the upload workspace is Postgres too, D-024), so two of six dialects are native.
- **Every DDL/DML gets a distinct AST node type** — `*tree.Insert`, `*tree.Update`,
  `*tree.Delete`, `*tree.CreateTable`, `*tree.DropTable`, `*tree.Call`, `*tree.CopyFrom`,
  … — so whole-tree blocking is a type-switch over the walked tree, and
  multi-statement is `len(stmts) > 1`. This is mechanically robust, not heuristic.
- **Dialect-specific SELECT extensions do NOT parse** (verified: MySQL backtick
  identifiers; T-SQL `TOP` and `[bracketed]` identifiers; Snowflake `QUALIFY`;
  BigQuery `SELECT * EXCEPT(...)`; Databricks `LATERAL VIEW`). Under the **fail-closed**
  posture these become a typed `parse.unsupported` rejection — **safe (never a silent
  pass)**, at a legitimate-query coverage cost. This *is* the standing risk-register
  entry ("Go SQL-parser dialect coverage falls short of the V1 warehouses"); the
  mitigation (the `ansi` sentinel + capability gating degrading unknown constructs to
  typed rejections) is realized exactly here.

**The `ansi` sentinel + dialect profiles.** `Dialect` maps each of the six engines (+
`ansi`) to a parse profile: the base grammar plus a per-dialect **escape blocklist** of
write-capable constructs that can masquerade as a read (even where a future dialect
parser would parse them as a `Select`): T-SQL `SELECT … INTO` and `EXEC`/`EXECUTE`,
MySQL `SELECT … INTO OUTFILE`/`INTO DUMPFILE` and `LOAD DATA`, Postgres/Snowflake
`COPY`/`COPY INTO`, `CALL`, and the base DDL/DML classes. Verified today these all
either surface as a distinct blockable node or fail to parse (→ `parse.unsupported`);
the blocklist is the belt to the parser's braces, so a grammar upgrade never silently
widens the write surface. An unknown dialect resolves to `ansi` (strictest common
subset), never to "parse permissively."

**Build/dependency caveat (evidenced, convention 8 "verify packaging against real
release assets").** The **static Linux target** — D-005's actual deliverable — builds
clean under `CGO_ENABLED=0 GOOS=linux`. A *local* `CGO_ENABLED=0` dev build on darwin
trips a stale transitive dep (`github.com/elastic/gosigar` v0.14.3, reached via
`pkg/sql/types → pkg/util/debugutil`; a Go 1.26 API drift, **not** a CGo issue —
v0.14.4 exists). Mitigation pinned in this plan: bump/replace `gosigar` to v0.14.4 in
`go.mod` so local `CGO_ENABLED=0` dev builds match CI. The parser drags a sizeable
transitive footprint (~120 CRDB packages); acceptable for a security-core gate, noted
so no one is surprised by `go.sum` size.

### Stage structure & the "never duplicate parser judgment" review rule

Pre-parse is deliberately dumb: it does *only* what a parser cannot (bytes, length,
encoding). A **binding review rule** (stated in the package doc and asserted by an
architecture test) forbids any pre-parse rule that inspects keywords, statement kind,
or SQL shape — that is the CTE-regression trap. The golden CTE fixture is the standing
guard: a representative `WITH`/recursive-CTE/set-op corpus must validate as SELECT-
family. If a pre-parse rule ever rejects a CTE, the guard fails.

### Upholding P1–P7

- **P1b** — the three stages + the fail-closed unparseable path; the AST allowlist is
  the sole injection guardrail (no regex control). **P1c** — the read/write type split.
- **P4** — every rejection is a typed error code + a metric increment; there is no
  "skip to execute" and no silent pass; an unparseable input is a loud typed rejection.
- **P5** — n/a to this phase (no model call); the error taxonomy is shaped so phase
  18's schema-constrained repair keys off typed codes, never free-text.
- **P7** — one validation core; the write-shape validator shares the same node-class
  engine; mode (a)/(b) both reach the same `Validate` (phase 18/19 parity keys off the
  fact that `ValidatedSQL` is the *only* executable type).

## Config keys added

**None.** Deliberate. Validation is pure, deterministic library code with no operator
knobs: the pre-parse byte/length caps are **safety constants**, not config — making
them tunable would let an operator *widen* the attack surface (a fail-closed constant
is the correct posture). The execution-side tunables (row caps, statement timeouts)
are config, but they belong to phase 10 (`internal/exec` execution half), not here.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none (see above) |

## Acceptance criteria

1. **Whole-tree DDL/DML rejection across all six V1 dialects.** A table-driven
   per-dialect corpus (postgres, mysql, sqlserver/tsql, bigquery, snowflake,
   databricks) of `INSERT`/`UPDATE`/`DELETE`/`MERGE`/`CREATE`/`ALTER`/`DROP`/`TRUNCATE`/
   `CALL`/`COPY`/`GRANT` — at top level **and nested** (e.g. inside a CTE / subquery) —
   is rejected with a typed error (`statement.blocked` where parsed, `parse.error`/
   `parse.unsupported` where not); **zero** entries yield a `ValidatedSQL`.
2. **CTE golden fixture passes.** A golden `WITH`/recursive-CTE/set-op SELECT corpus
   validates as SELECT-family and produces a `ValidatedSQL` (the CTE-regression guard).
3. **Multi-statement rejected.** Any input parsing to `len(stmts) > 1` yields typed
   `statement.multiple`; single-statement SELECT passes.
4. **`FuzzValidate` never panics, never passes a write.** A Go fuzz target with a seed
   corpus asserts two invariants on every input: (a) `Validate` never panics; (b) if it
   returns a `ValidatedSQL`, the parsed tree contains no write/DDL node class.
5. **`ValidatedSQL` unconstructible outside the package.** A compile-time proof (an
   external `exec_test` package) demonstrates no exported constructor/field can build a
   value; the type has zero exported mutable surface.
6. **Dialect-specific syntax degrades to a typed, fail-closed rejection.** A per-dialect
   "unsupported-but-safe" corpus (MySQL backticks; T-SQL `TOP`/`[brackets]`; Snowflake
   `QUALIFY`; BigQuery `EXCEPT(...)`; Databricks `LATERAL VIEW`) yields
   `parse.unsupported` — never a `ValidatedSQL`, never a panic.
7. **Typed error vocabulary is a closed, golden-pinned enum.** A golden test pins the
   exported error-code set; the reserved semantics-gated codes
   (`table.not_in_topic`/`table.not_granted`/`column.unknown`/`join.unreachable`) exist
   as distinct constants (not conflated) but are asserted **not emitted** by this phase.
8. **Pre-parse does only byte/encoding work.** An architecture/review test asserts the
   pre-parse stage references no SQL keyword / statement-kind token (the "never
   duplicate parser judgment" rule); the CTE golden fixture is the behavioural guard.
9. **Write-shape variant is correct and type-split.** `ValidateWriteShape` accepts a
   single CTAS / `INSERT … SELECT` writing only to the declared output and reading only
   declared inputs (→ `ValidatedWriteSQL`); it rejects a write outside the declared
   output, a second write, and any nested DDL/DML. A compile-time proof shows
   `exec.Query` does not accept `ValidatedWriteSQL` and the materializer signature does
   not accept a read `ValidatedSQL` (P1c split, no flag).
10. **No regex injection heuristic exists.** A source-level architecture test asserts
    the validate path contains no regex-based statement classifier — the AST allowlist
    is the sole guardrail.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven per-dialect corpora for criteria 1/3/6; the write-shape
  matrix (criterion 9); the error-vocabulary golden (criteria 2, 7). All DB-free and
  pure — deterministic golden tests.
- **Integration:** **n/a for this phase.** It closes no cross-subsystem seam and touches
  no real driver — it *opens* the `ValidatedSQL` contract phase 08/10 build on. The
  first integration proof (validator → adapter `Query`) lands in phase 10, and the
  per-dialect recorded-fixture dialect tests land in phase 14 (§17); a stub row for the
  reserved allowlist codes is handed to phase 18. (Stated deliberately, not skipped.)
- **Adversarial:** this is a SQL-safety path — the standing obligations (§11 / master
  plan convention 5) apply: the DDL/DML corpus (criterion 1), a write/DDL-injection
  probe (nested writes, stacked statements, comment-smuggled DDL), and a schema-escape
  probe seed. The *cross-tenant* and *fetch-then-filter* members of the access set are
  **n/a here** (no access set until phase 18) — noted, not silently dropped.
- **Fuzz:** **required** — `FuzzValidate` with a seed corpus and the two asserted
  invariants (criterion 4). This is a prime parse/decode surface (CLAUDE.md §11).
- **Bench:** `BenchmarkValidate` on a representative statement — the validator is a hot
  reusable artifact on every plan/run; a baseline, not a CI gate. A `-race`
  concurrent-reuse test proves the validator (a shared, immutable-after-construction
  singleton) is safe under concurrent `Validate` calls.

## Coverage targets

`internal/exec` sits in the **85% (`exec`)** band (master plan convention 4). This
phase creates the package, so it adds the band entry in the same PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/exec` | 85% | security-critical validation core; `exec` band (convention 4) |

## Smoke checks

`scripts/smoke/phase-09.sh` SKIPs cleanly until `internal/exec` exists, then runs one
assertion per criterion (all pure `go test` targets + two source-structural greps — no
binary, no DB needed).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `go test -run TestValidate_BlocksWritesAllDialects` passes (per-dialect DDL/DML corpus, zero pass) |
| 2 | `go test -run TestValidate_CTEGolden` passes (WITH/set-op corpus validates) |
| 3 | `go test -run TestValidate_RejectsMultiStatement` passes |
| 4 | `go test -run '^FuzzValidate$' -fuzz='^FuzzValidate$' -fuzztime=5s` (corpus run) never panics / never passes a write |
| 5 | `go build ./internal/exec/...` + `go vet`; the `ValidatedSQL` unconstructibility proof compiles (`TestValidatedSQL_Unconstructible` present) |
| 6 | `go test -run TestValidate_DialectSyntaxFailsClosed` passes (`parse.unsupported`, no panic) |
| 7 | `go test -run TestErrorVocabulary_Golden` passes; reserved codes present-but-unemitted |
| 8 | grep proves no SQL keyword token in the pre-parse source unit; `go test -run TestPreParse_NoParserJudgment` passes |
| 9 | `go test -run TestValidateWriteShape` passes; grep proves `Query(...)` signature takes `ValidatedSQL` not `ValidatedWriteSQL` |
| 10 | grep proves no `regexp` import in the validate source units; `go test -run TestNoRegexInjectionHeuristic` passes |

## Glossary additions

Only genuinely new terms (most exist already — `SQL-safety property`, `ValidatedSQL`
usage, `Data-source adapter`, `ansi` sentinel are covered):

- **Validated SQL** — an opaque value produced *only* by the validator (RFC §9.5) on a
  successful three-stage pass; the sole type a data-source adapter's read `Query`
  accepts. Unconstructible outside `internal/exec` — an adapter structurally cannot
  execute a raw string (P1b, D-021).
- **Validated write SQL** — the write-shape analogue for the materialization path
  (RFC §7.6): a single statement writing only to a declared destination and reading
  only declared inputs, on a **distinct** type the read path never accepts (P1c).
- **Dialect escape** — a write-capable or side-effecting construct that can masquerade
  as a read (`SELECT … INTO`, `INTO OUTFILE`, `LOAD DATA`, `COPY`/`COPY INTO`, `EXEC`,
  `CALL`); blocked by the per-dialect escape list even where the parser would accept it.
- **Fail-closed parse** — an input the base grammar cannot represent is a typed
  `parse.unsupported` rejection, never a silent pass — the posture that trades some
  legitimate-dialect-syntax coverage for a guarantee that nothing unproven executes.

## Decisions filed

- **References** D-021 (three-stage AST validation + `ValidatedSQL` + read/write split),
  D-032 (six-dialect set), D-017 (write-posture split), D-005 (CGo-free), D-024 (upload
  workspace is Postgres — why the Postgres grammar is native for two dialects).
- **Ratified as D-033** during the planning review (parser pin + fail-closed dialect
  posture + the phase-18 generation-side obligation). Original proposal text follows
  (number was for the orchestrator to assign; **not**
  written to `docs/decisions.md` by this plan): *"SQL validator parser pin —
  `github.com/cockroachdb/cockroachdb-parser` v0.25.2, a Postgres-grammar pure-Go base
  with fail-closed rejection of unparseable dialect syntax and a per-dialect escape
  blocklist; the `ansi` sentinel is the strictest-common-subset fallback. Includes the
  `gosigar` v0.14.4 dev-build pin."* The parser pin is a load-bearing, hard-to-reverse
  dependency choice with a security posture attached — it warrants its own `D-NNN`
  once the orchestrator accepts this plan.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->
