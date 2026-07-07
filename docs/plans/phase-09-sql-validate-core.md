# Phase 09 — `sql-validate-core`

> **Status:** draft
> **Owner:** orchestrator-assigned (Opus-first — high difficulty, security-critical)
> **Depends on:** phase-01-binary-config-telemetry

Copy per CLAUDE.md §16. This plan owns the **client-side validation layers** of
`internal/exec` (P1b, D-021 as amended by D-038, RFC §9.5 amended). The engine-side
layers (read-only credentials, dry-run/EXPLAIN) are phase 10's; the topic-pack ∩
grants allowlist walk is phase 18 (it needs `internal/semantics`).

---

## RFC / request sections

- **RFC §1.2 (P1b)** — SQL is untrusted regardless of origin; validation failure is a
  typed error, never a skip-to-execute.
- **RFC §9.5 (amended — layered, D-038)** — this phase implements **layer 1 (tokenizer
  screens, every dialect)** and **layer 2 (client-side AST validation via the parser
  seam)** in their semantics-independent form, plus the layer-record contract that
  lets phase 10 enforce **layer 3 (engine-side dry-run/EXPLAIN)** before execution.
  The `topic pack ∩ caller grants` allowlist and join-reachability walks are phase 18.
- **RFC §6.1** — the adapter seam owns `ValidatedSQL` as an opaque type constructible
  **only** by the validator.
- **RFC §7.6 (P1c)** — the write-shape variant this phase produces for phase 13's
  materialization path (single statement, declared output, declared inputs).
- **D-038** — the layered posture: engine-side enforcement is primary; the client AST
  layer adds depth via a **parser seam** with per-dialect, **evidence-gated** driver
  adoption; a dialect without a proven driver **skips** the AST layer with a typed
  marker — it never fakes it. Generation targets the **native dialect** (D-033's
  generation-subset obligation is superseded).
- **D-033** — the `crdb` driver pin (`cockroachdb-parser` v0.25.2), retained by D-038.
- **D-021 / D-017** — `ValidatedSQL` as the only executable type; split read/write
  interfaces, no flag on a shared entry point.
- **D-037** — CGo permitted per-dependency; both adopted parser drivers are pure Go,
  so the binary stays CGo-free in practice.
- **D-032** — the six V1 dialects: `postgres`, `mysql`, `sqlserver` (tsql),
  `bigquery`, `snowflake`, `databricks`, plus the `ansi` sentinel.

## Depends on

- **phase-01-binary-config-telemetry** — typed errors + telemetry primitives the
  typed validation vocabulary emits against, and the module the parser drivers pin
  into. No store, no auth, no gateway: layers 1–2 are pure, deterministic,
  DB-free library code (deliberate — the most security-sensitive gates stay testable
  without a DB or a live model).

## Informing briefs

Per `docs/research/INDEX.md` (`internal/exec` row): primary
`docs/research/02-predecessor-data-and-execution.md`,
`docs/research/04-predecessor-security-tenancy.md`; secondary
`docs/research/03-predecessor-nlq-pipeline.md`,
`docs/research/07-wrenai-ideas.md`, `docs/research/08-datus-agent-ideas.md`.

## Brief findings incorporated

- **Brief 02: AST allowlist, not regex.** The predecessors parsed with an AST library
  and allowlisted only SELECT-family top-level nodes with a base blocked-node list
  plus dialect-specific additions. Carried as the **shape** of the AST layer (walkable
  typed tree, SELECT-family gate, blocked-node classes, per-dialect escapes); error
  codes re-derived fresh.
- **Brief 02: the single-gate scar.** The predecessors' read-only guarantee lived
  *entirely* in one upstream validator and callers could reach `execute()` without
  it. D-038 inverts the emphasis — the engine-side credential + dry-run is primary and
  the client AST layer adds depth — and this phase ships the structural half: the
  **unforgeable `ValidatedSQL`** carrying a **layer record**, so phase 10 can refuse
  execution until every required layer has run. No layer's absence silently widens
  access (P4).
- **Brief 02/04: no regex "injection heuristic" presented as a control.** The
  statement blocking + allowlists *are* the injection guardrail; identifiers are never
  string-assembled from model output. An architecture test forbids a regex statement
  classifier in the validate path.
