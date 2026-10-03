package exec

import "context"

// BusinessScalarPredicateEffect is a closed, value-free discharge effect. Unlike
// a period application, it must carry an explicit empty parameter-position list.
type BusinessScalarPredicateEffect struct {
	Kind        string `json:"kind"`
	Resolution  string `json:"resolution"`
	Coverage    string `json:"coverage"`
	Occurrences int    `json:"occurrences"`
	Parameters  []int  `json:"parameters"`
}

// BindScalarEntailmentConstraints preserves v9 period SQL and parameter ordering
// byte-for-byte. It proves the unchanged selected populations before recording
// each exact request predicate's zero-parameter effect. Full native validation
// and CheckAnalyticalPlan remain mandatory before any source execution.
func BindScalarEntailmentConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, c AnalyticalContract) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) != 0 {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ValidateAnalyticalScalarEntailment(ctx, c, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	base := scalarEntailmentBase(c)
	bound, err := BindScalarPopulationConstraints(ctx, binding, statement, parameters, base)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	var root Relation
	for _, r := range binding.Relations {
		if r.ID == c.Dataset {
			root = r
		}
	}
	relation, err := analyticalScalarRelation(ctx, base, binding, root)
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
	if err := checker.queryIndependent(tree, expected); err != nil {
		return BusinessBoundQuery{}, err
	}
	effects := make([]BusinessScalarPredicateEffect, 0, len(c.ScalarEntailment.Constraints))
	offset := 0
	for _, constraint := range c.ScalarEntailment.Constraints {
		start := offset
		for offset < len(c.ScalarEntailment.Witnesses) && c.ScalarEntailment.Witnesses[offset].Resolution == constraint.Resolution {
			offset++
		}
		witnesses := c.ScalarEntailment.Witnesses[start:offset]
		if len(witnesses) == 0 {
			return BusinessBoundQuery{}, ErrBinding
		}
		coverage := Hash([]any{c.Binding, c.Semantics, c.Dataset, c.Metrics, c.ScalarPopulations, constraint, witnesses})
		effects = append(effects, BusinessScalarPredicateEffect{Kind: "entailed_scalar_predicate", Resolution: constraint.Resolution, Coverage: coverage, Occurrences: len(witnesses), Parameters: []int{}})
	}
	if offset != len(c.ScalarEntailment.Witnesses) {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	bound.Receipt.SchemaVersion = 6
	bound.Receipt.PopulationPolicy = AnalyticalScalarEntailmentPolicy
	bound.Receipt.Constraints = Hash([]any{c.ScalarPopulations, c.ScalarEntailment})
	bound.Receipt.Entailments = effects
	return bound, nil
}
