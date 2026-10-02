package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

const KnownAmountCompletenessPolicy = "known-amount-with-unknown-count-v1"

// KnownAmountCompleteness declares an exact scope-inheriting companion. It is
// semantic metadata, never source coverage, result-truncation or query authority.
type KnownAmountCompleteness struct {
	Policy       string    `json:"policy"`
	UnknownCount Reference `json:"unknown_count"`
}

// CompletenessCatalog can be built from an authorized private pack or public
// definition; validation here neither admits sources nor validates SQL.
type CompletenessCatalog struct {
	Measures []Measure
	KPIs     []KPI
	Columns  map[Reference]Column
}
type KnownAmountBinding struct {
	Measure          Reference
	UnknownCount     Reference
	RowCount         Reference
	KnownCount       Reference
	AmountField      Reference
	RowField         Reference
	PopulationDigest string
}

func cloneCompleteness(c *KnownAmountCompleteness) *KnownAmountCompleteness {
	if c == nil {
		return nil
	}
	out := *c
	return &out
}
func validateCompletenessShape(c *KnownAmountCompleteness) error {
	if c == nil {
		return nil
	}
	if c.Policy != KnownAmountCompletenessPolicy || !c.UnknownCount.Valid() || c.UnknownCount.Kind != KindKPI {
		return invalid(CodeInvalidValue, "measures.completeness")
	}
	return nil
}

// ResolveKnownAmountCompleteness checks one exact acyclic SUM→KPI→COUNT graph.
// The companion has no independent period: its output must be proved under the
// owning measure's exact query population and grouping by the execution consumer.
func ResolveKnownAmountCompleteness(c CompletenessCatalog, measureID string) (KnownAmountBinding, error) {
	bad := func() (KnownAmountBinding, error) {
		return KnownAmountBinding{}, invalid(CodeInvalidReference, "measures.completeness")
	}
	if len(c.Measures) > 1024 || len(c.KPIs) > 512 || len(c.Columns) > 8192 {
		return bad()
	}
	measures := map[string]Measure{}
	for _, m := range c.Measures {
		if _, ok := measures[m.ID]; ok {
			return bad()
		}
		measures[m.ID] = m
	}
	owner, ok := measures[measureID]
	if !ok || owner.Completeness == nil || validateCompletenessShape(owner.Completeness) != nil || owner.Aggregation != AggregationSum {
		return bad()
	}
	amount, ok := c.Columns[owner.Field]
	if !ok || amount.ID != owner.Field.ID || !numericCompletenessCategory(amount.Category) {
		return bad()
	}
	var companion KPI
	matches := 0
	for _, k := range c.KPIs {
		if k.ID == owner.Completeness.UnknownCount.ID {
			companion = k
			matches++
		}
	}
	if matches != 1 || len(companion.Expression) > 4096 || len(companion.Inputs) != 2 || companion.Periods != nil || len(companion.Filters) != 0 {
		return bad()
	}
	expression, err := parser.ParseExpr(companion.Expression)
	if err != nil {
		return bad()
	}
	unwrap := func(node ast.Expr) ast.Expr {
		for i := 0; i < 16; i++ {
			p, ok := node.(*ast.ParenExpr)
			if !ok {
				return node
			}
			node = p.X
		}
		return node
	}
	subtraction, ok := unwrap(expression).(*ast.BinaryExpr)
	if !ok || subtraction.Op != token.SUB {
		return bad()
	}
	left, ok := unwrap(subtraction.X).(*ast.Ident)
	if !ok {
		return bad()
	}
	right, ok := unwrap(subtraction.Y).(*ast.Ident)
	if !ok || left.Name == right.Name {
		return bad()
	}
	rowCount, rowOK := measures[left.Name]
	knownCount, knownOK := measures[right.Name]
	if !rowOK || !knownOK || rowCount.Aggregation != AggregationCount || knownCount.Aggregation != AggregationCount || rowCount.Completeness != nil || knownCount.Completeness != nil || knownCount.Field != owner.Field || rowCount.Field.Dataset != owner.Field.Dataset {
		return bad()
	}
	inputs := map[string]bool{}
	for _, input := range companion.Inputs {
		if !input.Valid() || input.Kind != KindMeasure || inputs[input.ID] {
			return bad()
		}
		inputs[input.ID] = true
	}
	if !inputs[rowCount.ID] || !inputs[knownCount.ID] {
		return bad()
	}
	identity, ok := c.Columns[rowCount.Field]
	if !ok || identity.ID != rowCount.Field.ID || identity.Nullable || identity.SemanticRole != SemanticRoleFactKey {
		return bad()
	}
	population := completenessPopulation(owner.Filters)
	if population == "" || completenessPopulation(rowCount.Filters) != population || completenessPopulation(knownCount.Filters) != population {
		return bad()
	}
	return KnownAmountBinding{Measure: Reference{Kind: KindMeasure, ID: owner.ID}, UnknownCount: owner.Completeness.UnknownCount, RowCount: Reference{Kind: KindMeasure, ID: rowCount.ID}, KnownCount: Reference{Kind: KindMeasure, ID: knownCount.ID}, AmountField: owner.Field, RowField: rowCount.Field, PopulationDigest: population}, nil
}
func numericCompletenessCategory(category string) bool {
	switch strings.ToLower(category) {
	case "numeric", "integer", "decimal", "number", "float":
		return true
	}
	return false
}
func completenessPopulation(filters []SemanticFilter) string {
	if !validFilters(filters) {
		return ""
	}
	type predicate struct {
		Field        Reference `json:"field"`
		Relationship string    `json:"relationship,omitempty"`
		Operator     string    `json:"operator"`
		Values       []string  `json:"values,omitempty"`
	}
	values := []string{}
	for _, f := range filters {
		v := append([]string(nil), f.Values...)
		sort.Strings(v)
		raw, _ := json.Marshal(predicate{f.Field, f.Relationship, f.Operator, v})
		values = append(values, string(raw))
	}
	sort.Strings(values)
	unique := values[:0]
	for _, value := range values {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	raw, _ := json.Marshal(unique)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
func validateCompletenessReferences(pack TopicPack) error {
	catalog := CompletenessCatalog{Measures: pack.Measures, KPIs: pack.KPIs, Columns: map[Reference]Column{}}
	for _, dataset := range pack.Datasets {
		for _, column := range dataset.Columns {
			catalog.Columns[Reference{Kind: KindColumn, Dataset: dataset.ID, ID: column.ID}] = column
		}
	}
	for _, measure := range pack.Measures {
		if measure.Completeness != nil {
			if _, err := ResolveKnownAmountCompleteness(catalog, measure.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
