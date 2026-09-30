package exec

import (
	"context"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec/signatureparser"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

// warehouseAnalyticalAST translates structural evidence, never SQL text. Every
// accepted node has an explicit topology; unhandled semantic modifiers fail.
func warehouseAnalyticalAST(ctx context.Context, sql string, binding Binding) (map[string]any, error) {
	dialect, ok := sqlpolicy.NativeDialect(binding.Dialect)
	if !ok || binding.Dialect == "postgres" {
		return nil, ErrBinding
	}
	root, err := signatureparser.Inspect(ctx, sql, dialect, 10000, 64)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, analyticalFailure("analytical_shape_unsupported", true)
	}
	n := warehouseAnalyticalNormalizer{ctx: ctx, binding: binding}
	return n.selectNode(root)
}

type warehouseAnalyticalNormalizer struct {
	ctx               context.Context
	binding           Binding
	nodes, parameters int
}

func warehouseEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case float64:
		return x == 0
	}
	return false
}
func warehouseAnalyticalFields(m map[string]any, allowed ...string) bool {
	if m == nil {
		return false
	}
	for key, value := range m {
		if strings.HasSuffix(key, "comments") {
			continue
		}
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found && !warehouseEmpty(value) {
			return false
		}
	}
	return true
}
func warehouseString(s string) any { return map[string]any{"String": map[string]any{"sval": s}} }
func warehouseNames(values ...string) []any {
	out := make([]any, len(values))
	for i, s := range values {
		out[i] = warehouseString(s)
	}
	return out
}
func warehouseConst(value string, stringLiteral bool) any {
	if stringLiteral {
		return map[string]any{"A_Const": map[string]any{"sval": map[string]any{"sval": value}}}
	}
	if v, err := strconv.ParseInt(value, 10, 32); err == nil {
		return map[string]any{"A_Const": map[string]any{"ival": map[string]any{"ival": float64(v)}}}
	}
	return map[string]any{"A_Const": map[string]any{"fval": map[string]any{"fval": value}}}
}
func (n *warehouseAnalyticalNormalizer) unsupported() (map[string]any, error) {
	return nil, analyticalFailure("analytical_shape_unsupported", true)
}
func (n *warehouseAnalyticalNormalizer) selectNode(root map[string]any) (map[string]any, error) {
	s := object(root["select"])
	if len(root) != 1 || !warehouseAnalyticalFields(s, "expressions", "from", "joins", "where_clause", "group_by", "having", "order_by", "limit", "offset", "fetch", "top") {
		return n.unsupported()
	}
	q := map[string]any{"op": "SETOP_NONE"}
	// TOP precedes every positional expression in its source grammar.
	if top := object(s["top"]); top != nil {
		if n.binding.Dialect != "sqlserver" || !warehouseAnalyticalFields(top, "this", "parenthesized") {
			return n.unsupported()
		}
		x, err := n.expr(top["this"], 0)
		if err != nil {
			return nil, err
		}
		q["limitCount"] = x
		q["limitOption"] = "LIMIT_OPTION_COUNT"
	}
	var targets []any
	for _, node := range array(s["expressions"]) {
		m := object(node)
		name := ""
		if alias := object(m["alias"]); alias != nil {
			if !warehouseAnalyticalFields(alias, "this", "alias") {
				return n.unsupported()
			}
			name = text(object(alias["alias"])["name"])
			node = alias["this"]
		}
		x, err := n.expr(node, 0)
		if err != nil {
			return nil, err
		}
		target := map[string]any{"val": x}
		if name != "" {
			target["name"] = name
		}
		targets = append(targets, map[string]any{"ResTarget": target})
	}
	q["targetList"] = targets
	from := object(s["from"])
	if !warehouseAnalyticalFields(from, "expressions") || len(array(from["expressions"])) != 1 {
		return n.unsupported()
	}
	relation, err := n.table(array(from["expressions"])[0])
	if err != nil {
		return nil, err
	}
	for _, node := range array(s["joins"]) {
		j := object(node)
		if !warehouseAnalyticalFields(j, "this", "on", "kind", "use_inner_keyword", "use_outer_keyword") {
			return n.unsupported()
		}
		kind := map[string]string{"Inner": "JOIN_INNER", "Left": "JOIN_LEFT", "Cross": "JOIN_INNER"}[text(j["kind"])]
		if kind == "" {
			return n.unsupported()
		}
		right, err := n.table(j["this"])
		if err != nil {
			return nil, err
		}
		join := map[string]any{"jointype": kind, "larg": relation, "rarg": right}
		if j["on"] != nil {
			x, err := n.expr(j["on"], 0)
			if err != nil {
				return nil, err
			}
			join["quals"] = x
		}
		relation = map[string]any{"JoinExpr": join}
	}
	q["fromClause"] = []any{relation}
	if w := object(s["where_clause"]); w != nil {
		if !warehouseAnalyticalFields(w, "this") {
			return n.unsupported()
		}
		x, err := n.expr(w["this"], 0)
		if err != nil {
			return nil, err
		}
		q["whereClause"] = x
	}
	if g := object(s["group_by"]); g != nil {
		if !warehouseAnalyticalFields(g, "expressions") {
			return n.unsupported()
		}
		var groups []any
		for _, v := range array(g["expressions"]) {
			x, err := n.expr(v, 0)
			if err != nil {
				return nil, err
			}
			groups = append(groups, x)
		}
		q["groupClause"] = groups
	}
	if h := object(s["having"]); h != nil {
		if !warehouseAnalyticalFields(h, "this") {
			return n.unsupported()
		}
		x, err := n.expr(h["this"], 0)
		if err != nil {
			return nil, err
		}
		q["havingClause"] = x
	}
	if o := object(s["order_by"]); o != nil {
		if !warehouseAnalyticalFields(o, "expressions") {
			return n.unsupported()
		}
		var sorts []any
		for _, v := range array(o["expressions"]) {
			item := object(v)
			if !warehouseAnalyticalFields(item, "this", "desc", "explicit_asc", "nulls_first") {
				return n.unsupported()
			}
			x, err := n.expr(item["this"], 0)
			if err != nil {
				return nil, err
			}
			dir := "SORTBY_ASC"
			if truth(item["desc"]) {
				dir = "SORTBY_DESC"
			}
			nulls := "SORTBY_NULLS_DEFAULT"
			if b, ok := item["nulls_first"].(bool); ok {
				if b {
					nulls = "SORTBY_NULLS_FIRST"
				} else {
					nulls = "SORTBY_NULLS_LAST"
				}
			} else {
				switch n.binding.Dialect {
				case "mysql", "sqlserver", "bigquery", "databricks":
					if truth(item["desc"]) {
						nulls = "SORTBY_NULLS_LAST"
					} else {
						nulls = "SORTBY_NULLS_FIRST"
					}
				case "snowflake": // Session DEFAULT_NULL_ORDERING is not pinned in the source contract.
					nulls = "WAREHOUSE_NULLS_UNMEASURED"
				}
			}
			sorts = append(sorts, map[string]any{"SortBy": map[string]any{"node": x, "sortby_dir": dir, "sortby_nulls": nulls}})
		}
		q["sortClause"] = sorts
	}
	// A positional OFFSET and LIMIT together have dialect-specific source order;
	// never assign slots from a reordered parser representation.
	before := n.parameters
	if l := object(s["limit"]); l != nil {
		if q["limitCount"] != nil || !warehouseAnalyticalFields(l, "this") {
			return n.unsupported()
		}
		x, err := n.expr(l["this"], 0)
		if err != nil {
			return nil, err
		}
		q["limitCount"] = x
		q["limitOption"] = "LIMIT_OPTION_COUNT"
	}
	if o := object(s["offset"]); o != nil {
		if !warehouseAnalyticalFields(o, "this") {
			return n.unsupported()
		}
		x, err := n.expr(o["this"], 0)
		if err != nil {
			return nil, err
		}
		q["limitOffset"] = x
		if n.parameters != before && s["limit"] != nil {
			return n.unsupported()
		}
	}
	if f := object(s["fetch"]); f != nil {
		if q["limitCount"] != nil || !warehouseAnalyticalFields(f, "direction", "count", "rows") {
			return n.unsupported()
		}
		count := f["count"]
		if count == nil {
			count = map[string]any{"literal": map[string]any{"literal_type": "number", "value": "1"}}
		}
		x, err := n.expr(count, 0)
		if err != nil {
			return nil, err
		}
		q["limitCount"] = x
		q["limitOption"] = "LIMIT_OPTION_COUNT"
	}
	return q, nil
}
func (n *warehouseAnalyticalNormalizer) table(node any) (any, error) {
	t := object(object(node)["table"])
	if !warehouseAnalyticalFields(t, "name", "schema", "catalog", "alias", "alias_explicit_as") {
		return n.unsupported()
	}
	name := text(object(t["name"])["name"])
	schema := text(object(t["schema"])["name"])
	catalog := text(object(t["catalog"])["name"])
	coordinate := name
	if schema != "" {
		coordinate = schema + "." + coordinate
	}
	if catalog != "" {
		coordinate = catalog + "." + coordinate
	}
	matches := 0
	var physical Relation
	for _, r := range n.binding.Relations {
		if warehouseRelationMatches(n.binding, r, coordinate, true) {
			matches++
			physical = r
		}
	}
	if matches != 1 {
		return nil, ErrBinding
	}
	out := map[string]any{"schemaname": physical.Schema, "relname": physical.Name, "inh": true}
	if alias := object(t["alias"]); alias != nil {
		out["alias"] = map[string]any{"Alias": map[string]any{"aliasname": text(alias["name"])}}
	}
	return map[string]any{"RangeVar": out}, nil
}
func (n *warehouseAnalyticalNormalizer) expr(node any, depth int) (any, error) {
	n.nodes++
	if n.nodes > 10000 || depth > 64 {
		return nil, ErrLimit
	}
	if err := n.ctx.Err(); err != nil {
		return nil, err
	}
	if literal, ok := node.(string); ok && literal == "null" {
		return map[string]any{"A_Const": map[string]any{"isnull": true}}, nil
	}
	m := object(node)
	if len(m) != 1 {
		return n.unsupported()
	}
	var kind string
	var b map[string]any
	for k, v := range m {
		kind = k
		b = object(v)
	}
	child := func(v any) (any, error) { return n.expr(v, depth+1) }
	switch kind {
	case "null":
		return map[string]any{"A_Const": map[string]any{"isnull": true}}, nil
	case "paren":
		if !warehouseAnalyticalFields(b, "this") {
			return n.unsupported()
		}
		return child(b["this"])
	case "column":
		if !warehouseAnalyticalFields(b, "name", "table") {
			return n.unsupported()
		}
		name := text(object(b["name"])["name"])
		table := text(object(b["table"])["name"])
		if table == "" && !truth(object(b["name"])["quoted"]) && (n.binding.Dialect == "sqlserver" || n.binding.Dialect == "bigquery") && strings.HasPrefix(name, "@p") {
			index, err := strconv.Atoi(strings.TrimPrefix(name, "@p"))
			if err != nil || index < 1 || index > 64 {
				return n.unsupported()
			}
			return map[string]any{"ParamRef": map[string]any{"number": float64(index)}}, nil
		}
		fields := warehouseNames(name)
		if table != "" {
			fields = warehouseNames(table, name)
		}
		return map[string]any{"ColumnRef": map[string]any{"fields": fields}}, nil
	case "literal":
		if !warehouseAnalyticalFields(b, "literal_type", "value") {
			return n.unsupported()
		}
		switch text(b["literal_type"]) {
		case "number":
			return warehouseConst(text(b["value"]), false), nil
		case "string":
			return warehouseConst(text(b["value"]), true), nil
		}
		return n.unsupported()
	case "boolean":
		return map[string]any{"A_Const": map[string]any{"boolval": map[string]any{"boolval": truth(b["value"])}}}, nil
	case "placeholder":
		if !warehouseAnalyticalFields(b, "index") || b["index"] != nil {
			return n.unsupported()
		}
		n.parameters++
		return map[string]any{"ParamRef": map[string]any{"number": float64(n.parameters)}}, nil
	case "parameter":
		if !warehouseAnalyticalFields(b, "name", "index", "style") {
			return n.unsupported()
		}
		if text(b["style"]) != "At" || (n.binding.Dialect != "sqlserver" && n.binding.Dialect != "bigquery") {
			return n.unsupported()
		}
		name := strings.TrimPrefix(text(b["name"]), "p")
		index, err := strconv.Atoi(name)
		if err != nil || index < 1 || index > 64 {
			return n.unsupported()
		}
		return map[string]any{"ParamRef": map[string]any{"number": float64(index)}}, nil
	case "and", "or", "not":
		if !warehouseAnalyticalFields(b, "left", "right", "this") {
			return n.unsupported()
		}
		var args []any
		for _, key := range []string{"left", "right", "this"} {
			if b[key] != nil {
				x, err := child(b[key])
				if err != nil {
					return nil, err
				}
				args = append(args, x)
			}
		}
		op := map[string]string{"and": "AND_EXPR", "or": "OR_EXPR", "not": "NOT_EXPR"}[kind]
		return map[string]any{"BoolExpr": map[string]any{"boolop": op, "args": args}}, nil
	case "neg":
		if !warehouseAnalyticalFields(b, "this") {
			return n.unsupported()
		}
		x, err := child(b["this"])
		if err != nil {
			return nil, err
		}
		return map[string]any{"A_Expr": map[string]any{"kind": "AEXPR_OP", "name": warehouseNames("-"), "rexpr": x}}, nil
	case "is_null":
		if !warehouseAnalyticalFields(b, "this", "not") {
			return n.unsupported()
		}
		x, err := child(b["this"])
		if err != nil {
			return nil, err
		}
		op := "IS_NULL"
		if truth(b["not"]) {
			op = "IS_NOT_NULL"
		}
		return map[string]any{"NullTest": map[string]any{"arg": x, "nulltesttype": op}}, nil
	case "in":
		if !warehouseAnalyticalFields(b, "this", "expressions", "not") {
			return n.unsupported()
		}
		x, err := child(b["this"])
		if err != nil {
			return nil, err
		}
		var args []any
		for _, v := range array(b["expressions"]) {
			a, err := child(v)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
		}
		op := "="
		if truth(b["not"]) {
			op = "<>"
		}
		return map[string]any{"A_Expr": map[string]any{"kind": "AEXPR_IN", "name": warehouseNames(op), "lexpr": x, "rexpr": args}}, nil
	case "case":
		if !warehouseAnalyticalFields(b, "whens", "else_") {
			return n.unsupported()
		}
		var args []any
		for _, v := range array(b["whens"]) {
			pair := array(v)
			if len(pair) != 2 {
				return n.unsupported()
			}
			x, err := child(pair[0])
			if err != nil {
				return nil, err
			}
			y, err := child(pair[1])
			if err != nil {
				return nil, err
			}
			args = append(args, map[string]any{"CaseWhen": map[string]any{"expr": x, "result": y}})
		}
		out := map[string]any{"args": args}
		if b["else_"] != nil {
			x, err := child(b["else_"])
			if err != nil {
				return nil, err
			}
			out["defresult"] = x
		}
		return map[string]any{"CaseExpr": out}, nil
	case "cast":
		return n.cast(b, depth)
	}
	if op := map[string]string{"add": "+", "sub": "-", "mul": "*", "div": "/", "eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[kind]; op != "" {
		if !warehouseAnalyticalFields(b, "left", "right") {
			return n.unsupported()
		}
		x, err := child(b["left"])
		if err != nil {
			return nil, err
		}
		y, err := child(b["right"])
		if err != nil {
			return nil, err
		}
		return map[string]any{"A_Expr": map[string]any{"kind": "AEXPR_OP", "name": warehouseNames(op), "lexpr": x, "rexpr": y}}, nil
	}
	return n.function(m, depth)
}

