# Grouped aggregate programs v2

Status: SQL recovery S5 implementation increment. Exact-source qualification is
recorded in the completion tracker. This extends the
[grouped population contract](analytical-grouped-populations-v1.md) under the
separate `analytical-metrics-v8` proof version. Retained v7 remains its direct
column, named-CTE grammar and receives no calendar or wrapper upgrade.

## Transparent derived projections

A lane may be enclosed by bounded non-lateral derived SELECT projections. An
entire proved grouped program may likewise be enclosed by such a projection.
Every wrapper preserves all selected aggregate outputs and exact grouping keys;
it may rename or reorder them. Only direct references to proved inner outputs
are admitted. Missing metrics, hidden unnamed outputs, duplicate or ambiguous
names, column-list aliases, expressions, stars and correlation do not borrow the
inner proof. At most four projection levels are proved.

The distinct UNION key spine may be a derived table over the named aggregate
lanes instead of its own named CTE. Every lane still contributes its entire key
exactly once, and each outer LEFT JOIN aligns that lane directly to the spine
using the complete NULL-safe key. This is representation equivalence, not a new
missing-group or zero-fill policy.

No wrapper clause is silently stripped. WHERE, DISTINCT, GROUP BY, HAVING,
windows, extra joins or limits inside a transparent projection are rejected.
Reviewed output ordering and limits are checked only at the final SELECT. A
buried order cannot promise the outer order, and a buried limit cannot replace
final-layer intent. Inner grouped programs under a wrapper must preserve all
groups without order or limit. Fully inlined repeated aggregate populations and
arbitrary correlated/window/set programs are not inferred from this proof.

## Shared reviewed calendar keys

A selected shared calendar bucket is now an exact grouped key. Every lane must
reach the same physical temporal field through its own proved source graph.
The key retains physical origin, calendar, unit and, for instant-valued inputs,
the exact reviewed timezone. The existing native calendar checker proves the
expression inside each lane, including date/civil versus instant distinctions.
The spine and every alignment edge preserve this complete bucket identity.

Direct and calendar keys may coexist. NULL calendar keys form one group, while
missing lane outputs remain NULL. Years are retained. Month numbers, display
strings, another lane's event timestamp or an unreviewed timezone are not
substitutes. Grouping refunds by their order timestamp and grouping refunds by
the refund event timestamp have different meanings; this contract supplies no
cross-field temporal-axis mapping.

## Retention and verification

Migration 065 adds v8 and its grouped calendar receipt scope without rewriting
v0–v7 records. V8 inherits mandatory intent and owned-population markers, private
parameter custody, native/source validation, exact reviewed null arithmetic and
replay fences. Each proof remains bound to selected semantic revisions, source
metadata, exact SQL/parameters, lane topology and grain. No new source authority
comes from a derived table or semantic receipt.

Recorded-provider PostgreSQL acceptance covers regional wrapper equivalence,
NULL/missing and empty populations, final-layer ranking, civil month/year/NULL
partitions, and shared instant days across daylight-saving gaps and folds.
Negative cases retain filters and limits rather than dropping them, substitute
bucket origin/zone, change the union or alignment, and omit required outputs.
Runtime test results are checkpoint-specific; these fixtures are not live model
quality or migration-owner parity evidence. Grouped owned-predicate projection
requires its separately reviewed placement proof.

## Reviewed group existence and the v7 correction

A lane's existence domain is independent from the NULL behavior of its aggregate
outputs. The reviewed semantic grouped policy now accepts a complete, sorted
`group_domains` list naming each fact and one of these meanings:

- `raw_source_groups`: group all rows produced by the lane's reviewed source
  joins. Metric predicates remain on their aggregates; no metric predicate may
  move into WHERE and remove a group
- `qualifying_population`: group the union of rows satisfying at least one of
  the selected lane's complete reviewed metric-filter populations. WHERE must
  enforce that exact union. Individual aggregates retain their own populations

The union is evaluated before aggregate NULL handling. A qualifying row with a
NULL measure still establishes its group; COUNT(column) can therefore be zero
while a missing lane remains NULL. No data sample, count result, label or join
cardinality declaration selects a domain. V8 requires an explicit reviewed
domain for every filtered lane. An unfiltered aggregate makes the qualifying
union TRUE, so raw and qualifying domains coincide; the compiler may then
canonicalize the equivalent raw domain without making a business choice.

The native checker proves the WHERE predicate using bounded positive AND/OR
normalization over exact typed filter atoms. It does not turn a union into an
intersection, move an aggregate's filter globally, or erase NULL predicates.
Domain choices are preserved by semantic review/diffs, detached copying,
portable mapping and dataset rebinding, and are included in each compiled lane
and the contract digest. No extra source reach follows from this metadata.

The original v7 checker incorrectly treated common WHERE and aggregate FILTER
placement as equivalent for group existence. A real-source regression showed an
all-cancelled, refund-free group becoming an extra NULL-valued output row. The
correction does not rewrite old receipt versions or invent a domain. Retained
v7 analytical revalidation and new source execution admit only unambiguous forms: a lane containing an unfiltered metric with
no row filter, or identical complete metric filters enforced by the full common
WHERE. Other filtered v7 forms return the closed
`analytical_group_domain_review_required` disposition and require explicit
review against independently reviewed domain semantics. The protected review
lifecycle owns that transition; an old receipt itself is not a new approval.
Terminal cached-result reads remain historical artifact reads and make no new
source or analytical-conformance claim; they do not rewrite old rows or receipts.

Real-source tests distinguish cancelled-only groups, all-NULL aggregate inputs,
zero COUNT(column), missing lanes and a qualifying union of different metric
populations. Ordinary single-base/grouped raw joins now use the separate
[reviewed group-domain contract](analytical-group-domain-v1.md) in v8; its policy
does not replace the per-lane domain or independent alignment proof described here.
