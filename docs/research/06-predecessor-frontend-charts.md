# Brief 06 — Predecessor frontend & chart-selection mining

> Status: draft · 2026-07-06 · sources: both predecessors

## Summary

- Both predecessors ship an identical, mature **presentation subsystem**: rules-based
  chart-kind selection (slot-binding + weighted scorers) with an optional, default-on
  small-model **ranker** that only reorders/relabels — it never invents a kind or sees row
  values. Result: deterministic, explainable, cheap.
- The **chart-spec contract** (`ResultPresentation` / `ChartRecipe`) is exactly the
  "reserved slot" D-013 refers to: column metadata + a scored, bound, kind-tagged recipe,
  with pre-inflated ECharts options computed server-side and shipped as opaque JSON.
- Frontend is SvelteKit + ECharts; a thin `EChartsCanvas` component just calls
  `setOption()` on whatever `prepared_options` the server sent — zero client-side chart
  logic. The `ask -> result -> chart -> refine (-> feedback)` loop is fully realized.
  The generalistic predecessor is functionally identical here; its only divergence is
  routing column-type classification through a shared helper instead of inline type sets.
- **UX scar confirmed**: a live, user-facing "repair"-class surface exists (topic pages
  warn of "broken tables" and prompt the user to "Repair tables" / call a `:repair`
  endpoint) — precisely the P6 anti-pattern CLAUDE.md calls out. Also flagged: the ranker
  and the SQL generator's column-origin mapping both parse **free-text JSON** from a model
  completion — forbidden under Chartworks' P5.
- Recommendation: V1 should emit `ColumnMetadata[]` + a scored `ChartRecipe` (kind,
  bindings, formatting, rationale, score) as part of the query-result envelope, but
  **not** pre-inflate ECharts options in Go — ship the declarative recipe only, leaving
  inflation to whichever frontend or dataviz asset consumes it later.

## 1. Chart selection

Both predecessors: `presentation/selection.py`, `presentation/catalog/`,
`presentation/suitability/*.py` + `weights.py`, `presentation/llm_ranker.py`,
`presentation/rebind.py`.

**Pipeline (rules stage — pure, deterministic, no I/O):**

1. **Catalog.** 14 chart kinds registered as `ChartDefinition` dataclasses in
   `catalog/definitions.py`: `table`, `kpi_card`, `bar`, `column`, `line`, `area`, `pie`,
   `donut`, `scatter`, `stacked_bar`, `stacked_column`, `grouped_bar`, `heatmap`,
   `treemap`. Each declares ordered named **slots** (e.g. `category_axis`/`value_axis`/
   `series` for `bar`), a `suitability_fn`, a `generator_fn`, and default ECharts knobs.
2. **Slot binding** (`bind_slots`). Columns map to a small role vocabulary — `measure`,
   `dimension`, `temporal`, `geo`, plus a universal `any` fallback for `table` — derived
   from `source_role`/`data_type`/`semantic_type`. Slots fill in declaration order,
   first-fit; an unfilled `required` slot early-exits that kind. A narrow tiebreaker lets
   a required slot reclaim a column an *optional* slot already took (never one a required
   slot took) — specifically to stop degenerate bindings like scatter `x == y` when only
   one measure exists.
3. **Suitability scoring** (`weights.py`). Each viable kind gets a base constant (bar/
   column/line at 25, table at 10, treemap at 12) plus `(label, delta)` events: bonuses
   for ideal cardinality buckets (`low <=12`, `medium <=50`, penalty above 50),
   multi-measure bonuses, and **question-intent keyword matching** (temporal/comparison/
   composition/correlation keyword sets bias line/bar/pie/scatter respectively). All
   weights and keywords live in one tunable file — "one PR rebalances everything."
4. **Adaptive-alternatives reducer**: sort by score, dedupe on `(kind, slot signature)`,
   apply a relative score floor (40% of primary, or 0 if primary < 0.1), cap 2 per kind,
   force at least one non-primary kind if variety-trimming left a single kind, cap total
   alternatives at 4.
