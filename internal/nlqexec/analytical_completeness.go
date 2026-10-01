package nlqexec

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

// selectedKnownAmountCompleteness inspects only directly selected SUM measures.
// A link on a transitive KPI leaf does not invent another selected output.
func selectedKnownAmountCompleteness(a admission) bool {
	if a.route.Selection == nil {
		return false
	}
	for i, pick := range a.route.Selection.Topics {
		if i >= len(a.publications) {
			return false
		}
		for _, root := range pick.Roots {
			if root.Reference.Kind != semantics.KindMeasure || root.Reason == "required_rule" {
				continue
			}
			for _, m := range a.publications[i].Definition.Measures {
				if m.ID == root.Reference.ID && m.Completeness != nil {
					return true
				}
			}
		}
	}
	return false
}

func compileAnalyticalCompleteness(a admission, c *exec.AnalyticalContract) error {
	if a.binding.Dialect != "postgres" && a.binding.Dialect != "mysql" {
		return analyticalUnsupported("analytical_dialect_unsupported")
	}
	if c == nil || c.Version != exec.AnalyticalScopedPopulationsVersion || c.ScalarPopulations != nil || c.GroupedPopulations != nil || len(c.Populations) != 0 || selectedMetricPeriods(a) || len(a.metricPeriods) != 0 {
		return exec.ErrBinding
	}
	p := &exec.AnalyticalCompleteness{Policy: exec.AnalyticalCompletenessPolicy}
	selected := map[string]bool{}
	for _, m := range c.Metrics {
		selected[m.ID] = true
	}
	for i, pick := range a.route.Selection.Topics {
		def := a.publications[i].Definition
		catalog := semantics.CompletenessCatalog{Measures: def.Measures, KPIs: def.KPIs, Columns: map[semantics.Reference]semantics.Column{}}
		for _, d := range def.Datasets {
			for _, column := range d.Columns {
				catalog.Columns[semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: column.ID}] = column
			}
		}
		for _, root := range pick.Roots {
			if root.Reference.Kind != semantics.KindMeasure || root.Reason == "required_rule" {
				continue
			}
			linked := false
			for _, m := range def.Measures {
				if m.ID == root.Reference.ID && m.Completeness != nil {
					linked = true
				}
			}
			if !linked {
				continue
			}
			resolved, err := semantics.ResolveKnownAmountCompleteness(catalog, root.Reference.ID)
			if err != nil {
				return exec.ErrBinding
			}
			metric := def.Topic + ":" + string(semantics.KindMeasure) + ":" + root.Reference.ID
			companion := def.Topic + ":" + string(semantics.KindKPI) + ":" + resolved.UnknownCount.ID
			if !selected[metric] || !selected[companion] {
				return analyticalUnsupported("analytical_completeness_output_required")
			}
			p.Obligations = append(p.Obligations, exec.AnalyticalCompletenessObligation{Metric: metric, UnknownCount: companion})
		}
	}
	sort.Slice(p.Obligations, func(i, j int) bool {
		a, b := p.Obligations[i], p.Obligations[j]
		if a.Metric != b.Metric {
			return a.Metric < b.Metric
		}
		return a.UnknownCount < b.UnknownCount
	})
	c.Completeness = p
	return exec.ValidateAnalyticalCompleteness(*c, a.binding)
}

// compiledKnownAmountCompleteness returns immutable compiled obligation evidence
// after current physical and reviewed semantic checks. Receipt reconstruction
// consumes it without parsing response labels or authoring prose.
func compiledKnownAmountCompleteness(c *exec.AnalyticalContract) *exec.AnalyticalCompleteness {
	if c == nil || c.Version != exec.AnalyticalScopedPopulationsVersion || c.Completeness == nil {
		return nil
	}
	return exec.CloneAnalyticalCompleteness(c.Completeness)
}

func (s *Service) verifyAnalyticalOutputReplay(ctx context.Context, e identity.Envelope, q QueryRecord, a admission) error {
	if q.AnalyticalVersion != analyticalScopedRecordVersion && q.AnalyticalVersion != analyticalGroupedOwnedRecordVersion {
		return nil
	}
	contract, err := s.expectedAnalytical(ctx, e, q, a)
	if err != nil {
		return err
	}
	if contract == nil {
		return exec.ErrBinding
	}
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: q.SQL, Parameters: q.Parameters}, a.relationScope)
	if err != nil {
		return err
	}
	proof, err := exec.CheckAnalyticalPlan(ctx, plan, *contract)
	if err != nil {
		return err
	}
	if exec.Hash(proof) != exec.Hash(q.Analytical) {
		return exec.ErrBinding
	}
	return nil
}
