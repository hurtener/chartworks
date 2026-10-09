# Synthetic native presentation journey

`presentation-native.json` is an unchanged export of public request and response
DTOs from `TestReportAppPresentationCapture` in
`test/acceptance/report_app_presentation_capture_test.go`. The test uses isolated
real PostgreSQL storage and a real PostgreSQL synthetic source. Browser replay is
separate evidence and is not live browser-to-database or deployed-host evidence.

## Provenance and regeneration

- Frozen implementation base: `3806aba74b5d2df2cd4dbfeb4ff30f6ab0d337e5`.
- Optional capture: set `CHARTWORKS_PRESENTATION_DTO_PATH` to an external path and
  run `go test -race ./test/acceptance -run '^TestReportAppPresentationCapture$' -count=1 -timeout=10m -v`
  in the repository's documented isolated native test runtime.
- Runtime: Go 1.27.1, PostgreSQL 17.11, `GOMAXPROCS=2`, `GOMEMLIMIT=768MiB`,
  `GOFLAGS=-p=1`. The isolated cluster is stopped when the command exits.
- Captured 2026-10-04. Native capture test PASS in 38.35 seconds; the capture
  and public-DTO scanner roots passed together under race detection, package
  39.625 seconds, zero failures/skips.
- Recording: 746,390 bytes (1 MiB maximum).
- Recording SHA-256:
  `36b42e0dc6c6a7a3a13e8453e10da5ff8a4b3a859c9edfd57c160dd627f0713b`.
- No source connection information belongs in this recording or README.

The optional exporter never normalizes timestamps, resource identities, revision
numbers, exact values, result shapes or digests. Regeneration naturally changes
native identifiers and timestamps and corresponding evidence/definition pins
where relevant. Those changes must travel together; do not relabel retained
responses to fit browser inputs. Any browser clock/request translation must be
explicitly tested and disclosed in its own adapter.

The source fixture contains only literal synthetic rows. The canonical table
contains two dates, exact decimal amounts `9007199254740993.125` and `5.500`, and
small floating-point amounts projected by native SQL. Canonical USD/revenue
formatting and reviewed source/topic provenance are assigned in the native
fixture before source publication; browser code does not construct them.
The modern KPI uses the small-amount column, the last row of descending dates,
and `previous_row` comparison, with retained delta, percent delta and sparkline.
Its exact value is `1.2345e-07`, comparison `2.7e-07`, delta `-0.00000014655`,
and percent delta `-54.278`. All remain byte-identical after the zero-digit
presentation override. Both values have actual native scientific notation; the
browser does not convert a decimal fixture to create this scenario.

## Exact public contract

- `source_block` and `source_block_after` are native SQL-free
  `AuthoringBlockView` DTOs for the same published immutable source revision.
- `initial_report`, `initial_drafts`, report-scoped `capabilities`, global
  `global_capabilities` and the empty `published_catalog` come from their actual
  native services. Each corresponding request is included.
- `stages.source` retains the initial private report's real baseline execution.
- `stages.copied_table`: copy only the table output to a new private block;
  Revenue becomes Displayed total with two fraction digits, and Small amount
  gets zero digits. The independent report Save pins that exact private copy.
- `stages.amended_table`: amend the private copy; Revenue becomes Revised display
  with zero digits. The scientific field's prior override remains unchanged.
- `stages.reset_table`: Reset clears both Revenue overrides and the Small amount
  precision override. The native definition digest equals the original source
  definition, but the new private revision has no inherited validation.
- `stages.formatted_kpi`: copy the published source KPI to a separate private
  block with zero digits on Small amount. Exact retained value, comparison,
  delta, percent delta and sparkline remain identical to the baseline.

Each stage includes the real block read/mutation response, exact report Save
request/response where applicable, authoritative report reopen, draft catalog,
separate explicit validation request/response and validated block, private
preview admission/execution request/response, retained root, and retained table,
KPI and Notes selections. A formatting Save does not save the report implicitly.
Validation is separate from presentation edits and does not publish. Notes and
all report identity/page/widget coordinates remain unchanged.

`response_contract.errors.duplicate_copy` records the genuine duplicate-target
error from both the registered HTTP handler and MCP adapter, using the already
created table-copy request. The response is not a guessed browser error shape.

## Counters and custody

Every stage's `counts` records `source_reads` (physical source attempts),
`source_lookups` and `model_calls` for distinct phases. Copy/amend/Reset, report
Save/reopen, preview admission, retained reads/redraws and rejection perform zero
source or model work. Explicit validation performs one source read. Explicit
preview execution performs the native admitted query-group count. The baseline
has one source group; later stages may contain separate immutable block pins.
The capture totals 13 physical reads and 30 source lookups: baseline preview
1 read/2 lookups, then each of four stages has validation 1 read/3 lookups and
preview execution 2 reads/4 lookups. Every metadata-only phase records 0/0/0.
The final aggregate is asserted equal to the sum of explicit work, catching any
unaccounted source/model activity between phase measurements.
Model counters remain unchanged throughout the presentation journey. Setup uses
only the existing synthetic/recorded fixtures, with no live provider.

The exporter always scans the resulting public DTO tree, even when file export
is disabled. It enforces a 1 MiB byte bound, 60,000-node bound and depth 64, and
rejects SQL, credentials/tokens/sessions/identity envelopes, native attempt/control
records and connection-string/authorization material. The only attempt-key
exception is the actual public four-field QueryLimits shape: max_rows, max_bytes,
timeout_ms and the scalar query_attempts ceiling (0–3), all natively bounded
integers. An operational query_attempts array/object, any extra field, or the same
name outside query_limits is rejected. Pure scanner regressions exercise those
boundaries as well as SQL/value, recursion, node-count and byte limits. Full block definitions and
operational source records are never exported. Report definitions are the public
manual-authoring DTOs and contain only safe widget references and text. Ordinary
resource, context, provenance, revision and digest pins remain intact.

Native assertions, recursive export hygiene, strict recorded-request replay and
actual browser interaction are distinct checks. This recording does not claim a
published Consumer report, real host admission, live authority, live model use,
or screenshot qualification. All retained report runs here are private previews.
