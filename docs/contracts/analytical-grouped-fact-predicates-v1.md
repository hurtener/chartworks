# Authenticated primary-fact predicates

The bounded PostgreSQL v12 contract (`analytical-metrics-v12`) adds reviewed row
predicates to independently aggregated primary facts. Binding schema 5 uses
`grouped-fact-predicates-v1`. This is a new implementation from the current
v8/v10/v11 contracts, with a deliberately partial ownership and SQL-shape matrix.
It does not close the wider grouped-filter or SQL recovery scope.

## Exact reviewed ownership

The compiler first reconstructs the current admitted publications, selected
transitive metric leaves, physical source/context/revision and source-backed
unique joins. Predicates come only from the current router's in-process seal or
its authenticated replay of retained reviewed rules and answers.

A new fact predicate must be row-level, target the exact physical dataset of a
selected primary fact, and have that dataset absent from every other lane's
reviewed join path. A joined-only coordinate is rejected even if only one lane
references it. A primary fact also used as another lane's joined parent is
ambiguous and rejected. Neither table membership, field names nor model SQL
creates global, filter-all or cross-population ownership.

The affected fact must have an explicit reviewed raw/qualifying group domain and
only INNER joins (or no joins). The generic binder's existing outer-join refusal
is unchanged. The original LEFT-join regression remains an unsupported case;
preserved-root LEFT placement could be added later only with a separately proved
preservation-side AST contract and scoped binder, not by dropping this guard.
Unfiltered lanes retain their existing independently proved join semantics.

## Separate populations and placement

Each v12 lane has a protected `FactPopulation`, separate from its optional
reviewed period `QueryPopulation`. Exact predicates are conjoined only inside
that fact's aggregate body before GROUP BY. Raw groups are the resulting owned
rows; qualifying groups additionally satisfy the union of the lane's complete
reviewed metric-filter populations. Every aggregate retains its own filters.

Direct selected grouping-key constraints retain v11 final-spine placement after
complete NULL-safe alignment. A mixed v12 query preserves that separate final
selection. Pure final-selection requests continue to produce unchanged v11
receipts. A v12 contract must contain at least one fact predicate.

The binder accepts only flat named aggregate CTEs plus one named UNION DISTINCT
key spine. Model parameters, preexisting placeholders, derived wrappers and
extra lane/spine/outer restrictions fail closed. Parameters are offset once and
each effect retains its fact population; an empty population is reserved for a
final-spine effect. Exact SQL, private values, complete constraints and placement
participate in protected digests and deterministic binder reconstruction. Native
validation and analytical provenance/output-ordinal proof remain mandatory.

No zero fill, group-domain conversion, join rewrite or additional NULL behavior
is inferred. All-NULL qualifying rows still establish groups; COUNT can be zero;
an absent fact lane stays NULL after alignment. Fact NULL-only constraints carry
an effect even without a scalar parameter.

## Versioning, replay and learning

Migration 080 appends the closed v12 analytical shape. Earlier migrations and
retained v0-v11 receipts are not relabeled. Old versions reject `FactPopulation`;
schemas 1-4 cannot impersonate schema 5. Provider guidance and public receipts
omit private population values. Protected unbound SQL supports current-answer
replacement; old private values are not sent back as edit context.

Original Plan-operation replay and terminal Run replay reconstruct current
source/rule/binding evidence and native-plus-analytical provenance before exposing
retained values. Terminal replay performs no model inference or result execution.
This remains subject to the existing signed authority, immutable source custody,
retained lifecycle and current admission policies.

Schemas 2-4 retain their separately implemented
[owned-base learning policies](owned-example-learning-v1.md). Schema 5 uses the
distinct `current-owned-grouped-fact-predicates-v1` policy with exact authenticated
base reconstruction, current sealed-family applicability and separately reviewed
requalification. It cannot borrow another owned policy; unknown schemas and
schema 6 scalar entailment remain excluded. Historical private fact values,
periods and final selections are never reusable example content.

## Unsupported and qualification boundary

Shared/join-only predicates, facts shared as other lanes' parents, affected LEFT
joins, HAVING ownership, derived bucket predicates, derived wrappers/spines,
more than four fact lanes, ordinary completeness composition and non-PostgreSQL
execution remain unsupported. Existing v8 derived/calendar programs, v9 scalar
ownership, v10 periods and v11 group selection retain their original meanings.

The new compiler regression is independently red on unchanged production for
both the original LEFT shape and adjusted INNER shape. Only the INNER slice is
implemented here. New native tests cover placement, NULLs, multiple fact effects,
period/final selection composition and version/parameter/source tampering. The
new real-PostgreSQL acceptance has an independently computed numeric and NULL
oracle, current-value replacement, zero-work terminal replay and schema/receipt
controls. Exact executed evidence is recorded separately; test inventory does not
establish live provider quality, hosted qualification or full recovery completion.

## Extended native and application matrix

`TestSQLRecoveryGroupedFactMatrixAcceptance` executes twelve recorded-provider
HTTP/SDK Plan/Run cases against real PostgreSQL: three and four independently
aggregated fact roots, each with direct region, composite region/segment, or
shared civil month keys, under both reviewed group domains. Every child fact has
its own authenticated numeric maximum; the common order parent remains
unrestricted. The calendar lanes all use the reviewed order-date origin, including
NULL dates, month boundaries and the same month in different years. Refund event
dates deliberately differ and do not become an interchangeable calendar axis.

A row-only oracle establishes each population before complete-key union and is
cross-checked against hand-calculated witnesses. The assertions distinguish a
genuine zero SUM, an existing all-NULL group with COUNT(column) zero, an absent
lane with NULL count, partial/full NULL composite keys, and raw versus qualifying
existence for cancelled-only groups. Each case also replaces current private
values from the original unbound base, checks original Plan and terminal Run
replay with zero model/result execution, and rejects eleven retained-evidence
mutations.

`TestGroupedFactsExtendedNativeMatrix` separately proves three/four-lane
composite keys and shared instant-valued calendar keys. It rejects missing or
reordered spine keys, incomplete alignment, ordinary equality, changed calendar
origin/unit/timezone, misplaced predicates and swapped private lane values.
`TestGroupedFactsExtendedOwnershipBoundaries` keeps the typed refusals for a
shared joined dimension and an affected LEFT lane. These checks extend evidence
for the existing exclusive-primary INNER contract; they add no production scope.
The preserved-root LEFT case remains unimplemented. The schema-5 learning
continuation is documented separately in the owned-base contract; this matrix
alone does not qualify that lifecycle. Exact executed source and logs accompany
each qualification result.
