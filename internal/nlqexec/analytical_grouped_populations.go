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
	if (c.Version != exec.AnalyticalGroupedPopulationsVersion && c.Version != exec.AnalyticalGroupedProgramsVersion) || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0 {
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
	domains := map[string]string{}
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
			for _, domain := range policy.GroupDomains {
				if old := domains[domain.Dataset]; old != "" && old != domain.Domain {
					return true, exec.ErrBinding
				}
				domains[domain.Dataset] = domain.Domain
			}
		}
	}
	if !reviewed {
		// Existing reviewed raw joins keep their own population semantics and
		// still require physical nonmultiplication proof. They gain no union.
		return false, nil
	}
	if (a.binding.Dialect != "postgres" && (c.Version != exec.AnalyticalGroupedProgramsVersion || a.binding.Dialect != "mysql")) || len(facts) > 4 || c.Version == exec.AnalyticalGroupedPopulationsVersion && len(c.Grain.Buckets) > 0 || c.QueryPopulation != nil && len(c.QueryPopulation.Constraints) > 0 {
		return true, analyticalUnsupported("analytical_shape_unsupported")
	}
	grouped := &exec.AnalyticalGroupedPopulations{Policy: exec.AnalyticalGroupedPopulationPolicy}
	for _, id := range ids {
		domain := ""
		if c.Version == exec.AnalyticalGroupedProgramsVersion {
			domain = domains[id]
			if domain == "" {
				unfiltered := false
				for _, leaf := range facts[id] {
					unfiltered = unfiltered || len(leaf.Filters) == 0
				}
				if !unfiltered {
					return true, analyticalUnsupported(exec.AnalyticalGroupDomainReviewCode)
				}
				domain = exec.AnalyticalGroupDomainRaw
			}
			if domain != exec.AnalyticalGroupDomainRaw && domain != exec.AnalyticalGroupDomainQualifying {
				return true, exec.ErrBinding
			}
		}
		lane := exec.AnalyticalContract{Version: exec.AnalyticalIntentVersion, Dataset: id, Grain: &exec.AnalyticalGrain{Policy: c.Grain.Policy, Dimensions: append([]string(nil), c.Grain.Dimensions...)}}
		for _, column := range c.Grain.Columns {
			e := rebaseAnalyticalExpression(exec.AnalyticalExpression{Column: column}, c.Dataset, id)
			lane.Grain.Columns = append(lane.Grain.Columns, e.Column)
		}
		for _, bucket := range c.Grain.Buckets {
			bucket.Column = rebaseAnalyticalExpression(exec.AnalyticalExpression{Column: bucket.Column}, c.Dataset, id).Column
			lane.Grain.Buckets = append(lane.Grain.Buckets, bucket)
		}
		sort.Slice(lane.Grain.Buckets, func(i, j int) bool { return exec.Hash(lane.Grain.Buckets[i]) < exec.Hash(lane.Grain.Buckets[j]) })
		sort.Strings(lane.Grain.Columns)
		for i, leaf := range facts[id] {
			lane.Metrics = append(lane.Metrics, exec.AnalyticalMetric{ID: string(rune('a' + i)), Expression: rebaseAnalyticalExpression(leaf, c.Dataset, id)})
		}
		// Reuse reviewed path selection and physical uniqueness, separately for each
		// population. The aggregate input from another fact is never present here.
		if err := compileAnalyticalJoins(ctx, a, &lane); err != nil {
			return true, err
		}
		grouped.Lanes = append(grouped.Lanes, exec.AnalyticalGroupedLane{Dataset: id, Joins: lane.Joins, Domain: domain})
	}
	c.GroupedPopulations = grouped
	if err := exec.ValidateAnalyticalGroupedPopulations(*c, a.binding); err != nil {
		return true, err
	}
	return true, nil
}

func analyticalGroupedPopulationGuidance(c *exec.AnalyticalContract, dialects ...string) string {
	if c == nil || c.GroupedPopulations == nil {
		return ""
	}
	public := *c.GroupedPopulations
	public.Lanes = append([]exec.AnalyticalGroupedLane(nil), c.GroupedPopulations.Lanes...)
	for i := range public.Lanes {
		public.Lanes[i].QueryPopulation = nil
	}
	lanes, _ := json.Marshal(public) // Closed source coordinates; no values or SQL.
	extensions := ""
	nullEquality := "IS NOT DISTINCT FROM"
	if len(dialects) > 0 && dialects[0] == "mysql" {
		nullEquality = "<=>"
	}
	spineForm := "CTE"
	if c.Version == exec.AnalyticalGroupedProgramsVersion || c.Version == exec.AnalyticalGroupedOwnedPopulationsVersion {
		spineForm = "CTE or derived SELECT"
		extensions = " Each lane contract explicitly names its group domain. raw_source_groups requires no row WHERE: keep reviewed metric predicates in FILTER/CASE aggregates. qualifying_population requires WHERE to be exactly the union (OR) of complete reviewed metric-filter populations, while each aggregate retains its own predicate. An unfiltered metric makes that union all rows. A qualifying all-NULL aggregate input still produces its group. Shared reviewed calendar buckets are exact group keys, including source field, calendar, unit and timezone. Transparent SELECT projections may wrap lanes or the complete query; a key spine may be a derived SELECT. Wrappers may only rename or reorder proved columns. Keep reviewed ORDER/LIMIT at the final SELECT, never inside a wrapped program. "
	}
	if c.Version == exec.AnalyticalGroupedOwnedPopulationsVersion {
		spineForm = "named CTE"
		extensions = " The service binds each authenticated reviewed fact-owned period inside that exact fact aggregate CTE. Supply no parameters or private period values. Use flat named aggregate CTEs and a named distinct UNION spine; derived wrappers are unsupported for owned periods. A raw domain has only the service-owned row restriction; a qualifying domain also has exactly the reviewed metric-population union. Keep each aggregate predicate. "
	}
	return extensions + " Reviewed grouped-population policy: independently aggregate each selected fact in its own named CTE at exactly the selected shared dimension columns. In each lane use only its reviewed physically unique dimension joins and metric predicates. Build one key-spine " + spineForm + " with UNION (distinct, never UNION ALL) of the complete named grouping keys from every lane, in the same order. LEFT JOIN every lane to that spine using " + nullEquality + " on every grouping key. Project grouping keys from the spine and selected metrics using lane aggregate outputs. Preserve missing lane measures as NULL, including counts; do not add unreviewed COALESCE, filters, limits, or raw fact joins inside a lane or the spine. Outer ordering/limit follows reviewed intent. Exact compiled lane contract: " + string(lanes)
}
