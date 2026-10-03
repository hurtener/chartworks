package semantics

// MetricPeriodBindings pins reviewed period meaning to exact transitive measure
// leaves. It contains no literal dates, SQL, permissions or execution plan.
type MetricPeriodBindings struct {
	Policy   string                `json:"policy"`
	Bindings []MetricPeriodBinding `json:"bindings"`
}
type MetricPeriodBinding struct {
	Measure   Reference `json:"measure"`
	Dimension Reference `json:"dimension"`
}

const MetricPeriodBindingsPolicy = "metric-period-bindings-v1"

func validateMetricPeriodsShape(periods *MetricPeriodBindings) error {
	if periods == nil {
		return nil
	}
	if periods.Policy != MetricPeriodBindingsPolicy || len(periods.Bindings) < 1 || len(periods.Bindings) > 4 {
		return invalid(CodeInvalidValue, "kpis.periods")
	}
	for _, binding := range periods.Bindings {
		if !binding.Measure.Valid() || binding.Measure.Kind != KindMeasure || !binding.Dimension.Valid() || binding.Dimension.Kind != KindDimension {
			return invalid(CodeInvalidReference, "kpis.periods.bindings")
		}
	}
	return nil
}

func cloneMetricPeriods(periods *MetricPeriodBindings) *MetricPeriodBindings {
	if periods == nil {
		return nil
	}
	out := *periods
	out.Bindings = append([]MetricPeriodBinding(nil), periods.Bindings...)
	return &out
}

func validateMetricPeriodReferences(pack TopicPack) error {
	measures := map[string]Measure{}
	dimensions := map[string]Dimension{}
	kpis := map[string]KPI{}
	sources := map[string]SourceReference{}
	for _, m := range pack.Measures {
		measures[m.ID] = m
	}
	for _, d := range pack.Dimensions {
		dimensions[d.ID] = d
	}
	for _, k := range pack.KPIs {
		kpis[k.ID] = k
	}
	for _, d := range pack.Datasets {
		sources[d.ID] = d.Source
	}
	for _, root := range pack.KPIs {
		if root.Periods == nil {
			continue
		}
		leaves := map[string]bool{}
		nested := map[string]KPI{}
		nodes := 0
		var walk func(Reference, int) error
		walk = func(ref Reference, depth int) error {
			nodes++
			if nodes > 1024 || depth > 32 {
				return invalid(CodeLimit, "kpis.periods")
			}
			if ref.Kind == KindMeasure {
				if _, ok := measures[ref.ID]; !ok {
					return invalid(CodeMissingReference, "kpis.periods")
				}
				leaves[ref.ID] = true
				return nil
			}
			child, ok := kpis[ref.ID]
			if ref.Kind != KindKPI || !ok {
				return invalid(CodeMissingReference, "kpis.periods")
			}
			nested[ref.ID] = child
			for _, input := range child.Inputs {
				if err := walk(input, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		for _, input := range root.Inputs {
			if err := walk(input, 0); err != nil {
				return err
			}
		}
		if len(leaves) != len(root.Periods.Bindings) {
			return invalid(CodeInvalidReference, "kpis.periods.coverage")
		}
		mapped := map[string]Reference{}
		byFact := map[string]Reference{}
		for _, binding := range root.Periods.Bindings {
			measure, ok := measures[binding.Measure.ID]
			if !ok || !leaves[measure.ID] {
				return invalid(CodeMissingReference, "kpis.periods.measure")
			}
			if _, duplicate := mapped[measure.ID]; duplicate {
				return invalid(CodeDuplicateID, "kpis.periods.measure")
			}
			dimension, ok := dimensions[binding.Dimension.ID]
			if !ok || dimension.Role != DimensionTemporal || dimension.Temporal == nil {
				return invalid(CodeInvalidReference, "kpis.periods.dimension")
			}
			left, right := sources[measure.Field.Dataset], sources[dimension.Field.Dataset]
			if left.Source != right.Source || left.Context != right.Context || left.SourceRevision != right.SourceRevision {
				return invalid(CodeEvidenceMismatch, "kpis.periods.dimension")
			}
			if prior, exists := byFact[measure.Field.Dataset]; exists && prior != binding.Dimension {
				return invalid(CodeAmbiguousTerm, "kpis.periods.fact")
			}
			if !periodDimensionReachable(pack, measure.Field.Dataset, dimension.Field.Dataset) {
				return invalid(CodeInvalidReference, "kpis.periods.relationship")
			}
			mapped[measure.ID] = binding.Dimension
			byFact[measure.Field.Dataset] = binding.Dimension
		}
		for _, child := range nested {
			if child.Periods != nil {
				for _, binding := range child.Periods.Bindings {
					if mapped[binding.Measure.ID] != binding.Dimension {
						return invalid(CodeEvidenceMismatch, "kpis.periods.nested")
					}
				}
			}
		}
	}
	return nil
}

// This is only a reviewed structural reach check. Runtime separately proves all
// physical uniqueness/FK properties and exact selected join placement.
func periodDimensionReachable(pack TopicPack, from, to string) bool {
	if from == to {
		return true
	}
	seen := map[string]bool{from: true}
	pending := []string{from}
	for depth := 0; depth < 3 && len(pending) > 0; depth++ {
		next := []string{}
		for _, current := range pending {
			for _, join := range pack.Joins {
				target := ""
				if join.Left.Dataset == current && (join.Cardinality == CardinalityManyToOne || join.Cardinality == CardinalityOneToOne) {
					target = join.Right.Dataset
				}
				if join.Type == JoinInner && join.Right.Dataset == current && join.Cardinality == CardinalityOneToOne {
					target = join.Left.Dataset
				}
				if target == to {
					return true
				}
				if target != "" && !seen[target] {
					seen[target] = true
					next = append(next, target)
				}
			}
		}
		pending = next
	}
	return false
}
