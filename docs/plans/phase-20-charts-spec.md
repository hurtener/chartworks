# Phase 20 — `charts-spec`

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-10-exec-read, phase-15-topics-lifecycle

---

## RFC / request sections

- **RFC-001 §10** (Charts — the declarative-contract-only, deterministic-selector
  design; this phase is the whole of §10). **D-026** (charts V1 = declarative spec +
  rules selector; no renderer, no LLM ranker).
- Supporting: **§9.6** (`ResultPreview` + per-column metadata that the derivation
  consumes), **§9.7** (the `run_query` answer envelope that carries the chart spec),
  **§8.3** (the capability contract that supplies measure/dimension/KPI definitions),
  **§7.2** (dataset profiles: bucketed cardinality, format hints, temporal grain).
- Binding properties in play: **P4** (the zero-candidate `table` fallback is a
  *rendering* degradation with provenance, never an access/validation fallback);
  **P5** (no ungated model call — the V1 selector makes none; any future ranker is
  gateway-schema-constrained, not free-text JSON); **P7** (one selector, consumed by
  every surface through the §9.7 envelope).

## Depends on

- **phase-10 `exec-read`** — supplies the executed result *shape*: ordered result
  columns (name, data type, query role) via `ResultPreview`. Charts derives
  `ColumnMetadata` from this shape; it never re-queries.
- **phase-15 `topics-lifecycle`** — supplies the semantic definitions (measures,
  dimensions, derived KPIs, temporal grains, format hints, `source_ref` targets) that
  enrich the raw shape into `ColumnMetadata`.

Both dependencies are consumed as **already-in-hand data** at derivation time (the row
set is executed; the routed pack is resolved). `internal/charts` imports neither
package's runtime machinery — the caller (phase-18 `run_query`) maps their outputs into
this package's narrow input DTOs. This keeps the selector pure and I/O-free (the §10
mandate) and is why phase 20 is slippable to Wave 6: nothing downstream of it exists
until the surfaces (21–22) wire it.

## Informing briefs

- **Brief 06** (`docs/research/06-predecessor-frontend-charts.md`) — the backbone: the
  selection pipeline mechanics (§1), the chart-spec contract shape (§2), and the
  recommended V1 declarative-only contract (§5).
- Secondary: **Brief 07** (`docs/research/07-wrenai-ideas.md`) — selective column
  exposure, echoed here as "derive metadata only from what the query already returned."

## Brief findings incorporated

From **brief 06**, ported structurally (not by copying — D-001):

1. **`ColumnMetadata` derived from the executed shape + pack definitions with zero extra
   I/O** (§2 "wire path": `PresentationResolver` builds column metadata from the executed
   shape + topic-pack definitions, no extra I/O). Fields carried: `name`,
   `display_name`, `source_role`, `data_type`, `semantic_type`, `temporal_grain`,
   `aggregation`, `format_hint`, `query_role`, **`cardinality_bucket`** (low/medium/high/
   unknown — *bucketed*, sampled from the executed rows, never a raw distinct count, §5),
   and **`source_ref`** (resolved origin — measure/dimension/dataset-column/derived/
   literal id+grain — the field that makes future rebind-on-drift possible, §2/§5).
2. **The 14-kind catalog** (§1.1): `table`, `kpi_card`, `bar`, `column`, `line`, `area`,
   `pie`, `donut`, `scatter`, `stacked_bar`, `stacked_column`, `grouped_bar`, `heatmap`,
   `treemap` — each a definition declaring ordered named **slots** with required/optional
   flags, a suitability rule, and a rationale source.
3. **Slot binding** (§1.2): a small role vocabulary (`measure`, `dimension`, `temporal`,
   `geo`, plus the universal `any` for `table`) derived from `source_role`/`data_type`/
   `semantic_type`; first-fit in declaration order; an unfilled **required** slot
   early-exits the kind; the **required-slot-reclaim tiebreaker** — a required slot may
   reclaim a column an *optional* slot already took, **never one another required slot
   took** — specifically to kill the degenerate scatter `x == y` binding when only one
   measure exists.
