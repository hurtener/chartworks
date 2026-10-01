package nlqroute

import (
	"context"
	"github.com/hurtener/chartworks/internal/semantics"
)

// selectCompletenessOutputs promotes only declared, structurally validated
// companions of directly selected SUM roots. Dependency-only SUM leaves of a
// different KPI do not silently acquire additional output obligations.
func selectCompletenessOutputs(ctx context.Context, in RouteRequest, item *admittedTopic) (bool, error) {
	if item.selection == nil {
		return false, nil
	}
	def := item.publication.Definition
	catalog := semantics.CompletenessCatalog{Measures: def.Measures, KPIs: def.KPIs, Columns: map[semantics.Reference]semantics.Column{}}
	for _, d := range def.Datasets {
		for _, c := range d.Columns {
			catalog.Columns[semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: c.ID}] = c
		}
	}
	changed := false
	for ref := range item.selection.roots {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if ref.Kind != semantics.KindMeasure {
			continue
		}
		for _, m := range def.Measures {
			if m.ID != ref.ID {
				continue
			}
			if m.Completeness == nil {
				if m.Aggregation == semantics.AggregationSum && requestedAmountDisclosure(in) {
					return false, &Clarification{Reason: "conflicting_semantic_completeness", Outcome: semantics.ClarificationMissing, Prompt: "This amount metric needs a reviewed unknown-count companion before completeness can be disclosed."}
				}
				continue
			}
			binding, err := semantics.ResolveKnownAmountCompleteness(catalog, m.ID)
			if err != nil {
				return false, ErrMetricContext
			}
			added, err := addSelectedRoot(item, binding.UnknownCount, "required_completeness", in.OmittedRoots)
			if err != nil {
				return false, selectionFailure(in.Locale, "conflicting_semantic_completeness")
			}
			changed = changed || added
		}
	}
	return changed, nil
}
