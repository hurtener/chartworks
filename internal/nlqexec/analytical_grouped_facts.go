package nlqexec

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
)

const analyticalGroupedFactsRecordVersion = 12

func compileAnalyticalGroupedFacts(ctx context.Context, a admission, supplied [][]exec.BusinessConstraint) (*exec.AnalyticalContract, error) {
	if ctx == nil || len(supplied) != 1 || len(supplied[0]) == 0 || !hasActiveBusinessEvidence(a.route) || a.route.SourceBindingDigest != exec.Hash(a.binding) {
		return nil, exec.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseVersion := analyticalRecordVersion
	if selectedMetricPeriods(a) || len(a.metricPeriods) > 0 {
		baseVersion = analyticalGroupedOwnedRecordVersion
	}
	base, err := compileAnalyticalVersion(ctx, a, baseVersion, []exec.BusinessConstraint{})
	if err != nil || base == nil || base.GroupedPopulations == nil {
		return compileAnalyticalGroupedSelection(ctx, a, supplied)
	}
	if selectedKnownAmountCompleteness(a) || a.binding.Dialect != "postgres" || base.Grain == nil {
		return nil, analyticalUnsupported("analytical_fact_predicate_unsupported")
	}
	var ids []string
	for _, scope := range a.relationScope {
		ids = append(ids, scope.Dataset)
	}
	canonical, err := exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, ids, supplied[0])
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for _, key := range base.Grain.Columns {
		keys[key] = true
	}
	facts := map[string][]exec.BusinessConstraint{}
	var groups []exec.BusinessConstraint
	for _, constraint := range canonical.Constraints {
		if keys[exec.AnalyticalColumnName(base.Dataset, constraint.Dataset, constraint.Column)] {
			groups = append(groups, constraint)
			continue
		}
		owner, err := exec.AnalyticalGroupedFactOwner(*base, constraint)
		if err != nil {
			return nil, err
		}
		facts[owner] = append(facts[owner], constraint)
	}
	// Preserve immutable v11 routing for pure final-group selection.
	if len(facts) == 0 {
		return compileAnalyticalGroupedSelection(ctx, a, supplied)
	}
	for i := range base.GroupedPopulations.Lanes {
		lane := &base.GroupedPopulations.Lanes[i]
		if len(facts[lane.Dataset]) == 0 {
			continue
		}
		// A newly filtered population must have an explicit reviewed domain, not
		// merely the unfiltered v8 default inferred from an aggregate's presence.
		reviewed := false
		for _, publication := range a.publications {
			policy := publication.Definition.GroupedPopulation
			if policy == nil {
				continue
			}
			var selected []string
			for _, l := range base.GroupedPopulations.Lanes {
				selected = append(selected, l.Dataset)
			}
			if exec.Hash(policy.Datasets) != exec.Hash(selected) {
				continue
			}
			for _, domain := range policy.GroupDomains {
				if domain.Dataset == lane.Dataset && domain.Domain == lane.Domain {
					reviewed = true
				}
			}
		}
		if !reviewed {
			return nil, analyticalUnsupported(exec.AnalyticalGroupDomainReviewCode)
		}
		lane.FactPopulation, err = exec.NewAnalyticalQueryPopulation(ctx, a.binding, lane.Dataset, facts[lane.Dataset])
		if err != nil {
			return nil, err
		}
	}
	if len(groups) > 0 {
		base.GroupSelection, err = exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, ids, groups)
		if err != nil {
			return nil, err
		}
	}
	base.Version = exec.AnalyticalGroupedFactsVersion
	if err := exec.ValidateAnalyticalGroupedPopulations(*base, a.binding); err != nil {
		return nil, err
	}
	return base, nil
}
