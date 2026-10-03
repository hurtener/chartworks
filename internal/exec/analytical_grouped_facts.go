package exec

import (
	"context"
	"sort"
)

// V12 adds exact primary-fact predicates, separate from retained periods and
// final group selection. It grants no ownership to joined/shared coordinates.
const AnalyticalGroupedFactsVersion = "analytical-metrics-v12"
const AnalyticalGroupedFactPolicy = "grouped-fact-predicates-v1"

// AnalyticalGroupedFactOwner accepts only an exact selected primary fact which
// is not also a dependency of another population. Coincidental join membership
// cannot create either lane ownership or a query-wide predicate.
func AnalyticalGroupedFactOwner(c AnalyticalContract, constraint BusinessConstraint) (string, error) {
	if c.GroupedPopulations == nil || c.Grain == nil || constraint.Aggregation != "" {
		return "", analyticalFailure("analytical_fact_predicate_unsupported", true)
	}
	for _, column := range c.Grain.Columns {
		if column == AnalyticalColumnName(c.Dataset, constraint.Dataset, constraint.Column) {
			return "", analyticalFailure("analytical_fact_predicate_unsupported", true)
		}
	}
	owner := ""
	for _, lane := range c.GroupedPopulations.Lanes {
		if lane.Dataset == constraint.Dataset {
			if owner != "" {
				return "", ErrBinding
			}
			owner = lane.Dataset
			continue
		}
		for _, join := range lane.Joins {
			if join.Left == constraint.Dataset || join.Right == constraint.Dataset {
				return "", analyticalFailure("analytical_fact_predicate_unsupported", true)
			}
		}
	}
	if owner == "" {
		return "", analyticalFailure("analytical_fact_predicate_unsupported", true)
	}
	return owner, nil
}

// Detach the new field before applying the unchanged v8/v10/v11 proof. Do not
// relabel the original contract or mutate a retained population.
func groupedFactsBase(c AnalyticalContract, selection bool) AnalyticalContract {
	out := c
	if c.GroupedPopulations != nil {
		grouped := *c.GroupedPopulations
		grouped.Lanes = append([]AnalyticalGroupedLane(nil), grouped.Lanes...)
		for i := range grouped.Lanes {
			grouped.Lanes[i].FactPopulation = nil
		}
		out.GroupedPopulations = &grouped
	}
	out.Version = AnalyticalGroupedProgramsVersion
	if groupedSelectionHasPeriods(c) {
		out.Version = AnalyticalGroupedOwnedPopulationsVersion
	}
	if selection && out.GroupSelection != nil {
		out.Version = AnalyticalGroupedSelectionVersion
	} else {
		out.GroupSelection = nil
	}
	return out
}

func validateGroupedFacts(c AnalyticalContract, b Binding) error {
	if c.Version != AnalyticalGroupedFactsVersion || b.Dialect != "postgres" || c.GroupedPopulations == nil || c.ScalarPopulations != nil || c.Completeness != nil || c.GroupDomain != nil || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0 || c.QueryPopulation == nil || c.QueryPopulation.Policy != AnalyticalQueryPopulationPolicy || len(c.QueryPopulation.Constraints) != 0 {
		return ErrBinding
	}
	if err := ValidateAnalyticalGroupedPopulations(groupedFactsBase(c, true), b); err != nil {
		return err
	}
	count := 0
	seen := map[string]bool{}
	for _, lane := range c.GroupedPopulations.Lanes {
		if lane.QueryPopulation != nil {
			for _, constraint := range lane.QueryPopulation.Constraints {
				seen[constraint.Resolution] = true
			}
		}
	}
	var all []BusinessConstraint
	for _, lane := range c.GroupedPopulations.Lanes {
		if lane.QueryPopulation != nil {
			all = append(all, lane.QueryPopulation.Constraints...)
		}
		if lane.FactPopulation == nil {
			continue
		}
		if lane.Domain == "" || len(lane.FactPopulation.Constraints) == 0 {
			return ErrBinding
		}
		for _, join := range lane.Joins {
			if join.Type != "inner" {
				return analyticalFailure("analytical_fact_predicate_unsupported", true)
			}
		}
		canonical, err := NewAnalyticalQueryPopulation(context.Background(), b, lane.Dataset, lane.FactPopulation.Constraints)
		if err != nil {
			return err
		}
		if Hash(canonical) != Hash(lane.FactPopulation) {
			return ErrBinding
		}
		for _, constraint := range canonical.Constraints {
			if seen[constraint.Resolution] {
				return ErrBinding
			}
			seen[constraint.Resolution] = true
			owner, err := AnalyticalGroupedFactOwner(c, constraint)
			if err != nil {
				return err
			}
			if owner != lane.Dataset {
				return ErrBinding
			}
			count++
		}
		all = append(all, canonical.Constraints...)
	}
	if count == 0 || count > 64 {
		return ErrBinding
	}
	if c.GroupSelection != nil {
		for _, constraint := range c.GroupSelection.Constraints {
			if seen[constraint.Resolution] {
				return ErrBinding
			}
			seen[constraint.Resolution] = true
		}
		all = append(all, c.GroupSelection.Constraints...)
	}
	// Period resolutions can intentionally occur in multiple reviewed lanes;
	// fact and final-selection resolutions must remain globally distinct from
	// those periods and each other; the merged lane is checked independently too.
	if len(all) > 64 {
		return ErrLimit
	}
	return nil
}

func groupedLanePopulation(lane AnalyticalGroupedLane) *AnalyticalQueryPopulation {
	if lane.FactPopulation == nil {
		return lane.QueryPopulation
	}
	out := &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	if lane.QueryPopulation != nil {
		out.Constraints = append(out.Constraints, lane.QueryPopulation.Constraints...)
	}
	out.Constraints = append(out.Constraints, lane.FactPopulation.Constraints...)
	// Canonical order is the same one used by the underlying population compiler.
	sort.Slice(out.Constraints, func(i, j int) bool { return out.Constraints[i].Resolution < out.Constraints[j].Resolution })
	return out
}
