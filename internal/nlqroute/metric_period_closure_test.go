package nlqroute

import (
	"context"
	"github.com/hurtener/chartworks/internal/semantics"
	"reflect"
	"testing"
)

func TestMetricPeriodAndBoundFilterClosureRemainDependencies(t *testing.T) {
	pub := recoveryPublication()
	def := pub.Definition
	def.Datasets[0].Columns = append(def.Datasets[0].Columns, semantics.Column{ID: "at", SourceName: "at", Name: "Event time", NativeType: "timestamptz", Category: "timestamp"})
	def.Dimensions = append(def.Dimensions, semantics.Dimension{ID: "time", Name: "Event time", Field: recoveryColumn("dataset", "at"), Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainYear}}})
	def.KPIs[1].Periods = &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "time"}}, {Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: "costs"}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "time"}}}}
	def.Measures[0].Filters = []semantics.SemanticFilter{{ID: "parent", Relationship: "sales_products", Field: recoveryColumn("products", "family"), Operator: "eq", Values: []string{"synthetic"}}}
	roots := []semantics.Reference{{Kind: semantics.KindKPI, ID: "margin_pct"}}
	before := append([]semantics.Reference(nil), roots...)
	deps, err := semanticClosure(context.Background(), def, roots)
	if err != nil {
		t.Fatal(err)
	}
	foundPeriod, foundJoin := false, false
	for _, dep := range deps {
		foundPeriod = foundPeriod || dep.Kind == "dimension" && dep.ID == "time"
		foundJoin = foundJoin || dep.Kind == "join" && dep.ID == "sales_products"
	}
	if !foundPeriod || !foundJoin || !reflect.DeepEqual(roots, before) {
		t.Fatal("period/join dependency missing or promoted to root")
	}
	def.Measures[0].Filters[0].Relationship = "foreign"
	if _, err := semanticClosure(context.Background(), def, roots); err == nil {
		t.Fatal("unbound relationship entered closure")
	}
}