- **Brief 03 §4 (the CTE-regression lesson).** A pre-parse rule once rejected any
  query not starting with `SELECT`, silently blocking every CTE query (~15–25pt
  acceptance cost) despite full parser-level CTE support. Standing review rule +
  golden CTE fixture: **the tokenizer layer does only what a parser cannot** —
  byte/encoding caps, statement separation, comment/quote hygiene — never a keyword or
  statement-shape judgment the parser makes correctly.
- **Brief 04 (D-017 requirement 2): topic and grant sets are intersected, not
  conflated.** Not implemented here (phase 18), but the error vocabulary **reserves
  the split** (`table.not_in_topic` vs `table.not_granted` as distinct codes) so phase
  18 lands against a fixed contract.
- **Brief 07 (WrenAI): the validate ladder + engine dry-run.** WrenAI's
  `dry-plan → dry-run → query` ladder is the direct ancestor of the D-038 layering;
  its typed `ErrorPhase` taxonomy informs the closed error-code enum (a
  validation-class code is model/caller-fixable — phase 18's bounded repair keys off
  it — distinct from execution/connection classes).
- **Brief 08 (Datus): several small independently-testable gates, not one "validate
  SQL" function.** Statement-count, statement-family, node-class, and escape gates are
  separate, each fails closed, each unit-testable — adopted as the internal structure
  of layers 1–2, and now also as the *layer record* the type carries.

## Findings I'm departing from

- **Brief 03's dialect-specific pre-parse syntax checks** (e.g. `DATEADD`/`INTERVAL`
  mixing) are NOT carried into the tokenizer layer — they are the "duplicating parser
  judgment" trap. Malformed constructs are the parser drivers' or the engine
  dry-run's job.
- **Brief 08's `read_only` runtime flag on a shared path is rejected** in favor of the
  D-017/P1c type split (`ValidatedWriteSQL` + a separate materializer interface).
