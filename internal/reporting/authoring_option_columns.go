package reporting

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func optionColumnType(kind string) bool {
	return slices.Contains([]string{"text", "identifier", "integer", "number", "boolean"}, kind)
}
func optionFilterParameter(p Parameter) bool {
	return p.Dimension != nil && p.Column == nil && slices.Contains([]string{"dimension_value", "dimension_set"}, p.Type) || p.Column != nil && p.Dimension == nil && optionColumnType(p.Column.Type) && slices.Contains([]string{"column_value", "column_set"}, p.Type)
}
func (r authoringOptionResolution) cursorField() string {
	if r.column != nil {
		return "column:" + digest(r.column)
	}
	return r.dimension
}
func (r authoringOptionResolution) cursorType() string {
	if r.column != nil {
		return r.column.Type
	}
	return "text"
}

func (s *Authoring) resolveAuthoringOptionColumn(ctx context.Context, e identity.Envelope, in AuthoringDatasetRequest, id string, expected *ColumnReference) (authoringOptionResolution, error) {
	blocks, p, dataset, err := s.datasetPublication(ctx, e, in)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if !blocks.CanValidate() {
		return authoringOptionResolution{}, ErrUnavailable
	}
	publications := []topics.Published{}
	if in.SourceDataset == nil {
		if reason := datasetUnsupported(p); reason != "" {
			return authoringOptionResolution{}, unsupportedPreparation(reason)
		}
		if reason, err := blocks.authoringRulesDisposition(ctx, e, in.Topic); err != nil {
			return authoringOptionResolution{}, err
		} else if reason != "" {
			return authoringOptionResolution{}, unsupportedPreparation(reason)
		}
		publications = append(publications, p)
	}
	var column *semantics.Column
	for i := range dataset.Columns {
		c := &dataset.Columns[i]
		if id != "" && c.ID == id || expected != nil && c.SourceName == expected.Name {
			if column != nil {
				return authoringOptionResolution{}, ErrInvalid
			}
			column = c
		}
	}
	if column == nil {
		return authoringOptionResolution{}, ErrInvalid
	}
	kind := columnFilterType(*column)
	if !optionColumnType(kind) {
		return authoringOptionResolution{}, unsupportedPreparation("option_column_type_unsupported")
	}
	binding, err := blocks.sources.ContextBinding(ctx, e, dataset.Source.Source, dataset.Source.Context)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if binding.Dialect != "postgres" {
		return authoringOptionResolution{}, unsupportedPreparation("dialect_unsupported")
	}
	if binding.Revision != dataset.Source.SourceRevision {
		return authoringOptionResolution{}, ErrStale
	}
	if err := exec.Require(e, binding, []string{in.Dataset}); err != nil {
		return authoringOptionResolution{}, err
	}
	relation, physical, ok := filterColumn(binding, in.Dataset, column.SourceName)
	if !ok || !physical.Safe || physical.NativeType != column.NativeType || physical.Category != column.Category || physical.Nullable != column.Nullable {
		return authoringOptionResolution{}, ErrStale
	}
	ref := &ColumnReference{SourceDataset: SourceDatasetPin{Source: binding.Source, Context: binding.Context, Dataset: in.Dataset, SourceRevision: binding.Revision, SchemaDigest: exec.Hash(relation)}, Name: physical.Name, Type: kind}
	if !ref.valid() || expected != nil && *expected != *ref || in.SourceDataset != nil && *in.SourceDataset != ref.SourceDataset {
		return authoringOptionResolution{}, ErrStale
	}
	definition := Definition{Source: binding.Source, Context: binding.Context, SourceDataset: in.SourceDataset}
	if in.SourceDataset == nil {
		definition.Topics = []TopicPin{in.Topic}
	}
	refs := definitionReferences(definition, publications)
	if err := RequireReferences(e, Read, refs); err != nil {
		return authoringOptionResolution{}, err
	}
	return authoringOptionResolution{filterOptionResolution: filterOptionResolution{binding: binding, relation: relation, physical: physical, semantic: *column, publications: publications, scope: []exec.RelationScope{{Dataset: in.Dataset, Columns: []string{physical.Name}}}}, topic: in.Topic, dataset: in.Dataset, column: ref, refs: refs, blocks: []AuthoringOptionBlock{}}, nil
}

