package drafts

import (
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestPeriodProposalUsesExactAdmittedMeasureAndDimensionIDs(t *testing.T) {
	measure := semantics.Reference{Kind: semantics.KindMeasure, ID: "gross"}
	dimension := semantics.Reference{Kind: semantics.KindDimension, ID: "order_time"}
	metric := enhancementMetric{Kind: semantics.KindMeasure, ID: measure.ID, Availability: "existing"}
	catalog := map[string]enhancementMetric{string(measure.Kind) + "\x00" + measure.ID: metric}
	kpi := semantics.KPI{ID: "net", Periods: &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: measure, Dimension: dimension}}}}
	if err := validatePeriodProposals([]semantics.KPI{kpi}, catalog, nil, []semantics.Reference{dimension}); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*semantics.KPI){"foreign measure": func(k *semantics.KPI) { k.Periods.Bindings[0].Measure.ID = "foreign" }, "foreign dimension": func(k *semantics.KPI) { k.Periods.Bindings[0].Dimension.ID = "foreign" }, "raw field": func(k *semantics.KPI) {
		k.Periods.Bindings[0].Dimension = semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "date"}
	}, "policy": func(k *semantics.KPI) { k.Periods.Policy = "guess" }} {
		t.Run(name, func(t *testing.T) {
			copy := kpi
			periods := *kpi.Periods
			periods.Bindings = append([]semantics.MetricPeriodBinding(nil), periods.Bindings...)
			copy.Periods = &periods
			edit(&copy)
			if err := validatePeriodProposals([]semantics.KPI{copy}, catalog, nil, []semantics.Reference{dimension}); !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("out-of-envelope period accepted", err)
			}
		})
	}
	metric.Availability = "current_step"
	catalog[string(measure.Kind)+"\x00"+measure.ID] = metric
	if err := validatePeriodProposals([]semantics.KPI{kpi}, catalog, nil, []semantics.Reference{dimension}); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("uncreated period measure admitted", err)
	}
	if err := validatePeriodProposals([]semantics.KPI{kpi}, catalog, map[string]bool{measure.ID: true}, []semantics.Reference{dimension}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if json.Unmarshal(enhancementSchema, &document) != nil {
		t.Fatal("schema")
	}
	periods := document["properties"].(map[string]any)["kpis"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["periods"].(map[string]any)
	if periods["additionalProperties"] != false {
		t.Fatal("period metadata permits unbound literal/SQL fields")
	}
}
