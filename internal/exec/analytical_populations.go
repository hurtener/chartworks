package exec

import "strings"

func analyticalPopulationRelation(c AnalyticalContract, b Binding, base Relation) (Relation, error) {
	if (c.Version != AnalyticalIntentVersion && (c.Version != AnalyticalGroupedPopulationsVersion && c.Version != AnalyticalGroupedProgramsVersion)) || len(c.Populations) < 2 || len(c.Populations) > 4 || len(c.Joins) > 0 || c.QueryPopulation != nil && ((c.Version != AnalyticalGroupedPopulationsVersion && c.Version != AnalyticalGroupedProgramsVersion) || len(c.QueryPopulation.Constraints) != 0) || c.Grain != nil && len(c.Grain.Columns)+len(c.Grain.Buckets) > 0 {
		return Relation{}, ErrBinding
	}
	out := base
	out.Columns = nil
	out.UniqueKeys = nil
	hasBase := false
	for i, id := range c.Populations {
		if i > 0 && c.Populations[i-1] >= id {
			return Relation{}, ErrBinding
		}
		found := false
		for _, r := range b.Relations {
			if r.ID == id {
				found = true
				hasBase = hasBase || id == base.ID
				for _, col := range r.Columns {
					col.Name = AnalyticalColumnName(base.ID, id, col.Name)
					out.Columns = append(out.Columns, col)
				}
			}
		}
		if !found {
			return Relation{}, ErrBinding
		}
	}
	if !hasBase {
		return Relation{}, ErrBinding
	}
	return out, nil
}

func (a *analyticalChecker) countPopulation(column string) string {
	if len(a.populations) == 0 {
		return ""
	}
	if column == "" && a.populationSource != "" {
		return a.populationSource
	}
	if p := strings.SplitN(column, "/", 2); len(p) == 2 {
		return p[0]
	}
	return a.relation.ID
}

func (a *analyticalChecker) derivedTerm(node any) (analyticalTerm, bool) {
	if len(a.derivedTerms) == 0 {
		return analyticalTerm{}, false
	}
	parts, ok := names(fieldObject(node, "ColumnRef")["fields"])
	if !ok || len(parts) < 1 || len(parts) > 2 {
		return analyticalTerm{}, false
	}
	if len(parts) == 2 {
		t, ok := a.derivedTerms[parts[0]+"."+parts[1]]
		return t, ok
	}
	var result analyticalTerm
	count := 0
	for name, t := range a.derivedTerms {
		if strings.HasSuffix(name, "."+parts[0]) {
			result = t
			count++
		}
	}
	return result, count == 1
}

