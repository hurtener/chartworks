# Confirmed-join context projection

S3 extends the existing render-only context projection. It is not S5 analytical
join/fan-out proof and does not authorize any relation, column or query shape.

## Production path

The existing multi-topic router first independently confirms each topic's chosen
reviewed relationship, including same source/context, endpoints, join direction
and current cardinality policy. After obtaining complete physical relation
contracts it adds one bounded `confirmed_join_projection` mandatory descriptor per
topic. Each descriptor maps the logical endpoint IDs to that topic's exact reviewed
physical column names. This is not model-selected or optional retrieved evidence.

The assembler checks the complete topic-owned set and resolves its columns through
the same `DependencyRelations` function used for selected semantic closure. Inner
relationships may reverse equivalent endpoints; left relationships remain
directional. Unknown fields, partial or duplicated topic sets, inconsistent
mappings and missing columns are rejected. The normal topic/source policy still
owns whether a choice is confirmed; JSON hashes are content identity, not grants.

For independently selected exact column closures, rendering is the union of
selected/mandatory dependencies and the confirmed join endpoint columns. Shared
relations no longer retain every unrelated column merely because both topics
reviewed them. Opaque metrics, dataset-only or filter-only choices still retain
their full topic context. In particular, a known private filter is not a complete
output selection. Each topic retains its own mapping; no column is borrowed from
another topic's contract.

## Immutable boundaries

`Relations` in assembled context, signed admission, native validation, query
persistence, saved views and replay remain the complete reviewed sets. The new
metadata changes rendering only. It carries no scalar values, SQL, identity or
execution authority. Optional evidence keeps its own verified mappings and is not
promoted to a required output. Generation and provider-envelope refitting preserve
required join descriptors and keys as a unit, under the same budgets.

New narrowed packets use `physical_projection:confirmed-join-closure-v3`. Retained
packets without the descriptor keep the prior `topic-selected-closure-v2` behavior,
including conservative shared columns. No old query or prompt is backfilled. No
migration, public operation, model role or additional inference is introduced.
Frozen report refresh does not reinterpret a join.

## Qualification

New tests cover actual producer/consumer wiring, growth of shared unrelated schema,
missing and mismatched endpoints, inner/left direction, topic-specific/private
columns, filter-only fallback, required refitting, detached state, replay and
cancellation. PostgreSQL/recorded-provider acceptance checks actual joined record
IDs, complete durable validation scope, denied source reach, unconfirmed choices,
private-value non-disclosure, saved result privacy and zero-work terminal replay.
Exact observed results belong to the completion tracker and PR; this test inventory
alone is not a passing or live-model/physical-cardinality qualification.
