package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// SourceDatasetPin is an immutable registered table origin, not reviewed topic
// semantics. It contains no connection credentials, SQL, rows or policy grants.
type SourceDatasetPin struct {
	Source         string `json:"source"`
	Context        string `json:"context"`
	Dataset        string `json:"dataset"`
	SourceRevision int64  `json:"source_revision"`
	SchemaDigest   string `json:"schema_digest"`
}

func (p SourceDatasetPin) valid() bool {
	return identity.Identifier(p.Source) && identity.Identifier(p.Context) && identity.Identifier(p.Dataset) && p.SourceRevision > 0 && hashValid(p.SchemaDigest)
}

// ParentTopic and ParentSource identify the exclusive durable block parent.
// Topic-backed definitions keep the original parent and authority behavior.
func (d Definition) ParentTopic() string {
	if d.SourceDataset == nil && len(d.Topics) > 0 {
		return d.Topics[0].Topic
	}
	return ""
}
func (d Definition) ParentSource() string {
	if d.SourceDataset != nil && len(d.Topics) == 0 {
		return d.Source
	}
	return ""
}

func sourceDatasetDefinitionValid(d Definition) bool {
	if d.SourceDataset == nil {
		return len(d.Topics) > 0 && len(d.Topics) <= 8
	}
	p := d.SourceDataset
	return d.SchemaVersion == CurrentSchemaVersion && p.valid() && p.Source == d.Source && p.Context == d.Context && len(d.Topics) == 0 && len(d.Rules) == 0 && d.Template == nil && len(d.Templates) == 0 && len(d.AmountCompleteness) == 0
}

func preparationOrigin(r AuthoringPreparationRecord) Definition {
	return Definition{SchemaVersion: CurrentSchemaVersion, SourceDataset: r.Request.Intent.SourceDataset, Source: r.Binding.Source, Context: r.Binding.Context, Topics: r.Topics}
}

// RequireOrigin enforces a real topic or source parent. A source-backed chart
// does not mutate source registration and never requires or gains source-write.
func RequireOrigin(e identity.Envelope, topic, source string, a Access, creating bool) error {
	if topic != "" && source == "" {
		return RequireParent(e, topic, a, creating)
	}
	if topic != "" || !identity.Identifier(source) {
		return ErrInvalid
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: source})
}

func sourceDatasetRelation(pin SourceDatasetPin, binding exec.Binding) (exec.Relation, error) {
	if !pin.valid() || !binding.Valid() || binding.Source != pin.Source || binding.Context != pin.Context || binding.Revision != pin.SourceRevision {
		return exec.Relation{}, ErrStale
	}
	for _, relation := range binding.Relations {
		if relation.ID == pin.Dataset && exec.Hash(relation) == pin.SchemaDigest {
			return relation, nil
		}
	}
	return exec.Relation{}, ErrStale
}

// CheckBinding verifies exact source/context/revision/schema continuity. It is
// not an authority proof; callers must independently enforce signed reach.
func (p SourceDatasetPin) CheckBinding(binding exec.Binding) error {
	_, err := sourceDatasetRelation(p, binding)
	return err
}

func (s *Authoring) sourceDataset(ctx context.Context, e identity.Envelope, in AuthoringDatasetRequest) (*Service, topics.Published, topics.Dataset, error) {
	var empty topics.Published
	var dataset topics.Dataset
	if ctx == nil || in.SourceDataset == nil || !in.SourceDataset.valid() || in.Topic != (TopicPin{}) || in.Dataset != in.SourceDataset.Dataset {
		return nil, empty, dataset, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return nil, empty, dataset, err
	}
	p := in.SourceDataset
	if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: p.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: p.Context}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: p.Dataset}); err != nil {
		return nil, empty, dataset, err
	}
	blocks, err := s.blockService()
	if err != nil {
		return nil, empty, dataset, err
	}
	if blocks.sources == nil {
		return nil, empty, dataset, ErrUnavailable
	}
	binding, err := blocks.sources.ContextBinding(ctx, e, p.Source, p.Context)
	if err != nil {
		return nil, empty, dataset, err
	}
	dataset, err = sourceDatasetFields(*p, binding)
	return blocks, empty, dataset, err
}

// sourceDatasetFields adapts registered physical metadata to the shared field
// compiler. It neither creates a topic nor asserts reviewed semantic meaning.
func sourceDatasetFields(pin SourceDatasetPin, binding exec.Binding) (topics.Dataset, error) {
	relation, err := sourceDatasetRelation(pin, binding)
	if err != nil {
		return topics.Dataset{}, err
	}
	d := topics.Dataset{ID: pin.Dataset, Name: relation.Name, Source: topics.Binding{Source: pin.Source, Context: pin.Context, Dataset: pin.Dataset, SourceRevision: pin.SourceRevision}}
	for _, column := range relation.Columns {
		if !column.Safe {
			continue
		}
		d.Columns = append(d.Columns, semantics.Column{ID: semanticBinding("column", column.Name), SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
	}
	return d, nil
}

func (s *Service) resolveSourceDataset(ctx context.Context, e identity.Envelope, d Definition) ([]topics.Published, []ResourceReference, error) {
	if !sourceDatasetDefinitionValid(d) || d.SourceDataset == nil || s.sources == nil {
		return nil, nil, ErrInvalid
	}
	refs := definitionReferences(d, nil)
	if err := RequireReferences(e, Read, refs); err != nil {
		return nil, nil, err
	}
	binding, err := s.sources.ContextBinding(ctx, e, d.Source, d.Context)
	if err != nil {
		return nil, nil, err
	}
	if _, err := sourceDatasetRelation(*d.SourceDataset, binding); err != nil {
		return nil, nil, err
	}
	for _, parameter := range d.Parameters {
		if parameter.Dimension != nil {
			return nil, nil, ErrInvalid
		}
	}
	for _, output := range d.Outputs {
		if output.Mapping == nil {
			continue
		}
		for _, column := range output.Mapping.Columns {
			p := column.Provenance
			if p.Topic != "" || p.TopicVersion != "" || p.SemanticID != "" || p.Source != d.Source || p.SourceRevision != d.SourceDataset.SourceRevision {
				return nil, nil, ErrInvalid
			}
		}
	}
	return nil, refs, nil
}

func definitionValidationScope(binding exec.Binding, d Definition, definitions []topics.Published) ([]exec.RelationScope, error) {
	if d.SourceDataset == nil {
		return validationScope(binding, definitions)
	}
	if !sourceDatasetDefinitionValid(d) {
		return nil, ErrInvalid
	}
	relation, err := sourceDatasetRelation(*d.SourceDataset, binding)
	if err != nil {
		return nil, err
	}
	columns := []string{}
	for _, column := range relation.Columns {
		if column.Safe {
			columns = append(columns, column.Name)
		}
	}
	if len(columns) == 0 {
		return nil, ErrInvalid
	}
	return []exec.RelationScope{{Dataset: relation.ID, Columns: columns}}, nil
}
