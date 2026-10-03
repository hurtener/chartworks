package exec

import "context"

// BindGroupedFactConstraints restricts only the compiled fact populations, then
// optionally selects complete groups after alignment. No generated parameter,
// coincidental join membership or model claim establishes predicate ownership.
func BindGroupedFactConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, c AnalyticalContract) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) != 0 || c.Binding != Hash(binding) {
		return BusinessBoundQuery{}, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	if err := ValidateAnalyticalGroupedPopulations(c, binding); err != nil {
		return BusinessBoundQuery{}, err
	}
	if c.Version != AnalyticalGroupedFactsVersion {
		return BusinessBoundQuery{}, ErrBinding
	}
	bound, err := bindGroupedPopulationConstraints(ctx, binding, statement, c)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	base := c
	base.GroupSelection = nil
	return finishGroupedSelectionBinding(ctx, binding, c, base, bound)
}