func (s *Authoring) resolveBlockOptionParameter(ctx context.Context, e identity.Envelope, r Revision, p Parameter, pins []TopicPin) (authoringOptionResolution, error) {
	if p.Column != nil {
		in := AuthoringDatasetRequest{SourceDataset: r.Definition.SourceDataset, Dataset: p.Column.SourceDataset.Dataset}
		if in.SourceDataset == nil {
			if len(pins) != 1 {
				return authoringOptionResolution{}, ErrStale
			}
			in.Topic = pins[0]
		}
		return s.resolveAuthoringOptionColumn(ctx, e, in, "", p.Column)
	}
	if p.Dimension == nil || len(pins) != 1 {
		return authoringOptionResolution{}, ErrInvalid
	}
	pin := pins[0]
	if p.Dimension.Topic != pin.Topic || p.Dimension.Version != pin.Version {
		return authoringOptionResolution{}, ErrStale
	}
	publication, err := s.documents.blocks.topics.Read(ctx, e, pin.Topic, pin.Version)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	dataset := ""
	for _, d := range publication.Definition.Dimensions {
		if d.ID == p.Dimension.Dimension {
			dataset = d.Field.Dataset
		}
	}
	return s.resolveAuthoringOptionDimension(ctx, e, pin, dataset, p.Dimension.Dimension)
}

// Native result values are exact strings for integer/decimal, JSON numbers for
// floating point and JSON booleans. Physical selectors always transport strings
// so browser parsing cannot round values before their typed binder sees them.
func physicalOptionValue(c ColumnReference, field exec.Field, raw json.RawMessage) (json.RawMessage, error) {
	valid := c.Type == "text" && field.Type == "text" || c.Type == "identifier" && field.Type == "text" || c.Type == "integer" && field.Type == "integer" || c.Type == "number" && slices.Contains([]string{"number", "decimal"}, field.Type) || c.Type == "boolean" && field.Type == "boolean"
	if !valid || len(raw) == 0 || len(raw) > filterEncodedMax || string(raw) == "null" {
		return nil, ErrInvalid
	}
	var value string
	if field.Type == "number" || field.Type == "boolean" {
		value = string(raw)
	} else if json.Unmarshal(raw, &value) != nil {
		return nil, ErrInvalid
	}
	if len(value) > filterScalarMax {
		return nil, ErrBudget
	}
	if _, err := columnFilterScalar(c, value); err != nil {
		return nil, err
	}
	out, err := json.Marshal(value)
	return out, err
}
func optionCursorParameter(r authoringOptionResolution, raw json.RawMessage) (exec.Parameter, error) {
	if r.column == nil {
		return filterParameter("text", raw)
	}
	var value string
	if len(raw) > filterEncodedMax || json.Unmarshal(raw, &value) != nil || len(value) > filterScalarMax {
		return exec.Parameter{}, ErrInvalid
	}
	return columnFilterScalar(*r.column, value)
}
func optionSearchParameter(r authoringOptionResolution, search string) (exec.Parameter, error) {
	if r.column != nil && r.column.Type != "text" {
		return columnFilterScalar(*r.column, search)
	}
	return exec.Parameter{Kind: "text", Value: "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search) + "%"}, nil
}
func authoringOptionStatement(r authoringOptionResolution, search, cursor bool, limit int) (string, error) {
	exact := r.column != nil && r.column.Type != "text"
	return filterOptionStatementSearchPolicy(r.binding, r.relation, r.physical, search, cursor, limit, true, exact)
}