func (n *warehouseAnalyticalNormalizer) function(m map[string]any, depth int) (any, error) {
	name, args, b, ok := warehouseCall(m)
	if !ok || !warehouseAnalyticalFields(b, "name", "original_name", "args", "this", "distinct", "star", "filter") {
		return n.unsupported()
	}
	var values []any
	for _, arg := range args {
		x, err := n.expr(arg, depth+1)
		if err != nil {
			return nil, err
		}
		values = append(values, x)
	}
	if name == "nullif" {
		if len(values) != 2 || truth(b["distinct"]) {
			return n.unsupported()
		}
		return map[string]any{"A_Expr": map[string]any{"kind": "AEXPR_NULLIF", "name": warehouseNames("="), "lexpr": values[0], "rexpr": values[1]}}, nil
	}
	switch name {
	case "sum", "avg", "min", "max", "count", "date_trunc":
	default:
		return n.unsupported()
	}
	f := map[string]any{"funcname": warehouseNames(name), "args": values}
	if truth(b["star"]) {
		if name != "count" || len(values) != 0 {
			return n.unsupported()
		}
		f["agg_star"] = true
	}
	if truth(b["distinct"]) {
		f["agg_distinct"] = true
	}
	if b["filter"] != nil {
		x, err := n.expr(b["filter"], depth+1)
		if err != nil {
			return nil, err
		}
		f["agg_filter"] = x
	}
	return map[string]any{"FuncCall": f}, nil
}
func (n *warehouseAnalyticalNormalizer) cast(b map[string]any, depth int) (any, error) {
	if !warehouseAnalyticalFields(b, "this", "to", "double_colon_syntax") {
		return n.unsupported()
	}
	typ := object(b["to"])
	name := text(typ["data_type"])
	target := ""
	switch name {
	case "decimal", "numeric", "big_numeric":
		// Warehouse decimal casts have finite/default scales. Only a known integral
		// count can be widened here without silently rounding a reviewed decimal.
		call, args, _, ok := warehouseCall(object(b["this"]))
		_ = args
		if !ok || call != "count" {
			return n.unsupported()
		}
		precision, _ := typ["precision"].(float64)
		scale, _ := typ["scale"].(float64)
		if n.binding.Dialect == "bigquery" && precision == 0 {
			precision = 38
			scale = 9
		}
		if precision < 19 || scale < 0 || precision-scale < 19 {
			return n.unsupported()
		}
		target = "numeric"
	case "date":
		target = "date"
	case "datetime", "timestamp", "timestamp_ntz":
		target = "timestamp"
	case "timestamp_tz":
		target = "timestamptz"
	default:
		return n.unsupported()
	}
	if !warehouseAnalyticalFields(typ, "data_type", "precision", "scale") {
		return n.unsupported()
	}
	x, err := n.expr(b["this"], depth+1)
	if err != nil {
		return nil, err
	}
	return map[string]any{"TypeCast": map[string]any{"arg": x, "typeName": map[string]any{"TypeName": map[string]any{"names": warehouseNames(target)}}}}, nil
}

// This detached view records source-engine scalar semantics for the proof. It
// never changes the source binding or its hash and never promotes float/money.
func warehouseAnalyticalRelation(dialect string, r Relation) Relation {
	r.Columns = append([]Column(nil), r.Columns...)
	for i := range r.Columns {
		c := &r.Columns[i]
		native := strings.ToLower(strings.TrimSpace(c.NativeType))
		base, _, _ := strings.Cut(native, "(")
		base = strings.TrimSpace(base)
		switch base {
		case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "int64", "small_int", "big_int":
			c.NativeType = "int8"
		case "decimal", "numeric", "number", "bignumeric":
			c.NativeType = "numeric"
		case "date":
			c.NativeType = "date"
		case "datetime", "datetime2", "timestamp_ntz", "timestamp without time zone":
			c.NativeType = "timestamp"
		case "timestamp":
			if dialect == "bigquery" {
				c.NativeType = "timestamptz"
			} else if dialect == "mysql" || dialect == "databricks" || dialect == "sqlserver" {
				c.NativeType = "unproved_session_timestamp"
			} else {
				c.NativeType = "timestamp"
			}
		case "timestamp_tz", "datetimeoffset":
			c.NativeType = "timestamptz"
		}
	}
	return r
}
