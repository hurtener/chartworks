// Package querydiagnostic is the closed, value-free vocabulary for confirmed
// native query rejections. A diagnosis never proves termination, safe SQL or
// permission to retry; each consumer must check those independent boundaries.
package querydiagnostic

// Version identifies the diagnostic vocabulary, not an analytical proof version.
const Version = "query-diagnostics-v1"

// Codes returns detached stable reason identifiers. The historical generic
// query_error remains valid and does not imply a more specific observed cause.
func Codes() []string {
	return []string{"query_error", "query_division_by_zero", "query_numeric_range", "query_invalid_text", "query_invalid_datetime", "query_datetime_range", "query_cardinality", "query_function_signature", "query_type_mismatch", "query_grouping", "query_windowing"}
}

// Known accepts complete exact identifiers, not SQLSTATE prefixes or user text.
func Known(code string) bool {
	switch code {
	case "query_error", "query_division_by_zero", "query_numeric_range", "query_invalid_text", "query_invalid_datetime", "query_datetime_range", "query_cardinality", "query_function_signature", "query_type_mismatch", "query_grouping", "query_windowing":
		return true
	}
	return false
}

// Postgres returns a diagnostic only for individually reviewed SQLSTATEs. It
// never examines localized messages, detail, hints, object names or source rows.
// Permissions, cancellation, resource exhaustion, transport and transaction
// states are deliberately not admitted here, even if their messages resemble SQL.
func Postgres(sqlstate string) string {
	switch sqlstate {
	case "22012":
		return "query_division_by_zero"
	case "22003":
		return "query_numeric_range"
	case "22P02", "22018":
		return "query_invalid_text"
	case "22007":
		return "query_invalid_datetime"
	case "22008":
		return "query_datetime_range"
	case "21000":
		return "query_cardinality"
	case "42883", "42725":
		return "query_function_signature"
	case "42804", "42P18":
		return "query_type_mismatch"
	case "42803":
		return "query_grouping"
	case "42P20":
		return "query_windowing"
	}
	return ""
}

// Hint is server-authored repair guidance, never a driver-provided suggestion.
// It does not prescribe a different population, private value, null policy or
// result grain to make an error disappear. Unsupported correction must stop.
func Hint(code string) string {
	switch code {
	case "query_division_by_zero":
		return "A denominator evaluated to zero. Preserve the reviewed arithmetic and zero-denominator policy; do not invent a row filter, replacement value or NULL policy. Stop for clarification if that policy is unresolved."
	case "query_numeric_range":
		return "A numeric value exceeded its expression type. Preserve exact arithmetic, units and result meaning. Do not round, clamp, omit rows or substitute private bindings to avoid the error."
	case "query_invalid_text":
		return "A value could not be converted to the expression's type. Review the admitted column/parameter types. Do not include the rejected value, guess replacement data or drop records."
	case "query_invalid_datetime":
		return "A temporal conversion rejected its input format. Use the reviewed temporal types and calendar policy, not a guessed format or replacement binding."
	case "query_datetime_range":
		return "A temporal expression exceeded its valid range. Keep the reviewed calendar, timezone and interval boundaries. Do not narrow the population or change a private binding."
	case "query_cardinality":
		return "A scalar expression produced more than one row. Preserve the requested cardinality and reviewed relationships; do not add an arbitrary LIMIT, aggregation or join to hide the conflict."
	case "query_function_signature":
		return "A function/operator overload did not resolve unambiguously for these arguments. Use the allowed vocabulary and reviewed types; no unlisted function or namespace is authorized."
	case "query_type_mismatch":
		return "The expression argument types were incompatible or indeterminate. Preserve the parameter slot kinds and reviewed physical types. Do not change bindings or coerce through lossy arithmetic."
	case "query_grouping":
		return "The projection or predicate is incompatible with SQL grouping. Preserve exactly the selected metric and grouping; do not add hidden grouping columns or replace the metric with a different aggregate."
	case "query_windowing":
		return "The window expression is invalid in this query context. Preserve the requested partition, ordering, frame and cardinality; do not remove the window meaning to make SQL pass."
	case "query_error":
		return "The source rejected the validated query; no more specific diagnostic was observed. Do not infer private values or source error text."
	}
	return ""
}
