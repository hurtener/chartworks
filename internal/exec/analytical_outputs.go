package exec

import "sort"

const AnalyticalCompletenessPolicy = "reviewed-known-amount-outputs-v1"
const AnalyticalCompletenessScope = ";reviewed_known_amount_completeness"

// AnalyticalOutput binds a proved selected metric to the zero-based final
// target ordinal. Aliases and driver-selected column labels grant no meaning.
type AnalyticalOutput struct {
	Metric string `json:"metric"`
	Column int    `json:"column"`
}

// AnalyticalCompleteness retains the reviewed requirement to expose an exact
// unknown-count companion in the same ordinary query population and grain.
type AnalyticalCompleteness struct {
	Policy      string                             `json:"policy"`
	Obligations []AnalyticalCompletenessObligation `json:"obligations"`
}

type AnalyticalCompletenessObligation struct {
	Metric       string `json:"metric"`
	UnknownCount string `json:"unknown_count"`
}

func validateAnalyticalCapabilities(c AnalyticalContract, b Binding) error {
	if c.Version == AnalyticalGroupedFactsVersion {
		return validateGroupedFacts(c, b)
	}
	if c.GroupedPopulations != nil {
		for _, lane := range c.GroupedPopulations.Lanes {
			if lane.FactPopulation != nil {
				return ErrBinding
			}
		}
	}
	if c.Version == AnalyticalGroupedSelectionVersion {
		return validateGroupedSelection(c, b)
	}
	if c.GroupSelection != nil {
		return ErrBinding
	}
	if c.Version == AnalyticalGroupedOwnedPopulationsVersion {
		if b.Dialect != "postgres" || c.GroupedPopulations == nil || c.ScalarPopulations != nil || c.Completeness != nil {
			return ErrBinding
		}
		return nil
	}
	if c.GroupedPopulations != nil {
		for _, lane := range c.GroupedPopulations.Lanes {
			if lane.QueryPopulation != nil {
				return ErrBinding
			}
		}
	}
	if c.Version != AnalyticalScopedPopulationsVersion {
		if c.ScalarPopulations != nil || c.Completeness != nil {
			return ErrBinding
		}
		return nil
	}
	if (c.ScalarPopulations == nil) == (c.Completeness == nil) {
		return ErrBinding
	}
	if c.Completeness == nil {
		return nil
	}
	if b.Dialect != "postgres" && b.Dialect != "mysql" {
		return analyticalFailure("analytical_dialect_unsupported", true)
	}
	p := c.Completeness
	if p.Policy != AnalyticalCompletenessPolicy || len(p.Obligations) < 1 || len(p.Obligations) > 32 || c.GroupedPopulations != nil || len(c.Populations) != 0 {
		return ErrBinding
	}
	// A base-table key can become nullable in an admitted LEFT-join population.
	// Completeness has no reviewed missing-row policy, so use the exact effective
	// join namespace (and its physical cardinality proof), not raw DDL alone.
	var base Relation
	matches := 0
	for _, relation := range b.Relations {
		if relation.ID == c.Dataset {
			base = relation
			matches++
		}
	}
	if matches != 1 {
		return ErrBinding
	}
	effective, err := analyticalJoinRelation(analyticalStandardPolicy(c), b, base)
	if err != nil {
		return err
	}
	metrics := map[string]AnalyticalExpression{}
	for _, m := range c.Metrics {
		metrics[m.ID] = m.Expression
	}
	previous := ""
	for _, obligation := range p.Obligations {
		key := obligation.Metric + "\x00" + obligation.UnknownCount
		if obligation.Metric == "" || obligation.UnknownCount == "" || obligation.Metric == obligation.UnknownCount || previous >= key {
			return ErrBinding
		}
		previous = key
		amount, ok := metrics[obligation.Metric]
		unknown, exists := metrics[obligation.UnknownCount]
		if !ok || !exists || amount.Op != "sum" || amount.Column == "" || len(amount.Args) != 0 || unknown.Op != "-" || len(unknown.Args) != 2 {
			return ErrBinding
		}
		rows, known := unknown.Args[0], unknown.Args[1]
		if rows.Op != "count" || rows.Column == "" || known.Op != "count" || known.Column != amount.Column || len(rows.Args)+len(known.Args) != 0 || !analyticalFiltersEqual(rows.Filters, amount.Filters) || !analyticalFiltersEqual(known.Filters, amount.Filters) {
			return ErrBinding
		}
		fact := analyticalColumnDataset(c.Dataset, amount.Column)
		if analyticalColumnDataset(c.Dataset, rows.Column) != fact {
			return ErrBinding
		}
		found := false
		for _, column := range effective.Columns {
			if column.Name == rows.Column && column.Safe && !column.Nullable {
				found = true
			}
		}
		if !found {
			return ErrBinding
		}
	}
	return nil
}

