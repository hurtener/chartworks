package exec

import "sort"

const AnalyticalGroupDomainRaw = "raw_source_groups"
const AnalyticalGroupDomainQualifying = "qualifying_population"
const AnalyticalGroupDomainReviewCode = "analytical_group_domain_review_required"

func groupDomainReview() error { return analyticalFailure(AnalyticalGroupDomainReviewCode, true) }

// Group existence is separate from aggregate NULL treatment. Qualifying rows
// satisfy at least one complete reviewed metric-filter population; COUNT(column)
// still includes a group whose qualifying inputs happen to be all NULL.
func groupedPopulationDNF(leaves []AnalyticalExpression) [][]string {
	out := make([][]string, 0, len(leaves))
	for _, leaf := range leaves {
		var atoms []string
		for _, f := range leaf.Filters {
			atoms = append(atoms, analyticalFilterKey(f))
		}
		out = append(out, atoms)
	}
	return canonicalGroupDNF(out)
}
func canonicalGroupDNF(in [][]string) [][]string {
	var out [][]string
	for _, clause := range in {
		clause = append([]string(nil), clause...)
		sort.Strings(clause)
		atoms := clause[:0]
		for _, atom := range clause {
			if len(atoms) == 0 || atoms[len(atoms)-1] != atom {
				atoms = append(atoms, atom)
			}
		}
		if len(atoms) == 0 {
			return [][]string{{}}
		}
		out = append(out, atoms)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return Hash(out[i]) < Hash(out[j])
	})
	var minimal [][]string
	for _, clause := range out {
		redundant := false
		for _, old := range minimal {
			n := 0
			for _, atom := range clause {
				if n < len(old) && atom == old[n] {
					n++
				}
			}
			if n == len(old) {
				redundant = true
				break
			}
		}
		if !redundant {
			minimal = append(minimal, clause)
		}
	}
	return minimal
}
func (a *analyticalChecker) groupedWhereDNF(node any, depth int) ([][]string, error) {
	return a.groupDomainDNF(node, depth, func(node any) (string, error) {
		filter, err := a.predicate(node)
		return analyticalFilterKey(filter), err
	})
}
func (a *analyticalChecker) groupDomainDNF(node any, depth int, atom func(any) (string, error)) ([][]string, error) {
	if depth > 32 {
		return nil, ErrLimit
	}
	if node == nil {
		return [][]string{{}}, nil
	}
	if b := object(object(node)["BoolExpr"]); b != nil {
		args := array(b["args"])
		op := text(b["boolop"])
		if !only(b, "boolop", "args", "location") || len(args) < 2 || len(args) > 64 || (op != "AND_EXPR" && op != "OR_EXPR") {
			return nil, analyticalFailure("analytical_population_mismatch", false)
		}
		out := [][]string{}
		if op == "AND_EXPR" {
			out = [][]string{{}}
		}
		for _, arg := range args {
			clauses, err := a.groupDomainDNF(arg, depth+1, atom)
			if err != nil {
				return nil, err
			}
			if op == "OR_EXPR" {
				out = append(out, clauses...)
			} else {
				if len(out)*len(clauses) > 64 {
					return nil, ErrLimit
				}
				var product [][]string
				for _, left := range out {
					for _, right := range clauses {
						if len(left)+len(right) > 64 {
							return nil, ErrLimit
						}
						clause := append([]string(nil), left...)
						clause = append(clause, right...)
						product = append(product, clause)
					}
				}
				out = product
			}
			if len(out) > 64 {
				return nil, ErrLimit
			}
			out = canonicalGroupDNF(out)
		}
		return out, nil
	}
	key, err := atom(node)
	if err != nil {
		return nil, err
	}
	return [][]string{{key}}, nil
}
func (a *analyticalChecker) checkGroupedDomain(q map[string]any) error {
	if !a.groupedLaneProof {
		return nil
	}
	if a.queryPopulation != nil && len(a.queryPopulation.Constraints) > 0 {
		// The owned grouped policy composes the same exact positive domain proof
		// as ordinary grouped inputs, inside this independently proved lane.
		a.ordinaryGroupProof = true
		a.ordinaryGroupDomain = &AnalyticalGroupDomain{Policy: AnalyticalGroupDomainPolicy, Domain: a.groupedDomain}
		if err := a.checkOrdinaryGroupDomain(q); err != nil {
			return err
		}
		a.groupedDomainProved = a.ordinaryDomainProved
		return nil
	}
	if len(a.leaves) == 0 || len(a.leaves) > 1024 {
		return ErrLimit
	}
	expected := groupedPopulationDNF(a.leaves)
	actual, err := a.groupedWhereDNF(q["whereClause"], 0)
	if err != nil {
		return err
	}
	mode := a.groupedDomain
	if mode == "" {
		// V8 requires an author-reviewed domain for filtered lanes. With an
		// unfiltered metric the union is TRUE and the two domains are identical.
		if len(expected) == 1 && len(expected[0]) == 0 {
			mode = AnalyticalGroupDomainRaw
		} else {
			if a.groupedExtensions {
				return groupDomainReview()
			}
			// Retained v7 has no domain marker. Only a full common population WHERE
			// is an unambiguous filtered form under that intended metric contract.
			first := canonicalGroupDNF([][]string{filterKeys(a.leaves[0].Filters)})
			for _, leaf := range a.leaves {
				if Hash(canonicalGroupDNF([][]string{filterKeys(leaf.Filters)})) != Hash(first) {
					return groupDomainReview()
				}
			}
			if Hash(actual) != Hash(first) {
				return groupDomainReview()
			}
			mode = AnalyticalGroupDomainQualifying
		}
	}
	switch mode {
	case AnalyticalGroupDomainRaw:
		expected = [][]string{{}}
	case AnalyticalGroupDomainQualifying:
	default:
		return ErrBinding
	}
	if Hash(actual) != Hash(expected) {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	a.groupedDomainProved = true
	return nil
}
func filterKeys(filters []AnalyticalFilter) []string {
	out := make([]string, 0, len(filters))
	for _, f := range filters {
		out = append(out, analyticalFilterKey(f))
	}
	return out
}
