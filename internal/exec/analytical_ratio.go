package exec

// caseRatioTerm recognizes only an exact alternate spelling of the reviewed
// NULL-on-zero division policy. It never removes a general CASE expression.
func (a *analyticalChecker) caseRatioTerm(c map[string]any, depth int) (analyticalTerm, error) {
	fail := func() (analyticalTerm, error) {
		return analyticalTerm{}, analyticalFailure("analytical_zero_policy", false)
	}
	branches := array(c["args"])
	if c["arg"] != nil || len(branches) != 1 {
		return fail()
	}
	branch := fieldObject(branches[0], "CaseWhen")
	condition := object(object(branch["expr"])["A_Expr"])
	ops, ok := names(condition["name"])
	if !ok || len(ops) != 1 || text(condition["kind"]) != "AEXPR_OP" || (ops[0] != "=" && ops[0] != "<>" && ops[0] != "!=") {
		return fail()
	}
	isNull := func(node any) bool { return node == nil || truth(object(object(node)["A_Const"])["isnull"]) }
	ratio := c["defresult"]
	if ops[0] == "=" {
		if !isNull(branch["result"]) {
			return fail()
		}
	} else {
		if !isNull(c["defresult"]) {
			return fail()
		}
		ratio = branch["result"]
	}
	division := object(object(ratio)["A_Expr"])
	divideOps, ok := names(division["name"])
	if !ok || len(divideOps) != 1 || divideOps[0] != "/" || text(division["kind"]) != "AEXPR_OP" || division["lexpr"] == nil || division["rexpr"] == nil {
		return fail()
	}
	denominator, err := a.term(division["rexpr"], depth+1)
	if err != nil {
		return analyticalTerm{}, err
	}
	if denominator.key == "" || denominator.guarded || !denominator.exact {
		return fail()
	}
	check, zero := condition["lexpr"], condition["rexpr"]
	value, valid := a.scalar(zero, Column{Category: "decimal"})
	if !valid || value != "0" {
		check, zero = zero, check
		value, valid = a.scalar(zero, Column{Category: "decimal"})
	}
	if !valid || value != "0" {
		return fail()
	}
	checked, err := a.term(check, depth+1)
	if err != nil {
		return analyticalTerm{}, err
	}
	if checked.key != denominator.key || checked.guarded {
		return fail()
	}
	// Only the analysis AST is rebuilt, without changing any caller-owned maps.
	guarded := map[string]any{"A_Expr": map[string]any{"kind": "AEXPR_NULLIF", "name": []any{map[string]any{"String": map[string]any{"sval": "="}}}, "lexpr": division["rexpr"], "rexpr": map[string]any{"A_Const": map[string]any{"ival": map[string]any{"ival": float64(0)}}}}}
	copy := map[string]any{}
	for key, value := range division {
		copy[key] = value
	}
	copy["rexpr"] = guarded
	return a.term(map[string]any{"A_Expr": copy}, depth+1)
}
