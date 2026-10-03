# Reviewed statistical narrative evidence

`bounded-narrative-v3` is an explicit authored policy paired with
`grounded-narrative-v2` and `statistical_evidence`. Earlier policies retain their
existing evidence and deterministic text contracts. This policy does not infer
statistical intent from a question, chart type, label or returned row order.

A published output declares one numeric series through exact expected-schema
coordinates. At most three declarations select distinct `trend`, `extrema` and
`population_variance` kinds in authored order. Trend additionally declares a
calendar date or an explicitly offset-bearing instant field. Local wall times,
partial dates, duplicate instants, inferred grain, resampling and interpolation
are unsupported. All dependencies remain subject to current model-egress policy.

## Population and calculations

The population is the first `min(max_rows, retained_rows)` source rows. NULL
numeric observations are excluded explicitly and their original row ordinals
are retained. Extrema require at least one included numeric observation; trend
and variance require at least two. Numeric input strings remain bounded to 4096
characters, exponent magnitude 1024 and decimal scale 1024; each output fraction
integer is bounded to 4096 characters. A statistic does not silently change that population to fit a byte
budget. Empty or insufficient evidence makes the narrative unavailable before
provider reservation. Tables and other independently selected outputs keep the
existing partial-output policy.

Trend sorts the included observations by the reviewed time coordinate and keeps
original source-row citations. It reports exact endpoint change and the actual
sequence classification: increasing, decreasing, constant, nondecreasing,
nonincreasing or mixed. An endpoint increase alone is not a monotonic trend.
Extrema retain exact minimum/maximum values, tie counts and the lowest original
row ordinal as each representative. Population variance uses the included
count `N`, never `N-1`, and retains an exact reduced numerator/denominator. Its
unit is squared value units, not a formatted amount or an inferred confidence
interval. Forecasts, significance, causal explanations and arbitrary formulas
are outside this contract.

Source truncation and narrative row reduction remain explicit. Even an
untruncated result describes retained observations, not an unobserved whole
source population. Cancellation, numeric parsing, rows, evidence bytes,
characters and provider budgets keep their existing bounds.

## Amount meaning and sensitivity

A declared amount requires explicit amount/count output bindings and authorized
companion evidence. Its statistical narrative describes known amounts and
carries complete/incomplete/unknown status for the considered rows. A status
cannot declassify a sensitive or redacted count. If any required value, temporal
coordinate or companion is excluded, no provider call or reservation occurs.
The separately authorized raw table remains available under its own read policy.
Raw count values and disclosure sidecars are not copied into the provider input.

## Claims, persistence and consumers

The provider selects only closed `{kind,evidence}` claims referencing one locally
derived matching record. It supplies no numbers, expressions, labels, prose or
statistical direction. Text is deterministic, with exact values, scope and
qualifiers. Evidence IDs follow declaration order.

Evidence binds the output, declaration, field coordinates, source-row ordinals,
counts, exact calculation and effective policy. PostgreSQL independently
recomputes it from the sealed retained result on write and read; recalculating an
outer hash cannot authorize changed evidence. Retained reads and exports perform
no new warehouse execution or provider work.

HTTP/MCP generated schemas and SDK authoring types carry the closed declaration.
The retained viewer shows narrative text and caveats. Static HTML also renders
escaped caveats, correcting their earlier omission without changing retained
narrative text/evidence hashes. Previously generated immutable artifacts are not
rewritten. Statistical narratives do not expand PNG or SVG output support or
change process isolation.

This policy uses existing JSON-backed versioned definitions and results; it
requires no database migration. Recorded-provider/source tests qualify the
closed calculations and consumers, not live-model quality or general statistical
inference.
