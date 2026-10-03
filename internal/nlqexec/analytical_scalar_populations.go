package nlqexec

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

const AnalyticalMetricPeriodReviewCode = "analytical_metric_period_review_required"

func selectedMetricPeriods(a admission) bool {
	if a.route.Selection == nil {
		return false
	}
	for i, pick := range a.route.Selection.Topics {
		if i >= len(a.publications) {
			return false
		}
		for _, root := range pick.Roots {
			if root.Reason == "required_rule" || root.Reference.Kind != semantics.KindKPI {
				continue
			}
			for _, k := range a.publications[i].Definition.KPIs {
				if k.ID == root.Reference.ID && k.Periods != nil {
					return true
				}
			}
		}
	}
	return false
}

// compileAnalyticalScalarPopulations consumes current sealed applications and
// independently matches every one to the selected immutable semantic policy.
// V9 produces scalar lanes; v10 adds reviewed grouped domains and grain.
// Period and filter dependencies never define the set of aggregated facts.
func compileAnalyticalScalarPopulations(ctx context.Context, a admission, c *exec.AnalyticalContract) (bool, error) {
	if !selectedMetricPeriods(a) {
		if len(a.metricPeriods) > 0 {
			return true, exec.ErrBinding
		}
		return false, nil
	}
	if len(a.metricPeriods) == 0 {
		return true, analyticalUnsupported(AnalyticalMetricPeriodReviewCode)
	}
	if len(a.metricPeriods) > 4 || (c.Version != exec.AnalyticalScopedPopulationsVersion && c.Version != exec.AnalyticalGroupedOwnedPopulationsVersion) {
		return true, exec.ErrBinding
	}
	if a.binding.Dialect != "postgres" || c.Version == exec.AnalyticalScopedPopulationsVersion && c.Grain != nil && len(c.Grain.Columns)+len(c.Grain.Buckets) > 0 || c.QueryPopulation != nil && len(c.QueryPopulation.Constraints) > 0 {
		return true, analyticalUnsupported("analytical_shape_unsupported")
	}
	applications := map[string]nlqroute.MetricPeriodApplication{}
	for _, application := range a.metricPeriods {
		key := application.Topic + ":" + application.KPI
		if _, duplicate := applications[key]; duplicate {
			return true, exec.ErrBinding
		}
		if application.Policy != nlqroute.MetricPeriodApplicationPolicy || application.SourceBindingDigest != exec.Hash(a.binding) || len(application.Bindings) < 1 || len(application.Bindings) > 4 {
			return true, exec.ErrBinding
		}
		applications[key] = application
	}
	allLeaves := map[string]bool{}
	mappedLeaves := map[string]bool{}
	periods := map[string]exec.BusinessConstraint{}
	dimensions := map[string]string{}
	required := map[string]map[string]bool{}
	usedApplications := map[string]bool{}
	for i, pick := range a.route.Selection.Topics {
		pub := a.publications[i]
		def := pub.Definition
		measures := map[string]semantics.Measure{}
		kpis := map[string]semantics.KPI{}
		dims := map[string]semantics.Dimension{}
		for _, m := range def.Measures {
			measures[m.ID] = m
		}
		for _, k := range def.KPIs {
			kpis[k.ID] = k
		}
		for _, d := range def.Dimensions {
			dims[d.ID] = d
		}
		var collect func(semantics.Reference, int, map[string]bool) error
		nodes := 0
		collect = func(ref semantics.Reference, depth int, leaves map[string]bool) error {
			nodes++
			if nodes > 1024 || depth > 32 {
				return exec.ErrLimit
			}
			if ref.Kind == semantics.KindMeasure {
				if _, ok := measures[ref.ID]; !ok {
					return exec.ErrBinding
				}
				leaves[ref.ID] = true
				return nil
			}
			k, ok := kpis[ref.ID]
			if ref.Kind != semantics.KindKPI || !ok {
				return exec.ErrBinding
			}
			for _, input := range k.Inputs {
				if err := collect(input, depth+1, leaves); err != nil {
					return err
				}
			}
			return nil
		}
		for _, root := range pick.Roots {
			if root.Reason == "required_rule" || root.Reference.Kind != semantics.KindMeasure && root.Reference.Kind != semantics.KindKPI {
				continue
			}
			leaves := map[string]bool{}
			if err := collect(root.Reference, 0, leaves); err != nil {
				return true, err
			}
			for id := range leaves {
				allLeaves[def.Topic+":"+id] = true
			}
			k := kpis[root.Reference.ID]
			if root.Reference.Kind != semantics.KindKPI || k.Periods == nil {
				continue
			}
			key := def.Topic + ":" + k.ID
			application, ok := applications[key]
			if !ok {
				return true, analyticalUnsupported(AnalyticalMetricPeriodReviewCode)
			}
			usedApplications[key] = true
			if application.TopicVersion != def.Version || application.PackDigest != pub.Digest || application.MappingDigest != exec.Hash(k.Periods) || k.Periods.Policy != semantics.MetricPeriodBindingsPolicy || len(k.Periods.Bindings) != len(leaves) || len(application.Bindings) != len(leaves) {
				return true, exec.ErrBinding
			}
			mapping := map[semantics.Reference]semantics.Reference{}
			for j, b := range k.Periods.Bindings {
				if !b.Measure.Valid() || b.Measure.Kind != semantics.KindMeasure || !b.Dimension.Valid() || b.Dimension.Kind != semantics.KindDimension || !leaves[b.Measure.ID] || j > 0 && k.Periods.Bindings[j-1].Measure.ID >= b.Measure.ID {
					return true, exec.ErrBinding
				}
				mapping[b.Measure] = b.Dimension
			}
			seen := map[semantics.Reference]bool{}
			for _, b := range application.Bindings {
				if seen[b.Measure] || mapping[b.Measure] != b.Dimension {
					return true, exec.ErrBinding
				}
				seen[b.Measure] = true
				measure, ok := measures[b.Measure.ID]
				if !ok || b.Population != measure.Field.Dataset {
					return true, exec.ErrBinding
				}
				dimension, ok := dims[b.Dimension.ID]
				if !ok {
					return true, exec.ErrBinding
				}
				if err := validateScalarPeriodBinding(ctx, a, def, dimension, b); err != nil {
					return true, err
				}
				fact := b.Population
				axis := def.Topic + ":" + dimension.ID
				if prior, exists := dimensions[fact]; exists && prior != axis {
					return true, analyticalUnsupported("analytical_period_axis_ambiguous")
				}
				dimensions[fact] = axis
				if prior, exists := periods[fact]; exists {
					left, right := prior, b.Constraint
					left.Resolution = ""
					right.Resolution = ""
					if exec.Hash(left) != exec.Hash(right) {
						return true, analyticalUnsupported("analytical_period_axis_ambiguous")
					}
					if b.Constraint.Resolution < prior.Resolution {
						periods[fact] = b.Constraint
					}
				} else {
					periods[fact] = b.Constraint
				}
				mappedLeaves[def.Topic+":"+measure.ID] = true
				for _, filter := range measure.Filters {
					if filter.Relationship == "" {
						continue
					}
					if _, ok := semantics.PopulationRelationship(semantics.TopicPack{Joins: def.Joins}, measure.Field.Dataset, filter.Field.Dataset, filter.Relationship); !ok {
						return true, exec.ErrBinding
					}
					if required[fact] == nil {
						required[fact] = map[string]bool{}
					}
					required[fact][def.Topic+":"+filter.Relationship] = true
				}
			}
		}
	}
	if len(usedApplications) != len(applications) || len(allLeaves) != len(mappedLeaves) {
		return true, exec.ErrBinding
	}
	for key := range allLeaves {
		if !mappedLeaves[key] {
			return true, exec.ErrBinding
		}
	}
	facts := map[string][]exec.AnalyticalExpression{}
	var visit func(exec.AnalyticalExpression)
	visit = func(e exec.AnalyticalExpression) {
		if e.Column != "" {
			id := c.Dataset
			if p := strings.SplitN(e.Column, "/", 2); len(p) == 2 {
				id = p[0]
			}
			facts[id] = append(facts[id], e)
		}
		for _, arg := range e.Args {
			visit(arg)
		}
	}
	for _, m := range c.Metrics {
		visit(m.Expression)
	}
	if len(facts) < 2 || len(facts) > 4 || len(facts) != len(periods) {
		return true, analyticalUnsupported("analytical_shape_unsupported")
	}
	ids := make([]string, 0, len(facts))
	for id := range facts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var grouped *exec.AnalyticalGroupedPopulations
	if c.Version == exec.AnalyticalGroupedOwnedPopulationsVersion {
		view := *c
		view.Version = exec.AnalyticalGroupedProgramsVersion
		handled, err := compileAnalyticalGroupedPopulations(ctx, a, &view)
		if err != nil {
			return true, err
		}
		if !handled || view.GroupedPopulations == nil {
			return true, exec.ErrBinding
		}
		grouped = view.GroupedPopulations
	}
	program := &exec.AnalyticalScalarPopulations{Policy: exec.AnalyticalScalarPopulationPolicy}
	for _, id := range ids {
		period, ok := periods[id]
		if !ok {
			return true, exec.ErrBinding
		}
		idsInScope := make([]string, 0, len(a.relationScope))
		for _, r := range a.relationScope {
			idsInScope = append(idsInScope, r.Dataset)
		}
		population, err := exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, idsInScope, []exec.BusinessConstraint{period})
		if err != nil {
			return true, err
		}
		lane := exec.AnalyticalContract{Version: exec.AnalyticalGroupedProgramsVersion, Dataset: id, QueryPopulation: population}
		if grouped != nil {
			lane.Grain = &exec.AnalyticalGrain{Policy: c.Grain.Policy, Dimensions: append([]string(nil), c.Grain.Dimensions...)}
			for _, column := range c.Grain.Columns {
				lane.Grain.Columns = append(lane.Grain.Columns, rebaseAnalyticalExpression(exec.AnalyticalExpression{Column: column}, c.Dataset, id).Column)
			}
			for _, bucket := range c.Grain.Buckets {
				bucket.Column = rebaseAnalyticalExpression(exec.AnalyticalExpression{Column: bucket.Column}, c.Dataset, id).Column
				lane.Grain.Buckets = append(lane.Grain.Buckets, bucket)
			}
		}
		for i, leaf := range facts[id] {
			lane.Metrics = append(lane.Metrics, exec.AnalyticalMetric{ID: string(rune('a' + i)), Expression: rebaseAnalyticalExpression(leaf, c.Dataset, id)})
		}
		if err := compileAnalyticalJoinTree(ctx, a, &lane, false, required[id]); err != nil {
			return true, err
		}
		for _, j := range lane.Joins {
			if j.Type != "inner" {
				return true, analyticalUnsupported("analytical_shape_unsupported")
			}
		}
		if grouped != nil {
			for i := range grouped.Lanes {
				if grouped.Lanes[i].Dataset == id {
					grouped.Lanes[i].Joins = lane.Joins
					grouped.Lanes[i].QueryPopulation = population
				}
			}
		}
		program.Lanes = append(program.Lanes, exec.AnalyticalScalarLane{Dataset: id, Joins: lane.Joins, QueryPopulation: population})
	}
	if grouped != nil {
		c.GroupedPopulations = grouped
		return true, exec.ValidateAnalyticalGroupedPopulations(*c, a.binding)
	}
	c.ScalarPopulations = program
	return true, exec.ValidateAnalyticalScalarPopulations(ctx, *c, a.binding)
}