4. **Weighted suitability in one tunable file** (§1.3): base constants (bar/column/line
   25, table 10, treemap 12), `(label, delta)` events for ideal cardinality buckets
   (`low ≤ 12`, `medium ≤ 50`, penalty above), multi-measure bonuses, and
   **question-intent keyword matching** (temporal/comparison/composition/correlation
   keyword sets biasing line/bar/pie/scatter) — all weights and keyword sets in one file
   so "one PR rebalances everything."
5. **The adaptive-alternatives reducer** (§1.4): sort by score; dedupe on
   `(kind, slot signature)`; relative score floor (40% of primary, or 0 if primary <
   0.1); cap 2 per kind; force ≥ 1 non-primary kind when variety-trimming left a single
   kind; cap total alternatives at 4.
6. **The `table` fallback** (§1.5): zero viable candidates ⇒ a synthetic `table` recipe
   at score 0.1. **Never raises.**
7. **The declarative-only V1 contract** (§5): `ColumnMetadata[]` + `ChartRecipe`
   (`kind`, `title`, `column_bindings`, `formatting`, `score`, `rationale`) +
   `ResultPresentation` (`column_metadata`, `primary_recipe`, `alternative_recipes`,
   `generator_version`, `picker`). `picker` provenance is `rules | rules_fallback` in V1.
8. **Rationale from the top-1/2 scoring events** (§4 keeper) — a legible one-liner
   ("Bar chosen: ideal cardinality, comparison intent."), not a raw score.

## Findings I'm departing from

- **`prepared_options` / ECharts option inflation (brief 06 §2).** Explicitly **omitted**
  (D-026, brief 06 §5): V1 is headless, so inflating a charting library's option dict in
  Go would do rendering work nobody consumes, couple the core to ECharts' JSON shape, and
  violate "outputs are normalized shapes" (CLAUDE.md §6). No `prepared_options`,
  `options_extra`, or `binding_origins` field exists in the V1 contract (an architecture
  test forbids them — criterion 9).
- **The LLM ranker (brief 06 §1 "reorders, never invents").** **Deferred** (D-026): V1
  makes no model call. When a ranker lands post-V1 it goes through the gateway
  schema-constrained (P5) — the predecessors' **free-text JSON parse** of the ranker
  completion (brief 06 §1/§4 scar) is the named anti-pattern this phase leaves behind, so
  the `picker` enum is `rules | rules_fallback` only (no `llm`) in V1.
- **`rebind` / `rebound*` provenance and `ChartRecipeRecord` persistence (brief 06 §1/§2).**
  **Deferred** — no saved-chart feature exists in V1. The contract already carries
  `source_ref`-qualified column metadata, so persistence and rebind-by-meaning are
  *additive later*, not a redesign (brief 06 §5). `picker` omits `user`/`rebound*`.
- **The live P6 "repair"/"broken" surface (brief 06 §4 scar).** Not adopted at all — this
  package emits no user-facing health copy; source-health language is owned by
  phase-15's domain-clean `re-check source` / "unavailable" vocabulary.

## Scope

Delivers **`internal/charts`** — a pure, deterministic, I/O-free package:

