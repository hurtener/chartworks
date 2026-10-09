package reporting

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func preparationCompileFixture() (AuthoringDatasetIntent, topics.Published, topics.Dataset, exec.Binding) {
	dim := semantics.Dimension{ID: "region", Name: "Region", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Role: semantics.DimensionCategorical}
	measure := semantics.Measure{ID: "revenue", Name: "Revenue", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "amount"}, Aggregation: semantics.AggregationSum, Unit: "USD"}
	dataset := topics.Dataset{ID: "sales", Name: "Sales", Source: topics.Binding{Source: "warehouse", Context: "readonly", Dataset: "sales", SourceRevision: 1}, Columns: []semantics.Column{{ID: "region", SourceName: "region", Name: "Region", NativeType: "text", Category: "text"}, {ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "int4", Category: "number"}}}
	p := topics.Published{State: topics.State{Topic: "sales", Version: "v1", Active: true}, Digest: strings.Repeat("a", 64), Definition: topics.Definition{Topic: "sales", Version: "v1", Datasets: []topics.Dataset{dataset}, Dimensions: []semantics.Dimension{dim}, Measures: []semantics.Measure{measure}}}
	b := exec.Binding{Tenant: "tenant", Source: "warehouse", Context: "readonly", Revision: 1, Dialect: "postgres", Contract: "postgres-read-v1", Fingerprint: strings.Repeat("f", 64), Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "region", NativeType: "text", Category: "text", Safe: true}, {Name: "amount", NativeType: "int4", Category: "number", Safe: true}}}}}
	in := AuthoringDatasetIntent{Topic: TopicPin{Topic: "sales", Version: "v1", Digest: p.Digest}, Dataset: "sales", Dimensions: []string{"region"}, Measure: "revenue", Mapping: AuthoringChartMapping{Kind: charts.Bar, Bindings: charts.Bindings{Category: semanticBinding("d", "region"), Value: semanticBinding("m", "revenue")}, Order: []charts.Order{}, Options: charts.DefaultOptions()}}
	return in, p, dataset, b
}
func TestAuthoringCompilerExactReviewedAggregateAndGrouping(t *testing.T) {
	in, p, d, b := preparationCompileFixture()
	out, err := compileAuthoringDataset(in, p, d, b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.SQL, `sum("amount")`) || !strings.Contains(out.SQL, `GROUP BY "region" ORDER BY "region"`) || strings.Contains(out.SQL, "LIMIT") || len(out.Scope) != 1 || len(out.Scope[0].Columns) != 2 || len(out.Columns) != 2 || out.Columns[1].Type != "" {
		t.Fatal("invented types or changed meaning", out)
	}
	for _, a := range []semantics.Aggregation{semantics.AggregationSum, semantics.AggregationAverage, semantics.AggregationMinimum, semantics.AggregationMaximum, semantics.AggregationCount, semantics.AggregationDistinctCount} {
		p.Definition.Measures[0].Aggregation = a
		compiled, err := compileAuthoringDataset(in, p, d, b)
		if err != nil || compiled.Columns[1].Aggregation != string(a) {
			t.Fatal(a, err)
		}
	}
}
func TestAuthoringCompilerRejectsUnsupportedReviewedMeaning(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AuthoringDatasetIntent, *topics.Published, *topics.Dataset, *exec.Binding)
	}{
		{"group domain", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: "qualifying_population"}
		}},
		{"measure filter", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "required"}}
		}},
		{"dimension filter", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Dimensions[0].Filters = []semantics.SemanticFilter{{ID: "required"}}
		}},
		{"completeness", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Measures[0].Completeness = &semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy}
		}},
		{"calendar", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Dimensions[0].Temporal = &semantics.TemporalPolicy{Calendar: "gregorian"}
		}},
		{"other fact", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Measures[0].Field.Dataset = "other"
		}},
		{"other dialect", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) {
			b.Dialect = "mysql"
		}},
		{"unknown dimension", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Dimensions = []string{"missing"}
		}},
		{"source drift", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) { b.Revision++ }},
		{"unsafe field", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) {
			b.Relations[0].Columns[1].Safe = false
		}},
		{"raw expression", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Measure = "sum(amount)"
		}},
		{"unsupported chart", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Mapping.Kind = "combo"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, p, d, b := preparationCompileFixture()
			tc.change(&in, &p, &d, &b)
			if _, err := compileAuthoringDataset(in, p, d, b); err == nil {
				t.Fatal("unsupported semantics accepted")
			}
		})
	}
}
