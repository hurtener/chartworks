package topicfeedback

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"reflect"
	"strings"
)

func applyEdits(p semantics.TopicPack, version string, edits []Edit, vocabulary ...[]drafts.AuthoringValue) (semantics.TopicPack, error) {
	if len(edits) < 1 || len(edits) > 16 {
		return semantics.TopicPack{}, gateway.ErrOutput
	}
	model, err := semantics.Compile(p)
	if err != nil {
		return semantics.TopicPack{}, gateway.ErrOutput
	}
	var catalog []drafts.AuthoringValue
	if len(vocabulary) > 0 {
		catalog = vocabulary[0]
	}
	catalog, err = drafts.AdmitAuthoringVocabulary(model, catalog)
	if err != nil {
		return semantics.TopicPack{}, err
	}
	raw, _ := json.Marshal(p)
	var out semantics.TopicPack
	if json.Unmarshal(raw, &out) != nil {
		return out, gateway.ErrOutput
	}
	out.Version = version
	seen := map[string]bool{}
	for _, edit := range edits {
		key := string(edit.Kind) + ":" + edit.ID
		if seen[key] {
			return out, gateway.ErrOutput
		}
		seen[key] = true
		if edit.Description != nil && !validText(*edit.Description, 4096) {
			return out, gateway.ErrOutput
		}
		if edit.Aliases != nil {
			if len(*edit.Aliases) > 32 {
				return out, gateway.ErrOutput
			}
			for _, a := range *edit.Aliases {
				if !validText(a, 256) {
					return out, gateway.ErrOutput
				}
			}
		}
		if edit.Description == nil && edit.Aliases == nil && edit.Aggregation == nil && edit.Expression == nil && edit.Inputs == nil && edit.Temporal == nil && edit.FilterIDs == nil && len(edit.NewFilters) == 0 && len(edit.VocabularyIDs) == 0 {
			return out, gateway.ErrOutput
		}
		found := false
		switch edit.Kind {
		case semantics.KindMeasure:
			if edit.Expression != nil || edit.Inputs != nil || edit.Temporal != nil || len(edit.VocabularyIDs) > 0 {
				return out, gateway.ErrOutput
			}
			for i := range out.Measures {
				v := &out.Measures[i]
				if v.ID != edit.ID {
					continue
				}
				found = true
				if edit.Description != nil {
					v.Description = *edit.Description
				}
				if edit.Aliases != nil {
					v.Aliases = append([]string(nil), (*edit.Aliases)...)
				}
				if edit.Aggregation != nil {
					v.Aggregation = *edit.Aggregation
				}
				if len(edit.NewFilters) > 16 {
					return out, gateway.ErrOutput
				}
				// Additions cannot overwrite even a filter omitted by filter_ids.
				used := map[string]bool{}
				for _, existing := range v.Filters {
					used[existing.ID] = true
				}
				additions := []semantics.SemanticFilter{}
				for _, proposed := range edit.NewFilters {
					if proposed.Measure != edit.ID || used[proposed.ID] {
						return out, gateway.ErrOutput
					}
					used[proposed.ID] = true
					resolved, resolveErr := drafts.ResolveAuthoringFilter(model, catalog, proposed)
					if resolveErr != nil {
						return out, resolveErr
					}
					additions = append(additions, resolved)
				}
				if edit.FilterIDs != nil {
					var err error
					v.Filters, err = selectFilters(v.Filters, *edit.FilterIDs)
					if err != nil {
						return out, err
					}
				}
				v.Filters = append(v.Filters, additions...)
			}
		case semantics.KindDimension:
			if len(edit.NewFilters) > 0 {
				return out, gateway.ErrOutput
			}
			if edit.Aggregation != nil || edit.Expression != nil || edit.Inputs != nil {
				return out, gateway.ErrOutput
			}
			for i := range out.Dimensions {
				v := &out.Dimensions[i]
				if v.ID != edit.ID {
					continue
				}
				found = true
				if len(edit.VocabularyIDs) > 0 {
					if v.Role != semantics.DimensionCategorical {
						return out, gateway.ErrOutput
					}
					values, resolveErr := drafts.ResolveAuthoringValues(model, catalog, v.Field, edit.VocabularyIDs)
					if resolveErr != nil {
						return out, resolveErr
					}
					for _, value := range values {
						for _, existing := range v.Values {
							if existing.ID == value.ID || existing.Value == value.Value {
								return out, gateway.ErrOutput
							}
						}
						v.Values = append(v.Values, value)
					}
				}
				if edit.Description != nil {
					v.Description = *edit.Description
				}
				if edit.Aliases != nil {
					v.Aliases = append([]string(nil), (*edit.Aliases)...)
				}
				if edit.Temporal != nil {
					if v.Role != semantics.DimensionTemporal {
						return out, gateway.ErrOutput
					}
					v.Temporal = edit.Temporal
				}
				if edit.FilterIDs != nil {
					var err error
					v.Filters, err = selectFilters(v.Filters, *edit.FilterIDs)
					if err != nil {
						return out, err
					}
				}
			}
		case semantics.KindKPI:
			if len(edit.NewFilters) > 0 {
				return out, gateway.ErrOutput
			}
			if edit.Aggregation != nil || edit.Temporal != nil || len(edit.VocabularyIDs) > 0 {
				return out, gateway.ErrOutput
			}
			for i := range out.KPIs {
				v := &out.KPIs[i]
				if v.ID != edit.ID {
					continue
				}
				found = true
				if edit.Description != nil {
					v.Description = *edit.Description
				}
				if edit.Aliases != nil {
					v.Aliases = append([]string(nil), (*edit.Aliases)...)
				}
				if edit.Expression != nil {
					if !validText(*edit.Expression, 4096) {
						return out, gateway.ErrOutput
					}
					v.Expression = *edit.Expression
				}
				if edit.Inputs != nil {
					for _, r := range *edit.Inputs {
						if !existingMetric(p, r) {
							return out, gateway.ErrOutput
						}
					}
					v.Inputs = append([]semantics.Reference(nil), (*edit.Inputs)...)
				}
				if edit.FilterIDs != nil {
					var err error
					v.Filters, err = selectFilters(v.Filters, *edit.FilterIDs)
					if err != nil {
						return out, err
					}
				}
			}
		default:
			return out, gateway.ErrOutput
		}
		if !found {
			return out, gateway.ErrOutput
		}
	}
	before := p
	before.Version = version
	if reflect.DeepEqual(before, out) {
		return out, gateway.ErrOutput
	}
	for _, edit := range edits {
		if edit.Expression != nil || edit.Inputs != nil {
			for _, k := range out.KPIs {
				if k.ID == edit.ID && !closedExpression(k) {
					return out, gateway.ErrOutput
				}
			}
		}
	}
	model, err = semantics.Compile(out)
	if err != nil {
		return out, gateway.ErrOutput
	}
	return model.Pack(), nil
}
func existingMetric(p semantics.TopicPack, r semantics.Reference) bool {
	if !r.Valid() {
		return false
	}
	if r.Kind == semantics.KindMeasure {
		for _, v := range p.Measures {
			if v.ID == r.ID {
				return true
			}
		}
	}
	if r.Kind == semantics.KindKPI {
		for _, v := range p.KPIs {
			if v.ID == r.ID {
				return true
			}
		}
	}
	return false
}
func selectFilters(existing []semantics.SemanticFilter, ids []string) ([]semantics.SemanticFilter, error) {
	if len(ids) > len(existing) {
		return nil, gateway.ErrOutput
	}
	out := make([]semantics.SemanticFilter, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, gateway.ErrOutput
		}
		seen[id] = true
		found := false
		for _, f := range existing {
			if f.ID == id {
				out = append(out, f)
				found = true
			}
		}
		if !found {
			return nil, gateway.ErrOutput
		}
	}
	return out, nil
}
func proposalSchema() (*gateway.Schema, error) {
	return gateway.NewSchema("semantic_feedback_proposal", []byte(`{"type":"object","additionalProperties":false,"required":["edits"],"properties":{"edits":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"object","additionalProperties":false,"required":["kind","id"],"properties":{"kind":{"type":"string","enum":["measure","dimension","kpi"]},"id":{"type":"string","minLength":1,"maxLength":128},"description":{"type":"string","minLength":1,"maxLength":4096},"aliases":{"type":"array","maxItems":32,"items":{"type":"string","minLength":1,"maxLength":256}},"aggregation":{"type":"string","enum":["sum","average","minimum","maximum","count","distinct_count"]},"expression":{"type":"string","minLength":1,"maxLength":4096},"inputs":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"object","additionalProperties":false,"required":["kind","id"],"properties":{"kind":{"type":"string","enum":["measure","kpi"]},"id":{"type":"string","minLength":1,"maxLength":128}}}},"vocabulary_ids":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"string","minLength":1,"maxLength":128}},"new_filters":{"type":"array","maxItems":16,"items":{"type":"object","additionalProperties":false,"required":["measure","id","operator","nulls","vocabulary_ids"],"properties":{"measure":{"type":"string","minLength":1,"maxLength":128},"id":{"type":"string","minLength":1,"maxLength":128},"operator":{"type":"string","enum":["eq","in"]},"nulls":{"type":"string","enum":["exclude"]},"vocabulary_ids":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"string","minLength":1,"maxLength":128}}}}},"filter_ids":{"type":"array","maxItems":32,"items":{"type":"string","minLength":1,"maxLength":128}},"temporal":{"type":"object","additionalProperties":false,"required":["calendar","timezone","grains"],"properties":{"calendar":{"type":"string","maxLength":32},"timezone":{"type":"string","maxLength":128},"grains":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","enum":["minute","hour","day","week","month","quarter","year"]}}}}}}}}}`))
}

// New expressions use a closed two-input arithmetic vocabulary, never SQL,
// constants, function calls, subqueries or historical literals.
func closedExpression(k semantics.KPI) bool {
	if len(k.Inputs) != 2 {
		return false
	}
	for _, op := range []string{"plus", "minus", "times", "divided by"} {
		if strings.TrimSpace(k.Expression) == k.Inputs[0].ID+" "+op+" "+k.Inputs[1].ID {
			return true
		}
	}
	return false
}