- **Input DTOs** (`internal/charts` owns them, so the package depends on no other
  subsystem's runtime): `ExecutedColumn` (name, data type, query role — mapped from
  phase-10 `ResultPreview`), `SampledColumnStats` (bucketed cardinality + sample-derived
  hints from the executed rows), and `PackColumnDef` (semantic type, grain, aggregation,
  format hint, `source_ref` target — mapped from phase-15's capability contract).
- **`DeriveColumnMetadata(cols []ExecutedColumn, stats map[...]SampledColumnStats, pack
  []PackColumnDef) []ColumnMetadata`** — the pure enrichment step (no extra I/O).
- **The catalog** — 14 `ChartDefinition`s, each with ordered `Slot`s (name, accepted
  role, required flag), a suitability rule, and rationale sources.
- **`BindSlots`** — first-fit binding with the required-slot-reclaim tiebreaker.
- **`weights.go`** — the single tunable file: base constants, cardinality-bucket deltas,
  and question-intent keyword sets.
- **`Score`** — weighted suitability producing `(score, []SuitabilityEvent)`.
- **`Select(question string, cols []ColumnMetadata) ResultPresentation`** — the top-level
  entry point: derive candidates → score → adaptive-alternatives reducer → envelope, with
  the `table` fallback at the floor.
- **The contract types** — `ColumnMetadata`, `SourceRef`, `ChartRecipe`,
  `ResultPresentation`, `ChartKind`, `Picker`.

## Non-goals

- **No rendering, no `prepared_options`, no ECharts (or any charting-library) types** —
  V1 emits the declarative recipe only (D-026, brief 06 §5).
- **No LLM ranker / no model call / no gateway import** (D-026, P5) — the selector is
  pure Go.
- **No saved-chart persistence, no `rebind`, no `ChartRecipeRecord`** — deferred; the
  `source_ref` field keeps them additive.
- **No profiling / no distinct-count query** — `cardinality_bucket` is bucketed from the
  already-executed rows the caller hands in; charts issues no query (P1 has nothing to
  bind here because charts touches no data source — it sees only the returned preview).
- **No wiring into surfaces** — phase-18 `run_query` and phase-21/22 place the envelope
  on the §9.7 answer; the eval chart-selection golden suite is phase-24. This phase ships
  the selector and its own goldens only.

## Design

### Data flow (all in-process, zero I/O)

```
phase-10 ResultPreview ─┐
                        ├─► DeriveColumnMetadata ─► []ColumnMetadata ─► Select ─► ResultPresentation
phase-15 pack contract ─┘        (pure enrich)          (question)     (rules)      (§9.7 envelope)
```

The caller (phase 18) has both inputs in hand the instant a query returns: the executed
row shape (already fetched) and the routed capability contract (already resolved). It
maps them into this package's DTOs and calls `DeriveColumnMetadata` then `Select`. **No
step reads a store, a warehouse, a file, or the gateway** — that purity is the §10
mandate and criterion 9's architecture test enforces it (no `net/http`, `database/sql`,
`os` file-read, or `internal/gateway` import; no renderer identifiers).

### `ColumnMetadata` derivation (from executed shape + pack definitions)

For each executed result column, `DeriveColumnMetadata` composes:

| Field | Source | Notes |
| --- | --- | --- |
| `name` | executed shape | the result alias |
| `display_name` | pack def, else `name` | label resolved before return (§9.7: never a raw id) |
| `source_role` | pack def | `measure` / `dimension` / `derived` / `literal` / `unknown` |
| `data_type` | executed shape | dialect-agnostic category (`TypeCategory`, §7.1) |
| `semantic_type` | pack def | `currency` / `percent` / `id` / `category` / `temporal` / `geo` / … |
| `temporal_grain` | pack def | for temporal columns |
| `aggregation` | pack def | measure aggregation, if any |
| `format_hint` | pack def / profile | decimals, unit, currency, locale, date format |
| `query_role` | executed shape | `group_by` / `aggregated` / `filter_echo` / `ordered_by` / `other` |
| `cardinality_bucket` | **sampled from the returned rows** | `low` / `medium` / `high` / `unknown` — bucketed, **never a raw distinct count** (brief 06 §5) |
| `source_ref` | pack def | resolved origin (`SourceRef{Kind, ID, Grain}`) enabling future rebind-by-meaning |

`cardinality_bucket` is computed over the already-executed preview rows only — a bounded
in-process count over the capped `ResultPreview`, not a distinct-count query. Where the
preview is too small to be meaningful the bucket is `unknown` (the honest value, P4 — no
guessed bucket).

### The 14-kind catalog + slot contracts

A `ChartDefinition{Kind, Slots []Slot, Suitability, RationaleSources}` per kind. A `Slot`
is `{Name, AcceptedRole (measure|dimension|temporal|geo|any), Required bool}`. Indicative
slot contracts (exact per-kind lists pinned in the catalog and golden-tested):

| Kind | Required slots | Optional slots |
| --- | --- | --- |
| `table` | — (universal `any`) | all columns |
| `kpi_card` | `value` (measure) | `label` |
| `bar` / `column` | `category_axis` (dimension), `value_axis` (measure) | `series` (dimension) |
| `line` / `area` | `x_axis` (temporal\|dimension), `value_axis` (measure) | `series` |
| `pie` / `donut` | `category` (dimension), `value` (measure) | — |
| `scatter` | `x` (measure), `y` (measure) | `size`, `series` |
| `stacked_bar` / `stacked_column` / `grouped_bar` | `category_axis` (dimension), `value_axis` (measure), `series` (dimension) | — |
| `heatmap` | `x` (dimension), `y` (dimension), `value` (measure) | — |
| `treemap` | `category` (dimension), `value` (measure) | `parent` (dimension) |

