package exec

import "strings"

func warehouseDecimalBounds(dialect, name string, typ map[string]any) (int, int, bool) {
	precision, hasPrecision := typ["precision"].(float64)
	scale, hasScale := typ["scale"].(float64)
	if typ["precision"] != nil && !hasPrecision || typ["scale"] != nil && !hasScale {
		return 0, 0, false
	}
	if !hasPrecision {
		switch dialect {
		case "bigquery":
			precision, scale = 38, 9
			if name == "big_numeric" {
				precision, scale = 76, 38
			}
		case "snowflake":
			precision = 38
		case "sqlserver":
			precision = 18
		default:
			precision = 10
		}
	}
	if precision < 1 || precision > 76 || scale < 0 || scale > precision || precision != float64(int(precision)) || scale != float64(int(scale)) {
		return 0, 0, false
	}
	return int(precision), int(scale), true
}
func warehouseExactDecimalText(value string, precision, scale int) bool {
	if _, ok := analyticalNumber(value); !ok {
		return false
	}
	unsigned := strings.TrimPrefix(strings.TrimPrefix(value, "-"), "+")
	integer, fraction, _ := strings.Cut(unsigned, ".")
	return len(strings.TrimLeft(integer, "0")) <= precision-scale && len(strings.TrimRight(fraction, "0")) <= scale
}

// Only widen integral values or parse an exact decimal string. Arbitrary
// decimal-to-decimal casts may round, so their precision cannot be erased.
func (n *warehouseAnalyticalNormalizer) numericCast(typ map[string]any, value any) (any, bool, error) {
	name := text(typ["data_type"])
	precision, scale, ok := warehouseDecimalBounds(n.binding.Dialect, name, typ)
	if !ok {
		return nil, false, nil
	}
	if lit := object(object(value)["literal"]); lit != nil && text(lit["literal_type"]) == "string" {
		if !warehouseAnalyticalFields(lit, "literal_type", "value") || !warehouseExactDecimalText(text(lit["value"]), precision, scale) {
			return nil, false, nil
		}
		// The source engine parses this string directly as an exact decimal, rather
		// than first rounding through an untyped FLOAT64 literal.
		return warehouseConst(text(lit["value"]), false), true, nil
	}
	integer := false
	if call, _, _, ok := warehouseCall(object(value)); ok && call == "count" {
		integer = true
	}
	if col, ok := n.calendarColumn(value); ok {
		view := warehouseAnalyticalRelation(n.binding.Dialect, Relation{Columns: []Column{col}})
		integer = analyticalNumericKind(view.Columns[0]) == "integer"
	}
	if !integer || precision-scale < 20 {
		return nil, false, nil
	}
	return nil, true, nil
}
