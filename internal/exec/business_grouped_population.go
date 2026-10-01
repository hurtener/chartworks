package exec

import "context"

// BindGroupedPopulationConstraints applies authenticated reviewed periods only
// inside flat grouped fact CTEs. The separate key spine owns no predicate.
// The result still needs full native validation and analytical proof.
func BindGroupedPopulationConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, contract AnalyticalContract) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) != 0 || contract.Binding != Hash(binding) || contract.Version != AnalyticalGroupedOwnedPopulationsVersion {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ValidateAnalyticalGroupedPopulations(contract, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	tokens, err := businessScan(ctx, statement, false)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	// No model marker can capture one of the private parameters inserted below.
	for _, token := range tokens {
		if token.kind == 'p' || token.kind == 'm' {
			return BusinessBoundQuery{}, businessSQLFailure("unsupported_population_model_parameters")
		}
	}
	bodies, err := businessGroupedPopulationBodies(ctx, statement, tokens, binding)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if len(bodies) != len(contract.GroupedPopulations.Lanes) {
		return BusinessBoundQuery{}, ErrBinding
	}
	lanes := map[string]AnalyticalGroupedLane{}
	for _, lane := range contract.GroupedPopulations.Lanes {
		lanes[lane.Dataset] = lane
	}
	used := map[string]bool{}
	var edits []businessEdit
	var outParameters []Parameter
	var bindings []BusinessParameterBinding
	for _, body := range bodies {
		if len(body.layout.ranges) == 0 {
			return BusinessBoundQuery{}, ErrBinding
		}
		for _, r := range body.layout.ranges {
			if r.virtual != "" {
				return BusinessBoundQuery{}, businessSQLFailure("unsupported_population_dependency")
			}
		}
		fact := body.layout.ranges[0].relation.ID
		lane, ok := lanes[fact]
		if !ok || used[fact] {
			return BusinessBoundQuery{}, businessSQLFailure("unsupported_missing_or_ambiguous_target")
		}
		used[fact] = true
		bound, err := BindBusinessConstraints(ctx, binding, statement[body.start:body.end], nil, lane.QueryPopulation.Constraints)
		if err != nil {
			return BusinessBoundQuery{}, err
		}
		offset := len(outParameters)
		if offset+len(bound.Parameters) > 64 {
			return BusinessBoundQuery{}, ErrLimit
		}
		boundTokens, err := businessScan(ctx, bound.SQL, false)
		if err != nil {
			return BusinessBoundQuery{}, err
		}
		var markers []businessEdit
		for _, token := range boundTokens {
			if token.kind != 'p' {
				continue
			}
			position, err := businessParameterIndex(token.text, binding.Dialect, 0)
			if err != nil || position < 1 || position > len(bound.Parameters) {
				return BusinessBoundQuery{}, ErrBinding
			}
			markers = append(markers, businessEdit{token.start, token.end, businessPlaceholder(binding.Dialect, offset+position)})
		}
		sql, err := businessApplyEdits(bound.SQL, markers)
		if err != nil {
			return BusinessBoundQuery{}, err
		}
		edits = append(edits, businessEdit{body.start, body.end, sql})
		outParameters = append(outParameters, bound.Parameters...)
		for _, b := range bound.Receipt.Bindings {
			b.Population = fact
			b.Parameters = append([]int(nil), b.Parameters...)
			for i := range b.Parameters {
				b.Parameters[i] += offset
			}
			bindings = append(bindings, b)
		}
	}
	if len(used) != len(lanes) {
		return BusinessBoundQuery{}, ErrBinding
	}
	bound, err := businessApplyEdits(statement, edits)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if len(bound) > 32<<10 {
		return BusinessBoundQuery{}, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	return BusinessBoundQuery{SQL: bound, Parameters: outParameters, Receipt: BusinessBindingReceipt{SchemaVersion: 3, PopulationPolicy: AnalyticalGroupedOwnedPopulationPolicy, SourceBinding: Hash(binding), Constraints: Hash(contract.GroupedPopulations), Statement: Hash([]any{bound, outParameters}), Bindings: bindings}}, nil
}

// Extract only physical aggregate bodies; the key-spine UNION is left intact.
// Final native/analytical proof checks the exact spine topology and outputs.
func businessGroupedPopulationBodies(ctx context.Context, statement string, tokens []businessToken, binding Binding) ([]businessPopulationBody, error) {
	if len(tokens) == 0 || !tokens[0].word("with") || binding.Dialect != "postgres" || len(tokens) > 4096 {
		return nil, businessSQLFailure("unsupported_select_shape")
	}
	tree, err := analyticalSQLTree(ctx, statement, binding)
	if err != nil {
		return nil, err
	}
	with := object(tree["withClause"])
	if with == nil || truth(with["recursive"]) || !only(tree, "targetList", "fromClause", "withClause", "sortClause", "limitOffset", "limitCount", "limitOption", "op") {
		return nil, ErrBinding
	}
	ctes := array(with["ctes"])
	if len(ctes) < 3 || len(ctes) > 5 {
		return nil, ErrBinding
	}
	var bodies []businessPopulationBody
	names := map[string]bool{}
	spine := 0
	i := 1
	for _, raw := range ctes {
		cte := fieldObject(raw, "CommonTableExpr")
		if !only(cte, "ctename", "ctematerialized", "ctequery", "location") || i+4 >= len(tokens) {
			return nil, ErrBinding
		}
		name, ok := businessName(tokens[i], binding.Dialect)
		if !ok || name != text(cte["ctename"]) || names[name] || !tokens[i+1].word("as") || tokens[i+2].text != "(" || tokens[i+2].depth != 0 {
			return nil, ErrBinding
		}
		names[name] = true
		open := i + 2
		close := open + 1
		for close < len(tokens) && !(tokens[close].text == ")" && tokens[close].depth == 0) {
			close++
		}
		if close >= len(tokens) || !tokens[open+1].word("select") {
			return nil, ErrBinding
		}
		start, end := tokens[open+1].start, tokens[close].start
		q := fieldObject(cte["ctequery"], "SelectStmt")
		if text(q["op"]) == "SETOP_UNION" {
			spine++
		} else {
			if !only(q, "targetList", "fromClause", "whereClause", "groupClause", "limitOption", "op") || len(array(q["groupClause"])) == 0 {
				return nil, businessSQLFailure("unsupported_select_shape")
			}
			bodyTokens, err := businessScan(ctx, statement[start:end], false)
			if err != nil {
				return nil, err
			}
			layout, err := businessLayout(statement[start:end], bodyTokens, binding)
			if err != nil {
				return nil, err
			}
			bodies = append(bodies, businessPopulationBody{name: name, start: start, end: end, layout: layout})
		}
		i = close + 1
		if i < len(tokens) && tokens[i].text == "," && tokens[i].depth == 0 {
			i++
		}
	}
	if spine != 1 || i >= len(tokens) || !tokens[i].word("select") {
		return nil, ErrBinding
	}
	return bodies, nil
}
