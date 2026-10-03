package exec

import (
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

func (r *sqlResolver) functionSignature(m map[string]any, name string) bool {
	args := array(m["args"])
	types := make([]string, len(args))
	for i, arg := range args {
		types[i] = signatureExpressionType(arg)
	}
	return sqlpolicy.AllowsCall("postgres", name, types, truth(m["agg_star"]), m["over"] != nil, truth(m["agg_distinct"]), m["agg_filter"] != nil, len(array(m["agg_order"])) > 0, truth(m["agg_within_group"]))
}

// Only AST evidence with an unambiguous primitive type is used here. Bare
// string/NULL literals have PostgreSQL's unknown type, and column/parameter/
// derived expression types are resolved by the mandatory source EXPLAIN. Never
// infer a column type from its spelling: aliases and CTEs can shadow base names.
func signatureExpressionType(v any) string {
	m := object(v)
	if c := object(m["A_Const"]); c != nil {
		if c["ival"] != nil {
			return "integer"
		}
		if c["fval"] != nil {
			return "numeric"
		}
		if c["boolval"] != nil {
			return "boolean"
		}
	}
	if c := object(m["TypeCast"]); c != nil {
		p, ok := names(fieldObject(c["typeName"], "TypeName")["names"])
		if !ok || !sqlpolicy.AllowsPostgresType(p) {
			return "unknown"
		}
		switch p[len(p)-1] {
		case "int2", "int4", "int8":
			return "integer"
		case "numeric":
			return "numeric"
		case "float4", "float8":
			return "float"
		case "text", "varchar", "bpchar":
			return "text"
		case "bool":
			return "boolean"
		case "date", "timestamp", "timestamptz", "time", "timetz":
			return "temporal"
		case "interval":
			return "interval"
		case "bytea":
			return "binary"
		}
	}
	if c := object(m["FuncCall"]); c != nil {
		p, ok := names(c["funcname"])
		if !ok || !sqlpolicy.AllowsFunction("postgres", p) {
			return "unknown"
		}
		args := array(c["args"])
		types := make([]string, len(args))
		for i, arg := range args {
			types[i] = signatureExpressionType(arg)
		}
		return sqlpolicy.FunctionResultType("postgres", p[len(p)-1], types)
	}
	return "unknown"
}

func (r *sqlResolver) specialSignature(name string, args []any) bool {
	types := make([]string, len(args))
	for i, arg := range args {
		types[i] = signatureExpressionType(arg)
	}
	return sqlpolicy.AllowsExpression("postgres", name, types)
}
