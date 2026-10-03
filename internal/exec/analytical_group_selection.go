package exec

import (
	"context"
	"strings"
)

// V11 selects complete groups after independent fact alignment. It never
// assigns a shared dimension predicate to a guessed fact-owned population.
const AnalyticalGroupedSelectionVersion = "analytical-metrics-v11"
const AnalyticalGroupedSelectionPolicy = "owned-group-spine-predicates-v1"

func groupedSelectionHasPeriods(c AnalyticalContract) bool {
	if c.GroupedPopulations == nil {
		return false
	}
	for _, lane := range c.GroupedPopulations.Lanes {
		if lane.QueryPopulation != nil {
			return true
		}
	}
	return false
}

func validateGroupedSelection(c AnalyticalContract, b Binding) error {
	if c.ScalarEntailment != nil {
		return ErrBinding
	}
	if c.Version != AnalyticalGroupedSelectionVersion || b.Dialect != "postgres" || c.GroupedPopulations == nil || c.ScalarPopulations != nil || c.Completeness != nil || c.GroupDomain != nil || c.Grain == nil || len(c.Grain.Columns) == 0 || c.GroupSelection == nil || c.GroupSelection.Policy != AnalyticalQueryPopulationPolicy || len(c.GroupSelection.Constraints) == 0 || c.QueryPopulation == nil || len(c.QueryPopulation.Constraints) != 0 {
		return ErrBinding
	}
	columns := map[string]bool{}
	for _, column := range c.Grain.Columns {
		columns[column] = true
	}
	var datasets []string
	seen := map[string]bool{}
	for _, constraint := range c.GroupSelection.Constraints {
		if constraint.Aggregation != "" || !columns[AnalyticalColumnName(c.Dataset, constraint.Dataset, constraint.Column)] {
			return analyticalFailure("analytical_group_selection_unsupported", true)
		}
		if !seen[constraint.Dataset] {
			datasets = append(datasets, constraint.Dataset)
			seen[constraint.Dataset] = true
		}
	}
	canonical, err := NewAnalyticalQueryPopulationWithin(context.Background(), b, datasets, c.GroupSelection.Constraints)
	if err != nil {
		return err
	}
	if Hash(canonical) != Hash(c.GroupSelection) {
		return ErrBinding
	}
	return nil
}

// Observe only direct keys on the proven spine, never a nullable lane output.
func (a *analyticalChecker) rememberGroupSelectionKeys(keys map[string]analyticalTerm, aliases map[string]string, spine string) error {
	a.groupSelectionKeys = map[string][2]string{}
	for alias, name := range aliases {
		if name != spine {
			continue
		}
		for output, term := range keys {
			if term.column == "" || term.bucket != "" || term.aggregate {
				continue
			}
			if _, duplicate := a.groupSelectionKeys[term.column]; duplicate {
				return ErrBinding
			}
			a.groupSelectionKeys[term.column] = [2]string{alias, output}
		}
	}
	if a.groupSelection == nil {
		return nil
	}
	for _, constraint := range a.groupSelection.Constraints {
		if _, ok := a.groupSelectionKeys[AnalyticalColumnName(a.relation.ID, constraint.Dataset, constraint.Column)]; !ok {
			return analyticalFailure("analytical_group_selection_unsupported", true)
		}
	}
	return nil
}

func (a *analyticalChecker) checkGroupSelection(q map[string]any) error {
	if a.groupSelection == nil {
		if q["whereClause"] != nil {
			return analyticalFailure("analytical_query_population_mismatch", false)
		}
		return nil
	}
	predicate, parameters, _, err := groupedSelectionPredicate(a.ctx, a.binding, a.relation.ID, a.groupSelectionKeys, a.groupSelection.Constraints, 0)
	if err != nil {
		return err
	}
	expected, err := analyticalSQLTree(a.ctx, "SELECT 1 WHERE "+predicate, a.binding)
	if err != nil {
		return err
	}
	return a.matchPopulationClause(q["whereClause"], expected["whereClause"], parameters, false)
}

func (a *analyticalChecker) selectionColumn(node any) (string, bool) {
	parts, ok := groupedQualifiedColumn(node)
	if !ok {
		return "", false
	}
	for column, qualified := range a.groupSelectionKeys {
		if parts == qualified {
			return column, true
		}
	}
	return "", false
}

func groupedSelectionBase(c AnalyticalContract) AnalyticalContract {
	out := c
	out.GroupSelection = nil
	out.Version = AnalyticalGroupedProgramsVersion
	if groupedSelectionHasPeriods(c) {
		out.Version = AnalyticalGroupedOwnedPopulationsVersion
	}
	return out
}

func groupedSelectionScope(scope string) string {
	return strings.ReplaceAll(scope, "independent_grouped_populations", "independent_selected_grouped_populations")
}