The catalog completeness test asserts exactly these 14 kinds, each with a non-empty slot
list and a suitability rule (criterion 2).

### Slot binding + the required-slot-reclaim tiebreaker

`BindSlots(def, cols)` fills slots **first-fit in declaration order** from columns whose
derived role matches `AcceptedRole` (`any` matches all). An unfilled **required** slot
early-exits the kind (it produces no candidate). The **tiebreaker**: when a required slot
finds no free matching column, it may **reclaim** a column currently held by an *optional*
slot of the same kind — **never a column held by another required slot**. This is the
scatter guard: with a single measure, `scatter` needs `x` and `y` both measures; rather
than binding `x == y`, `y` (required) cannot reclaim from `x` (required), so `scatter`
early-exits — exactly the degenerate binding brief 06 §1.2 calls out. Table-driven test
covers: normal fill, required early-exit, reclaim-from-optional succeeds, reclaim-from-
required refused (criterion 3).

### Weighted suitability (one tunable file)

`Score(def, boundCols, question)` returns `(float64, []SuitabilityEvent{Label, Delta})`.
All tunables live in **`weights.go`** and nowhere else:

- **Base constants** per kind (bar/column/line 25, table 10, treemap 12, …).
- **Cardinality-bucket events** — bonus for the kind's ideal bucket (`low ≤ 12`,
  `medium ≤ 50`), penalty above 50; multi-measure bonus where the kind benefits.
- **Question-intent keyword events** — temporal / comparison / composition / correlation
  keyword sets bias line / bar / pie / scatter respectively (lexicon-light, D-028 — no
  NLP library). Keyword matching is lowercase-substring over the question; the keyword
  sets are `weights.go` data.

Scores are normalized to 0–1 for the recipe `score`. A single test asserts scores are
exactly reconstructable from the emitted events, and an architecture assertion checks
that the base constants and keyword sets are declared only in `weights.go` (criterion 4).

### Adaptive-alternatives reducer

`adaptiveAlternatives(scored []scoredCandidate)`:

1. Sort by score descending (stable — deterministic tie order by kind declaration index).
2. **Dedupe** on `(kind, slot signature)` — the slot signature is the ordered bound
   column names, so two `bar`s with different bindings both survive but identical ones
   collapse.
3. **Relative score floor** — drop candidates below 40% of the primary's score; if the
   primary score < 0.1, the floor is 0 (keep what exists rather than empty out).
4. **Per-kind cap** — at most 2 candidates per kind.
5. **Variety guarantee** — if trimming left only one distinct kind, re-admit the
   highest-scoring candidate of a *different* kind (so alternatives are never all-one-kind).
6. **Total cap** — at most 4 alternatives (excluding the primary).

Table-driven test exercises each rule in isolation and combined (criterion 5).

### The `ResultPresentation` envelope + picker provenance

```
ResultPresentation{
  ColumnMetadata     []ColumnMetadata
  PrimaryRecipe      ChartRecipe
  AlternativeRecipes []ChartRecipe
  GeneratorVersion   string        // stamped for eval regression tracking (phase 24)
  Picker             Picker        // rules | rules_fallback  (V1)
}
ChartRecipe{ Kind ChartKind; Title string; ColumnBindings map[string][]string;
             Formatting map[string]FormatHint; Score float64; Rationale string }
```

**Picker semantics (V1):**

- `rules` — the rules engine produced ≥ 1 viable candidate; the primary is
  `candidates[0]`.
- `rules_fallback` — zero viable candidates; the selector degraded to the synthetic
  `table` recipe at score 0.1. This is a **rendering** degradation (P4 distinction, brief
  06 §5/§4): it is *not* an access or validation fallback and carries no error — a table
  is always a valid presentation of a result set.

`llm` / `user` / `rebound*` are reserved for post-V1 and absent from the V1 enum.

### Determinism & purity

`Select` is a pure function of `(question, []ColumnMetadata)`: no map-iteration-order
leakage (all ordering keyed on the fixed kind-declaration index and stable sorts), no
clock, no randomness, no I/O. The property test runs the same input N times and asserts
byte-identical envelopes (criterion 6); the architecture test proves the no-I/O /
no-renderer property (criterion 9); the fuzz target proves "never raises" over randomized
column shapes (criterion 7).

