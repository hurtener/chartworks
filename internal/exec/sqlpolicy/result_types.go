package sqlpolicy

import "strings"

// resultTypes records only primitive families that the source grammar proves.
// PostgreSQL's integer family includes multiple widths, so SUM's promotion is
// deliberately a union rather than a fabricated exact numeric type.
func resultTypes(dialect, name string) map[string]string {
	switch name {
	case "sum", "avg", "round", "abs", "ceil", "ceiling", "floor":
	default:
		return nil
	}
	out := map[string]string{"numeric": "numeric", "float": "float", "integer": "integer"}
	switch name {
	case "sum":
		switch dialect {
		case "postgres":
			out["integer"] = "integer|numeric"
		case "mysql", "snowflake":
			out["integer"] = "numeric"
		}
	case "avg":
		out["integer"] = "numeric"
		if dialect == "sqlserver" {
			out["integer"] = "integer"
		}
		if dialect == "bigquery" || dialect == "databricks" {
			out["integer"] = "float"
		}
	case "round":
		switch dialect {
		case "postgres", "snowflake":
			out["integer"] = "numeric"
		case "bigquery":
			out["integer"] = "float"
		}
	case "ceil", "ceiling", "floor":
		if dialect == "postgres" {
			out["integer"] = "numeric"
		}
	}
	if (name == "sum" || name == "avg") && (dialect == "postgres" || dialect == "bigquery" || dialect == "databricks") {
		out["interval"] = "interval"
	}
	return out
}

// FunctionResultType never upgrades unknown input or a union to exact type
// evidence. Implicit conversions and unresolved native widths remain the source
// planner's responsibility. Both inference consumers use these same rules.
func FunctionResultType(dialect, name string, arguments []string) string {
	s, ok := FunctionSignature(dialect, name)
	if !ok {
		return "unknown"
	}
	name = strings.ToLower(name)
	if len(s.ResultByInput) > 0 {
		if len(arguments) == 0 {
			return "unknown"
		}
		result := s.ResultByInput[arguments[0]]
		if result == "" || strings.Contains(result, "|") {
			return "unknown"
		}
		return result
	}
	if s.Result == "same" {
		if len(arguments) == 0 {
			return "unknown"
		}
		if name == "coalesce" {
			return commonResultType(arguments)
		}
		if (name == "lag" || name == "lead") && len(arguments) == 3 {
			return commonResultType([]string{arguments[0], arguments[2]})
		}
		return arguments[0]
	}
	if strings.Contains(s.Result, "|") {
		return "unknown"
	}
	return s.Result
}

func commonResultType(arguments []string) string {
	result := ""
	for _, a := range arguments {
		if a == "unknown" {
			return "unknown"
		}
		if result == "" || result == a {
			result = a
			continue
		}
		if !numericType(a) || !numericType(result) {
			return "unknown"
		}
		if result == "float" || a == "float" {
			result = "float"
		} else {
			result = "numeric"
		}
	}
	if result == "" {
		return "unknown"
	}
	return result
}