5. **Fallback:** zero viable candidates -> synthetic `table` recipe at score 0.1. Never
   raises.

**LLM ranker (`llm_ranker.py`) — reorders, never invents:**

- Default-on; its only job is picking the **primary** from the rules-produced list
  (`candidates[0]` otherwise). Cannot introduce a new kind or binding.
- Guardrails worth inheriting: prompt carries only question text + column metadata
  (names/types/roles/cardinality) + candidate descriptors — **zero row values**; every
  call has a hard single-digit-second timeout; any timeout/error/disabled-flag/parse
  failure falls back to `candidates[0]` (`picker="rules_fallback"`); a deterministic
  SHA-256 cache key (question + candidate + column signatures) backs an in-memory LRU;
  a single candidate skips the LLM entirely (`picker="rules"`).
- **Anti-pattern to leave behind**: it parses the completion as *free-text JSON*
  (`_parse_lm_output` strips code fences, `json.loads`s, manually type-checks the
  result). Chartworks' P5 forbids this — the call must be schema-constrained through the
  gateway seam instead.
- The `picker` provenance (`rules | llm | user | rules_fallback | rebound | rebound_llm`)
  rides to the frontend, which renders an inline disclosure banner — worth keeping.

**Rebind (saved-chart re-execution, `rebind.py`):** when a saved recipe's SQL now emits
different aliases, a best-effort rebind matches each slot's saved `SourceRef` (measure/
dimension id+grain, or dataset column name) against the live `ColumnMetadata`. All
required slots rebind -> `picker="rebound"`; otherwise the caller regenerates candidates
seeded with the saved recipe (`picker="rebound_llm"`) — this is what lets a saved chart
survive schema drift without going stale.

**Only material divergence between predecessors:** the generalistic predecessor routes
type classification through a shared `column_types.classify_type`/`TypeCategory` enum
instead of the client predecessor's inline numeric/temporal type-name sets. Same
behavior, cleaner seam — worth adopting if Chartworks builds an equivalent.

## 2. Chart-spec shape

Defined once in `presentation/contracts.py`; mirrored as TypeScript in
`frontend/src/lib/types/presentation.ts` ("mirroring `.../presentation/contracts.py`").

**Column-level (`ColumnMetadata`, one per executed result column):** `name`,
`display_name`, `source_role` (measure/dimension/derived/literal/unknown), `data_type`,
`semantic_type` (currency/percent/id/category/temporal/geo/...), `temporal_grain`,
`aggregation`, `format_hint` (decimals, unit, currency code/symbol, locale, date format),
`query_role` (group_by/aggregated/filter_echo/ordered_by/other), `cardinality_bucket`
(low/medium/high/unknown, sampled — never the full distinct set), and `source_ref` — a
resolved origin (topic measure/dimension/dataset column/derived/literal, with the
relevant id/grain) so a saved chart can rebind by *meaning*, not column-alias string.

**Chart-level (`ChartRecipe`):** `kind` (the 14-way enum), `title`, `column_bindings`
(slot name -> ordered column names, e.g. `{"category_axis": ["region"], "value_axis":
["revenue"]}`), `formatting` (per-column overrides carried from bound-column metadata),
`options_extra` (bounded declarative ECharts knobs, explicitly "must not embed values"),
`score` (0-1), `rationale` (one-line, from the top 1-2 scoring events),
`prepared_options` (the **inflated** ECharts dict, computed server-side from executed
rows — never persisted), and `binding_origins` (slot -> `SourceRef[]`, stamped only on
save, so rebind has something to match).

**Envelope (`ResultPresentation`):** `column_metadata`, `primary_recipe`,
`alternative_recipes`, `generator_version` (stamped for eval regression tracking),
`picker` (provenance enum, §1).

**Persistence (`ChartRecipeRecord`, only on explicit save):** wraps a `ChartRecipe` with
`id`, `tenant_id`, optional `saved_query_id`, `created_by`/`created_at`/`updated_at`. The
store layer enforces the recipe stays **declarative-only** — never row values, never
`prepared_options` — aligned with never persisting customer data rows.

