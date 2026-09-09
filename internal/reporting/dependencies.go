package reporting

import (
	"context"
	"slices"
	"sort"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func definitionReferences(d Definition, definitions []topics.Published) []ResourceReference {
	seen := map[ResourceReference]bool{}
	add := func(kind, permission, id string) {
		seen[ResourceReference{Kind: kind, Permission: permission, ID: id}] = true
	}
	add("source", "read", d.Source)
	add("execution_context", "use", d.Context)
	for _, pin := range d.Topics {
		add("topic", "read", pin.Topic)
	}
	for _, pin := range d.Topics {
		add("topic", "read", pin.Topic)
	}
	for _, publication := range definitions {
		add("topic", "read", publication.Definition.Topic)
		for _, dataset := range publication.Definition.Datasets {
			add("source", "read", dataset.Source.Source)
			add("execution_context", "use", dataset.Source.Context)
			add("dataset", "query", dataset.ID)
		}
	}
	out := make([]ResourceReference, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Permission != b.Permission {
			return a.Permission < b.Permission
		}
		return a.ID < b.ID
	})
	return out
}

// resolveDefinitions loads retained semantic metadata only. It never calls a
// warehouse or model. Authoring may inspect old pins; validation requires active
// exact pins so a publication change cannot silently refresh reviewed meaning.
func (s *Service) resolveDefinitions(ctx context.Context, e identity.Envelope, d Definition, current bool) ([]topics.Published, []ResourceReference, error) {
	if len(d.Topics) == 0 || len(d.Topics) > 8 {
		return nil, nil, ErrInvalid
	}
	out := make([]topics.Published, 0, len(d.Topics))
	for _, pin := range d.Topics {
		if err := access.Require(e, "topics.read", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic}); err != nil {
			return nil, nil, err
		}
		p, err := s.topics.Read(ctx, e, pin.Topic, pin.Version)
		if err != nil {
			return nil, nil, err
		}
		if p.Definition.Topic != pin.Topic || p.Definition.Version != pin.Version || p.State.Topic != pin.Topic || p.State.Version != pin.Version || p.Digest != pin.Digest || len(p.Definition.Datasets) == 0 || current && (!p.State.Active || p.State.Archived) {
			return nil, nil, ErrStale
		}
		for _, dataset := range p.Definition.Datasets {
			if dataset.Source.Source != d.Source || dataset.Source.Context != d.Context || dataset.Source.Dataset != dataset.ID || dataset.Source.SourceRevision < 1 {
				return nil, nil, ErrInvalid
			}
		}
		out = append(out, p)
	}
	for _, parameter := range d.Parameters {
		if parameter.Dimension == nil {
			continue
		}
		found := false
		for _, p := range out {
			if p.Definition.Topic != parameter.Dimension.Topic || p.Definition.Version != parameter.Dimension.Version {
				continue
			}
			for _, dimension := range p.Definition.Dimensions {
				if dimension.ID == parameter.Dimension.Dimension {
					found = true
				}
			}
		}
		if !found {
			return nil, nil, ErrInvalid
		}
	}
	// Saved mapping provenance can describe real reviewed semantics only. A
	// presentation's display name never substitutes for a stable semantic ID.
	for _, output := range d.Outputs {
		if output.Mapping == nil {
			continue
		}
		for _, column := range output.Mapping.Columns {
			p := column.Provenance
			if p.Source != "" {
				found := false
				for _, publication := range out {
					for _, dataset := range publication.Definition.Datasets {
						if dataset.Source.Source == p.Source && dataset.Source.SourceRevision == p.SourceRevision {
							found = true
						}
					}
				}
				if !found {
					return nil, nil, ErrInvalid
				}
			}
			if p.Topic != "" {
				found := false
				for _, publication := range out {
					definition := publication.Definition
					if definition.Topic != p.Topic || definition.Version != p.TopicVersion {
						continue
					}
					for _, measure := range definition.Measures {
						if measure.ID == p.SemanticID {
							found = true
						}
					}
					for _, dimension := range definition.Dimensions {
						if dimension.ID == p.SemanticID {
							found = true
						}
					}
					for _, kpi := range definition.KPIs {
						if kpi.ID == p.SemanticID {
							found = true
						}
					}
					for _, dataset := range definition.Datasets {
						for _, field := range dataset.Columns {
							if field.ID == p.SemanticID {
								found = true
							}
						}
					}
				}
				if !found {
					return nil, nil, ErrInvalid
				}
			}
		}
	}
	return out, definitionReferences(d, out), nil
}

