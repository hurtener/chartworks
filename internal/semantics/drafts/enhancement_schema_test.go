package drafts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestRichEnhancementSchemaIsClosedAndAccepted(t *testing.T) {
	if _, err := gateway.NewSchema("rich_topic_enhancement_test", enhancementSchema); err != nil {
		t.Fatalf("rich enhancement schema rejected: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(enhancementSchema, &document); err != nil {
		t.Fatal(err)
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok || properties["results"] == nil || properties["kpis"] == nil || properties["relationships"] == nil {
		t.Fatalf("rich authoring branches absent: %#v", properties)
	}
}

func TestEnhancementOutputUsesSealedSameStepMetricCatalog(t *testing.T) {
	first := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}
	second := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "tax"}
	generated := semantics.GeneratedEntityID(semantics.EnhancementMeasure, first.Dataset, first.ID)
	catalog := []enhancementMetric{{Kind: semantics.KindMeasure, ID: "prior_measure", Availability: "existing"}, {Kind: semantics.KindMeasure, ID: generated, Availability: "current_step"}}
	wire := enhancementWire{
		Results: []semantics.Enhancement{{Dataset: first.Dataset, Column: first.ID, Kind: semantics.EnhancementMeasure, Name: "Amount", Aggregation: semantics.AggregationSum}},
		KPIs:    []semantics.KPI{{ID: "total", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: generated}}}},
	}
	if err := validateEnhancementOutput([]semantics.Reference{first}, catalog, wire); err != nil {
		t.Fatalf("same-step generated measure was not usable by KPI: %v", err)
	}
	wire.Results[0].Kind = semantics.EnhancementDimension
	if err := validateEnhancementOutput([]semantics.Reference{first}, catalog, wire); !errors.Is(err, gateway.ErrOutput) {
		t.Fatalf("non-measure same-step ID accepted: %v", err)
	}
	wire.Results[0].Kind = semantics.EnhancementMeasure
	wire.KPIs[0].Inputs[0].ID = "future_measure"
	if err := validateEnhancementOutput([]semantics.Reference{first}, catalog, wire); !errors.Is(err, gateway.ErrOutput) {
		t.Fatalf("unknown/future metric accepted: %v", err)
	}
	wire.KPIs[0].Inputs[0].ID = "prior_measure"
	if err := validateEnhancementOutput([]semantics.Reference{first}, catalog, wire); err != nil {
		t.Fatalf("prior paginated metric was not accepted as existing: %v", err)
	}
	wire.Relationships = []semantics.RelationshipDecision{{Left: first, Right: second}}
	if err := validateEnhancementOutput([]semantics.Reference{first}, catalog, wire); !errors.Is(err, gateway.ErrOutput) {
		t.Fatalf("relationship escaped supplied page: %v", err)
	}
}

func TestEnhancementMetricCatalogHasDeterministicBudget(t *testing.T) {
	pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "topic", Version: "v1", Name: "Topic", Description: "Synthetic", Datasets: []semantics.Dataset{{ID: "orders", Name: "Orders", Source: semantics.SourceReference{Source: "warehouse", Context: "context", Dataset: "orders", ProfileVersion: "profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "numeric", Category: "decimal"}}}}}
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}
	for i := 0; i < 1024; i++ {
		pack.Measures = append(pack.Measures, semantics.Measure{ID: fmt.Sprintf("measure_%04d", i), Name: "Measure", Description: "Synthetic", Field: field, Aggregation: semantics.AggregationSum})
	}
	for i := 0; i < 512; i++ {
		pack.KPIs = append(pack.KPIs, semantics.KPI{ID: fmt.Sprintf("kpi_%04d", i), Name: "KPI", Description: "Synthetic", Expression: "measure", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "measure_0000"}}})
	}
	model, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = enhancementMetricCatalog(model, []semantics.Reference{field}); !errors.Is(err, gateway.ErrBudget) {
		t.Fatalf("oversized metric catalog was not rejected before provider work: %v", err)
	}
}

func TestEnhancementMetricCatalogIsSelectedDatasetScoped(t *testing.T) {
	pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "topic", Version: "v1", Name: "Topic", Description: "Synthetic", Datasets: []semantics.Dataset{
		{ID: "orders", Name: "Orders", Source: semantics.SourceReference{Source: "warehouse", Context: "orders_context", Dataset: "orders", ProfileVersion: "orders_profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "numeric", Category: "decimal"}}},
		{ID: "private", Name: "Private", Source: semantics.SourceReference{Source: "warehouse", Context: "private_context", Dataset: "private", ProfileVersion: "private_profile", ProfileDigest: strings.Repeat("b", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "secret", SourceName: "secret", Name: "Secret", NativeType: "numeric", Category: "decimal"}}},
	}, Measures: []semantics.Measure{
		{ID: "amount", Name: "Amount", Description: "Amount", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}, Aggregation: semantics.AggregationSum},
		{ID: "secret", Name: "Secret", Description: "Secret", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "private", ID: "secret"}, Aggregation: semantics.AggregationSum},
	}, KPIs: []semantics.KPI{{ID: "secret_kpi", Name: "Secret KPI", Description: "Secret", Expression: "secret", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "secret"}}}}}
	model, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	selected := []semantics.Reference{{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}}
	catalog, err := enhancementMetricCatalog(model, selected)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range catalog {
		if metric.ID == "secret" || metric.ID == "secret_kpi" {
			t.Fatalf("unselected context metric escaped catalog: %#v", catalog)
		}
	}
}

func TestEnhancementMetricCatalogCarriesPriorPageMeasure(t *testing.T) {
	pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "topic", Version: "v1", Name: "Topic", Description: "Synthetic", Datasets: []semantics.Dataset{{ID: "orders", Name: "Orders", Source: semantics.SourceReference{Source: "warehouse", Context: "context", Dataset: "orders", ProfileVersion: "profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "numeric", Category: "decimal"}, {ID: "tax", SourceName: "tax", Name: "Tax", NativeType: "numeric", Category: "decimal"}}}}}
	model, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	pageOne, err := semantics.ApplyEnhancements(model, "v2", []semantics.Enhancement{{Dataset: "orders", Column: "amount", Kind: semantics.EnhancementMeasure, Name: "Amount", Aggregation: semantics.AggregationSum}})
	if err != nil {
		t.Fatal(err)
	}
	tax := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "tax"}
	catalog, err := enhancementMetricCatalog(pageOne, []semantics.Reference{tax})
	if err != nil {
		t.Fatal(err)
	}
	prior := semantics.GeneratedEntityID(semantics.EnhancementMeasure, "orders", "amount")
	wire := enhancementWire{Results: []semantics.Enhancement{{Dataset: "orders", Column: "tax", Kind: semantics.EnhancementMeasure}}, KPIs: []semantics.KPI{{Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: prior}}}}}
	if err = validateEnhancementOutput([]semantics.Reference{tax}, catalog, wire); err != nil {
		t.Fatalf("prior page generated measure unavailable to next page KPI: %v %#v", err, catalog)
	}
}
