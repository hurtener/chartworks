package exec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec/signatureparser"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"strings"
)

// Call evidence is extracted by the same pinned warehouse grammar. This pass
// supplements, and never replaces, positive statement/dependency inspection.
func warehouseFunctionSignatures(ctx context.Context, sql, dialect, native string, nodes, depth int) error {
	_, err := warehouseSignatureEvidence(ctx, sql, dialect, native, nodes, depth)
	return err
}

func warehouseSignatureEvidence(ctx context.Context, sql, dialect, native string, nodes, depth int) (warehouseCallEvidence, error) {
	evidence := warehouseCallEvidence{}
	root, err := signatureparser.Inspect(ctx, sql, native, nodes, depth)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return evidence, err
		}
		return evidence, ErrUnsupported
	}
	evidence.tree = root
	visited := 0
	var walk func(any, bool) error
	walk = func(v any, over bool) error {
		visited++
		if visited > 100000 {
			return ErrUnsupported
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if list, ok := v.([]any); ok {
			for _, item := range list {
				if err := walk(item, false); err != nil {
					return err
				}
			}
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		if len(m) == 1 {
			if w := object(m["window_function"]); w != nil {
				if w["this"] == nil || w["over"] == nil || w["keep"] != nil {
					return ErrUnsupported
				}
				if err := walk(w["this"], true); err != nil {
					return err
				}
				return walk(w["over"], false)
			}
			if object(m["within_group"]) != nil {
				return ErrUnsupported
			}
			name, args, body, call := warehouseCall(m)
			if call {
				if dialect == "sqlserver" && name == "len" {
					name = "length"
				}
				if body["ignore_nulls"] != nil || body["limit"] != nil || body["having_max"] != nil || truth(body["use_bracket_syntax"]) || truth(body["no_parens"]) || truth(body["quoted"]) {
					return ErrUnsupported
				}
				types := make([]string, len(args))
				star := truth(body["star"])
				for i, arg := range args {
					if _, ok := object(arg)["star"]; ok {
						if len(args) != 1 {
							return ErrUnsupported
						}
						star = true
						types = nil
						break
					}
					if form, unit := sqlpolicy.CalendarArgument(dialect, name, i); unit {
						if _, ok := warehouseCalendarUnit(dialect, name, i, arg); !ok {
							return ErrUnsupported
						}
						types[i] = "unit-" + form
						continue
					}
					types[i] = warehouseSignatureType(arg, dialect)
					if literal := object(object(arg)["literal"]); text(literal["literal_type"]) == "string" && !sqlpolicy.AllowsLiteralArgument(dialect, name, i, text(literal["value"])) {
						return ErrUnsupported
					}
				}
				if !sqlpolicy.AllowsCall(dialect, name, types, star, over, truth(body["distinct"]), body["filter"] != nil, len(array(body["order_by"])) > 0, false) {
					return ErrUnsupported
				}
				// Traverse every ordinary argument and modifier. Only the exact validated
				// unit occurrence is omitted; equal names elsewhere are still collected.
				for i, arg := range args {
					if _, unit := sqlpolicy.CalendarArgument(dialect, name, i); unit {
						continue
					}
					if err := walk(arg, false); err != nil {
						return err
					}
				}
				for key, child := range body {
					switch key {
					case "args", "this", "expression", "expressions", "decimals", "field":
						continue
					}
					if err := walk(child, false); err != nil {
						return err
					}
				}
				return nil
			} else if over {
				return ErrUnsupported
			}
		}
		for _, child := range m {
			if err := walk(child, false); err != nil {
				return err
			}
		}
		return nil
	}
	return evidence, walk(root, false)
}

// Every admitted function node has an explicit argument topology. Unknown
// callable nodes still fail the primary name gate; generic functions fail here.
func warehouseCall(m map[string]any) (string, []any, map[string]any, bool) {
	if len(m) != 1 {
		return "", nil, nil, false
	}
	for kind, v := range m {
		b := object(v)
		if b == nil {
			return "", nil, nil, false
		}
		name := kind
		var args []any
		switch kind {
		case "function", "aggregate_function":
			name = strings.ToLower(text(b["name"]))
			args = array(b["args"])
		case "count", "sum", "avg", "min", "max", "abs", "lower", "upper", "length":
			if b["this"] != nil {
				args = []any{b["this"]}
			}
			if explicit := text(b["name"]); explicit != "" {
				name = strings.ToLower(explicit)
			}
		case "round":
			if b["this"] != nil {
				args = append(args, b["this"])
			}
			if b["decimals"] != nil {
				args = append(args, b["decimals"])
			}
		case "nullif", "null_if":
			name = "nullif"
			if b["this"] != nil {
				args = append(args, b["this"])
			}
			if b["expression"] != nil {
				args = append(args, b["expression"])
			}
		case "extract":
			args = []any{map[string]any{"calendar_unit": strings.ToLower(text(b["field"]))}, b["this"]}
		case "coalesce":
			args = array(b["expressions"])
		default:
			return "", nil, nil, false
		}
		if original := text(b["original_name"]); original != "" {
			name = strings.ToLower(original)
		}
		return name, args, b, true
	}
	return "", nil, nil, false
}

func warehouseSignatureType(v any, dialect string) string {
	m := object(v)
	if _, ok := m["boolean"]; ok {
		return "boolean"
	}
	if lit := object(m["literal"]); lit != nil {
		switch text(lit["literal_type"]) {
		case "number":
			if strings.ContainsAny(text(lit["value"]), ".eE") {
				if dialect == "bigquery" || strings.ContainsAny(text(lit["value"]), "eE") {
					return "float"
				}
				return "numeric"
			}
			return "integer"
		case "string", "national_string", "raw_string", "escape_string", "dollar_string":
			return "text"
		case "hex_string", "byte_string":
			return "binary"
		case "date", "time", "timestamp", "datetime":
			return "temporal"
		}
	}
	if cast := object(m["cast"]); cast != nil {
		if dialect == "mysql" {
			if _, ok := mysqlUTCInstantShape(cast); ok {
				return "temporal"
			}
		}
		switch text(object(cast["to"])["data_type"]) {
		case "tiny_int", "small_int", "int", "big_int":
			return "integer"
		case "float", "double":
			return "float"
		case "decimal", "numeric", "big_numeric":
			return "numeric"
		case "char", "var_char", "string", "text", "text_with_length", "n_char", "n_var_char":
			return "text"
		case "boolean":
			return "boolean"
		case "binary", "var_binary", "bytes":
			return "binary"
		case "date", "time", "timestamp", "timestamp_tz", "datetime":
			return "temporal"
		case "interval":
			return "interval"
		}
	}
	if name, args, _, ok := warehouseCall(m); ok {
		if dialect == "sqlserver" && name == "len" {
			name = "length"
		}
		types := make([]string, len(args))
		for i, arg := range args {
			types[i] = warehouseSignatureType(arg, dialect)
		}
		return sqlpolicy.FunctionResultType(dialect, name, types)
	}
	return "unknown"
}