- **The predecessors' "one parser understands every dialect" assumption is replaced**,
  per D-038, by the parser seam + per-dialect evidence-gated adoption + engine-side
  dialect truth. (The prior revision's ANSI-conservative generation framing is
  dropped — superseded by D-038's native-dialect rule.)

## Scope

Delivers, in `internal/exec` (client-side validation layers only):

1. **The tokenizer layer (layer 1, every dialect)** — byte-length cap, UTF-8/encoding
   sanity, NUL/control-byte rejection, **dialect-aware single-statement enforcement**
   and **comment/quote hygiene** (a separator inside a string literal or comment is
   not a statement boundary; an unterminated quote/comment is a typed rejection).
   Nothing keyword- or statement-shape-aware beyond separation.
2. **The parser seam (layer 2)** — interface + factory + driver (§4.4):
   - driver **`crdb`** = `github.com/cockroachdb/cockroachdb-parser` **v0.25.2**
     (D-033, retained by D-038) — postgres-family: postgres sources and every upload
     workspace (D-024);
   - driver **`sqlglotgo`** = `jonathan-fulton/sqlglot-go` **v0.4.0** (D-038; MIT,
     pure Go, all six dialects, walkable AST). **Per-dialect adoption is
     evidence-gated** through the conformance-reproduction harness below.
   - Where a driver covers the dialect: whole-tree statement blocking (top-level ∈
     SELECT-family; blocked node classes anywhere in the tree; dialect escapes),
     CTE-scope-aware name resolution (warning on shadowing, never hard-fail).
   - A dialect/input without a proven driver **skips the AST layer with the typed
     `parse.unsupported` marker** — recorded on the layer record, never a fake pass;
     the engine-side layers (phase 10) still apply to it.
3. **The conformance-reproduction harness** — the per-dialect fixture corpus (read
   fixtures incl. dialect-specific syntax; write/DDL fixtures top-level and nested;
   escape fixtures; multi-statement; CTE goldens) run mechanically through **each**
   parser driver. A dialect flips to a driver **only** on a 100% corpus pass
   (every write classified blockable, every read fixture parsed, every escape
   detectable in-tree). The harness emits a committed per-dialect conformance report,
   and the driver-adoption table is **generated from that report** — never
   hand-flipped. Re-run on every driver version bump (D-035 discipline).
4. **The typed validation vocabulary** — a closed, exported, golden-pinned enum.
   `parse.unsupported` means **"the AST layer was skipped for this input; engine-side
   layers still apply"** — a capability *marker*, not a rejection of the query.
   Semantics-gated codes (`table.not_in_topic`, `table.not_granted`, `column.unknown`,
   `join.unreachable`) are reserved constants, not emitted here.
5. **The unforgeable `ValidatedSQL` type + layer record** — opaque, no exported
   fields/constructor; the only path to a value is a successful `Validate`. It records
   **which layers ran and their outcome**: `tokenizer: passed`,
   `ast: passed | skipped(parse.unsupported)`, `engine: pending`. `engine: pending` is
   the **mandatory initial state**: the phase-10 contract (declared here as an
   interface expectation, implemented there) is that `exec.Query` refuses any
   `ValidatedSQL` whose engine-side layer has not completed. The layer record is
   append-only and package-private to mutation.
6. **The write-shape variant** — `ValidateWriteShape(ctx, raw, dialect, spec) →
   (ValidatedWriteSQL, error)` for phase 13: a single statement whose only write
   target is the one declared output, whose reads are ⊆ declared inputs, and whose
   body passes the same node-class blocking. A **distinct** type with the same layer
   record; the read `Query` path does not accept it and the materializer does not
   accept a read `ValidatedSQL` (P1c, no flag). Where the destination dialect lacks a
   proven AST driver, the write-shape AST check is skipped with the same typed marker
   and phase 13's remaining gates (declared destinations, `bruin validate`, D-036)
   carry the load — recorded, never silent.

## Non-goals

- **No execution and no engine-side layers.** Read-only credential posture, dry-run/
  EXPLAIN, referenced-table extraction, timeouts, row caps — all phase 10. This phase
  only defines the `engine: pending` layer-record state phase 10 consumes.
- **No topic-pack ∩ grants allowlist, no join reachability** — phase 18.
- **No bounded repair / self-curation** — phase 18 / RFC §9.6.
- **No transpilation.** sqlglot-go can transpile; Chartworks does not use it —
  generation targets the native dialect (D-038) and validation never rewrites.
- **No new surface** (no CLI command, endpoint, MCP tool, config key).

## Design

### Parser seam & drivers (evidence, convention 8)

Both drivers were verified against a real fixture corpus through their actual APIs (a
throwaway module against the live proxy — not from memory):

**`crdb` — cockroachdb-parser v0.25.2** (D-033, verified previously): pure Go; full
Postgres grammar; ANSI-common SELECT/CTE/set-ops parse as `*tree.Select`; every
DDL/DML is a distinct AST node type (`*tree.Insert`, `*tree.CreateTable`, …) so
blocking is a type-switch over `tree.WalkStmt`; multi-statement = `len(stmts) > 1`.
It does **not** parse dialect-specific SELECT extensions (MySQL backticks, T-SQL
`TOP`/brackets, Snowflake `QUALIFY`, BigQuery `EXCEPT()`, Databricks `LATERAL VIEW`)
— under D-038 those inputs route to the `sqlglotgo` driver or skip the AST layer
with the marker; they are no longer framed as a capability limit of the product.
Dev-build caveat: transitive `gosigar` v0.14.3 breaks a darwin `CGO_ENABLED=0` build
(Go 1.26 API drift; the static Linux target builds clean); pin/bump `gosigar` v0.14.4.

**`sqlglotgo` — jonathan-fulton/sqlglot-go v0.4.0** (D-038), probed against the
six-dialect corpus:

- Parses all six dialects' distinctive read syntax (verified: MySQL backticks, T-SQL
  `TOP` + `[bracketed]`, Snowflake `QUALIFY`, BigQuery `SELECT * EXCEPT(...)`,
  Databricks `LATERAL VIEW`) via `sqlglot.Parse(sql, Options{Read: dialect})`.
- Statement classification via `Expression.Kind()` with distinct kinds for
  `Insert`/`Update`/`Delete`/`Merge`/`Create`/`Drop`/`Command`(CALL)/`Copy` — all
  verified blockable; multi-statement detectable (`len(exprs) > 1`); whole-tree walk
  via `Expression.Walk`/`Find`/`FindAll`.
