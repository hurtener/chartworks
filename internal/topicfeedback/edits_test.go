package topicfeedback

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/semantics"
	"reflect"
	"strings"
	"testing"
)

func proposalTestPack() semantics.TopicPack {
	return semantics.TopicPack{SchemaVersion: 1, Topic: "sales", Version: "v1", Name: "Sales", Description: "Synthetic definitions", Datasets: []semantics.Dataset{{ID: "orders", Name: "Orders", Source: semantics.SourceReference{Source: "warehouse", Context: "partition", Dataset: "orders", ProfileVersion: "profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "numeric", Category: "decimal"}}}}, Measures: []semantics.Measure{{ID: "revenue", Name: "Revenue", Description: "Total", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}, Aggregation: semantics.AggregationSum, Unit: "USD", Filters: []semantics.SemanticFilter{{ID: "known", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}, Operator: "not_null"}}}}}
}
func TestProposalEditsMeaningAndPreservesOrigins(t *testing.T) {
	p := proposalTestPack()
	agg := semantics.AggregationAverage
	out, err := applyEdits(p, "v2", []Edit{{Kind: semantics.KindMeasure, ID: "revenue", Aggregation: &agg}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Measures[0].Aggregation != agg || p.Measures[0].Aggregation != semantics.AggregationSum || !reflect.DeepEqual(out.Datasets, p.Datasets) || out.Version != "v2" {
		t.Fatal("semantic correction mutated origin or input")
	}
}
func TestProposalRejectsUnknownAndStructuralEdits(t *testing.T) {
	p := proposalTestPack()
	desc := "Corrected"
	ids := []string{"invented"}
	agg := semantics.AggregationAverage
	expr := "SELECT secret FROM hidden"
	for _, edits := range [][]Edit{nil, {{Kind: semantics.KindMeasure, ID: "other", Description: &desc}}, {{Kind: semantics.KindColumn, ID: "amount", Description: &desc}}, {{Kind: semantics.KindMeasure, ID: "revenue", Expression: &expr}}, {{Kind: semantics.KindMeasure, ID: "revenue", FilterIDs: &ids}}, {{Kind: semantics.KindMeasure, ID: "revenue", Aggregation: &agg}, {Kind: semantics.KindMeasure, ID: "revenue", Aggregation: &agg}}} {
		if _, err := applyEdits(p, "v2", edits); err == nil {
			t.Fatalf("accepted invalid edits %#v", edits)
		}
	}
}
func TestProposalSchemaRejectsSQLAndUnknownValues(t *testing.T) {
	s, err := proposalSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"edits":[{"kind":"measure","id":"revenue","sql":"select secret"}]}`, `{"edits":[{"kind":"measure","id":"revenue","filter_values":["private"]}]}`, `{"edits":[],"grant":"all"}`} {
		if err = s.Validate([]byte(raw), 65536); err == nil {
			t.Fatal("open proposal schema", raw)
		}
	}
	raw, _ := json.Marshal(map[string]any{"edits": []Edit{{Kind: semantics.KindMeasure, ID: "revenue"}}})
	if err = s.Validate(raw, 65536); err != nil {
		t.Fatal(err)
	}
}
func TestProposalKPIExpressionIsClosed(t *testing.T) {
	k := semantics.KPI{Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "gross"}, {Kind: semantics.KindMeasure, ID: "refund"}}}
	for _, s := range []string{"gross minus refund", "gross divided by refund"} {
		k.Expression = s
		if !closedExpression(k) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"SELECT gross FROM secret", "gross minus 12345", "gross + refund", "gross minus refund; DROP TABLE x"} {
		k.Expression = s
		if closedExpression(k) {
			t.Fatal(s)
		}
	}
}

func TestProposalSemanticEditKinds(t *testing.T) {
	p := proposalTestPack()
	p.Dimensions = []semantics.Dimension{{ID: "period", Name: "Period", Description: "Calendar grouping", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth}}}}
	second := p.Measures[0]
	second.ID = "refund"
	second.Name = "Refund"
	p.Measures = append(p.Measures, second)
	p.KPIs = []semantics.KPI{{ID: "net", Name: "Net", Description: "Net amount", Expression: "revenue minus refund", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindMeasure, ID: "refund"}}, Unit: "USD"}}
	description := "Corrected meaning"
	aliases := []string{"Reviewed label"}
	empty := []string{}
	expression := "revenue divided by refund"
	inputs := p.KPIs[0].Inputs
	edits := []Edit{{Kind: semantics.KindMeasure, ID: "revenue", Description: &description, Aliases: &aliases, FilterIDs: &empty}, {Kind: semantics.KindDimension, ID: "period", Description: &description, Aliases: &aliases, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainQuarter}}, FilterIDs: &empty}, {Kind: semantics.KindKPI, ID: "net", Description: &description, Aliases: &aliases, Expression: &expression, Inputs: &inputs, FilterIDs: &empty}}
	out, err := applyEdits(p, "v2", edits)
	if err != nil {
		t.Fatal(err)
	}
	var revenue semantics.Measure
	for _, measure := range out.Measures {
		if measure.ID == "revenue" {
			revenue = measure
		}
	}
	if len(revenue.Filters) != 0 || out.KPIs[0].Expression != expression || out.Dimensions[0].Temporal.Grains[0] != semantics.GrainQuarter {
		t.Fatal("semantic edits not applied")
	}
	if !existingMetric(p, inputs[0]) || !existingMetric(p, semantics.Reference{Kind: semantics.KindKPI, ID: "net"}) || existingMetric(p, semantics.Reference{Kind: semantics.KindMeasure, ID: "unknown"}) {
		t.Fatal("metric whitelist")
	}
	if _, err = selectFilters(p.Measures[0].Filters, []string{"known", "known"}); err == nil {
		t.Fatal("duplicate filter")
	}
	if f, err := selectFilters(p.Measures[0].Filters, []string{"known"}); err != nil || len(f) != 1 {
		t.Fatal(err)
	}
	bad := []string{""}
	edits[0].Aliases = &bad
	if _, err = applyEdits(p, "v2", edits); err == nil {
		t.Fatal("empty alias")
	}
}
