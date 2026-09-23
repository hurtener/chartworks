package exec

import (
	"sort"
	"strconv"
	"strings"
)

type analyticalTerm struct {
	key                                string
	column                             string
	aggregate, numeric, guarded, exact bool
	constant                           string
}

func (a *analyticalChecker) query(q map[string]any, expected map[string]int) error {
	if !only(q, "targetList", "fromClause", "whereClause", "groupClause", "havingClause", "sortClause", "limitOffset", "limitCount", "limitOption", "op") || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	sources := array(q["fromClause"])
	if len(sources) != 1 || object(sources[0])["RangeVar"] == nil {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	r := fieldObject(sources[0], "RangeVar")
	if text(r["schemaname"]) != a.relation.Schema || text(r["relname"]) != a.relation.Name {
		return analyticalFailure("analytical_relation_mismatch", false)
	}
	a.alias = a.relation.Name
	if alias := object(r["alias"]); alias != nil {
		alias = fieldObject(alias, "Alias")
		if len(array(alias["colnames"])) > 0 {
			return analyticalFailure("analytical_shape_unsupported", true)
		}
		a.alias = text(alias["aliasname"])
	}
	// Even a same-table nested SELECT can alter cardinality/populations. This
	// narrower checker admits one base scope, never dead or shadowed CTE evidence.
	if !analyticalFlat(q, 0) {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	common := map[string]int{}
	filters := map[string]AnalyticalFilter{}
	for _, leaf := range a.leaves {
		seen := map[string]bool{}
		for _, f := range leaf.Filters {
			key := analyticalFilterKey(f)
			if !seen[key] {
				common[key]++
				seen[key] = true
			}
			filters[key] = f
		}
	}
	a.common = map[string]bool{}
	for key, n := range common {
		if n == len(a.leaves) {
			a.common[key] = true
		}
	}
	a.global = map[string]bool{}
	for _, node := range analyticalConjuncts(q["whereClause"]) {
		f, err := a.predicate(node)
		if err != nil {
			continue
		} // Query-wide user constraints are a separate proof.
		key := analyticalFilterKey(f)
		if _, metricFilter := filters[key]; metricFilter && !a.common[key] {
			return analyticalFailure("analytical_population_mismatch", false)
		}
		if a.common[key] {
			a.global[key] = true
		}
	}
	targets := array(q["targetList"])
	if len(targets) == 0 || len(targets) > 256 {
		return ErrLimit
	}
	terms := make([]analyticalTerm, len(targets))
	aliases := map[string]analyticalTerm{}
	matched := map[string]bool{}
	for i, target := range targets {
		t := fieldObject(target, "ResTarget")
		term, err := a.term(t["val"], 0)
		if err != nil {
			return err
		}
		if term.guarded {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
		terms[i] = term
		if name := text(t["name"]); name != "" {
			if _, exists := aliases[name]; exists && a.grain != nil {
				return analyticalFailure("analytical_output_ambiguous", true)
			}
			aliases[name] = term
		}
		if term.aggregate {
			if expected[term.key] == 0 {
				return analyticalFailure("analytical_metric_mismatch", false)
			}
			matched[term.key] = true
		} else if term.column == "" {
			return analyticalFailure("analytical_shape_unsupported", true)
		}
	}
	for key := range expected {
		if !matched[key] {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
	}
	groups := map[string]bool{}
	for _, node := range array(q["groupClause"]) {
		var term analyticalTerm
		// PostgreSQL GROUP BY prefers input columns over output aliases. Ordinals
		// resolve only whole targets; no expression/name fallback crosses scopes.
		if n, ok := analyticalIntegerConstant(node); ok && n > 0 && n <= len(terms) {
			term = terms[n-1]
		} else {
			var err error
			term, err = a.term(node, 0)
			if err != nil {
				parts, valid := names(fieldObject(node, "ColumnRef")["fields"])
				if !valid || len(parts) != 1 {
					return analyticalFailure("analytical_shape_unsupported", true)
				}
				term = aliases[parts[0]]
			}
		}
		if term.column == "" || term.aggregate || term.guarded {
			return analyticalFailure("analytical_shape_unsupported", true)
		}
		groups[term.column] = true
	}
	for _, term := range terms {
		if term.column != "" && !groups[term.column] {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
	}
	if err := a.checkGrain(groups, terms); err != nil {
		return err
	}
	return a.ctx.Err()
}
func analyticalFlat(node any, depth int) bool {
	if depth > 128 {
		return false
	}
	switch v := node.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "SelectStmt" || key == "SubLink" || key == "RangeSubselect" || key == "JoinExpr" || key == "WindowDef" {
				return false
			}
			if !analyticalFlat(item, depth+1) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !analyticalFlat(item, depth+1) {
				return false
			}
		}
	}
	return true
}
func analyticalConjuncts(node any) []any {
	if node == nil {
		return nil
	}
	if b := object(object(node)["BoolExpr"]); b != nil && text(b["boolop"]) == "AND_EXPR" {
		var out []any
		for _, n := range array(b["args"]) {
			out = append(out, analyticalConjuncts(n)...)
		}
		return out
	}
	return []any{node}
}
func analyticalIntegerConstant(node any) (int, bool) {
	m := object(object(node)["A_Const"])
	v := object(m["ival"])
	if v == nil {
		return 0, false
	}
	n, _ := v["ival"].(float64)
	return int(n), n == float64(int(n))
}
func (a *analyticalChecker) field(node any) (Column, bool) {
	m := object(object(node)["ColumnRef"])
	if m == nil {
		return Column{}, false
	}
	parts, ok := names(m["fields"])
	if !ok {
		return Column{}, false
	}
	if len(parts) == 2 {
		if parts[0] != a.alias {
			return Column{}, false
		}
		parts = parts[1:]
	}
	if len(parts) != 1 {
		return Column{}, false
	}
	return a.column(parts[0])
}
func (a *analyticalChecker) scalar(node any, column Column) (string, bool) {
	root := object(node)
	if p := object(root["ParamRef"]); p != nil {
		n, _ := p["number"].(float64)
		i := int(n) - 1
		if n != float64(i+1) || i < 0 || i >= len(a.parameters) {
			return "", false
		}
		v := a.parameters[i]
		if v.Kind == "null" {
			return "", false
		}
		return analyticalScalar(column, v.Value)
	}
	m := object(root["A_Const"])
	if m == nil || truth(m["isnull"]) {
		return "", false
	}
	var value string
	switch {
	case m["sval"] != nil:
		value = text(object(m["sval"])["sval"])
	case m["ival"] != nil:
		n, _ := object(m["ival"])["ival"].(float64)
		value = strconv.FormatInt(int64(n), 10)
	case m["fval"] != nil:
		value = text(object(m["fval"])["fval"])
	case m["boolval"] != nil:
		value = strconv.FormatBool(truth(object(m["boolval"])["boolval"]))
	default:
		return "", false
	}
	return analyticalScalar(column, value)
}
func (a *analyticalChecker) predicate(node any) (AnalyticalFilter, error) {
	bad := func() (AnalyticalFilter, error) {
		return AnalyticalFilter{}, analyticalFailure("analytical_population_unsupported", true)
	}
	m := object(node)
	if n := object(m["NullTest"]); n != nil {
		c, ok := a.field(n["arg"])
		if !ok || truth(n["argisrow"]) || text(n["nulltesttype"]) != "IS_NOT_NULL" {
			return bad()
		}
		return AnalyticalFilter{Column: c.Name, Kind: "not_null"}, nil
	}
	n := object(m["A_Expr"])
	if n == nil {
		return bad()
	}
	op, ok := names(n["name"])
	if !ok || len(op) != 1 || op[0] != "=" {
		return bad()
	}
	c, ok := a.field(n["lexpr"])
	right := n["rexpr"]
	if !ok && text(n["kind"]) == "AEXPR_OP" {
		c, ok = a.field(right)
		right = n["lexpr"]
	}
	if !ok {
		return bad()
	}
	f := AnalyticalFilter{Column: c.Name, Kind: "eq"}
	switch text(n["kind"]) {
	case "AEXPR_OP":
		value, ok := a.scalar(right, c)
		if !ok {
			return bad()
		}
		f.Values = []string{value}
	case "AEXPR_IN":
		for _, item := range array(fieldObject(right, "List")["items"]) {
			value, ok := a.scalar(item, c)
			if !ok {
				return bad()
			}
			f.Values = append(f.Values, value)
		}
		if len(f.Values) == 0 || len(f.Values) > 32 {
			return bad()
		}
		f.Kind = "in"
	default:
		return bad()
	}
	return canonicalAnalyticalFilter(f), nil
}
func canonicalAnalyticalFilter(f AnalyticalFilter) AnalyticalFilter {
	values := append([]string(nil), f.Values...)
	sort.Strings(values)
	f.Values = nil
	for _, v := range values {
		if len(f.Values) == 0 || f.Values[len(f.Values)-1] != v {
			f.Values = append(f.Values, v)
		}
	}
	if f.Kind == "in" && len(f.Values) == 1 {
		f.Kind = "eq"
	}
	return f
}
func (a *analyticalChecker) aggregateKey(op, column string, filters []AnalyticalFilter) string {
	if op == "count" {
		c, ok := a.column(column)
		if column == "" || ok && !c.Nullable {
			op, column = "count_rows", ""
		}
	}
	return analyticalExprKey(op, column, "", filters, nil)
}
func (a *analyticalChecker) term(node any, depth int) (analyticalTerm, error) {
	a.nodes++
	if a.nodes > 4096 || depth > 64 {
		return analyticalTerm{}, ErrLimit
	}
	if err := a.ctx.Err(); err != nil {
		return analyticalTerm{}, err
	}
	fail := func(code string, unsupported bool) (analyticalTerm, error) {
		return analyticalTerm{}, analyticalFailure(code, unsupported)
	}
	root := object(node)
	if len(root) != 1 {
		return fail("analytical_expression_unsupported", true)
	}
	if c, ok := a.field(node); ok {
		kind := analyticalNumericKind(c)
		return analyticalTerm{column: c.Name, numeric: kind == "decimal", exact: kind != ""}, nil
	}
	if cast := object(root["TypeCast"]); cast != nil {
		t := fieldObject(cast["typeName"], "TypeName")
		parts, ok := names(t["names"])
		if len(parts) == 2 && parts[0] == "pg_catalog" {
			parts = parts[1:]
		}
		if !ok || len(parts) != 1 || parts[0] != "numeric" || len(array(t["typmods"])) > 0 || len(array(t["arrayBounds"])) > 0 || truth(t["setof"]) {
			return fail("analytical_expression_unsupported", true)
		}
		term, err := a.term(cast["arg"], depth+1)
		if err != nil {
			return term, err
		}
		if !term.exact {
			return fail("analytical_type_unsupported", true)
		}
		term.numeric = true
		return term, nil
	}
	if value, ok := a.scalar(node, Column{Category: "decimal"}); ok {
		if object(root["A_Const"]) == nil {
			return fail("analytical_expression_unsupported", true)
		}
		_, integer := analyticalIntegerConstant(node)
		return analyticalTerm{key: analyticalExprKey("number", "", value, nil, nil), numeric: !integer, exact: true, constant: value}, nil
	}
	if f := object(root["FuncCall"]); f != nil {
		return a.aggregate(f, depth)
	}
	if e := object(root["A_Expr"]); e != nil {
		op, ok := names(e["name"])
		if !ok || len(op) != 1 {
			return fail("analytical_expression_unsupported", true)
		}
		if text(e["kind"]) == "AEXPR_NULLIF" && op[0] == "=" {
			x, err := a.term(e["lexpr"], depth+1)
			if err != nil {
				return x, err
			}
			zero, ok := a.scalar(e["rexpr"], Column{Category: "decimal"})
			if !ok || zero != "0" {
				return fail("analytical_expression_unsupported", true)
			}
			x.guarded = true
			return x, nil
		}
		if text(e["kind"]) != "AEXPR_OP" || !strings.Contains("+-*/", op[0]) || len(op[0]) != 1 || e["lexpr"] == nil || e["rexpr"] == nil {
			return fail("analytical_expression_unsupported", true)
		}
		x, err := a.term(e["lexpr"], depth+1)
		if err != nil {
			return x, err
		}
		y, err := a.term(e["rexpr"], depth+1)
		if err != nil {
			return y, err
		}
		if x.key == "" || y.key == "" || x.guarded || y.guarded && op[0] != "/" {
			return fail("analytical_metric_mismatch", false)
		}
		if !x.exact || !y.exact {
			return fail("analytical_type_unsupported", true)
		}
		if op[0] == "/" {
			if !x.numeric && !y.numeric {
				return fail("analytical_integer_division", false)
			}
			if !y.guarded && (y.constant == "" || y.constant == "0") {
				return fail("analytical_zero_policy", false)
			}
		}
		return analyticalTerm{key: analyticalExprKey(op[0], "", "", nil, []string{x.key, y.key}), numeric: x.numeric || y.numeric, exact: true, aggregate: x.aggregate || y.aggregate}, nil
	}
	return fail("analytical_expression_unsupported", true)
}
func (a *analyticalChecker) aggregate(f map[string]any, depth int) (analyticalTerm, error) {
	fail := func(code string, unsupported bool) (analyticalTerm, error) {
		return analyticalTerm{}, analyticalFailure(code, unsupported)
	}
	parts, ok := names(f["funcname"])
	if len(parts) == 2 && parts[0] == "pg_catalog" {
		parts = parts[1:]
	}
	if !ok || len(parts) != 1 || f["over"] != nil || len(array(f["agg_order"])) > 0 || truth(f["agg_within_group"]) || truth(f["func_variadic"]) {
		return fail("analytical_expression_unsupported", true)
	}
	op := parts[0]
	switch op {
	case "sum", "avg", "min", "max", "count":
	default:
		return fail("analytical_expression_unsupported", true)
	}
	if truth(f["agg_distinct"]) {
		if op != "count" {
			return fail("analytical_metric_mismatch", false)
		}
		op = "distinct_count"
	}
	args := array(f["args"])
	column := ""
	numeric := op == "avg"
	exact := op == "count" || op == "distinct_count"
	predicates := analyticalConjuncts(f["agg_filter"])
	if truth(f["agg_star"]) {
		if op != "count" || len(args) != 0 {
			return fail("analytical_metric_mismatch", false)
		}
	} else {
		if len(args) != 1 {
			return fail("analytical_expression_unsupported", true)
		}
		arg := args[0]
		// CASE's implicit/explicit NULL else preserves the aggregate's row population;
		// ELSE 0 does not (AVG/COUNT and all-null groups), so it is never normalized away.
		if c := object(object(arg)["CaseExpr"]); c != nil {
			cases := array(c["args"])
			if c["arg"] != nil || len(cases) != 1 || c["defresult"] != nil && !truth(object(object(c["defresult"])["A_Const"])["isnull"]) {
				return fail("analytical_population_unsupported", true)
			}
			w := fieldObject(cases[0], "CaseWhen")
			predicates = append(predicates, analyticalConjuncts(w["expr"])...)
			arg = w["result"]
		}
		term, err := a.term(arg, depth+1)
		if err != nil {
			return term, err
		}
		if term.column == "" || term.aggregate || term.guarded {
			return fail("analytical_metric_mismatch", false)
		}
		column = term.column
		numeric = numeric || term.numeric
		exact = exact || term.exact
	}
	filters := map[string]AnalyticalFilter{}
	for _, p := range predicates {
		v, err := a.predicate(p)
		if err != nil {
			return analyticalTerm{}, err
		}
		filters[analyticalFilterKey(v)] = v
	}
	for _, leaf := range a.leaves {
		for _, f := range leaf.Filters {
			key := analyticalFilterKey(f)
			if a.global[key] {
				filters[key] = f
			}
		}
	}
	values := make([]AnalyticalFilter, 0, len(filters))
	for _, f := range filters {
		values = append(values, f)
	}
	if op == "count" || op == "distinct_count" {
		numeric = false // COUNT always returns bigint, regardless of input type.
	}
	return analyticalTerm{key: a.aggregateKey(op, column, values), numeric: numeric, exact: exact, aggregate: true}, nil
}
