package drafts

import (
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

func periodDimensionCatalog(pack semantics.TopicPack, selected []semantics.Reference) []semantics.Reference {
	out := []semantics.Reference{}
	for _, dimension := range pack.Dimensions {
		if dimension.Role == semantics.DimensionTemporal && dimension.Temporal != nil {
			out = append(out, semantics.Reference{Kind: semantics.KindDimension, ID: dimension.ID})
		}
	}
	for _, field := range selected {
		out = append(out, semantics.Reference{Kind: semantics.KindDimension, ID: semantics.GeneratedEntityID(semantics.EnhancementDimension, field.Dataset, field.ID)})
	}
	return out
}

func validatePeriodProposals(kpis []semantics.KPI, metrics map[string]enhancementMetric, current map[string]bool, dimensions []semantics.Reference) error {
	for _, kpi := range kpis {
		if kpi.Periods != nil {
			if len(kpi.Periods.Bindings) > 4 || kpi.Periods.Policy != semantics.MetricPeriodBindingsPolicy {
				return gateway.ErrOutput
			}
			for _, binding := range kpi.Periods.Bindings {
				item, ok := metrics[string(semantics.KindMeasure)+"\x00"+binding.Measure.ID]
				if !ok || binding.Measure.Kind != semantics.KindMeasure || item.Availability == "current_step" && !current[binding.Measure.ID] || !wantReference(dimensions, binding.Dimension) {
					return gateway.ErrOutput
				}
			}
		}
	}
	return nil
}

func addMetricPeriodSchema(properties map[string]any) {
	ref := func(kind string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "id"}, "properties": map[string]any{"kind": map[string]any{"const": kind}, "id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}}
	}
	periods := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy", "bindings"}, "properties": map[string]any{"policy": map[string]any{"const": semantics.MetricPeriodBindingsPolicy}, "bindings": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"measure", "dimension"}, "properties": map[string]any{"measure": ref("measure"), "dimension": ref("dimension")}}}}}
	properties["kpis"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["periods"] = periods
}
