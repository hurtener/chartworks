package exec

import (
	"context"
	"strconv"
	"strings"
)

// BindGroupedSelectionConstraints selects only complete groups on the proven
// final key spine. Private values never enter a model prompt or SQL literal.
// Ordinary native validation and the v11 proof are still mandatory afterwards.
func BindGroupedSelectionConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, c AnalyticalContract) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) != 0 || c.Binding != Hash(binding) {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	if err := validateGroupedSelection(c, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	if err := ValidateAnalyticalGroupedPopulations(c, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	tokens, err := businessScan(ctx, statement, false)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	for _, token := range tokens {
		if token.kind == 'p' || token.kind == 'm' {
			return BusinessBoundQuery{}, businessSQLFailure("unsupported_population_model_parameters")
		}
	}
	if _, err = businessGroupedPopulationBodies(ctx, statement, tokens, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	base := groupedSelectionBase(c)
	bound := BusinessBoundQuery{SQL: statement}
	if groupedSelectionHasPeriods(c) {
		bound, err = BindGroupedPopulationConstraints(ctx, binding, statement, nil, base)
		if err != nil {
			return BusinessBoundQuery{}, err
		}
	}
	return finishGroupedSelectionBinding(ctx, binding, c, base, bound)
}

// Prove all aggregate lanes before locating direct keys on the final spine.
func finishGroupedSelectionBinding(ctx context.Context, binding Binding, c, base AnalyticalContract, bound BusinessBoundQuery) (BusinessBoundQuery, error) {
	var err error
	var relation Relation
	found := 0
	for _, r := range binding.Relations {
		if r.ID == c.Dataset {
			relation = r
			found++
		}
	}
	if found != 1 {
		return BusinessBoundQuery{}, ErrBinding
	}
	relation, err = analyticalGroupedRelation(base, binding, relation)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	checker, expected, _, err := prepareAnalyticalProgram(ctx, binding, base, relation, bound.Parameters)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	tree, err := analyticalSQLTree(ctx, bound.SQL, binding)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if err = checker.queryGroupedPopulations(tree, expected, base.GroupedPopulations); err != nil {
		return BusinessBoundQuery{}, err
	}
	schema, policy := 4, AnalyticalGroupedSelectionPolicy
	if c.Version == AnalyticalGroupedFactsVersion {
		schema, policy = 5, AnalyticalGroupedFactPolicy
	}
	if c.GroupSelection == nil {
		if schema != 5 {
			return BusinessBoundQuery{}, ErrBinding
		}
		bound.Receipt = BusinessBindingReceipt{SchemaVersion: schema, PopulationPolicy: policy, SourceBinding: Hash(binding), Constraints: Hash([]any{c.GroupedPopulations, c.GroupSelection}), Statement: Hash([]any{bound.SQL, bound.Parameters}), Bindings: bound.Receipt.Bindings}
		return bound, nil
	}
	predicate, selected, bindings, err := groupedSelectionPredicate(ctx, binding, c.Dataset, checker.groupSelectionKeys, c.GroupSelection.Constraints, len(bound.Parameters))
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	tokens, err := businessScan(ctx, bound.SQL, false)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	position := len(bound.SQL)
	for _, token := range tokens {
		if token.depth == 0 && (token.word("order") || token.word("limit") || token.word("offset") || token.word("fetch") || token.text == ";") {
			position = token.start
			break
		}
	}
	sql, err := businessApplyEdits(bound.SQL, []businessEdit{{position, position, " WHERE " + predicate + " "}})
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if len(sql) > 32<<10 || len(bindings)+len(bound.Receipt.Bindings) > 64 {
		return BusinessBoundQuery{}, ErrLimit
	}
	all := append(append([]Parameter(nil), bound.Parameters...), selected...)
	effects := append(append([]BusinessParameterBinding(nil), bound.Receipt.Bindings...), bindings...)
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	return BusinessBoundQuery{SQL: sql, Parameters: all, Receipt: BusinessBindingReceipt{SchemaVersion: schema, PopulationPolicy: policy, SourceBinding: Hash(binding), Constraints: Hash([]any{c.GroupedPopulations, c.GroupSelection}), Statement: Hash([]any{sql, all}), Bindings: effects}}, nil
}

// Empty Population denotes the final group spine in schemas 4 and 5; fact-owned
// period and row bindings retain their nonempty fact identity. Exact replay verifies every item.
func groupedSelectionPredicate(ctx context.Context, binding Binding, base string, keys map[string][2]string, constraints []BusinessConstraint, offset int) (string, []Parameter, []BusinessParameterBinding, error) {
	if ctx == nil || offset < 0 || offset > 64 || len(constraints) == 0 {
		return "", nil, nil, ErrBinding
	}
	if err := ValidateBusinessConstraints(binding, constraints); err != nil {
		return "", nil, nil, err
	}
	var predicates []string
	var scalars []businessScalar
	var bindings []BusinessParameterBinding
	for _, constraint := range constraints {
		if err := ctx.Err(); err != nil {
			return "", nil, nil, err
		}
		key, ok := keys[AnalyticalColumnName(base, constraint.Dataset, constraint.Column)]
		if !ok || key[0] == "" || key[1] == "" || constraint.Aggregation != "" {
			return "", nil, nil, analyticalFailure("analytical_group_selection_unsupported", true)
		}
		column := businessQuote(binding.Dialect, key[0]) + "." + businessQuote(binding.Dialect, key[1])
		predicate, err := businessPredicate(binding.Dialect, column, constraint, &scalars)
		if err != nil {
			return "", nil, nil, err
		}
		predicates = append(predicates, predicate)
		bindings = append(bindings, BusinessParameterBinding{Resolution: constraint.Resolution, Dataset: constraint.Dataset, Column: constraint.Column, Kind: constraint.Kind, Operator: constraint.Operator, Nulls: constraint.Nulls, Parameters: []int{}})
	}
	predicate := strings.Join(predicates, " AND ")
	tokens, err := businessScan(ctx, predicate, true)
	if err != nil {
		return "", nil, nil, err
	}
	var edits []businessEdit
	var parameters []Parameter
	for _, token := range tokens {
		if token.kind == 'p' {
			return "", nil, nil, ErrBinding
		}
		if token.kind != 'm' {
			continue
		}
		index, err := strconv.Atoi(token.text)
		if err != nil || index < 0 || index >= len(scalars) || offset+len(parameters) >= 64 {
			return "", nil, nil, ErrBinding
		}
		scalar := scalars[index]
		parameters = append(parameters, scalar.parameter)
		position := offset + len(parameters)
		edits = append(edits, businessEdit{token.start, token.end, businessPlaceholder(binding.Dialect, position)})
		for i := range bindings {
			if bindings[i].Resolution == scalar.resolution {
				bindings[i].Parameters = append(bindings[i].Parameters, position)
			}
		}
	}
	predicate, err = businessApplyEdits(predicate, edits)
	if err != nil {
		return "", nil, nil, err
	}
	return predicate, parameters, bindings, nil
}
