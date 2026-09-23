package nlqroute

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
)

type semanticEvidenceGroup struct {
	Version      string                 `json:"version"`
	Topic        string                 `json:"topic"`
	TopicVersion string                 `json:"topic_version"`
	Root         semantics.Reference    `json:"candidate_root"`
	Dependencies []nlq.MetricDependency `json:"dependencies"`
}

func (h hitWithTopic) contextEvidence() string {
	if h.contextText != "" {
		return h.contextText
	}
	return h.hit.Text
}

// hydrateSemanticEvidence fills retrieved measures, KPIs and dimensions from
// the exact admitted publication, not natural-language search text. The vector
// origin text stays unchanged for receipts/reranking. The context packer admits
// or omits each complete group atomically; retrieval does not pin every candidate.
func hydrateSemanticEvidence(ctx context.Context, admitted []admittedTopic, hits []hitWithTopic) ([]hitWithTopic, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	byTopic := make(map[string]topics.Published, len(admitted))
	for _, item := range admitted {
		if _, duplicate := byTopic[item.id]; duplicate {
			return nil, ErrInvalid
		}
		byTopic[item.id] = item.publication
	}
	out := append([]hitWithTopic(nil), hits...)
	seen := make(map[string]bool, len(out))
	for i := range out {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hit := &out[i]
		switch hit.hit.Kind {
		case "measure", "kpi", "dimension":
		default:
			continue
		}
		publication, ok := byTopic[hit.topic]
		if !ok || hit.hit.Version != publication.State.Version || hit.hit.SourceGeneration != publication.Digest {
			return nil, gateway.ErrOutput
		}
		key := hit.topic + "\x00" + hit.hit.ID
		if seen[key] {
			return nil, gateway.ErrOutput
		}
		seen[key] = true
		root, canonical, err := catalogFacet(publication.Definition, hit.hit)
		if err != nil {
			return nil, err
		}
		// The publication produced this exact facet body. Matching identity with
		// a different body is stale/corrupt evidence, never model instructions.
		if string(canonical) != hit.hit.Text {
			return nil, gateway.ErrOutput
		}
		dependencies, err := semanticClosure(ctx, publication.Definition, []semantics.Reference{root})
		if err != nil {
			return nil, err
		}
		if !semanticFacetOriginMatches(publication.Definition, root, hit.hit.SourceID) {
			return nil, gateway.ErrOutput
		}
		raw, err := json.Marshal(semanticEvidenceGroup{
			Version: "semantic-evidence-v1", Topic: hit.topic, TopicVersion: publication.State.Version,
			Root: root, Dependencies: dependencies,
		})
		if err != nil {
			return nil, gateway.ErrOutput
		}
		if len(raw) > 16<<10 {
			return nil, nlq.ErrInsufficient
		}
		hit.contextText = string(raw)
	}
	return out, nil
}

func catalogFacet(def topics.Definition, hit vindex.Hit) (semantics.Reference, []byte, error) {
	var root semantics.Reference
	var value any
	matches := 0
	match := func(id string, candidate any) {
		if hit.ID == vindex.Digest([]string{hit.Kind, hit.SourceID, id}) {
			root, value = semantics.Reference{Kind: semantics.Kind(hit.Kind), ID: id}, candidate
			matches++
		}
	}
	switch hit.Kind {
	case "measure":
		for _, candidate := range def.Measures {
			match(candidate.ID, candidate)
		}
	case "kpi":
		for _, candidate := range def.KPIs {
			match(candidate.ID, candidate)
		}
	case "dimension":
		for _, candidate := range def.Dimensions {
			match(candidate.ID, candidate)
		}
	}
	if matches != 1 || !root.Valid() {
		return semantics.Reference{}, nil, gateway.ErrOutput
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return semantics.Reference{}, nil, gateway.ErrOutput
	}
	return root, raw, nil
}

// Only a root's expression/field/filter dependencies establish its facet source.
// A relationship bridge or an associated dimension is not an additional origin.
func semanticFacetOriginMatches(def topics.Definition, root semantics.Reference, source string) bool {
	refs := map[semantics.Reference][]semantics.Reference{}
	add := func(ref semantics.Reference, inputs []semantics.Reference, filters []semantics.SemanticFilter) {
		values := append([]semantics.Reference(nil), inputs...)
		for _, filter := range filters {
			values = append(values, filter.Field)
		}
		refs[ref] = values
	}
	for _, value := range def.Measures {
		add(semantics.Reference{Kind: semantics.KindMeasure, ID: value.ID}, []semantics.Reference{value.Field}, value.Filters)
	}
	for _, value := range def.Dimensions {
		add(semantics.Reference{Kind: semantics.KindDimension, ID: value.ID}, []semantics.Reference{value.Field}, value.Filters)
	}
	for _, value := range def.KPIs {
		add(semantics.Reference{Kind: semantics.KindKPI, ID: value.ID}, value.Inputs, value.Filters)
	}
	seen := map[semantics.Reference]bool{}
	queue := []semantics.Reference{root}
	for len(queue) > 0 {
		ref := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		if len(seen) > maxSemanticNodes {
			return false
		}
		if ref.Kind == semantics.KindColumn {
			for _, dataset := range def.Datasets {
				if dataset.ID == ref.Dataset && dataset.Source.Source == source {
					return true
				}
			}
		} else {
			queue = append(queue, refs[ref]...)
		}
	}
	return false
}
