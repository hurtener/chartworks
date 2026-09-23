# Governed routing and interpretation v1

Status: CW-07 implementation contract, 2026-09-22. This contract extends phases
17, 18 and 24 on the reviewed rich-semantic substrate. It does not replace Pengui
authority, current topic/source admission, the model gateway, the tokenizer owner,
SQL validation or the opaque execution plan.

## Server-owned topic decision

A route may omit `topic` and `topics`. Chartworks then lists at most 32 active
publications through the authority-constrained topic catalog. Every candidate must
be current, signed-reachable in its complete source/dataset/context dependency
manifest, healthy through `Contract`, and usable in the requested execution
context. An incomplete page fails with a typed limit; it is never treated as the
whole candidate universe.

The service embeds the question through the one Bifrost gateway, retrieves bounded
facets for every admitted topic, and aggregates up to three leading facets per
topic. `evidence-v1` combines best similarity, mean complementary evidence and
distinct facet-kind coverage. It therefore does not report the closest vector
distance as confidence or mistake two complementary facets from one topic for two
competing topics. Optional reranking sees only already-authorized topic evidence,
must return a complete permutation, and contributes a bounded rank component.

The retained decision records exact topic version/digest, best and mean similarity,
facet kinds/count, pre-rerank score, optional rerank position, final score, policy
version, floor and margin. A score below the floor returns `no_route`; competing
topics inside the margin return `ambiguous_topic` with bounded choices. Deterministic
ID ordering is only a tie-breaker for evidence display, never silent topic choice.
This policy is inspectable and reproducible; repository fixtures do not claim live
population calibration.

Caller-pinned single and multi-topic routes remain supported. Multi-topic execution
still requires each publication to independently confirm the same same-source,
same-context one-to-one relationship. Relationship candidates and rejections are
evidence only and never executable joins.

## Deterministic semantic interpretation

`semantic-interpretation-v1` runs after exact publication admission and before any
question embedding for a resolved route. It scans only reviewed non-sensitive
governed values and aliases, reviewed dimension/column aliases, and reviewed
Gregorian temporal dimensions. English and Spanish governed-value negation
produce `ne` rather than silently losing exclusion. Geographic classification comes only from the
reviewed categorical-dimension designation; labels and aliases never infer it.
Named months, the supported English/Spanish year connectors and supported relative-month phrases become
half-open month windows against an explicit `YYYY-MM-DD` anchor. The parser also
recognizes `in 2026` / `en 2026` as the Gregorian calendar year
`[2026-01-01, 2027-01-01)`, and bounded `from January through March 2026` /
`de enero a marzo de 2026` as `[2026-01-01, 2026-04-01)`. The server supplies
and retains the anchor when a caller omits it, so replay never depends on a later
wall clock. A year window is a filter interval, not a request to aggregate by year.
Explicit month, quarter or year grouping must be supported by the reviewed
temporal policy in addition to the interval grain; month intervals require
reviewed month support. Calendar-year filters may use any reviewed grouping
grain, including month or quarter without reviewed year aggregation. A quarter
or year grouping over a month interval also requires aligned window boundaries.
Negated temporal requests such as `not in 2026`, `no en 2026`, or
`excluding the period from January through March 2026` clarify before model
work rather than becoming inclusive windows. The parser distinguishes a
sentence-initial modal `May I` from the named month `May`; exclusions of other
objects, such as refunds, do not negate a nearby positive date range.

One spelling matching multiple reviewed dimensions, one period with multiple
unnamed temporal dimensions, an unsupported calendar, or a changed source binding
returns a typed clarification/conflict before model work. Sensitive or
unknown-sensitivity values are never scanned, retained or sent. The result records
locale, parser/version, anchor, exact topic/source/context/revision pins, canonical
governed-value IDs, physical constraint coordinates, operators, time bounds,
provenance and one digest. Date and wall-clock timestamp constraints retain local
calendar dates. Instant timestamps convert unique reviewed-zone midnights to
RFC3339 UTC; missing or folded civil boundaries clarify before provider work. No
raw profile sample enters the result.

Callers may remove an inferred target or replace it with another reviewed governed
value ID. Arbitrary replacement literals are rejected. Edits are retained in the
route request, inherited by refinement and replace earlier edits for the same stable
target. A changed question reruns deterministic interpretation; persisted runs
replay against current publication, authority and source bindings and fail if the
sealed interpretation changes.

## Generation and execution boundary

Every interpretation becomes a mandatory context constraint and a closed
`exec.BusinessConstraint`. The model sees a typed instruction, while execution
binds the exact reviewed dataset/column/value or temporal range before the existing
validator issues a plan. Interpretation is not authority: Pengui reach applies
first, source binding is rechecked, business constraints cannot construct a plan,
and SQL still passes whole-statement validation and read-only execution.
For PostgreSQL, the binder also accepts up to four flat named CTEs: each CTE
may read reviewed schema-qualified base relations or an earlier CTE, and each
CTE must have a downstream path to the final SELECT. A CTE may feed multiple
later scopes, but self-joins within one scope are rejected. All active
constraints must identify one base relation occurrence in one CTE body; row
predicates are inserted before that CTE's aggregation or join. Missing,
duplicated or unreachable targets, recursive CTEs, unions, derived or
correlated SELECTs and other dialects remain typed unsupported cases. This
bounded transformation does not
replace the ordinary opaque read plan or reviewed relation scope.
The binder also rejects a target SELECT scope containing a LEFT, RIGHT or FULL
outer join. Inserting its predicate into WHERE could remove preserved rows when
the target is on the nullable side; no ON or prejoin rewrite is inferred.

The in-process route seal binds the interpretation, source binding digest and typed
constraints. A JSON reconstruction cannot manufacture executable filters. Any
active clarification or interpretation constraint stores the protected unbound SQL,
parameters and exact binding receipt. Planning, execution, terminal replay and
refinement reconstruct and compare that receipt through current authorized
topic/source services; publication, parser, vocabulary or binding drift fails.

## Evidence and limits

`TestCW07/AC01` through `AC11` cover server selection and reranking, competing-topic
ambiguity, Spanish governed geography and month interpretation, correction/removal,
stale source denial, ambiguous values, pre-provider candidate authority, tenant and
context isolation, deterministic replay and interpretation budget rejection before
provider work, plus connector parsing, DST boundary behavior and RFC3339 instant
binding. AC10 covers bilingual year and explicit month-range bounds, reviewed
timezone conversion, sealed constraints and replay. AC11 covers unsupported
grouping/interval grains, unaligned quarter windows, temporal negation,
conflicting periods and locale negatives before provider work.
`TestCW07InterpretationBusinessEvidencePlanRunAndDrift` covers ordinary and
terminal plan/run replay, exact receipts, corrections/removal and drift. Existing phase-17 tests retain confirmed multi-topic joins,
context isolation, tokenizer budgets and bilingual routing; CW-04 tests retain
metric-closure budget overflow.

Live confidence calibration, cloud-source population behavior, full date grammar
and stress/performance qualification remain final-gap/manual-suite work. Current
automatic PR checks remain the fast D-074 lane.
