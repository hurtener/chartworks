# Phase 20 — charts-spec

Status: planned. Owner: internal/charts. Hard dependencies: 10, 15.

## Authority and design

RFC-001 §10, RFC-002 §5, D-047/D-049 and [COMMON.md](COMMON.md) apply. The spec layer stays provider-neutral; rendering is phase 31/32. A frozen block stores its chosen outputs rather than calling the selector every refresh.

## Brief findings incorporated

Briefs 06, 14: deterministic suitability/slot binding, metadata/format contracts, alternatives and the established presentation catalog.

## Findings I'm departing from

No chart-rendering exclusion for the product as a whole. Exact labels must not inherit floating-point loss. A table fallback cannot be counted as parity for a required chart kind.

## Scope and implementation tasks

1. Implement provider-neutral column metadata/recipes/bindings/format hints, deterministic rules-first selection and bounded alternatives.
2. Cover the fourteen output catalog entries and safe semantic rebinding for authoring; preserve exact labels even if geometry approximates numeric coordinates.
3. Offer optional gateway-based rank assistance only in exploration/authoring; frozen output definitions never invoke the picker.

Required catalog: area, bar, column, donut, grouped bar, heatmap, KPI card, line, pie, scatter, stacked bar, stacked column, table and treemap.

## Non-goals

No arbitrary chart JavaScript/formatter, remote resource URLs, warehouse requery to draw a chart or driver-specific UI contract.

## Config and persistence

Charts category/series/options-depth limits, selection floor and alternatives; optional exploratory ranker with gateway limits. Persist selected output definitions in block revisions later, not a second chart-state store. Carry units/currency/percent/grain and exact-value labels explicitly.

## Acceptance criteria

1. **AC01** — Rules-first selection is pure/deterministic, validates all required slots and returns a labeled table fallback only where appropriate.
2. **AC02** — Metadata carries unit/currency/percent/grain/aggregation/source role and versioned provenance without driver-specific UI types.
3. **AC03** — All catalog entries have binding/order/empty/negative/null/long-label goldens; fallback is not counted as chart parity.
4. **AC04** — Exact decimal/large-integer labels and totals are preserved; truncated result totals never masquerade as full-source totals.
5. **AC05** — Saved mappings fail on incompatible columns; optional authoring rebinding creates a proposal rather than changing approved output.
6. **AC06** — Chart options cannot carry executable JS/remote URLs; budgets and optional ranking provenance are visible without rerunning SQL.

## Tests, coverage and smoke

Implement `TestPhase20/AC01` through `TestPhase20/AC06` with exhaustive catalog fixtures, deterministic selection properties and schema/mapping negatives. Renderer pixel/layout tests belong to phase 31/32 but consume these same fixtures. COMMON.md sets coverage; `scripts/smoke/phase-20.sh` requires all six results.

## Glossary, decisions and deviations

Spec selection and rendering are distinct; both are in product scope. D-047/D-049 apply. No runtime completion is claimed.