func validateScalarPeriodBinding(ctx context.Context, a admission, def topics.Definition, d semantics.Dimension, b nlqroute.MetricPeriodBinding) error {
	compiler := analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, joins: true}
	column, err := compiler.column(d.Field)
	if err != nil {
		return err
	}
	constraint := b.Constraint
	if constraint.Kind != "time_window" || constraint.Aggregation != "" || constraint.Operator != "range" || constraint.Nulls != "exclude" || constraint.Null || constraint.Bounds != "[)" || constraint.Dataset != d.Field.Dataset || constraint.Column != column.SourceName || constraint.SourceRevision != a.binding.Revision {
		return exec.ErrBinding
	}
	if _, err := compileCalendarBucket(grainDimension{field: d.Field, role: d.Role, filters: d.Filters, temporal: d.Temporal}, column, constraint.Grain, a.binding.Dialect); err != nil {
		return err
	}
	zone := d.Temporal.Timezone
	if zone == "" {
		zone = "UTC"
	}
	kind := exec.AnalyticalCalendarKindForDialect(a.binding.Dialect, column.NativeType, column.Category)
	native := map[string]string{"date": "date", "civil": "timestamp", "instant": "timestamptz"}[kind]
	if native == "" || constraint.TemporalType != native || constraint.Calendar != d.Temporal.Calendar || constraint.TimeZone != zone {
		return exec.ErrBinding
	}
	return exec.ValidateBusinessConstraints(a.binding, []exec.BusinessConstraint{constraint})
}

