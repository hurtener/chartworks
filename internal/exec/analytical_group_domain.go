package exec

// AnalyticalGroupDomain is a hash-bound, reviewed v8 policy for group existence
// over one ordinary source or physically proved raw-join population.
type AnalyticalGroupDomain struct {
	Policy string `json:"policy"`
	Domain string `json:"domain"`
}

const AnalyticalGroupDomainPolicy = "metric-group-domain-v1"

func validateAnalyticalGroupDomain(c AnalyticalContract) error {
	if c.GroupDomain == nil {
		return nil
	}
	if c.Version != AnalyticalGroupedProgramsVersion || c.GroupedPopulations != nil || len(c.Populations) != 0 || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0 || c.GroupDomain.Policy != AnalyticalGroupDomainPolicy || c.GroupDomain.Domain != AnalyticalGroupDomainRaw && c.GroupDomain.Domain != AnalyticalGroupDomainQualifying {
		return ErrBinding
	}
	return nil
}

// checkOrdinaryGroupDomain proves group existence independently of each
// aggregate's population and NULL policy. Query-owned row predicates are built
// by the existing binder and conjoined with the reviewed metric group domain;
// they can never be supplied by a topic policy or replaced with model guesses.
func (a *analyticalChecker) checkOrdinaryGroupDomain(q map[string]any) error {
	if !a.ordinaryGroupProof || len(array(q["groupClause"])) == 0 {
		return nil
	}
	if len(a.leaves) == 0 || len(a.leaves) > 1024 {
		return ErrLimit
	}
	expected := groupedPopulationDNF(a.leaves)
	mode := ""
	if a.ordinaryGroupDomain != nil {
		mode = a.ordinaryGroupDomain.Domain
	}
	if mode == "" {
		if len(expected) != 1 || len(expected[0]) != 0 {
			return groupDomainReview()
		}
		mode = AnalyticalGroupDomainRaw
	}
	if mode == AnalyticalGroupDomainRaw {
		expected = [][]string{{}}
	} else if mode != AnalyticalGroupDomainQualifying {
		return ErrBinding
	}
	for i := range expected {
		for j := range expected[i] {
			expected[i][j] = "metric:" + expected[i][j]
		}
	}
	if a.queryPopulation != nil && len(a.queryPopulation.Constraints) > 0 {
		base, err := a.populationBaseSQL()
		if err != nil {
			return err
		}
		bound, err := BindBusinessConstraints(a.ctx, a.binding, base, nil, a.queryPopulation.Constraints)
		if err != nil {
			return err
		}
		if len(bound.SQL) > 32<<10 {
			return ErrLimit
		}
		tree, err := analyticalSQLTree(a.ctx, bound.SQL, a.binding)
		if err != nil {
			return err
		}
		owned := *a
		owned.parameters = bound.Parameters
		predicates, err := owned.groupDomainDNF(tree["whereClause"], 0, owned.groupDomainAtom)
		if err != nil {
			return err
		}
		expected, err = groupDomainProduct(expected, predicates)
		if err != nil {
			return err
		}
	}
	actual, err := a.groupDomainDNF(q["whereClause"], 0, a.groupDomainAtom)
	if err != nil {
		return err
	}
	proved := canonicalGroupDNF(actual)
	if Hash(proved) != Hash(canonicalGroupDNF(expected)) {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	// A distributed WHERE can imply the same common metric predicate as its
	// factored form. Promote only reviewed atoms shared by every metric and
	// present in every already-proved WHERE clause. Non-common populations stay
	// on their aggregates; unproved OR branches and owned predicates grant no
	// additional metric meaning. Retained policies never enter this v8 proof.
	for key := range a.common {
		present := len(proved) > 0
		for _, clause := range proved {
			found := false
			for _, atom := range clause {
				found = found || atom == "metric:"+key
			}
			present = present && found
		}
		if present {
			a.global[key] = true
		}
	}
	a.ordinaryDomainProved = true
	return nil
}

func (a *analyticalChecker) groupDomainAtom(node any) (string, error) {
	if filter, err := a.predicate(node); err == nil {
		return "metric:" + analyticalFilterKey(filter), nil
	}
	key, err := a.populationNodeKey(node, a.parameters)
	return "owned:" + key, err
}

func groupDomainProduct(left, right [][]string) ([][]string, error) {
	if len(left)*len(right) > 64 {
		return nil, ErrLimit
	}
	var product [][]string
	for _, a := range left {
		for _, b := range right {
			if len(a)+len(b) > 64 {
				return nil, ErrLimit
			}
			clause := append([]string(nil), a...)
			product = append(product, append(clause, b...))
		}
	}
	return canonicalGroupDNF(product), nil
}