func analyticalFiltersEqual(a, b []AnalyticalFilter) bool {
	keys := func(filters []AnalyticalFilter) []string {
		out := make([]string, len(filters))
		for i, f := range filters {
			out[i] = analyticalFilterKey(f)
		}
		sort.Strings(out)
		return out
	}
	return Hash(keys(a)) == Hash(keys(b))
}

func (a *analyticalChecker) provedOutputs(ids []string) ([]AnalyticalOutput, error) {
	out := make([]AnalyticalOutput, 0, len(ids))
	for _, id := range ids {
		found := -1
		for i, term := range a.finalTerms {
			if term.aggregate && !term.guarded && term.key == a.intentMetricKeys[id] {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, ErrBinding
		}
		out = append(out, AnalyticalOutput{Metric: id, Column: found})
	}
	return out, nil
}

// AnalyticalOutputsValid checks structural coverage only. Consumers still must
// compare the receipt to a fresh native+analytical proof before interpreting rows.
func AnalyticalOutputsValid(r *AnalyticalReceipt) bool {
	if r == nil {
		return false
	}
	if r.Version != AnalyticalScopedPopulationsVersion && r.Version != AnalyticalGroupedOwnedPopulationsVersion && r.Version != AnalyticalGroupedSelectionVersion && r.Version != AnalyticalGroupedFactsVersion {
		return len(r.Outputs) == 0 && r.Completeness == nil
	}
	if len(r.Outputs) != len(r.Metrics) || len(r.Outputs) == 0 {
		return false
	}
	for i, output := range r.Outputs {
		if output.Metric != r.Metrics[i] || output.Column < 0 || output.Column > 255 {
			return false
		}
	}
	if (r.Version == AnalyticalGroupedOwnedPopulationsVersion || r.Version == AnalyticalGroupedSelectionVersion || r.Version == AnalyticalGroupedFactsVersion) && r.Completeness != nil {
		return false
	}
	if r.Completeness != nil {
		p := r.Completeness
		if p.Policy != AnalyticalCompletenessPolicy || len(p.Obligations) < 1 || len(p.Obligations) > 32 {
			return false
		}
		metrics := map[string]bool{}
		for _, id := range r.Metrics {
			metrics[id] = true
		}
		previous := ""
		for _, o := range p.Obligations {
			key := o.Metric + "\x00" + o.UnknownCount
			if !metrics[o.Metric] || !metrics[o.UnknownCount] || o.Metric == o.UnknownCount || previous >= key {
				return false
			}
			previous = key
		}
	}
	return true
}

func CloneAnalyticalCompleteness(p *AnalyticalCompleteness) *AnalyticalCompleteness {
	if p == nil {
		return nil
	}
	out := *p
	out.Obligations = append([]AnalyticalCompletenessObligation(nil), p.Obligations...)
	return &out
}

// ValidateAnalyticalCompleteness verifies the compiled ordinary capability,
// including current physical count-identity nullability, before inference.
func ValidateAnalyticalCompleteness(c AnalyticalContract, b Binding) error {
	if c.Completeness == nil {
		return ErrBinding
	}
	return validateAnalyticalCapabilities(c, b)
}
