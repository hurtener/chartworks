package sqlpolicy

import "strings"

func warehouseFunctionVocabulary(dialect string) string {
	switch dialect {
	case "mysql":
		return warehouseFunctions + " date_format extract makedate"
	case "sqlserver":
		return warehouseFunctions + " datetrunc"
	case "bigquery":
		return warehouseFunctions + " date_trunc datetime_trunc timestamp_trunc"
	case "snowflake", "databricks":
		return warehouseFunctions + " date_trunc"
	default:
		return warehouseFunctions
	}
}

// CalendarArgument identifies an exact syntax position, not a data column.
func CalendarArgument(dialect, name string, index int) (string, bool) {
	s, ok := FunctionSignature(dialect, name)
	return s.UnitForm, ok && s.UnitForm != "" && index == s.UnitIndex
}

// CalendarUnit checks the closed reviewed calendar grain vocabulary. String
// units and native keyword units are deliberately different grammar contracts.
func CalendarUnit(dialect, name string, index int, form, value string) (string, bool) {
	s, ok := FunctionSignature(dialect, name)
	if !ok || s.UnitForm != form || s.UnitForm == "" || s.UnitIndex != index {
		return "", false
	}
	for _, unit := range s.Units {
		if strings.EqualFold(unit, value) {
			return unit, true
		}
	}
	return "", false
}
