package exec

// AnalyticalGrain is a closed grouping intent over the contract's base relation.
// Nil means unmeasured grain, not an instruction to return a total. The compiler
// supplies sorted unique physical columns and reviewed dimension identities.
// Values, SQL, user prose and generated aliases do not belong in this structure.
type AnalyticalGrain struct {
	Policy     string   `json:"policy"`
	Columns    []string `json:"columns"`
	Dimensions []string `json:"dimensions"`
}

// AnalyticalGrainPolicy identifies the exact reviewed-dimension suffix grammar.
// Recognition is deliberately separate from native SQL conformance.
const AnalyticalGrainPolicy = "reviewed-dimension-suffix-v1"

func validateAnalyticalGrain(c AnalyticalContract, relation Relation) error {
	if c.Grain == nil {
		return nil
	}
	g := c.Grain
	if c.Version != AnalyticalGrainVersion || g.Policy != AnalyticalGrainPolicy || len(g.Columns) < 1 || len(g.Columns) > 16 || len(g.Dimensions) < 1 || len(g.Dimensions) > 16 {
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
	} // Retained/unspecified grain remains unmeasured.
	mismatch := func() error { return analyticalFailure("analytical_grain_mismatch", false) }
	if len(groups) != len(a.grain.Columns) {
		return mismatch()
	}
	projected := map[string]bool{}
	for _, term := range terms {
		if term.column != "" {
			projected[term.column] = true
		}
	}
	if len(projected) != len(a.grain.Columns) {
		return mismatch()
	}
	for _, column := range a.grain.Columns {
		// Missing projected keys make distinct groups indistinguishable even when
		// SQL's grouping set itself is correct. Extra invisible keys change grain.
		if !groups[column] || !projected[column] {
			return mismatch()
		}
	}
	return nil
}
