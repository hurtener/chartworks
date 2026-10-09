# Bounded dataset filter core

2026-10-04. Local native-core checkpoint, based on `0979c72`; qualified source
commit `5aa72d6`. This does not claim filter-editor, option-search or release
completion.

## Behavior

Dataset chart intent can declare up to four reviewed field filters. The server
resolves their physical columns, bind names, parameter declarations and positive
predicates. Existing parameter-free compiler-v1 custody remains valid. Filtered
preparations use compiler v2 and the existing native block schema 2 with explicit
output intent. They still create private unvalidated drafts; validation and
preview are independent explicit source reads.

- Text select uses a required `dimension_value` default and equality.
- `dimension_set` requires 1–16 distinct exact text values. Values sort into a
  fixed sixteen-slot bind list; unused slots contain SQL NULL. Actual NULL
  selection and implicit All are unavailable. Empty text is an ordinary value.
- `date_range` has a required canonical `start`/`end_exclusive` ISO date pair.
  PostgreSQL date columns only; no timestamp/calendar policy is inferred.
  Binding ignores local timezone and accepts at most a 100-year-scale interval.
  The UI labels its inclusive end explicitly and converts it by one
  calendar day, rejecting overflow before submission.

The existing sixty-four-slot and source execution ceilings remain enforced.
Selected mandatory semantic filters, calendar/completeness/group policies,
active rules and multi-relation compilation remain unsupported.

## Native SQL proof

The new parameter types require native parser proof, including for arbitrary
native block creation and import. Protected bind slots appear exactly once in a
root WHERE conjunction: the complete positive IN list or a same-column >= / <
pair. NOT IN, negation, OR, partial/reused slots and casts are rejected. Nested
SELECTs, scalar subqueries, CTEs and set operations cannot return an unfiltered
population while filtering only the outer query.

Current authorized metadata resolves one direct source relation and checks each
protected column against the exact reviewed dimension, source/context/revision,
physical type, nullability and safety. Plain, table/schema-qualified and alias
references retain that identity. Other columns, ambiguous relation shapes and
column-renaming aliases are rejected. These restrictions apply only to the new
parameter types; legacy scalar/fixed-list SQL and omitted JSON fields retain
their existing behavior and digest surface.

The first audit found that an unfiltered scalar subquery could bypass the outer
predicate's meaning. Commit `3c365d0` added whole-tree nested-query rejection and
regressions before qualification. No caller provenance is trusted as a substitute
for these checks.

## Focused evidence

The dedicated local PostgreSQL 17 fixture and Go cache were separate from the
other project's runtime. Serial execution used `-race`, `-p=1`, GOMAXPROCS 2 and
GOMEMLIMIT 768 MiB. The process completed successfully and stopped its fixture.

Selected tests in `internal/reporting` and `internal/store/postgres` passed:

- New typed filter compiler, declaration/binder and SQL/semantic-column proof.
- Filtered preparation/custody, existing preparation replay and overflow/unknown
  outcome tests, rule-absence/derived-origin tests.
- Existing scalar and fixed-list parameter resolution/rejection tests.

Five real PostgreSQL acceptance roots passed:

- `TestReportAppTypedFilterJourney`: fresh source/profile publication, positive
  IN with one/two/sixteen selected values and NULL source rows, exact empty text
  and injection-shaped values, half-open date boundaries, private create/save /
  separate validation / explicit report runs, and unchanged saved defaults.
  The fixture's six runs plus Prepare and Validate produce exactly eight reads,
  with no model calls during the measured journey. Native NOT IN and mislabeled
  column definitions reject. Rule activation rejects further validation and a
  previously sealed run without another source query.
- `TestReportAppSelectFilterJourney`: actual PostgreSQL equality-filter result.
- `TestReportAppDeterministicPreparation`: existing parameter-free private
  chart/report journey remains valid.
- `TestReportAppPreparedRuleActivationDuringValidation`.
- `TestReportAppPreparedRulePublicationFence`, including Publish and Certify.

This was focused qualification, not full packages, all source dialects or a full
release gate. No native DTO fixture was exported or published by this work.

## Subsequent integration

The governed option custody, native metadata, HTTP/MCP/SDK and manual filter
controls are integrated locally. See [option custody](report-app-option-custody.md)
and [manual filter integration](report-app-filter-integration.md) for exact later
proof. The original focused evidence above is not a substitute for those runs.

Saved business-filter defaults belong in
`ReportPage.Filters[].Parameter.Default`. `ReportPage.Defaults` is a distinct
fallback layer keyed by block parameter names and must be preserved when editing
or removing a business filter. Temporary selections use `PageInput.Filters`.
Editing controls does not read source data. Inclusive-end conversion is civil-date
arithmetic independent of browser time zone. Current hosted browser and genuine
host allocation qualification remain outstanding.
