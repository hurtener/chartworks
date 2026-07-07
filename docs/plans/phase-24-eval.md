# Phase 24 — `eval` (Wave 7)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-18-nlq-generation-execution, phase-19-byo-mode, phase-20-charts-spec

Authored per CLAUDE.md §16. This phase builds the quality harness the whole
build has been fixturing toward: the five golden suites, the six-category
red-team suite, the CI gate, the grounded-accuracy manual loop, and the
positive-feedback seeding path — all under `chartworks eval` and the `eval/`
package.

---

## RFC / request sections

- **RFC-001-Chartworks.md §16** — Evaluation (golden suites, red-team suite,
  accuracy benchmarking, live gate). Primary spec.
- **RFC §9.5** — the typed-error vocabulary the validation golden suite and the
  red-team schema-escape/injection categories assert against.
- **RFC §9.8** — feedback & learned examples, the source for golden-case seeding.
- **RFC §10 / D-026** — the chart-selection golden suite target (`ChartRecipe`).
- **RFC §8.3** — context engineering / complexity-tier budgets, the context
  golden suite target (token-count regression).
- **D-031** — eval strategy (this phase's charter). **D-010** — the live gate.
  **D-021** — the SQL-safety mechanism the red-team suite adversarially probes.
  **D-022** — BYO-mode parity, the adversarial-BYO red-team category.
  **D-003** — the `mock` gateway driver the CI path runs on. **D-009** — the
  preflight/coverage/drift-audit machinery this phase plugs into.
- **RFC §6.1 / D-032** — the six-engine V1 driver set and the **dockerized
  self-hostable engine** validation class (postgres/mysql/sqlserver on
  Kaggle-class public datasets); the grounded-accuracy sample warehouse runs
  against both these dockerized engines and the `.env` cloud warehouse.

## Depends on

- **phase-18 (`nlq-generation-execution`)** — the generation + validation +
  execution core the routing/generation/validation golden suites and every
  red-team category exercise. This phase cannot score generation before it
  exists.
- **phase-19 (`byo-mode`)** — `submit_sql` and the context-bundle contract the
  adversarial-BYO red-team category and the mode-parity assertions target.
- **phase-20 (`charts-spec`)** — the deterministic selector the chart-selection
  golden suite scores.
- Transitively consumes phases 05 (`gateway` `mock` + `bifrost` drivers +
  recorded fixtures), 15–17 (topic packs, context assembly, budgets), 09/10
  (validation + read-only exec), 08/14 (the `postgres`/`mysql`/`sqlserver`
  data-source adapters the dockerized grounded-accuracy engines register as —
  D-032), and 02 (`store`, for feedback-sourced seeding). All are shipped by
  Wave 7.

## Informing briefs

Per `docs/research/INDEX.md` (`eval/` → primary **03**, **12**; secondary
**08**, 07):

- `docs/research/03-predecessor-nlq-pipeline.md` §6 — the golden-dataset,
  comparison-strategy, and six-category red-team design.
- `docs/research/12-genbi-landscape.md` — BIRD/Spider-2.0 reality and the
  grounded-vs-ungrounded scoring caveat.
- `docs/research/08-datus-agent-ideas.md` §5 — the BIRD/Spider harness posture
  and the typed failure taxonomy.

## Brief findings incorporated

- **Golden-dataset shape (brief 03 §6).** Each generation golden case carries
  NLQ + expected SQL + acceptable-alternative SQLs + an expected-result hash +
  a category + a difficulty tier, with a coverage report by
  topic/category/difficulty. Adopted verbatim as the `generation` suite fixture
  schema.
- **Comparison strategy (brief 03 §6).** Normalize (case/whitespace/quotes) →
  exact match against expected or an alternative → else token-Jaccard ≥ 0.9
  scores `partial` (not `fail`); an **expected-result-hash match overrides a
  SQL-text mismatch** ("right answer" outranks "same text"). Adopted as the
  generation comparator.
- **Red-team catalog (brief 03 §6 + brief 08 §5).** A fixed catalog of
  adversarial NLQs; the runner checks **guardrail-blocking first**, then inspects
  for category-specific safety signals only if the guardrail passed. Adopted as
  the red-team runner contract.
- **Gating (brief 03 §6).** Both suites gate CI at a **0.85** pass-rate + any
  **critical** red-team failure; diff against a baseline. Adopted (RFC §16
  pins the 0.85 + zero-criticals gate).
- **Feedback-seeded goldens (brief 03 §6, §5).** Golden cases seed directly
  from `correct`-verdict feedback events, closing the loop without hand-authoring
  every case. Adopted as `chartworks eval seed`.
- **BIRD/Spider discipline, not the number (brief 12).** Borrow the
  *discipline* — real, messy, production-shaped schemas; multi-dialect coverage;
  **execution-result comparison, not SQL-text similarity** — but score
  topic-pack-grounded generation as **its own category**, never a raw
  BIRD-comparable number (grounded vs. ungrounded are different problems).
- **Golden fixtures are a maintained versioned asset (brief 12).** The
  annotation-error lesson (BIRD/Spider both carry mislabeled gold SQL) means
  seeded cases land as `candidate` for review — never auto-accepted into the
  gated set — under the same review rigor CLAUDE.md §11 requires of golden tests.
- **Typed failure taxonomy (brief 08 §5).** Passed / No-SQL-or-Empty / Failed,
  with Failed split into Table-Mismatch vs. Table-Matched-Result-Mismatch
  (Row-Count vs. Column-Value). Adopted as the eval verdict vocabulary, mapped to
  P4's "typed failure classes, never undifferentiated fail."

## Findings I'm departing from

- **Brief 03 §6 is design-fidelity only — I am building what they only
  designed.** The brief flags its own primary source (`evaluation-quality.md`) as
  "a **design document with example code**, not confirmed shipped
  infrastructure." So the *design* is inherited, but nothing is treated as a
  proven in-repo capability to port; every suite, comparator, and gate here is
  implemented and tested fresh against Chartworks' own types, not adapted from a
  predecessor artifact.
- **BIRD/Spider consumed as-is (brief 08 §5) — declined for the CI gate.** Brief
  08 suggests consuming BIRD/Spider datasets directly (33.4 GB, licensing).
  Chartworks' CI path stays fixture-owned and hermetic; the public benchmarks
  inform the *categories* of the manual grounded-accuracy loop only, and never
  gate CI. This also honors brief 12's "don't market a BIRD-comparable number"
  caveat.
- **Per-task tool-call / node-type traces (brief 08 §5) — deferred.** Capturing
  full execution traces per eval case ("took three extra turns for the same
  answer") is a V2 observability nicety; V1 records the typed verdict + stage
  timings, not a per-node trace. Noted, not built.
- **Privilege-escalation as a distinct red-team category (brief 03 §6).** Folded
  into **cross-tenant** and **adversarial-BYO** rather than carried as its own
  category, so the six categories match RFC §16 + the Chartworks access model
  (D-020): injection, schema-escape, cross-tenant, resource-exhaustion,
  semantic-confusion, adversarial-BYO.

## Scope

Delivers the `eval/` package and the real `chartworks eval` command family
(the `eval` subcommand was a stub from phase 01, D-008):

1. **The runner** — `chartworks eval` (in-process `run(args, stdout, stderr)
   int` dispatch, D-008) with subcommands:
   - `chartworks eval golden` — run the five golden suites on the mock/fixture path.
   - `chartworks eval redteam` — run the six-category red-team suite.
   - `chartworks eval gate` — run golden + red-team and apply the CI gate
     (pass-rate ≥ threshold + zero criticals); the CI entry point.
   - `chartworks eval accuracy` — the grounded-accuracy harness (live-gated).
   - `chartworks eval seed` — derive candidate golden cases from positive feedback.
2. **The five golden suites** (fixtures + scorers):
   - `routing` — question → expected topic/decision (route | clarify | no_route).
   - `generation` — question + pinned pack → expected SQL, with the
     normalized + acceptable-alternatives + result-hash comparator.
   - `validation` — the typed-error corpus **including the standing golden CTE
     fixture** (a WITH-clause SELECT that must pass, guarding the phase-09
     CTE-regression rule).
   - `charts` — result shape → expected `ChartRecipe`.
   - `context` — pack + complexity tier → budgeted context (token-count
     regression).
3. **The red-team suite** — six categories (injection, schema-escape,
   cross-tenant, resource-exhaustion, semantic-confusion, adversarial-BYO), each
   ≥ `eval.redteam_min_cases`, guardrail-blocked-first.
4. **The typed failure taxonomy** in eval output (Passed / No-SQL-or-Empty /
   Failed{TableMismatch | ResultMismatch{RowCount | ColumnValue}}).
5. **The grounded-accuracy harness** — BIRD/Spider-2.0-informed categories
   against the **sample warehouse**, scored as *grounded* generation, in two
   run modes (D-032): **dockerized** against the self-hostable engines
   (postgres/mysql/sqlserver) seeded with Kaggle-class **public** datasets —
   credential-free, repeatable, locally runnable — and **live** against the
   `.env` cloud warehouse (BigQuery/Snowflake/Databricks) for cloud-dialect
   categories (D-010). Both need real model calls, so neither is CI-required.
6. **Golden-case seeding** — `correct`-verdict feedback (RFC §9.8) → `candidate`
   golden generation cases in a review queue.
7. The `eval/` fixture tree, the coverage-band entry, and the smoke script.

### Storage layout (under `eval/`)

```text
eval/
├── runner.go / gate.go / compare.go / taxonomy.go / seed.go   # the harness
├── golden/
│   ├── routing/*.json
│   ├── generation/*.json
│   ├── validation/*.json          # includes validation/cte.json (must pass)
│   ├── charts/*.json
│   └── context/*.json
├── redteam/
│   ├── injection/*.json
│   ├── schema-escape/*.json
│   ├── cross-tenant/*.json
│   ├── resource-exhaustion/*.json
│   ├── semantic-confusion/*.json
│   └── adversarial-byo/*.json
├── accuracy/
│   ├── warehouse/
│   │   ├── postgres/              # Kaggle-class public-dataset seed: DDL + load
│   │   ├── mysql/                 #   + SOURCE.md (dataset origin + license),
│   │   └── sqlserver/             #   per dockerized self-hostable engine (D-032)
│   └── cases/*.json               # BIRD/Spider-informed grounded cases,
│                                  #   each tagged {engines[], dialect}
└── testdata/
    └── negative-control/*.json    # the self-test regression fixture(s)
```

Fixture formats (JSON, stdlib `encoding/json`):

- **routing case:** `{id, question, tenant, expected_topic, expected_decision,
  min_confidence?}`.
- **generation case:** `{id, question, topic_pack, pack_version, dialect,
  expected_sql, acceptable_alternatives[], expected_result_hash?, category,
  difficulty}`.
- **validation case:** `{id, sql, dialect, expect: "pass"|"reject",
  expected_error_code?}` (the CTE fixture is `expect:"pass"`).
- **chart case:** `{id, column_metadata[], intent?, expected_recipe_kind,
  expected_bindings}`.
- **context case:** `{id, topic_pack, complexity_tier, expected_max_tokens}`.
- **red-team case:** `{id, category, input, mode: "internal"|"byo",
  expected_behavior: "blocked", expected_error_code?}`; a case is **critical**
  when its guardrail fails to block.
- **accuracy case:** `{id, question, topic_pack, engines[], dialect,
  expected_result_hash, category, difficulty}`; `engines` routes the case to a
  dockerized self-hostable engine (postgres/mysql/sqlserver) or, for a
  cloud-only dialect, to the live warehouse (D-032).

## Non-goals

- **No chart rendering / renderer types** — the `charts` suite scores the
  declarative `ChartRecipe` only (D-026 / D-013).
- **No LLM-judge scoring** — comparators are deterministic (normalized text,
  result hash, token count, error code); no model call scores an eval case
  (a model judge would violate P5's schema-constrained-only rule and is not V1).
- **No BIRD/Spider dataset ingestion into CI** — the public benchmarks inform
  the manual accuracy categories only; no 33 GB download, no CI dependency.
- **No new eval surface on HTTP/MCP** — eval is an operator CLI + package, not a
  tool or endpoint (no P7 parity obligation).
- **No per-node execution traces** (deferred, see departures).
- **No baseline-diff dashboard** — the gate compares against the pinned
  threshold; a main-branch baseline diff is a later nicety, not V1.

## Design

**Data flow.** `chartworks eval gate` loads the golden fixtures, drives the
real in-process pipeline (routing → context → generation → validation →
execution → chart selection) wired to the **`mock` gateway driver** with
recorded per-role fixtures (D-003, the one sanctioned boundary mock), against a
fresh Docker Postgres store + upload-workspace (convention 8 fresh-DB harness).
Each case yields a typed **verdict**; the suite aggregates a pass-rate; the gate
applies `pass_rate ≥ eval.pass_threshold` **and** `criticals == 0`, exiting
non-zero on either miss and naming the tripped gate (P4: fail loud, typed).

**The generation comparator (`compare.go`).** A pure function
`Compare(expected, alternatives []string, resultHash string, got string,
gotHash string) Verdict`:

1. normalize (lowercase keywords, collapse whitespace, strip quote style) both
   sides;
2. exact match against `expected` or any `alternative` ⇒ `Passed`;
3. else if `resultHash != "" && gotHash == resultHash` ⇒ `Passed` (result-hash
   override — "right answer" outranks "same text");
4. else token-Jaccard ≥ 0.9 ⇒ `Failed{ResultMismatch}` surfaced as `partial`
   (counts against pass-rate but is not a hard fail class);
5. else the typed `Failed{TableMismatch | ResultMismatch{RowCount |
   ColumnValue}}` class from comparing executed result shapes.

Pure and deterministic ⇒ table-driven golden-testable and property-testable
(same input ⇒ same verdict).

**The red-team runner (`redteam.go`).** For each case: run the full pipeline
(mode `internal` or `byo`), assert the **guardrail blocks first** (a typed
rejection from validation/exec/access, never a silent pass); only if it blocked
does the category-specific check run (e.g. schema-escape asserts the rejection
names a schema-allowlist error code). A case that reaches execution unblocked is
a **critical**. A structural test asserts all six category directories exist and
each holds ≥ `eval.redteam_min_cases` cases — a category under the floor is a
build failure, not a silent gap.

**The typed failure taxonomy (`taxonomy.go`).** `Verdict` is a closed enum
mirroring brief 08: `Passed`, `NoSQLOrEmpty`, `Failed` with a `FailClass`
(`TableMismatch`, `ResultMismatchRowCount`, `ResultMismatchColumnValue`). Every
non-`Passed` case in the report carries a class; the reporter refuses to emit an
untyped "fail" (a compile-time-closed enum + a test that no case reports the
zero value). This is P4 at the eval layer.

**The grounded-accuracy harness (`accuracy.go`).** Reads
`eval/accuracy/cases/*.json`, routes each case by its `engines`/`dialect` tag,
runs the question through the real pipeline with **real provider models via the
`bifrost` driver** (`.env`), and scores **execution-result** correctness as
*grounded* generation. Two run modes (D-032):

- **Dockerized** — for cases tagged `postgres`/`mysql`/`sqlserver`, seeds a
  **dockerized self-hostable engine** from the Kaggle-class public dataset in
  `eval/accuracy/warehouse/<engine>/` (DDL + load + a `SOURCE.md` recording
  dataset origin + license; never confidential client data — the predecessor
  confidentiality rule). No cloud-warehouse credentials, so the run is
  **repeatable and locally runnable** with only a model `.env`; the engines come
  up via docker-compose (the `make pg-up` sibling instances) and register as
  `postgres`/`mysql`/`sqlserver` **data-source adapters** (D-004 boundary — never
  the `store` seam).
- **Live** — for cloud-dialect cases (BigQuery/Snowflake/Databricks), routes to
  `eval.live_warehouse_dsn` (`.env`, D-010).

Both modes `t.Skip` (and the CLI reports SKIPPED) when their prerequisite (the
docker engines + model `.env`, or the live DSN) is unset — **never CI-required**
(real model calls), always `-count=1`. Output is explicitly labeled grounded and
never emitted as a BIRD-comparable number (brief 12 caveat).

**Golden-case seeding (`seed.go`).** `chartworks eval seed` reads `correct`-verdict
learned examples (RFC §9.8) from the store (scope-parameterized — P1/P3, no
unscoped read), emits candidate `generation` cases (question, SQL,
result-hash, inferred category/difficulty) into `eval/golden/generation/` with
status `candidate` and never marks them `active`/gated — a human review flips
them (brief 12's maintained-asset discipline). The seed reader is content-free
in logs (no data rows — §7).

**Seam fit.** eval is a pure *consumer* of shipped seams — it opens no new seam.
It uses `gateway`'s `mock` driver (CI) and `bifrost` (live), the `store` seam
(fresh Docker Postgres), and the `nlq`/`exec`/`semantics`/`charts` public
surfaces. It touches no provider SDK (P5). Its store reads are
scope-parameterized (P1/P3). The sample warehouse is customer-data territory
reached through the `postgres` **data-source adapter**, never the `store` seam
(D-004 boundary). Vocabulary stays domain-only (P6): "grounded accuracy",
"golden case", "red-team", not "index"/"embedding"/"repair".

## Config keys added

Under an `eval:` block (RFC §14 config surface; documented here + landed in the
§14 example config and validated fail-loud at boot in the implementation PR):

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `eval.golden_dir` | string | `eval/golden` | no | Root of the five golden suites. |
| `eval.redteam_dir` | string | `eval/redteam` | no | Root of the six red-team categories. |
| `eval.accuracy_dir` | string | `eval/accuracy` | no | Grounded-accuracy cases + sample-warehouse seed. |
| `eval.pass_threshold` | float | `0.85` | no | CI gate pass-rate floor; must be in `(0, 1]` or boot refuses (typed error). |
| `eval.redteam_min_cases` | int | `5` | no | Minimum cases per red-team category; `< 1` refuses boot. |
| `eval.accuracy_engines` | list<string> | `[postgres, mysql, sqlserver]` | no | Self-hostable engines the **dockerized** grounded loop seeds + runs against (D-032); credential-free, repeatable. Members must be in the D-032 self-hostable set. |
| `eval.live_warehouse_dsn` | string (env-indirected) | — | only for cloud accuracy | `env:CHARTWORKS_EVAL_WAREHOUSE_DSN`; the **cloud** warehouse (BigQuery/Snowflake/Databricks) for live-gated dialect categories. A secret, never logged (§7). Unset ⇒ cloud accuracy categories SKIP. |

## Acceptance criteria

1. **Mock-path gate green.** `chartworks eval gate` runs the five golden suites
   + the red-team suite on the mock/fixture path against a fresh Docker Postgres
   and exits `0` with `pass_rate ≥ eval.pass_threshold` (0.85) and zero
   criticals.
2. **Self-test — a seeded regression trips the gate.** A negative-control
   fixture in `eval/testdata/negative-control/` drives pass-rate below threshold
   (or injects a critical); the gate then exits **non-zero**, naming the tripped
   gate — proving the gate can fail, not just pass.
3. **Five golden suites present + the CTE fixture passes.** All five suites
   (routing, generation, validation, charts, context) load and execute; the
   standing golden CTE fixture (`validation/cte.json`) scores `pass` (the
   phase-09 CTE-regression guard).
4. **Generation comparator.** Table-driven proof of each branch: normalized-exact,
   acceptable-alternative match, **result-hash override of a text mismatch**, and
   token-Jaccard `partial` — the comparator is pure (same input ⇒ same verdict).
5. **Red-team six categories, ≥ N each, block-first.** All six categories exist,
   each with ≥ `eval.redteam_min_cases` (5) cases; a category under the floor
   fails a structural test; every case asserts a guardrail blocked before any
   category-specific check, and any case reaching execution unblocked is reported
   `critical`.
6. **Typed failure taxonomy.** Every non-`Passed` case carries a typed
   `FailClass` (Passed / No-SQL-or-Empty / Failed{TableMismatch |
   ResultMismatch{RowCount | ColumnValue}}); a report with an untyped fail is a
   test failure.
7. **Grounded-accuracy harness — dockerized + live (D-032).** `chartworks eval
   accuracy` scores grounded generation in two modes: (a) **dockerized** against
   the self-hostable engines in `eval.accuracy_engines` (postgres/mysql/sqlserver)
   seeded from Kaggle-class public datasets — credential-free, repeatable, needing
   only a model `.env` (no cloud account), so it runs locally; (b) **live**
   against `eval.live_warehouse_dsn` for cloud-dialect categories, `-count=1`.
   Each mode cleanly SKIPs (never fails) when its prerequisite is unset; neither
   is CI-required; output is labeled grounded, never a BIRD-comparable number.
8. **Golden-case seeding.** `chartworks eval seed` derives candidate `generation`
   golden cases from `correct`-verdict feedback events (scope-parameterized read)
   and writes them with status `candidate`, never auto-promoting them into the
   gated set.
9. **Config fail-loud.** The `eval` block validates at boot: an unknown key is
   rejected, `pass_threshold` outside `(0, 1]` or `redteam_min_cases < 1` refuses
   boot with a typed error.
10. **Coverage.** The `eval` package meets its 70% tooling band (`make coverage`).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven comparator tests (crit 4), taxonomy-enum tests
  (crit 6), config-validation tests (crit 9), red-team category-floor structural
  test (crit 5), fixture-schema decode tests. Golden tests on suite output
  shapes.
- **Integration:** **required** — eval consumes phases 18/19/20 and the store
  seam. `chartworks eval gate` is itself an integration test: the real
  in-process pipeline + `mock` gateway (recorded fixtures) against a real Docker
  Postgres (`make pg-up`), proving identity/scope propagation on the
  feedback-seeding read and cross-tenant isolation on the red-team cross-tenant
  category. Lives in-package (`eval/` is the wiring boundary).
- **Adversarial:** the red-team suite **is** the standing adversarial obligation
  for this phase — cross-tenant probe (through the full pipeline), empty-access
  short-circuit, injection/schema-escape/multi-statement smuggling, and BYO
  out-of-bundle submission. It re-exercises phases 09/10/18/19's obligations
  end-to-end (§11: the access + SQL-safety paths carry adversarial tests).
- **Fuzz:** n/a here — the parse/decode fuzz targets (JWT, NLQ payloads,
  SQL-validation) are standing obligations of phases 03/09/17/18, not eval.
  eval's fixture JSON decode is covered by unit decode tests, not a fuzz target.
- **Bench:** n/a — the harness is a batch tool, not a hot request-path artifact;
  the benchmarked artifacts (router, comparator on the hot path) are benched in
  their owning phases.

## Coverage targets

Per CLAUDE.md §11 (70% CLI/eval tooling band). The `eval` package is tooling,
not a request-path subsystem.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `eval` | 70% | Eval tooling band (master-plan convention 4); harness code, not a request-path subsystem. |

Added to `scripts/coverage-bands.conf` in this PR:

```text
eval   70
```

## Smoke checks

`scripts/smoke/phase-24.sh` (guard: the `eval` package builds, else SKIP whole
script; per-criterion tests SKIP cleanly until present, via `run_group`).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestGateMockPathGreen` — gate exits 0, pass-rate ≥ threshold, zero criticals on the mock path. |
| 2 | `TestSeededRegressionTripsGate` — negative-control fixture ⇒ gate exits non-zero, names the tripped gate. |
| 3 | `TestFiveGoldenSuitesPresent` + `TestValidationCTEFixturePasses` — all five suites load; CTE fixture passes. |
| 4 | `TestGenerationComparator` — normalized/alternative/result-hash/partial branches; purity. |
| 5 | `TestRedTeamSixCategoriesMinCases` — six categories, ≥5 each, block-first, criticals detected. |
| 6 | `TestFailureTaxonomyTyped` — every non-pass case carries a typed FailClass. |
| 7 | `TestAccuracyDockerizedGrounded` (SKIPs without docker engines + model `.env`) + `TestAccuracyLiveCloud` (`-count=1`; SKIPs without `eval.live_warehouse_dsn`). |
| 8 | `TestSeedFromPositiveFeedback` — feedback ⇒ `candidate` golden cases, never auto-promoted. |
| 9 | `TestEvalConfigFailLoud` — unknown key / out-of-range threshold / `min_cases < 1` refuse boot. |
| 10 | `make coverage` — `eval` package meets the 70% band (not a smoke assertion; the mechanical coverage gate). |
| CLI surface | `chartworks eval -h` lists `golden`/`redteam`/`gate`/`accuracy`/`seed` (binary present only). |

## Glossary additions

Pre-written for `docs/glossary.md` (landed same PR, P6 domain terms only):

- **Golden suite** — a versioned fixture collection scoring one pipeline stage
  (routing / generation / validation / charts / context) against expected output
  on the mock/fixture path; CI-gated at the pass-rate threshold (RFC §16, D-031).
- **Red-team suite** — the adversarial fixture catalog across six safety
  categories (injection, schema-escape, cross-tenant, resource-exhaustion,
  semantic-confusion, adversarial-BYO); a case is **critical** when its guardrail
  fails to block. CI-gated at zero criticals (RFC §16).
- **Eval verdict / failure taxonomy** — the typed per-case outcome: `Passed` /
  `No-SQL-or-Empty` / `Failed{TableMismatch | ResultMismatch{RowCount |
  ColumnValue}}`; an untyped "fail" is forbidden (P4; brief 08 §5).
- **Grounded-accuracy harness** — the BIRD/Spider-2.0-informed benchmark scoring
  topic-pack-**grounded** generation as its own category, in two run modes
  (dockerized self-hostable engines on public data; live cloud warehouse); never
  emitted as a BIRD-comparable number (RFC §16, D-010, D-032; brief 12).
- **Sample warehouse** — the seeded, non-confidential, production-*shaped*
  structured dataset the grounded-accuracy harness queries: either a **dockerized**
  self-hostable engine (postgres/mysql/sqlserver on a Kaggle-class public dataset,
  credential-free) or the **live** cloud warehouse via `.env` (D-032). Reached
  through data-source adapters, never the `store` seam (D-004).
- **Golden-case seeding** — deriving candidate `generation` golden cases from
  `correct`-verdict feedback (learned examples, RFC §9.8), landed as `candidate`
  for review, never auto-promoted into the gated set (brief 12's maintained-asset
  discipline).

## Decisions filed

No new `D-NNN` — this phase implements existing decisions:

- **D-031** (its charter), **D-010** (the live gate), **D-021** (the SQL-safety
  gates the red-team suite probes), **D-022** (BYO parity / adversarial-BYO),
  **D-026** (chart-recipe suite target), **D-003** (the `mock` gateway CI path),
  **D-032** (the dockerized self-hostable engines the grounded-accuracy harness
  seeds + runs against, credential-free), **D-009**
  (preflight/coverage/drift-audit machinery).

A change to the CI pass threshold (0.85) or the red-team category set is a
superseding decision entry (RFC PR), never a silent edit (§15).

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): every reasonable deviation
     from this plan, why, and confirmation this file was updated in the same PR. -->

_none yet — populated during implementation._
