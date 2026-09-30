package nlqexec

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

// compileAnalyticalGroupedPopulations consumes only an explicit published
// alignment policy. A policy never introduces a selected metric or source grant.
func compileAnalyticalGroupedPopulations(ctx context.Context, a admission, c *exec.AnalyticalContract) (bool, error) {
	if c.Version != exec.AnalyticalGroupedPopulationsVersion || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0 {
		return false, nil
	}
	facts := map[string][]exec.AnalyticalExpression{}
	var visit func(exec.AnalyticalExpression)
	visit = func(e exec.AnalyticalExpression) {
		if e.Column != "" {
			id := c.Dataset
			if parts := strings.SplitN(e.Column, "/", 2); len(parts) == 2 {
				id = parts[0]
			}
			facts[id] = append(facts[id], e)
		}
		for _, arg := range e.Args {
			visit(arg)
		}
	}
	for _, metric := range c.Metrics {
		visit(metric.Expression)
	}
	if len(facts) < 2 {
		return false, nil
	}
	ids := make([]string, 0, len(facts))
	for id := range facts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	reviewed := false
	for _, pub := range a.publications {
		policy := pub.Definition.GroupedPopulation
		if policy == nil {
			continue
		}
		if policy.Policy != semantics.GroupedPopulationUnionPolicy {
			return true, exec.ErrBinding
		}
		if exec.Hash(policy.Datasets) == exec.Hash(ids) {
			reviewed = true
		}
	}
	if !reviewed {
		// Existing reviewed raw joins keep their own population semantics and
		// still require physical nonmultiplication proof. They gain no union.
		return false, nil
	}
	if a.binding.Dialect != "postgres" || len(facts) > 4 || len(c.Grain.Buckets) > 0 || c.QueryPopulation != nil && len(c.QueryPopulation.Constraints) > 0 {
		return true, analyticalUnsupported("analytical_shape_unsupported")
	}
	grouped := &exec.AnalyticalGroupedPopulations{Policy: exec.AnalyticalGroupedPopulationPolicy}
	for _, id := range ids {
		lane := exec.AnalyticalContract{Version: exec.AnalyticalIntentVersion, Dataset: id, Grain: &exec.AnalyticalGrain{Policy: c.Grain.Policy, Dimensions: append([]string(nil), c.Grain.Dimensions...)}}
		for _, column := range c.Grain.Columns {
			e := rebaseAnalyticalExpression(exec.AnalyticalExpression{Column: column}, c.Dataset, id)
			lane.Grain.Columns = append(lane.Grain.Columns, e.Column)
		}
		sort.Strings(lane.Grain.Columns)
		for i, leaf := range facts[id] {
			lane.Metrics = append(lane.Metrics, exec.AnalyticalMetric{ID: string(rune('a' + i)), Expression: rebaseAnalyticalExpression(leaf, c.Dataset, id)})
		}
		// Reuse reviewed path selection and physical uniqueness, separately for each
		// population. The aggregate input from another fact is never present here.
		if err := compileAnalyticalJoins(ctx, a, &lane); err != nil {
			return true, err
		}
		grouped.Lanes = append(grouped.Lanes, exec.AnalyticalGroupedLane{Dataset: id, Joins: lane.Joins})
	}
	c.GroupedPopulations = grouped
	if err := exec.ValidateAnalyticalGroupedPopulations(*c, a.binding); err != nil {
		return true, err
	}
	return true, nil
}

func analyticalGroupedPopulationGuidance(c *exec.AnalyticalContract) string {
	if c == nil || c.GroupedPopulations == nil {
		return ""
	}
	lanes, _ := json.Marshal(c.GroupedPopulations) // Closed source coordinates; no values or SQL.
	return " Reviewed grouped-population policy: independently aggregate each selected fact in its own named CTE at exactly the selected shared dimension columns. In each lane use only its reviewed physically unique dimension joins and metric predicates. Build one key-spine CTE with UNION (distinct, never UNION ALL) of the complete named grouping keys from every lane, in the same order. LEFT JOIN every lane to that spine using IS NOT DISTINCT FROM on every grouping key. Project grouping keys from the spine and selected metrics using lane aggregate outputs. Preserve missing lane measures as NULL, including counts; do not COALESCE, filter, limit, or join raw facts inside a lane or the spine. Outer ordering/limit follows reviewed intent. Exact compiled lane contract: " + string(lanes)
}
