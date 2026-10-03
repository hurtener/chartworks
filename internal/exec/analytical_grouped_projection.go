package exec

// AnalyticalGroupedProgramsVersion pins transparent projection and common
// calendar-key proofs separately from retained v7 direct-column CTE programs.
const AnalyticalGroupedProgramsVersion = "analytical-metrics-v8"

// groupedDerivedSource exposes only a local, non-lateral, unnamed-column-list
// subquery. It does not erase any clause in the SELECT it returns.
func groupedDerivedSource(raw any) (map[string]any, string, bool) {
	r := fieldObject(raw, "RangeSubselect")
	if r == nil || truth(r["lateral"]) || !only(r, "subquery", "alias", "lateral") {
		return nil, "", false
	}
	alias := fieldObject(r["alias"], "Alias")
	if !only(alias, "aliasname") || text(alias["aliasname"]) == "" {
		return nil, "", false
	}
	q := fieldObject(r["subquery"], "SelectStmt")
	return q, text(alias["aliasname"]), q != nil
}

func groupedOutputName(t map[string]any) string {
	if name := text(t["name"]); name != "" {
		return name
	}
	parts, ok := names(fieldObject(t["val"], "ColumnRef")["fields"])
	if ok && len(parts) > 0 && len(parts) <= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

func groupedTermIdentity(t analyticalTerm) string {
	if t.aggregate {
		return "metric:" + t.key
	}
	return t.groupKey()
}

// projectGroupedTerms proves a complete transparent projection. Expressions,
// stars, dropping a metric/key, and ambiguous/duplicate names cannot borrow the
// inner proof. Renaming and reordering do not change populations or grain.
func projectGroupedTerms(q map[string]any, alias string, input map[string]analyticalTerm) (map[string]analyticalTerm, []analyticalTerm, error) {
	fail := func() (map[string]analyticalTerm, []analyticalTerm, error) {
		return nil, nil, analyticalFailure("analytical_shape_unsupported", true)
	}
	targets := array(q["targetList"])
	if len(targets) < 1 || len(targets) > 48 {
		return nil, nil, ErrLimit
	}
	want := map[string]bool{}
	got := map[string]bool{}
	for _, term := range input {
		want[groupedTermIdentity(term)] = true
	}
	out := map[string]analyticalTerm{}
	terms := make([]analyticalTerm, len(targets))
	for i, raw := range targets {
		t := fieldObject(raw, "ResTarget")
		if !only(t, "name", "val", "location") {
			return fail()
		}
		parts, ok := names(fieldObject(t["val"], "ColumnRef")["fields"])
		if !ok || len(parts) < 1 || len(parts) > 2 || len(parts) == 2 && parts[0] != alias {
			return fail()
		}
		term, found := input[parts[len(parts)-1]]
		if !found || term.guarded {
			return fail()
		}
		name := groupedOutputName(t)
		if name == "" {
			return fail()
		}
		if _, duplicate := out[name]; duplicate {
			return fail()
		}
		out[name] = term
		terms[i] = term
		got[groupedTermIdentity(term)] = true
	}
	if len(want) != len(got) {
		return fail()
	}
	for key := range want {
		if key == "" || !got[key] {
			return fail()
		}
	}
	return out, terms, nil
}

func groupedTransparentSelect(q map[string]any) bool {
	return only(q, "targetList", "fromClause", "limitOption", "op") && (text(q["op"]) == "" || text(q["op"]) == "SETOP_NONE") && (text(q["limitOption"]) == "" || text(q["limitOption"]) == "LIMIT_OPTION_DEFAULT")
}

func (a *analyticalChecker) groupedFinalProjection(q map[string]any, expected map[string]int, policy *AnalyticalGroupedPopulations) (bool, error) {
	from := array(q["fromClause"])
	if len(from) != 1 {
		return false, nil
	}
	inner, alias, ok := groupedDerivedSource(from[0])
	if !ok {
		return false, nil
	}
	fail := func() error { return analyticalFailure("analytical_shape_unsupported", true) }
	if a.groupedDepth >= 4 || !only(q, "targetList", "fromClause", "sortClause", "limitOffset", "limitCount", "limitOption", "op") || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return true, fail()
	}
	child := *a
	child.groupedDepth++
	child.groupedOutput = nil
	// Buried ORDER/LIMIT never prove final-layer intent. The inner program must
	// retain all groups, and its output ordering cannot be assumed by the wrapper.
	child.intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{}}
	if err := child.queryGroupedPopulations(inner, expected, policy); err != nil {
		return true, err
	}
	out, terms, err := projectGroupedTerms(q, alias, child.groupedOutput)
	if err != nil {
		return true, err
	}
	matched := map[string]bool{}
	groups := map[string]bool{}
	for _, term := range terms {
		if term.aggregate {
			matched[term.key] = true
		}
		if key := term.groupKey(); key != "" {
			groups[key] = true
		}
	}
	for key := range expected {
		if !matched[key] {
			return true, analyticalFailure("analytical_metric_mismatch", false)
		}
	}
	if err := a.checkGrain(groups, terms); err != nil {
		return true, err
	}
	local := *a
	local.relation.Columns = nil
	local.joinAliases = nil
	local.derivedTerms = map[string]analyticalTerm{}
	for name, term := range child.groupedOutput {
		local.derivedTerms[alias+"."+name] = term
	}
	if err := local.checkIntent(q, terms, out); err != nil {
		return true, err
	}
	a.groupedOutput = out
	a.finalTerms = terms
	return true, a.ctx.Err()
}

func (a *analyticalChecker) groupedLaneProjection(q map[string]any, policy *AnalyticalGroupedPopulations, used map[string]bool) (analyticalGroupedOutput, bool, error) {
	from := array(q["fromClause"])
	if len(from) != 1 {
		return analyticalGroupedOutput{}, false, nil
	}
	inner, alias, ok := groupedDerivedSource(from[0])
	if !ok {
		return analyticalGroupedOutput{}, false, nil
	}
	if a.groupedDepth >= 4 || !groupedTransparentSelect(q) {
		return analyticalGroupedOutput{}, true, analyticalFailure("analytical_shape_unsupported", true)
	}
	child := *a
	child.groupedDepth++
	proof, err := child.groupedLane(inner, policy, used)
	if err != nil {
		return analyticalGroupedOutput{}, true, err
	}
	projected, _, err := projectGroupedTerms(q, alias, proof.terms)
	if err != nil {
		return analyticalGroupedOutput{}, true, err
	}
	proof.terms = projected
	proof.groups = map[string]analyticalTerm{}
	for name, term := range projected {
		if term.groupKey() != "" {
			proof.groups[name] = term
		}
	}
	return proof, true, nil
}