- **The decisive escape case is proven**: T-SQL `SELECT a INTO newt FROM t` parses as
  kind `Select` — a write masquerading as a read — and the `exp.Into` node **is
  findable in-tree** (`e.Find(exp.Into) != nil`), so the escape blocklist catches it
  structurally. MySQL `INTO OUTFILE` / `LOAD DATA` fail to parse → AST-skip marker →
  the engine-side read-only credential still blocks them (the layered guarantee).
- Pure Go (zero CGo files — consistent with D-037's preference).
- **Packaging defect (convention-8 finding, load-bearing):** every published version
  (v0.1.0–v0.4.0) declares module path `github.com/jonathanfulton/sqlglot-go` (no
  hyphen) while the repo lives at `github.com/jonathan-fulton/sqlglot-go`, and no
  repo exists at the declared path. The module is **unconsumable without a `replace`
  directive**: `replace github.com/jonathanfulton/sqlglot-go =>
  github.com/jonathan-fulton/sqlglot-go v0.4.0` (verified working). This strengthens
  D-038's anticipated endgame — a fork under our org (which also fixes the module
  path) once the driver proves out; until then the replace + exact pin is the recorded
  posture. Bus factor 1 / 4-weeks-old is exactly why adoption is harness-gated.

### The conformance-reproduction harness (the D-038 gate, designed here)

A table-driven harness (`internal/exec`, ordinary `go test` + a generator step): for
each `(dialect, driver)` pair it runs the full fixture corpus and scores four
families — *read-parses* (incl. dialect-specific syntax), *write-classification*
(every DDL/DML fixture, top-level and nested, maps to a blockable node), *escapes*
(every escape fixture detectable in-tree or a parse failure — never a clean
`Select`-and-nothing-else), *statement-count* (multi-statement fixtures). A pair is
**adopted** only at 100% on all four; the result is written to a committed
per-dialect conformance report (golden — a driver bump that changes it fails the
diff), and the factory's dialect→driver table is generated from the report. Expected
V1 outcome per the probes: `postgres` → `crdb`; the other five → `sqlglotgo` where
the full corpus confirms the probe results; any pair that fails stays AST-skipped
with the typed marker. **The harness, not this plan, is the authority** — that is the
point.

### Layer record & the "never duplicate parser judgment" rule

`Validate` runs tokenizer → (adopted driver? AST walk : skip-with-marker) and stamps
the layer record. The tokenizer is deliberately dumb; a **binding review rule**
(package doc + architecture test) forbids any tokenizer-layer rule that inspects
keywords or statement shape beyond separator/quote/comment mechanics — the
CTE-regression trap. The golden CTE fixture is the behavioural guard on both drivers.

### Upholding P1–P7

- **P1b** — layers 1–2 + the honest layer record; the allowlist/blocking is the
  injection guardrail (no regex control). **P1c** — the read/write type split.
- **P4** — every rejection and every skip is typed + metered; a skipped layer is a
  recorded marker, never a silent pass; no layer's absence widens access.
- **P5** — n/a (no model call); error taxonomy shaped for phase 18's typed repair.
- **P7** — one validation core; both generation modes and the write path reach the
  same seam; `ValidatedSQL` is the only executable type.

## Config keys added

**None.** Deliberate: tokenizer caps are fail-closed **safety constants**, not
operator knobs; the driver-adoption table is **harness-generated**, not configured (a
config override would let an operator widen the AST-skip surface silently — the exact
P4 failure). Engine-side tunables (timeouts, row caps) are phase 10's.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none (see above) |

## Acceptance criteria

1. **Conformance-reproduction harness gates driver adoption.** The harness runs the
   full per-dialect fixture corpus against each parser driver, emits the committed
   per-dialect conformance report (golden), and the factory's dialect→driver table is
   generated from it; a hand-edit to the table without a matching report diff fails
   the build. No `(dialect, driver)` pair below a 100% corpus pass is adopted.
2. **Whole-tree DDL/DML rejection across the V1 dialects.** For every dialect with an
   adopted AST driver, the table-driven write corpus (`INSERT`/`UPDATE`/`DELETE`/
   `MERGE`/`CREATE`/`ALTER`/`DROP`/`TRUNCATE`/`CALL`/`COPY`/`GRANT`, top-level **and
   nested**) is rejected with typed `statement.blocked`; **zero** corpus entries
   yield a `ValidatedSQL` whose record claims `ast: passed`; a non-adopted pair shows
   `ast: skipped` for the same corpus — never a fake pass.