**Wire path:** SQL executes -> `PresentationResolver` builds `ColumnMetadata[]` from the
executed shape + topic-pack definitions (no extra I/O) -> `select_candidates` (rules) ->
optional LLM ranker reorders the primary -> per-kind `generator_fn` inflates
`prepared_options` from the rows -> `ResultPresentation` rides the API response envelope
-> `ChartRenderer.svelte` hands `prepared_options` straight to `echarts.setOption()`. No
chart-type logic lives client-side at all.

Adjacent contract worth flagging: the SQL generator's `result_schema_mapping` is
validated by `mapping_validator.py` (missing-column/extra-key/match-rate, feeding an eval
accuracy metric). Like the ranker, its parser consumes **free-text JSON** — another P5
candidate for schema-constrained generation.

## 3. Frontend architecture & flows

- **Stack:** SvelteKit (Svelte 5 runes), `echarts` (^5.6) via one `EChartsCanvas.svelte`
  wrapper, `svelte-i18n` for every user-facing string. Identical stack and component
  names in both predecessors.
- **Route surfaces:** `query` (ask/answer/chart), `topics/[id]` (semantic-model
  authoring — dimensions/measures/KPIs sub-routes), `datasets` (+ schemas), `discover`
  (+ topics), `sessions`, `templates`, `schedules`, `jobs`, `feedback`, `relationships`,
  `access`, `maintenance`, `login`/`signup`, and `admin/` (`cache`, `security`, `gepa`,
  `rules`, plus — generalistic-predecessor-only — `business-domain` and `connections`,
  reflecting its broader warehouse-adapter surface).
- **Ask -> result -> chart -> refine loop** (`lib/components/query/`): `QueryInput` ->
  `ResponsePanel` (orchestrates SQL display, result table, `ChartRenderer`, confidence/
  risk panels, cache-hit badge, clarification sub-flows, inline thumbs-up/down feedback)
  -> `ChartRenderer` (title + rationale + `ChartAlternativesSwitcher` — an icon-button row
  per candidate kind, tooltip = title + rationale — + `ChartToolbar` for table-toggle/PNG
  export) -> body switches on table/KPI/ECharts view, with a `dataIssue` banner surfaced
  from `prepared_options.x_dataIssue` when a generator can't produce a usable chart. A
  fresh response resets any user override via an effect keyed on the primary recipe's
  identity. PNG export uses ECharts' own `getDataURL` — no server round-trip.
- **Feedback flow:** inline thumbs-up/down plus a dedicated `/feedback` route
  (`FeedbackForm`, `TopicAccuracyView`, `CalibrationView` — predicted-vs-actual
  confidence scatter, positive-rate summary) — feedback closes the loop into the
  eval/calibration surface rather than disappearing into a log.
- **Topic management:** `topics/[topic_id]/{dimensions,measures,kpis}`, each with an
  empty/error state tied to source-table health (see scar below).
- **Admin:** cache invalidation, security (JWT/JWKS config), routing-rule authoring,
  prompt-optimization runs; the generalistic predecessor adds `business-domain` and
  `connections` (warehouse adapter management) — a reasonable proxy for a future
  Chartworks data-source admin surface.

## 4. UX keepers & scars

**Keepers:**

- Chart choice is **disclosed, not hidden** — `picker` provenance renders as an inline
  hint so a user isn't confused by a chart that quietly changed after a schema edit.
- **Alternatives are first-class**, not a hidden dropdown — a compact icon-button row,
  with the override auto-reset on the next question.
- **Graceful degradation ladder**: data-issue banner -> table toggle -> "no
  `prepared_options`" all resolve to a working table, never a blank panel or client
  crash (`EChartsCanvas` swallows a malformed options dict defensively).