func analyticalScalarPopulationGuidance(c *exec.AnalyticalContract) string {
	if c == nil || c.ScalarPopulations == nil {
		return ""
	}
	type lane struct {
		Dataset       string                `json:"dataset"`
		Joins         []exec.AnalyticalJoin `json:"joins,omitempty"`
		PeriodColumns []string              `json:"period_columns"`
	}
	var lanes []lane
	for _, p := range c.ScalarPopulations.Lanes {
		l := lane{Dataset: p.Dataset, Joins: p.Joins}
		for _, owned := range p.QueryPopulation.Constraints {
			l.PeriodColumns = append(l.PeriodColumns, owned.Dataset+"/"+owned.Column)
		}
		lanes = append(lanes, l)
	}
	raw, _ := json.Marshal(lanes)
	return " Independently aggregate each fact in its own named flat CTE with named aggregate outputs, no model parameters, GROUP BY, HAVING, ORDER BY or LIMIT. Use exactly the reviewed INNER parent joins and complete composite equality keys inside the owning fact lane. Keep each metric's reviewed predicates. The service alone binds the current period to each listed fact-owned target; never supply dates, copy interval values, or move a period to another lane. CROSS JOIN the singleton CTEs and compute only the reviewed outer arithmetic; preserve NULL and count-zero behavior, with no unreviewed COALESCE. Exact value-free lane placement: " + string(raw)
}
