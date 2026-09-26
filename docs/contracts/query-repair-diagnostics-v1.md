# Closed query-repair diagnostics

Status: S7 implementation checkpoint in PR #62; exact-source runtime qualification
is recorded in the completion tracker and PR. This contract is not new authority,
a larger attempt budget, or cross-engine/analytical correctness certification.

## One value-free classification

The source adapter classifies individually reviewed PostgreSQL SQLSTATEs at native
EXPLAIN and actual read boundaries. It never classifies localized message text,
detail, hints, schema/column names, positions, internal SQL or result values. The
same driver error at a metadata boundary remains an unavailable metadata outcome,
not an invitation to fix user SQL. `internal/exec/querydiagnostic` is the single
closed code/static-guidance registry shared by consumers.

| Observed cause | Native states | Stored diagnostic |
|---|---|---|
| Division by zero | 22012 | query_division_by_zero |
| Numeric overflow/range | 22003 | query_numeric_range |
| Invalid conversion text | 22P02, 22018 | query_invalid_text |
| Invalid date/time format | 22007 | query_invalid_datetime |
| Date/time field range | 22008 | query_datetime_range |
| Scalar cardinality | 21000 | query_cardinality |
| Missing/ambiguous function overload | 42883, 42725 | query_function_signature |
| Incompatible/indeterminate type | 42804, 42P18 | query_type_mismatch |
| SQL grouping error | 42803 | query_grouping |
| SQL window context | 42P20 | query_windowing |

The PostgreSQL 17 Appendix A definitions control these native categories. This
version does not assume other engines use the same numbers or text. Unlisted
states are not assigned guessed diagnoses. Historical generic `query_error`
remains generic; the existing generic class-22 read disposition is preserved.
Permissions, changed context, timeout/cancellation, connection loss, transaction
conflicts, resource exhaustion and unknown source/ledger outcomes are not repair
reasons, even when joined to a classified query exception.

The concrete in-process error wraps only the existing ErrQuery sentinel and a
closed code, not its originating error. Error text stays generic. Classified
identity survives the source-revision metadata boundary without keeping driver
prose. Formatting/JSON cannot expose the discarded source error.

## Existing bounded consumers

Validation correction retains the failed unbound SQL, positions/kinds and reviewed
instructions. It additionally receives the closed diagnostic and its static
server-authored guidance before complete effective-envelope fitting. Original
private bindings remain server-side and replace model placeholder values as
before. Runtime correction gets the same guidance through its existing suffix.
No extra generation, model role or attempt allowance is introduced.

A diagnostic alone never proves that execution stopped. Specific runtime reasons
require a failed, finished, row-free, explicitly stopped physical receipt. A
conflicting error identity, lost ledger, uncertainty, cancellation, changed source
or access denial wins over the query cause. The existing historical generic
explicit-error seam remains compatible; nil-error receipt consumption still
requires its durable stopped outcome. Retry reuses the existing one-correction
policy. SQL/parameter equivalence and all source/native/analytical checks remain
mandatory; a useful hint cannot authorize a changed denominator, hidden filter,
arbitrary LIMIT or replacement binding. A model may stop for clarification rather
than manufacture an equivalent repair that does not exist.

## Persistence and compatibility

Forward migration 060 extends only the existing read-attempt code enum. New
specific codes require failed, zero-row/byte, finished outcomes and stopped or
not-issued state. Not-issued is valid journal evidence but does not authorize
physical correction. The immutable manifest/terminal guard is preserved. There is
no SQLSTATE/message/error-detail column and no historical backfill. Public surfaces
continue to use their existing generic query-error family; the attempt receipt can
carry the closed reason. Historical replay does not infer more specific causes.

Frozen reporting receives the same content-free source classifications but gains
no model call or hidden retry. Source grants, read-only sessions, credential checks,
result schemas, parameter kinds, analytical versions and public operations stay
unchanged. Other engines keep their existing outcomes until separately qualified.

## Tests and qualification

Pure tests independently specify all native mappings, excluded terminal states,
registry copies/concurrency and unknown-value handling. Native adapter tests cover
redaction and safe-boundary preservation. NLQ tests cover every code in the actual
repair packet, private parameter restoration, contradicted/missing/partial receipts
and cancellation/journal/access exclusions. Migration tests retain the closed enum
and outcome shape. The existing real division-error regression now expects the
specific code while preserving its original two-attempt ceiling and result checks.

Recorded-provider/actual-PostgreSQL acceptance covers EN/ES signature, type and
grouping correction; native conversion/date/range/window categories; private
invalid-text values; data-dependent division/cardinality; a blocked correction,
persisted diagnostic, saved-result privacy and no-extra-work replay. Native error
classification tests do not themselves count as corrected-result qualification;
actual source IDs are checked for the supported corrected queries. This is not a
claim that every diagnostic has a safe automatic rewrite or that live models will
choose it. S4/S5/S6/S11 and Q1/Q2/Q3 remain separate obligations.
