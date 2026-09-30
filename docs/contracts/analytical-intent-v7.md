# Complete typed non-narrowing intent, v7

New selected-metric records use analytical version 7. Records 0–6 replay their
original policy, including explicitly unmeasured filtering/limits on old records.
This policy change does not reinterpret old questions or add a model role/call.

Every v7 contract has both an intent and owned-query-population contract. In the
absence of an admitted limit the query cannot add LIMIT, nonzero OFFSET, FETCH
truncation or ties expansion. In the absence of resolved reviewed query predicates,
WHERE/HAVING must be empty except for a reviewed metric filter shared by every
selected aggregate. Ranking may remain unspecified; that never permits truncating
or filtering its result. The same rule applies to bounded correction candidates.
Executor row/byte/time caps remain separate resource controls and are not widened.

The ordinary compiler obtains predicates from the existing sealed route or verified
replay. A successful typed predicate compilation and the native business binder
remain required; model SQL is not evidence of user intent. Missing intent/population
contracts or receipt markers fail the v7 checker and durable record validator.
Contract/query hashes and the existing immutable evidence trigger bind replay.
Independent singleton and grouped-population lanes enforce their own exact admitted
populations and no accidental local limits; they do not borrow an outer certificate.

## Explicit reviewed empty/null fallback

The closed KPI grammar additionally accepts catalog-reviewed numeric COALESCE calls
with 2–8 ordered arguments. Arguments use the same exact metric/arithmetic/constant
tree and native numeric-type constraints. This permits an explicit definition such
as `coalesce(refund_amount,0)` without inventing zero-fill from source nullability,
an absent join lane, an empty result or prose. The canonical tree retains argument
order and every fallback. SQL COALESCE must match that tree exactly; an unreviewed
COALESCE, a different fallback, a missing fallback or row-level substitution inside
an aggregate fails. Older analytical versions do not acquire this expression policy.

The required numeric expression family now includes reviewed aggregate leaves,
independent metric filters, nested KPI arithmetic, exact constants, unary signs,
NULLIF and equivalent CASE denominator guards, and explicit reviewed COALESCE.
Calendar/direct grouping and admitted ordering/limits remain separate exact parts
of the same contract. This is typed semantic completeness for the published closed
contracts, not a claim of arbitrary SQL equivalence or exhaustive natural-language
understanding. Calibration/general-language quality remains an independent owner
cohort requirement; unsupported typed forms are not counted as implemented support.

Actual tests distinguish all-NULL/empty population zero fallback from null-preserving
SUM, verify no unrequested filter/limit, preserve old-policy replay, reject dropped
markers, exercise bounded correction and verify durable terminal results. Their
executed source/toolchain evidence belongs in the completion ledger.

## Retained private predicate continuation boundary

Exact replay of a retained v6 query preserves its private model-parameter slots and
original population policy. A grouping refinement creates a current v7 child;
it cannot inherit authority for an unreviewed predicate merely from the old SQL.
Such a refinement returns a value-free insufficient-context review request before
model generation or source execution, while the parent and private values remain
unchanged. Native validation/EXPLAIN checks the original proof before the transition.
Current reviewed owned predicates continue across grouping edits.

Successful grouping changes over legacy unreviewed predicates remain a concrete
continuation gap: they require a versioned, exact-origin inherited-predicate proof
that cannot authorize newly invented filters. Existing S9 continuity must not be
reported as unchanged until that policy and its runtime proof are implemented.
