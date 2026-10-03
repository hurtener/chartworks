package exec

import "context"

// BindScalarPopulationConstraints applies only compiler-owned periods to exact
// fact-rooted CTE lanes. Repeated dimension relations never select a placement.
// The result still needs full native validation and analytical proof.
func BindScalarPopulationConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, contract AnalyticalContract) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) != 0 || contract.Binding != Hash(binding) {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ValidateAnalyticalScalarPopulations(ctx, contract, binding); err != nil {
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
	bodies, err := businessPopulationBodies(ctx, statement, tokens, binding)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if len(bodies) != len(contract.ScalarPopulations.Lanes) {
		return BusinessBoundQuery{}, ErrBinding
	}
	lanes := map[string]AnalyticalScalarLane{}
	for _, lane := range contract.ScalarPopulations.Lanes {
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
	return BusinessBoundQuery{SQL: bound, Parameters: outParameters, Receipt: BusinessBindingReceipt{SchemaVersion: 2, PopulationPolicy: AnalyticalScalarPopulationPolicy, SourceBinding: Hash(binding), Constraints: Hash(contract.ScalarPopulations), Statement: Hash([]any{bound, outParameters}), Bindings: bindings}}, nil
}