- Rationale strings from the **top-2 scoring events** give a legible one-liner ("Bar
  chosen: ideal cardinality, comparison intent.") instead of a raw score.

**Scars — do not repeat:**

- **Confirmed P6 violation** (the exact anti-pattern CLAUDE.md's P6 calls out): topic
  pages warn "Source tables are missing or inaccessible... until broken tables are
  **repaired**" with a "Review broken tables" button and a "Retry enhancement" action;
  a separate templates surface exposes a `validate`/`repair` API action
  (`POST /templates/{id}:repair`) and a `TemplateAuditAction` enum literally containing
  `"repair"`. Plumbing vocabulary ("repair", "broken", "enhancement") leaking straight
  into user copy and a wire-level path. A future Chartworks frontend must render this
  failure class in domain terms ("this data source needs reconnecting") and never expose
  a verb describing *our* internal remediation step.
- **Free-text JSON parsing of model output**, twice: the LLM ranker and the SQL
  generator's `result_schema_mapping` parser. Both hand-roll fence-stripping + `json
  .loads` + manual type checks. P5 forbids this outright — route through the gateway's
  schema-constrained structured-output path instead.
- **Chart selection can't distinguish a user's deliberate edit from an auto-pick** — the
  rebind story only matches by `SourceRef` identity, so a schema change can silently
  replace a user-authored binding with the next best rules candidate rather than
  flagging that specifically.

## 5. Recommended V1 chart-spec contract

Chartworks V1 has no frontend (D-013), so V1 should emit the **declarative** half of
this contract only — never inflate ECharts options in Go, never persist row-shaped data:

- `ColumnMetadata[]` — name, display name, source role, data type, semantic type,
  temporal grain, aggregation, a format hint, query role, a *bucketed* cardinality
  (never a raw distinct count), and a source ref back to the semantic model or dataset
  column — the field that lets any future rebind-on-schema-drift story work at all.
- `ChartRecipe` — `kind` (a small fixed enum, seeded from the ~10-14 kinds proven useful
  here), `title`, `column_bindings`, `formatting`, `score` (0-1), `rationale` (short,
  human-facing). Explicitly **omit** `prepared_options` from V1 — it is a
  provider-specific (ECharts) rendering artifact, not a data contract; pre-inflating it
  in Go would do rendering work nobody consumes yet, couple the core to one charting
  library's JSON shape, and contradict "outputs are normalized shapes" (§6) by embedding
  a third-party wire format into the core response.
- `ResultPresentation` envelope — `column_metadata`, `primary_recipe`,
  `alternative_recipes`, a `generator_version` string (mandatory once an eval harness
  exists), and a `picker` provenance enum (`rules | llm | rules_fallback` at minimum —
  `user`/`rebound*` can wait for a saved-chart feature).
- The selection logic itself (slot binding + weighted scoring + adaptive alternatives)
  is cheap, deterministic, and P4/P5-compatible as designed — a reasonable rules-engine
  template to port almost as-is; any LLM-ranker step must go through the gateway's
  schema-constrained structured-output path (P5), never free-text JSON parsing.
- A persistence shape (saved charts) can wait for a "saved queries" feature, but the
  contract above should already carry enough (`source_ref`-qualified bindings) that
  persistence is additive, not a redesign.

## 6. Open questions for the RFC

1. Does the rules-engine selection need to live behind the gateway seam at all (it's
   pure Go, no model call needed for the base picker), with only an *optional* future
   LLM-ranker step gated behind P5's schema-constrained-call requirement?
2. Should the chart-kind enum be fixed at ~14 kinds (matches both predecessors) or
   trimmed for V1 given no renderer consumes it yet?
3. Where does `cardinality_bucket` sampling happen without becoming a hidden query — the
   predecessors compute it in-process from already-executed rows; does P1
   (computed-in-query-path) impose any extra constraint on that sampling step?
4. Should `SourceRef`/rebind-by-origin be in V1's contract now (cheap, since the
   semantic model already carries measure/dimension ids), even though nothing persists a
   saved chart yet?
5. Is there an ecosystem-wide chart-spec shape Dockyard/Harbor already expect from an
   MCP Apps surface (a `ui://` chart resource, or a shared dataviz asset contract) that
   this contract should conform to instead of inventing its own field names?
