package exec

// AnalyticalGrain is a closed grouping intent over the contract's base relation.
// Nil means unmeasured grain, not an instruction to return a total. The compiler
// supplies sorted unique physical columns and reviewed dimension identities.
// Values, SQL, user prose and generated aliases do not belong in this structure.
type AnalyticalGrain struct {
	Policy     string             `json:"policy"`
	Columns    []string           `json:"columns"`
	Dimensions []string           `json:"dimensions"`
	Buckets    []AnalyticalBucket `json:"buckets,omitempty"`
}

// AnalyticalGrainPolicy identifies the exact reviewed-dimension suffix grammar.
// Recognition is deliberately separate from native SQL conformance.
const AnalyticalGrainPolicy = "reviewed-dimension-suffix-v1"

func validateAnalyticalGrain(c AnalyticalContract, relation Relation) error {
	if c.Grain == nil {
		return nil
	}
	g := c.Grain
	validPolicy := c.Version == AnalyticalGrainVersion && g.Policy == AnalyticalGrainPolicy && len(g.Buckets) == 0 || (c.Version == AnalyticalCalendarVersion || c.Version == AnalyticalQueryPopulationVersion || c.Version == AnalyticalGroupingVersion) && g.Policy == AnalyticalCalendarPolicy || c.Version == AnalyticalGroupingVersion && g.Policy == AnalyticalGroupingPolicy
	empty := g.Policy == AnalyticalGroupingPolicy && len(g.Columns)+len(g.Buckets) == 0 && len(g.Dimensions) == 0
	if validPolicy && empty {
		return nil
	}
	if !validPolicy || len(g.Columns)+len(g.Buckets) < 1 || len(g.Columns)+len(g.Buckets) > 16 || len(g.Dimensions) < 1 || len(g.Dimensions) > 16 {
		return ErrBinding
	}
	for i, name := range g.Columns {
		if name == "" || i > 0 && g.Columns[i-1] >= name {
			return ErrBinding
		}
		matches := 0
		for _, col := range relation.Columns {
			if col.Name == name && col.Safe {
				matches++
			}
		}
		if matches != 1 {
			return ErrBinding
		}
	}
	for i, bucket := range g.Buckets {
		if i > 0 && analyticalBucketKey(g.Buckets[i-1]) >= analyticalBucketKey(bucket) {
			return ErrBinding
		}
		matches := 0
		for _, col := range relation.Columns {
			if validAnalyticalBucket(bucket, col) {
				matches++
			}
		}
		if matches != 1 {
			return ErrBinding
		}
	}
	for i, id := range g.Dimensions {
		if len(id) == 0 || len(id) > 256 || i > 0 && g.Dimensions[i-1] >= id {
			return ErrBinding
		}
	}
	return nil
}

func (a *analyticalChecker) checkGrain(groups map[string]bool, terms []analyticalTerm) error {
	if a.grain == nil {
		return nil
	}
	expected := map[string]bool{}
	for _, column := range a.grain.Columns {
		expected["column:"+column] = true
	}
	for _, bucket := range a.grain.Buckets {
		expected[analyticalBucketKey(bucket)] = true
	}
	mismatch := func() error { return analyticalFailure("analytical_grain_mismatch", false) }
	if len(groups) != len(expected) {
		return mismatch()
	}
	projected := map[string]bool{}
	for _, term := range terms {
		if key := term.groupKey(); key != "" {
			projected[key] = true
		}
	}
	if len(projected) != len(expected) {
		return mismatch()
	}
	for key := range expected {
		if !groups[key] || !projected[key] {
			return mismatch()
		}
	}
	return nil
}
