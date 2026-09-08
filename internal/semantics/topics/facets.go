package topics

import (
	"encoding/json"
	"sort"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/vindex"
)

const MaxFacets = 4096
const MaxFacetBytes = 256 << 10
const MaxVectorValues = 4 << 20

type facetGroup struct {
	generation vindex.Generation
	facets     []vindex.Facet
}

// facetPlan associates every input with one actual source/context. Cross-context
// KPI expressions remain unsupported rather than leaking a joint text into either
// context. Same-context multi-source KPIs get an origin for each dependency source.
func facetPlan(model semantics.Model, space gateway.EmbeddingSpace) ([]facetGroup, error) {
	p := model.Pack()
	if model.Digest() == "" || !vindex.Space(space).Valid() {
		return nil, readexec.ErrUnsupported
	}
	groups := map[string]*facetGroup{}
	datasets := map[string]semantics.Dataset{}
	refs := map[string][]semantics.Reference{}
	for _, m := range p.Measures {
		refs["measure:"+m.ID] = []semantics.Reference{m.Field}
	}
	for _, d := range p.Dimensions {
		refs["dimension:"+d.ID] = []semantics.Reference{d.Field}
	}
	for _, k := range p.KPIs {
		refs["kpi:"+k.ID] = k.Inputs
	}
	total, bytes := 0, 0
	add := func(context, source, kind, id string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil || len(raw) > 4096 {
			return readexec.ErrLimit
		}
		total++
		bytes += len(raw)
		if total > MaxFacets || bytes > MaxFacetBytes || total*space.Dimensions > MaxVectorValues {
			return readexec.ErrLimit
		}
		group := groups[context]
		if group == nil {
			group = &facetGroup{generation: vindex.Generation{ID: vindex.Digest([]any{"topic-facets-v1", model.Digest(), context, space.Key()}), Topic: p.Topic, Version: p.Version, Context: context, SourceGeneration: model.Digest(), Space: vindex.Space(space)}}
			groups[context] = group
		}
		facetID := vindex.Digest([]string{kind, source, id})
		text := string(raw)
		group.generation.Expected = append(group.generation.Expected, vindex.Origin{ID: facetID, Kind: kind, SourceID: source, TextHash: vindex.TextHash(text)})
		group.facets = append(group.facets, vindex.Facet{ID: facetID, Kind: kind, SourceID: source, Text: text})
		return nil
	}
	for _, d := range p.Datasets {
		datasets[d.ID] = d
		if err := add(d.Source.Context, d.Source.Source, "entity", vindex.Digest([]string{"dataset", d.ID}), struct{ ID, Name string }{d.ID, d.Name}); err != nil {
			return nil, err
		}
		for _, c := range d.Columns {
			if err := add(d.Source.Context, d.Source.Source, "entity", vindex.Digest([]string{"column", d.ID, c.ID}), struct {
				Dataset string
				Column  semantics.Column
			}{d.ID, c}); err != nil {
				return nil, err
			}
		}
	}
	// Canonical identity and vocabulary are global, while physical keys remain
	// local to the source/context that can actually use them. A multi-context
	// entity therefore yields one independently bounded facet per source.
	for _, entity := range p.CanonicalEntities {
		type origin struct{ context, source string }
		keys := map[origin][]semantics.Reference{}
		for _, key := range entity.Keys {
			dataset, ok := datasets[key.Dataset]
			if !ok {
				return nil, readexec.ErrUnsupported
			}
			where := origin{dataset.Source.Context, dataset.Source.Source}
			keys[where] = append(keys[where], key)
		}
		origins := make([]origin, 0, len(keys))
		for where := range keys {
			origins = append(origins, where)
		}
		sort.Slice(origins, func(i, j int) bool {
			if origins[i].context != origins[j].context {
				return origins[i].context < origins[j].context
			}
			return origins[i].source < origins[j].source
		})
		for _, where := range origins {
			local := semantics.CanonicalEntity{ID: entity.ID, Revision: entity.Revision, Name: entity.Name, Aliases: append([]string(nil), entity.Aliases...), Keys: append([]semantics.Reference(nil), keys[where]...)}
			id := vindex.Digest([]any{"canonical", entity.ID, entity.Revision})
			if err := add(where.context, where.source, "entity", id, local); err != nil {
				return nil, err
			}
		}
	}
	// Each context gets topic metadata associated with its actual sources; no
	// physical column/reference information from another context is included.
	contextSources := map[string]map[string]bool{}
	for _, d := range p.Datasets {
		if contextSources[d.Source.Context] == nil {
			contextSources[d.Source.Context] = map[string]bool{}
		}
		contextSources[d.Source.Context][d.Source.Source] = true
	}
	for context, sources := range contextSources {
		for source := range sources {
			if err := add(context, source, "topic", p.Topic, struct{ Name, Description string }{p.Name, p.Description}); err != nil {
				return nil, err
			}
		}
	}
	memo := map[string]map[string]bool{}
	var resolved func(semantics.Reference) map[string]bool
	resolved = func(ref semantics.Reference) map[string]bool {
		key := string(ref.Kind) + "\x00" + ref.Dataset + "\x00" + ref.ID
		if cached, ok := memo[key]; ok {
			return cached
		}
		result := map[string]bool{}
		if ref.Kind == semantics.KindColumn {
			result[ref.Dataset] = true
		} else {
			for _, next := range refs[string(ref.Kind)+":"+ref.ID] {
				for dataset := range resolved(next) {
					result[dataset] = true
				}
			}
		}
		memo[key] = result
		return result
	}
	resolve := func(ref semantics.Reference, sources map[string]bool) {
		for dataset := range resolved(ref) {
			sources[dataset] = true
		}
	}
	addEntity := func(kind, id string, value any, references []semantics.Reference) error {
		used := map[string]bool{}
		for _, ref := range references {
			resolve(ref, used)
		}
		context := ""
		sources := map[string]bool{}
		for id := range used {
			d := datasets[id]
			if context != "" && context != d.Source.Context {
				return readexec.ErrUnsupported
			}
			context = d.Source.Context
			sources[d.Source.Source] = true
		}
		if context == "" {
			return readexec.ErrUnsupported
		}
		for source := range sources {
			if err := add(context, source, kind, id, value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, m := range p.Measures {
		if err := addEntity("measure", m.ID, m, []semantics.Reference{m.Field}); err != nil {
			return nil, err
		}
	}
	for _, d := range p.Dimensions {
		if err := addEntity("dimension", d.ID, d, []semantics.Reference{d.Field}); err != nil {
			return nil, err
		}
	}
	for _, k := range p.KPIs {
		if err := addEntity("kpi", k.ID, k, k.Inputs); err != nil {
			return nil, err
		}
	}
	for _, j := range p.Joins {
		if err := addEntity("relationship", j.ID, j, []semantics.Reference{j.Left, j.Right}); err != nil {
			return nil, err
		}
	}
	out := make([]facetGroup, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.facets, func(i, j int) bool { return group.facets[i].ID < group.facets[j].ID })
		sort.Slice(group.generation.Expected, func(i, j int) bool { return group.generation.Expected[i].ID < group.generation.Expected[j].ID })
		if !group.generation.Valid() {
			return nil, readexec.ErrLimit
		}
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].generation.Context < out[j].generation.Context })
	return out, nil
}