func (a *analyticalChecker) queryIndependent(q map[string]any, expected map[string]int) error {
	if a.scalarPopulations != nil && object(q["withClause"]) == nil {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	if !only(q, "targetList", "fromClause", "withClause", "sortClause", "limitOffset", "limitCount", "limitOption", "op") || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	ctes := map[string]map[string]any{}
	if with := object(q["withClause"]); with != nil {
		if truth(with["recursive"]) || !only(with, "ctes", "recursive", "location") {
			return analyticalFailure("analytical_shape_unsupported", true)
		}
		for _, raw := range array(with["ctes"]) {
			cte := fieldObject(raw, "CommonTableExpr")
			name := text(cte["ctename"])
			if name == "" || ctes[name] != nil || len(array(cte["aliascolnames"])) > 0 || !only(cte, "ctename", "ctematerialized", "ctequery", "location") {
				return analyticalFailure("analytical_shape_unsupported", true)
			}
			query := fieldObject(cte["ctequery"], "SelectStmt")
			if query == nil {
				return ErrBinding
			}
			ctes[name] = query
		}
	}
	a.derivedTerms = map[string]analyticalTerm{}
	usedSources := map[string]bool{}
	usedCTEs := map[string]bool{}
	aliases := map[string]bool{}
	var source func(any, int) error
	source = func(raw any, depth int) error {
		if depth > 8 {
			return ErrLimit
		}
		if join := object(object(raw)["JoinExpr"]); join != nil {
			if text(join["jointype"]) != "JOIN_INNER" || !only(join, "jointype", "larg", "rarg", "rtindex") {
				return analyticalFailure("analytical_join_mismatch", false)
			}
			if err := source(join["larg"], depth+1); err != nil {
				return err
			}
			return source(join["rarg"], depth+1)
		}
		var query map[string]any
		alias := ""
		if rv := object(object(raw)["RangeVar"]); rv != nil {
			name := text(rv["relname"])
			if text(rv["schemaname"]) != "" || usedCTEs[name] {
				return analyticalFailure("analytical_shape_unsupported", true)
			}
			query = ctes[name]
			usedCTEs[name] = true
			alias = name
			if av := object(rv["alias"]); av != nil {
				v := fieldObject(av, "Alias")
				if len(array(v["colnames"])) > 0 {
					return ErrBinding
				}
				alias = text(v["aliasname"])
			}
		} else if rs := object(object(raw)["RangeSubselect"]); rs != nil {
			if a.scalarPopulations != nil {
				return analyticalFailure("analytical_shape_unsupported", true)
			}
			if truth(rs["lateral"]) || !only(rs, "subquery", "alias", "lateral") {
				return analyticalFailure("analytical_shape_unsupported", true)
			}
			query = fieldObject(rs["subquery"], "SelectStmt")
			v := fieldObject(rs["alias"], "Alias")
			if len(array(v["colnames"])) > 0 {
				return ErrBinding
			}
			alias = text(v["aliasname"])
		}
		if query == nil || alias == "" || aliases[alias] {
			return analyticalFailure("analytical_shape_unsupported", true)
		}
		aliases[alias] = true
		return a.singletonLane(query, alias, usedSources)
	}
	from := array(q["fromClause"])
	if len(from) == 0 || len(from) > 4 {
		return ErrLimit
	}
	for _, raw := range from {
		if err := source(raw, 0); err != nil {
			return err
		}
	}
	if len(usedSources) != len(a.populations) || len(usedCTEs) != len(ctes) {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	targets := array(q["targetList"])
	if len(targets) == 0 || len(targets) > 32 {
		return ErrLimit
	}
	terms := make([]analyticalTerm, len(targets))
	output := map[string]analyticalTerm{}
	matched := map[string]bool{}
	for i, raw := range targets {
		t := fieldObject(raw, "ResTarget")
		term, err := a.term(t["val"], 0)
		if err != nil {
			return err
		}
		if term.guarded || !term.aggregate || expected[term.key] == 0 {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
		terms[i] = term
		matched[term.key] = true
		if name := text(t["name"]); name != "" {
			if _, ok := output[name]; ok {
				return ErrBinding
			}
			output[name] = term
		}
	}
	for key := range expected {
		if !matched[key] {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
	}
	if err := a.checkIntent(q, terms, output); err != nil {
		return err
	}
	a.finalTerms = terms
	return a.ctx.Err()
}

func (a *analyticalChecker) singletonLane(q map[string]any, alias string, used map[string]bool) error {
	if a.scalarPopulations != nil {
		return a.scopedSingletonLane(q, alias, used)
	}
	if !only(q, "targetList", "fromClause", "whereClause", "limitOption", "op") || text(q["limitOption"]) != "" && text(q["limitOption"]) != "LIMIT_OPTION_DEFAULT" || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	from := array(q["fromClause"])
	if len(from) != 1 {
		return ErrBinding
	}
	rv := object(object(from[0])["RangeVar"])
	if rv == nil {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	var r Relation
	for _, candidate := range a.binding.Relations {
		if candidate.Schema == text(rv["schemaname"]) && candidate.Name == text(rv["relname"]) {
			r = candidate
		}
	}
	allowed := false
	for _, id := range a.populations {
		allowed = allowed || r.ID == id
	}
	if !allowed || used[r.ID] {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	used[r.ID] = true
	local := *a
	local.derivedTerms = nil
	local.leaves = nil
	local.grain = nil
	local.intent = nil
	local.queryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	local.populationSource = r.ID
	local.joins = nil
	local.nodes = 0
	local.relation.Schema, local.relation.Name = r.Schema, r.Name
	tableAlias := r.Name
	if av := object(rv["alias"]); av != nil {
		v := fieldObject(av, "Alias")
		if len(array(v["colnames"])) > 0 {
			return ErrBinding
		}
		tableAlias = text(v["aliasname"])
	}
	local.joinAliases = map[string]string{tableAlias: r.ID}
	want := map[string]int{}
	for _, leaf := range a.leaves {
		id := a.relation.ID
		if p := strings.SplitN(leaf.Column, "/", 2); len(p) == 2 {
			id = p[0]
		}
		if id == r.ID {
			key, err := local.expected(leaf, 0)
			if err != nil {
				return err
			}
			want[key]++
		}
	}
	if len(want) == 0 {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	// The ordinary checker proves each aggregate/filter over its raw source. No
	// GROUP/HAVING/limit is admitted, so SQL guarantees exactly one output row,
	// including an empty population (NULL sum/average and zero count).
	if err := local.query(q, want); err != nil {
		return err
	}
	for _, raw := range array(q["targetList"]) {
		target := fieldObject(raw, "ResTarget")
		name := text(target["name"])
		if name == "" {
			return analyticalFailure("analytical_output_ambiguous", true)
		}
		term, err := local.term(target["val"], 0)
		if err != nil {
			return err
		}
		if !term.aggregate || term.guarded {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
		key := alias + "." + name
		if _, ok := a.derivedTerms[key]; ok {
			return ErrBinding
		}
		a.derivedTerms[key] = term
	}
	return nil
}
