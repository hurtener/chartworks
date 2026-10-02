package semantics

// PopulationRelationship is the closed structural admission for a direct
// relationship-bound measure filter. It is not physical uniqueness proof.
func PopulationRelationship(pack TopicPack, fact, target, id string) (Join, bool) {
	if fact == target || id == "" {
		return Join{}, false
	}
	for _, join := range pack.Joins {
		if join.ID == id && validRelationshipEvidence(join.Evidence, false) && join.Type == JoinInner && join.Left.Dataset == fact && join.Right.Dataset == target && (join.Cardinality == CardinalityManyToOne || join.Cardinality == CardinalityOneToOne) {
			return join, true
		}
	}
	return Join{}, false
}
func validateRelationshipFilters(pack TopicPack) error {
	for _, measure := range pack.Measures {
		for _, filter := range measure.Filters {
			if filter.Relationship != "" {
				if _, ok := PopulationRelationship(pack, measure.Field.Dataset, filter.Field.Dataset, filter.Relationship); !ok {
					return invalid(CodeInvalidReference, "measures.filters.relationship")
				}
			}
		}
	}
	// The initial consumer binds each predicate to exactly one measure fact.
	// A KPI-wide or dimension-wide relationship predicate has no such placement.
	for _, kpi := range pack.KPIs {
		for _, filter := range kpi.Filters {
			if filter.Relationship != "" {
				return invalid(CodeInvalidReference, "kpis.filters.relationship")
			}
		}
	}
	for _, dimension := range pack.Dimensions {
		for _, filter := range dimension.Filters {
			if filter.Relationship != "" {
				return invalid(CodeInvalidReference, "dimensions.filters.relationship")
			}
		}
	}
	return nil
}