3. **CTE golden fixture passes on both drivers.** The `WITH`/recursive-CTE/set-op
   SELECT corpus validates as SELECT-family through `crdb` and `sqlglotgo` (the
   CTE-regression guard).
4. **Tokenizer screens hold on every dialect.** Multi-statement input is rejected
   typed (`statement.multiple`) **dialect-aware**: a separator inside a string
   literal or comment does not trip it; a genuine second statement does; an
   unterminated quote/comment and an over-cap/malformed-encoding input are typed
   rejections. Runs identically for all six dialects + `ansi`.
5. **`FuzzValidate` invariants.** A fuzz target with a seed corpus asserts, per input:
   (a) never panics; (b) a returned `ValidatedSQL` claiming `ast: passed` contains no
   write/DDL/escape node; (c) the layer record never claims a layer that did not run.
6. **`ValidatedSQL` unconstructible + honest layer record.** Compile-time proof
   (external `exec_test` package) that no exported constructor/field can build a
   value; the layer record is read-only outside the package; `engine: pending` is the
   mandatory initial state (the phase-10 refusal contract is asserted as an interface
   expectation test against the exported contract type).
7. **AST-skip is typed and fail-honest.** For a dialect/input without a proven
   driver, `Validate` returns a `ValidatedSQL` with `ast: skipped` + the
   `parse.unsupported` marker (never an `ast: passed` claim, never a panic); the
   golden error/marker vocabulary test pins the closed enum, including the reserved
   (unemitted) semantics codes as distinct constants.
8. **Tokenizer never duplicates parser judgment.** An architecture test asserts the
   tokenizer-layer source references no SQL keyword/statement-kind token beyond
   separator/quote/comment mechanics; the CTE golden fixture is the behavioural
   guard.
9. **Write-shape variant correct and type-split.** `ValidateWriteShape` accepts a
   single CTAS / `INSERT … SELECT` writing only to the declared output reading only
   declared inputs (→ `ValidatedWriteSQL`); rejects a write outside the declared
   output, a second write, and nested DDL/DML; skips-with-marker on a non-adopted
   destination dialect. Compile-time proof that `exec.Query` does not accept
   `ValidatedWriteSQL` and the materializer signature does not accept `ValidatedSQL`.
10. **Dialect escapes are caught structurally; no regex heuristic.** The escape corpus
    (T-SQL `SELECT … INTO`, `EXEC`; MySQL `INTO OUTFILE`/`LOAD DATA`; Snowflake
    `COPY INTO`; `CALL`) each either yields a typed block (in-tree detection — e.g.
    the verified `exp.Into` node) or an AST-skip marker — never an `ast: passed`
    `ValidatedSQL`; an architecture test asserts no regex-based statement classifier
    exists in the validate path.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** the conformance harness itself (criterion 1); per-dialect corpora for
  criteria 2/3/4/10; the write-shape matrix (criterion 9); vocabulary + report
  goldens (criteria 1, 7). All DB-free, deterministic.
- **Integration:** **n/a for this phase** — it closes no seam and touches no real
  driver-at-the-boundary; it *opens* the `ValidatedSQL` + layer-record contract that
  phase 10 (engine layer + refusal contract) and phase 14 (per-engine recorded
  fixtures) integrate against (§17). Stated deliberately, not skipped.
- **Adversarial:** SQL-safety path — the standing obligations apply: the write/DDL
  corpus (criterion 2), injection probes (nested writes, stacked statements,
  comment-smuggled DDL, quote-confusion around separators), the escape corpus
  (criterion 10), and a layer-record forgery attempt (criterion 5c/6). Cross-tenant
  and fetch-then-filter members are n/a until phase 18 — noted, not dropped.
- **Fuzz:** **required** — `FuzzValidate` with seed corpus + the three invariants
  (criterion 5). Prime parse/decode surface.
- **Bench:** `BenchmarkValidate` (hot path on every plan/run); a `-race`
  concurrent-reuse test proves the validator + both parser drivers are safe under
  concurrent `Validate` (shared, immutable after construction).