## Config keys added

**None.** The weights and keyword sets are a **compile-time tunable source file**
(`internal/charts/weights.go`), not runtime config — matching the predecessors' `weights.py`
posture and brief 06 §1.3 ("one PR rebalances everything"). Rebalancing is a code change
plus an eval golden-regression check (phase 24), which is stronger than a silently
mutable runtime key for a determinism-critical component. RFC §14's config surface
deliberately carries no `charts` domain.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none — weights live in `internal/charts/weights.go` (compile-time) |

## Acceptance criteria

1. **`ColumnMetadata` derivation is pure and I/O-free.** `DeriveColumnMetadata` composes
   metadata solely from the passed executed shape + pack definitions (incl. `source_ref`
   and bucketed `cardinality_bucket`); a golden test asserts the derived metadata matches
   the expected shape for the brief-06 fixture columns, and the derivation issues no query
   (no I/O import — see criterion 9).
2. **The catalog holds exactly the 14 kinds, each with a slot contract.** A completeness
   test asserts the kind enum is precisely the 14 kinds and every `ChartDefinition`
   declares a non-empty ordered slot list (with required/optional flags), a suitability
   rule, and a rationale source.
3. **Slot binding honors the required-slot-reclaim tiebreaker.** First-fit in declaration
   order; an unfilled required slot early-exits the kind; a required slot may reclaim a
   column from an *optional* slot but never from another *required* slot — proven by the
   scatter `x == y` degenerate case producing no `scatter` candidate.
4. **Suitability is weighted from one tunable file.** Score = base constant + cardinality-
   bucket events + question-intent keyword events; the score is exactly reconstructable
   from the emitted `(label, delta)` events, and the base constants + keyword sets are
   declared only in `weights.go` (architecture assertion).
5. **The adaptive-alternatives reducer applies all six rules.** Sort-by-score, dedupe on
   `(kind, slot signature)`, relative score floor (40% of primary; 0 when primary < 0.1),
   per-kind cap 2, variety guarantee (≥ 1 non-primary kind), total cap 4 — each proven by
   a table-driven case.
6. **Selection is deterministic and pure.** A property test runs the same
   `(question, columns)` input N times and gets byte-identical `ResultPresentation` (no
   map-order or randomness leakage).
7. **The `table` fallback never raises.** Zero viable candidates ⇒ a synthetic `table`
   recipe at score 0.1 with `picker="rules_fallback"`; a fuzz target over randomized
   column shapes asserts `Select` never panics and always returns a valid recipe with
   `picker ∈ {rules, rules_fallback}`.
8. **The `ResultPresentation` envelope + golden suite.** The envelope carries
   `column_metadata`, `primary_recipe`, `alternative_recipes`, `generator_version`, and
   `picker ∈ {rules, rules_fallback}`; a golden suite over the brief-06 fixture shapes
   pins primary + alternatives + picker.
9. **No renderer types anywhere (architecture test).** The package declares no
   `prepared_options` / `options_extra` / `binding_origins` field and no ECharts / charting-
   library / `setOption` identifier; it imports no `internal/gateway`, `net/http`,
   `database/sql`, or file-reading `os` API — proving no model call, no inflation, no I/O.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven for slot binding (criterion 3), the suitability scorer (4), and
  the adaptive-alternatives reducer (5); catalog completeness (2); a `-race`
  concurrent-reuse test on the selector (it is a hot reusable artifact — the same
  `Select` called from many `run_query` goroutines; it holds no per-request receiver
  state, all inputs are parameters).
- **Integration:** **n/a as a cross-subsystem Docker test** — the package does no I/O, so
  there is no seam with a real driver to close here. An **in-package integration test**
  feeds a realistic executed-shape + capability-contract fixture (real DTOs, no mocks
  needed since no boundary is crossed) through `DeriveColumnMetadata` → `Select` end to
  end. The *cross-package* wiring (exec preview + pack → charts → §9.7 envelope) is
  exercised by phase-18/21's integration tests, which own that wiring boundary (§17).
