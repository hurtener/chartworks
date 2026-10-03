# Exact scalar predicate entailment

Status: fresh bounded implementation under local qualification. This is
not recovered capability evidence. The [observed missing case](../reviews/scalar-predicate-characterization.md)
is an explicit paid-order selection already required by every selected scalar
SUM/COUNT leaf, including the reviewed parent filters on refund leaves.

## Closed admission and proof

The new `analytical-metrics-v13` proof is restricted to PostgreSQL scalar scoped
singleton programs already valid under v9, with two to four fact lanes and the
same separately authenticated period applications. It additionally accounts for
one or more authenticated non-period constraints. Every constraint must be a
canonical text `eq`, `nulls=exclude`, `null=false` with one exact byte string and
an eligible physical text column. Aggregation, upper bounds, range metadata,
units, precision, scale, temporal type, calendar, timezone and grain are absent.
The reviewed witness is also exactly `eq`, not singleton IN or a finite set.
No numeric, interval, arbitrary Boolean, outer-join, grouped, unscoped or other
engine admission is implied. All non-period constraints must qualify together;
there is no residual drop, partial discharge or fallback on an unproved leaf.

Each selected root is recompiled from its immutable topic/version/publication and
current source binding. The proof covers every aggregate expression occurrence,
identified by selected MetricID and argument path. Repeated measure references
and inherited KPI filters remain separate occurrences. Only SUM and COUNT leaves
are qualified in this first increment. Constants in the already reviewed outer
arithmetic do not create row populations. A leaf must contain the exact target
physical field and equality value among its mandatory reviewed filters. No
predicate is inferred from names, related fact membership, cardinality labels,
sampled distinctness or another root's filter.

A same-fact witness has no relationship. A cross-fact witness names the exact
reviewed INNER relationship selected for that semantic filter and permitted by
the current route's relationship choices. It retains the topic, publication,
measure and filter identity, relationship identity and full reviewed relationship
digest. Its canonical physical join must be present in that fact's admitted lane,
with complete equality keys and current source-backed uniqueness on the opposite
side of the aggregate input. Multi-hop inference is outside this increment.
Inherited same-fact KPI predicates retain their path-specific owner. The active
semantic contract still forbids relationship-bearing KPI filters; they cannot
acquire parent reach through this increment.

The semantic compiler owns identity reconstruction. The execution checker cannot
authenticate semantic IDs using a physical Binding alone: it independently
checks every expression occurrence, exact predicate, lane, physical join and
uniqueness, and compares complete coverage. Current authenticated semantic/route
reconstruction establishes the identity pins. Recomputing a public digest never
substitutes for either check. Constraints are capped at 64, expression traversal at 1024 nodes and depth 32,
and the complete constraint-by-leaf product at 512 before allocation. Overflow
fails closed. Native target types are text, varchar and character varying only;
UUID, fixed-width character, name, domain and extension types remain excluded.

The logical claim is limited to contributing row populations. Every admitted
aggregate already requires the equality to be TRUE; NULL equality is UNKNOWN and
contributes no row. An explicit identical equality removes no contributing row.
SUM over empty or all-NULL input stays NULL, COUNT stays zero on empty input, and
unknown-count companions retain their distinction. The proof cannot certify a
grouped output row domain.

## Binder and evidence

The binder does not remove SQL predicates or spread request predicates across
facts. It binds the existing fact-owned periods exactly as before and records an
explicit closed `entailed_scalar_predicate` effect for every non-period request
constraint. Such effects add zero parameters and carry a complete coverage digest
and occurrence count. Empty, duplicate, extra or missing effects are invalid.
The selected metric filters stay in the original SQL and must still pass the
ordinary native and analytical population checks.

Binding schema 6, policy `entailed-scalar-predicates-v1`, and the v13 analytical
scope are accepted together only. The protected contract retains canonical
constraints and occurrence-level semantic/physical witnesses; the public binding
and analytical receipts expose only policy, stable IDs, hashes, counts and period
parameter positions. They expose no predicate values or protected SQL. The full
source binding, contract, final statement and parameter vector remain pinned.
Legacy versions reject new-only fields. V1-v12 retain their original dispatch and
receipt meaning; no retained record is relabeled.

## Replay, refinement and learning

Run, terminal-result replay and refinement reconstruct current authorized routing,
immutable semantic revisions, current source reach, period applications and exact
request constraints. They recompile v13, rebuild from the protected original base
statement and compare the entire binding/analytical receipt before accepting
fresh native proof or cached output. Saved SQL, serialized route JSON, proof
objects, IDs and recomputed hashes cannot manufacture an in-process route seal.
Refinement starts from the verified protected base and resolves its own current
constraints; it never inherits discharged authority from its parent.

Schema 6 is ineligible for automatic example learning and reusable owned-example
consumption until a separate consumer is qualified. Feedback remains recordable.
Migration 081 adds the closed v13 receipt shape, retaining old database shapes
without reinterpreting their rows.

## Required qualification

The expected-success regression must fail on the previous production baseline,
then succeed through the real PostgreSQL 17 service and HTTP/SDK seams. Separate
exact-rational row oracles cover cohort/activity, all-NULL and empty populations.
Negatives cover each selected leaf, repeated occurrences and inherited contexts,
NULL-only/inexact/unsupported constraints, source/context/revision changes,
physical key/uniqueness changes, relationship identity/choices, authority and JSON
forgery, missing/extra effects and recomputed digests. Stored receipts, terminal
replay, refinement and old-version controls must be checked, with no model/source
execution on pre-execution denial. Focused, full unit/race and migration checks
are separately reported. No live-provider or broader release claim follows from
recorded model fixtures or these bounded controls.