// validationScope is a positive intersection: only reviewed, currently safe
// columns from the actual source revision can reach the common validator.
func validationScope(binding exec.Binding, definitions []topics.Published) ([]exec.RelationScope, error) {
	if !binding.Valid() {
		return nil, ErrStale
	}
	relations := map[string]exec.Relation{}
	for _, relation := range binding.Relations {
		relations[relation.ID] = relation
	}
	allowed := map[string]map[string]bool{}
	for _, publication := range definitions {
		for _, dataset := range publication.Definition.Datasets {
			b := dataset.Source
			if b.Source != binding.Source || b.Context != binding.Context || b.SourceRevision != binding.Revision {
				return nil, ErrStale
			}
			r, ok := relations[dataset.ID]
			if !ok {
				return nil, ErrStale
			}
			if allowed[dataset.ID] == nil {
				allowed[dataset.ID] = map[string]bool{}
			}
			for _, column := range dataset.Columns {
				found := false
				for _, actual := range r.Columns {
					if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
						found = true
					}
				}
				if !found {
					return nil, ErrStale
				}
				allowed[dataset.ID][column.SourceName] = true
			}
		}
	}
	out := []exec.RelationScope{}
	for dataset, fields := range allowed {
		columns := make([]string, 0, len(fields))
		for name := range fields {
			columns = append(columns, name)
		}
		sort.Strings(columns)
		if len(columns) == 0 || len(columns) > 256 {
			return nil, ErrInvalid
		}
		out = append(out, exec.RelationScope{Dataset: dataset, Columns: columns})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dataset < out[j].Dataset })
	if len(out) == 0 || len(out) > 32 {
		return nil, ErrInvalid
	}
	return out, nil
}

func deriveDependencies(binding exec.Binding, scope []exec.RelationScope, ids []string) ([]Dependency, error) {
	if len(ids) == 0 || len(ids) > 32 {
		return nil, ErrInvalid
	}
	allowed := map[string][]string{}
	for _, item := range scope {
		allowed[item.Dataset] = item.Columns
	}
	seen := map[string]bool{}
	out := []Dependency{}
	for _, id := range ids {
		if seen[id] || len(allowed[id]) == 0 {
			return nil, ErrInvalid
		}
		seen[id] = true
		found := false
		for _, relation := range binding.Relations {
			if relation.ID != id {
				continue
			}
			dep := Dependency{Source: binding.Source, Context: binding.Context, SourceRevision: binding.Revision, Dataset: id, Schema: relation.Schema, Name: relation.Name, Columns: []exec.Column{}}
			for _, column := range relation.Columns {
				if slices.Contains(allowed[id], column.Name) {
					dep.Columns = append(dep.Columns, column)
				}
			}
			sort.Slice(dep.Columns, func(i, j int) bool { return dep.Columns[i].Name < dep.Columns[j].Name })
			if len(dep.Columns) != len(allowed[id]) {
				return nil, ErrInvalid
			}
			out = append(out, dep)
			found = true
		}
		if !found {
			return nil, ErrInvalid
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dataset < out[j].Dataset })
	return out, nil
}

// DependencyDigest commits both semantic publications and source-derived
// columns. It does not treat a display label or caller manifest as authority.
func DependencyDigest(dependencies []Dependency, pins []TopicPin) string {
	return digest(struct {
		Version      string
		Dependencies []Dependency
		Topics       []TopicPin
	}{CanonicalizationVersion, dependencies, pins})
}

func snapshotsReferences(snapshot Snapshot) []ResourceReference {
	if len(snapshot.References) > 0 {
		return clone(snapshot.References)
	}
	publications := []topics.Published{}
	if snapshot.Validation != nil {
		for _, d := range snapshot.Validation.Definitions {
			publications = append(publications, topics.Published{Definition: d})
		}
	}
	return definitionReferences(snapshot.Revision.Definition, publications)
}