- **Golden:** the derivation golden (criterion 1) and the selection golden suite over the
  brief-06 fixture shapes (criterion 8), with `generator_version` stamped so phase-24's
  eval chart-selection suite can track regressions.
- **Adversarial:** **n/a** — `internal/charts` touches no ACL/auth/data path; it sees only
  an already-executed, already-access-scoped preview. (The access adversarial obligations
  live in the phases that own the store/exec/access seams.)
- **Fuzz:** **`FuzzSelect`** — seed corpus of column-shape combinations; invariant: never
  panics, always returns a valid recipe (`table` at minimum), `picker ∈ {rules,
  rules_fallback}` (backs criterion 7).
- **Bench:** **`BenchmarkSelect`** — the selector is on the `run_query` hot path; a
  baseline over a representative multi-column shape (not a CI gate).

## Coverage targets

Per CLAUDE.md §11 default for a new `internal/` package: **80%**. No override — the
package is pure Go with no hermetically-unreachable path (the fallback and error branches
are all exercisable in-process).

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/charts` | 80% | default new-package band |

## Smoke checks

`scripts/smoke/phase-20.sh` SKIPs entirely until `internal/charts` exists, then runs one
assertion per criterion via `run_group` (lib.bash) over `./internal/charts/...`.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestDeriveColumnMetadataGolden` passes |
| 2 | `TestCatalogFourteenKinds` passes |
| 3 | `TestBindSlotsReclaimTiebreaker` passes |
| 4 | `TestSuitabilityScoringWeights` passes |
| 5 | `TestAdaptiveAlternativesReducer` passes |
| 6 | `TestSelectDeterministic` passes |
| 7 | `TestTableFallbackNeverRaises` passes |
| 8 | `TestResultPresentationGolden` passes |
| 9 | `TestNoRendererTypes` passes |

## Glossary additions

New terms this phase introduces (landed in `docs/glossary.md` in the same PR; the umbrella
term **Chart spec** is already seeded):

- **Column metadata** — one `ColumnMetadata` per executed result column: name, display
  name, source role, data type, semantic type, temporal grain, aggregation, format hint,
  query role, *bucketed* cardinality, and a `source_ref` back to the semantic model —
  derived from the executed shape + topic-pack definitions with no extra I/O (RFC §10).
- **Chart recipe** — a scored, kind-tagged presentation candidate: `kind` (one of the 14
  catalog kinds), title, column bindings (slot → columns), formatting, score, and a
  human-facing rationale. Declarative only — never a rendered or option-inflated artifact
  (RFC §10, D-026).
- **Chart kind** — one of the 14 catalog kinds (table, kpi_card, bar, column, line, area,
  pie, donut, scatter, stacked_bar, stacked_column, grouped_bar, heatmap, treemap).
- **Slot binding** — the deterministic assignment of result columns to a kind's ordered,
  role-typed slots (first-fit; unfilled required slot early-exits; required-slot-reclaim
  tiebreaker) (RFC §10).
- **Suitability score** — a chart kind's fit for a result shape + question: a base
  constant plus weighted `(label, delta)` events (cardinality buckets, question intent),
  all from one tunable file; normalized 0–1 (RFC §10).
- **Adaptive alternatives** — the reduced set of non-primary recipes after dedupe, score
  floor, per-kind cap, variety guarantee, and total cap (RFC §10).
- **Result presentation** — the envelope carrying column metadata + primary recipe +
  alternative recipes + generator version + `picker` provenance (`rules | rules_fallback`
  in V1) (RFC §10).
- **Picker provenance** — how the primary recipe was chosen: `rules` (rules engine) or
  `rules_fallback` (degraded to `table` because zero viable candidates — a rendering
  degradation, never an access/validation one, P4). The `llm`/`user`/`rebound*` values are
  reserved for post-V1.

## Decisions filed

- **No new decision.** This phase implements **D-026** (charts V1 = declarative spec +
  deterministic rules selector; no renderer, no LLM ranker) verbatim, within the scope of
  **D-013** (charts deferred from V1 but the normalized output reserves the chart-spec
  slot), and upholds **P4** (fallback distinction) and **P5** (no ungated model call / no
  free-text JSON parse). A future LLM-ranker or saved-chart/rebind feature would each file
  their own decision entry when proposed.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): each reasonable deviation, why, and
     confirmation this file was updated in the same PR. -->

- none yet.