## Coverage targets

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/exec` | 85% | security-critical validation core; `exec` band (convention 4) |

## Smoke checks

`scripts/smoke/phase-09.sh` SKIPs cleanly until `internal/exec` exists; all
assertions are pure `go test` targets + source-structural greps (no binary, no DB).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `go test -run TestConformanceHarness_GatesAdoption` (report golden + generated table match) |
| 2 | `go test -run TestValidate_BlocksWritesAllDialects` (adopted pairs reject typed; zero ast-passed) |
| 3 | `go test -run TestValidate_CTEGolden` (both drivers) |
| 4 | `go test -run TestTokenizer_Screens` (dialect-aware multi-statement, quote/comment hygiene, caps) |
| 5 | `go test -run '^FuzzValidate$' -fuzz='^FuzzValidate$' -fuzztime=5s` (three invariants on corpus) |
| 6 | `go test -run TestValidatedSQL_Unconstructible` + `TestLayerRecord_EnginePendingContract` |
| 7 | `go test -run TestValidate_ASTSkipTypedMarker` + `TestErrorVocabulary_Golden` |
| 8 | grep proves no SQL keyword token in tokenizer source; `go test -run TestTokenizer_NoParserJudgment` |
| 9 | `go test -run TestValidateWriteShape`; grep proves `Query(...)` takes `ValidatedSQL`, not `ValidatedWriteSQL` |
| 10 | `go test -run TestDialectEscapes_Structural` + `TestNoRegexInjectionHeuristic`; grep for `regexp` import |

## Glossary additions

Only genuinely new terms:

- **Validated SQL** — an opaque value produced *only* by the validator (RFC §9.5) and
  the sole type a data-source adapter's read `Query` accepts; unconstructible outside
  `internal/exec` (P1b, D-021). Carries the **layer record**.
- **Layer record** — the per-value record of which validation layers ran and their
  outcome (`tokenizer: passed`, `ast: passed | skipped`, `engine: pending |
  completed`); append-only, package-private to mutation. Execution refuses a value
  whose engine-side layer is pending (D-038, phase 10).
- **Validated write SQL** — the write-shape analogue for the materialization path
  (RFC §7.6): a single statement writing only to a declared destination, reading only
  declared inputs, on a **distinct** type the read path never accepts (P1c).
- **Parser seam** — the interface + factory + driver seam for client-side AST
  validation (D-038); V1 drivers `crdb` and `sqlglotgo`, adoption per dialect gated
  by the conformance-reproduction harness.
- **Conformance-reproduction harness** — the mechanical gate that adopts a
  `(dialect, parser-driver)` pair for the AST layer only after independently
  reproducing the driver's conformance against the phase-09 fixture corpus; its
  committed report generates the dialect→driver table.
- **Dialect escape** — a write-capable or side-effecting construct that can
  masquerade as a read (`SELECT … INTO`, `INTO OUTFILE`, `LOAD DATA`,
  `COPY`/`COPY INTO`, `EXEC`, `CALL`); blocked structurally in-tree where a driver is
  adopted, otherwise covered by the AST-skip marker + engine-side layers.
- **AST-skip marker (`parse.unsupported`)** — the typed marker recording that the
  client AST layer was skipped for an input (no proven driver for the dialect, or a
  construct the driver cannot represent); engine-side layers still apply — it is a
  coverage marker, never a silent pass and no longer a capability rejection.

## Decisions filed

- **References** D-038 (layered validation, parser seam, both driver pins,
  native-dialect generation), D-033 (the `crdb` pin — retained; its
  generation-subset obligation superseded by D-038), D-021 (typed vocabulary,
  `ValidatedSQL`, read/write split), D-017 (write-posture split), D-032 (dialect
  set), D-036 (Bruin as the write-path executor phase 13 pairs the write-shape
  variant with), D-037 (CGo posture; both drivers pure Go), D-024 (upload workspaces
  are Postgres — why `crdb` natively covers them), D-035 (pin-and-reverify
  discipline the harness re-run rule follows).
- **Proposal for the orchestrator** (not written to `docs/decisions.md` by this
  plan): record the **sqlglot-go packaging defect** — all published versions declare
  module path `github.com/jonathanfulton/sqlglot-go` (hyphen-less) with no repo at
  that path, so consumption requires an exact-pin `replace` directive — as an
  addendum to D-038 (or a small follow-up entry), since it materially strengthens
  D-038's fork-under-our-org endgame and any contributor touching `go.mod` will trip
  over it.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->
