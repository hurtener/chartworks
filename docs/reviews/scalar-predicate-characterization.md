# Scalar predicate characterization

Historical baseline evidence. The separately designed
[scalar entailment continuation](../contracts/analytical-scalar-entailment-v1.md)
now addresses this case under its own proof, version and qualification gates.
The executed results below describe the unchanged baseline at the stated commit.

## Finding

There is a bounded, user-meaningful missing case in the current implementation:
an explicit reviewed paid-order selection prevents an otherwise executable scalar
net query, even when every selected measure population already entails that exact
selection. This is a fresh characterization, not a recovered design or a new
runtime capability. No production code, receipt version or migration is changed.

The existing [scalar contract](../contracts/analytical-scoped-populations-v1.md)
explicitly excludes unrelated generic owned constraints. The observed behavior
is consistent with that broad limitation. The case below identifies a narrower
candidate for a separately designed extension; it does not silently amend the
contract or authorize general predicate propagation.

## Concrete request and logical basis

The generated, reviewed synthetic topic defines paid gross less posted refunds
on paid parent orders, with separate cohort and activity period meanings. A
user selecting the governed `orders_paid` value explicitly is asking for the
same paid-order restriction already required by the definitions.

The selected closure has six measure leaves: two SUMs and four COUNTs supporting
the two unknown-amount companions. Each order leaf requires order status `P`.
Each refund leaf requires both posted-refund status and parent-order status `P`
through the same reviewed `confirmed_refunds_to_orders` INNER relationship.
The source-backed complete division/order key and the exact relationship remain
mandatory. A same-named field on a different fact is not equivalent evidence.

Conjoining that exact paid-parent predicate cannot remove any row contributing
to any selected aggregate. SQL NULL behavior is retained: NULL status never
satisfies equality, SUM over all-NULL or empty input remains NULL, and unknown
COUNT differences distinguish those populations. This reasoning concerns scalar
aggregate outputs only; it makes no claim about grouped row domains.

## Executed evidence

Production baseline: `bde060fefd3b760d0420e055f07ab08577d2cdbf`.
Baseline tree: `c4855946901917bc1c8aab167cdab993e63b4602`.
The only test-time changes are the two new characterization files below.
Execution used Go 1.27.1, PostgreSQL 17, the existing native parser, shared caches,
`-p=1`, `GOMAXPROCS=2` and `GOMEMLIMIT=768MiB` on 2026-10-03 UTC.

1. `go test -p=1 -count=1 -run '^TestScalarPredicateCharacterization' -v ./internal/nlqexec`
   passed. The ordinary scoped controls compile. Explicit redundant paid
   predicates return `analytical_shape_unsupported` under retained v9/v10 and
   current v12 dispatch. Seven existing-negative-style mutations remain denied:
   one selected leaf without entailment, nonentailed value, wrong fact, wrong
   source, wrong revision, wrong relationship, and NULL-only selection.
2. Built `./test/acceptance` once with `go test -p=1 -c` and ran only
   `TestScalarPredicateCharacterizationPublicBoundary` against an isolated real
   PostgreSQL 17 fixture. It passed in 14.10 seconds. Recorded provider responses
   supply authoring/generation controls; they are not live-provider evidence.
3. The public service's unfiltered scalar Plan/Run controls match a separate Go
   row oracle using exact rational arithmetic, complete parent keys and civil
   calendar periods. Output order is net, unknown orders, unknown refunds:
   - Numeric cohort: `[813.00, 1, 1]`
   - Numeric activity: `[805.00, 1, 1]`
   - All-NULL cohort and activity: `[NULL, 11, 8]`
   - Empty cohort and activity: `[NULL, 0, 0]`
4. For all six controls, the independent oracle is unchanged by explicitly
   conjoining paid-order selection. Public Preflight resolves the reviewed
   selection, service Plan returns `analytical_shape_unsupported`, and HTTP/SDK
   Plan returns an error without a query ID. Neither refusal makes an additional
   SQL-generation call. The SDK assertion establishes refusal, not a particular
   wire error code.

The unit test is compiler-only evidence. The second test separately establishes
the public service/source and SDK-refusal behavior for this synthetic case. It
does not establish successful execution of the requested explicit-predicate
query: that is precisely what current production refuses.

The native CGo identity reported by the environment was
`5f562c2959496a04d57f5f199f5e3ad22159fa9f` plus
`b68325ec48d9055c08fa7afcecc39035d8b023fb18c4bc3d0cb5fa404508e5b0`.
Characterization source SHA-256 values at execution:

- `internal/nlqexec/scalar_predicate_characterization_test.go`:
  `7da35c6eaa8a60745a5f5305875d568bc5bdbd53f609f34cff8f4cc17af6c8ed`
- `test/acceptance/scalar_predicate_characterization_test.go`:
  `a575722fae1dabdb5694ec23e2f15fcb27816b803d913bb603485f6051e10aa4`

## Limits and next decision

No general entailment solver, predicate discharge rule, binder behavior, durable
receipt, replay policy, learning eligibility, migration or new analytical
version is proposed here. Existing negatives are preserved by the current broad
refusal, not by a newly implemented distinction between safe and unsafe cases.
The public test covers exact text equality; it does not qualify finite sets,
numeric intervals, arbitrary Boolean logic, other facts, outer joins, grouped
programs or other engines. This focused non-race run is not the full suite,
hosted CI, live model quality, release qualification or broader feature parity.

Any implementation needs its own reviewed proof contract covering every selected
transitive leaf, exact source/relationship identity, NULL semantics, protected
request evidence, retained replay, and failure when even one leaf does not imply
the request. The current characterization deliberately stops before that design.
