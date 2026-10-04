package reporting

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

type authoringOptionResolution struct {
	filterOptionResolution
	topic     TopicPin
	dataset   string
	dimension string
	refs      []ResourceReference
	blocks    []AuthoringOptionBlock
}

func (s *Authoring) resolveAuthoringOptionDimension(ctx context.Context, e identity.Envelope, pin TopicPin, datasetID, dimensionID string) (authoringOptionResolution, error) {
	blocks, p, dataset, err := s.datasetPublication(ctx, e, AuthoringDatasetRequest{Topic: pin, Dataset: datasetID})
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if !blocks.CanValidate() {
		return authoringOptionResolution{}, ErrUnavailable
	}
	if reason := datasetUnsupported(p); reason != "" {
		return authoringOptionResolution{}, unsupportedPreparation(reason)
	}
	if reason, err := blocks.authoringRulesDisposition(ctx, e, pin); err != nil {
		return authoringOptionResolution{}, err
	} else if reason != "" {
		return authoringOptionResolution{}, unsupportedPreparation(reason)
	}
	var dimension *semantics.Dimension
	for i := range p.Definition.Dimensions {
		if p.Definition.Dimensions[i].ID == dimensionID {
			if dimension != nil {
				return authoringOptionResolution{}, ErrInvalid
			}
			dimension = &p.Definition.Dimensions[i]
		}
	}
	if dimension == nil {
		return authoringOptionResolution{}, ErrInvalid
	}
	if reason := dimensionUnsupported(*dimension, datasetID); reason != "" {
		return authoringOptionResolution{}, unsupportedPreparation(reason)
	}
	var column *semantics.Column
	for i := range dataset.Columns {
		if dataset.Columns[i].ID == dimension.Field.ID {
			if column != nil {
				return authoringOptionResolution{}, ErrInvalid
			}
			column = &dataset.Columns[i]
		}
	}
	if column == nil || !authoringTextColumn(*column) {
		return authoringOptionResolution{}, unsupportedPreparation("option_dimension_type_unsupported")
	}
	binding, err := blocks.sources.ContextBinding(ctx, e, dataset.Source.Source, dataset.Source.Context)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if binding.Dialect != "postgres" {
		return authoringOptionResolution{}, unsupportedPreparation("dialect_unsupported")
	}
	if binding.Source != dataset.Source.Source || binding.Context != dataset.Source.Context || binding.Revision != dataset.Source.SourceRevision {
		return authoringOptionResolution{}, ErrStale
	}
	if err := exec.Require(e, binding, []string{datasetID}); err != nil {
		return authoringOptionResolution{}, err
	}
	relation, physical, ok := filterColumn(binding, datasetID, column.SourceName)
	if !ok || physical.NativeType != column.NativeType || physical.Category != column.Category || physical.Nullable != column.Nullable || !physical.Safe {
		return authoringOptionResolution{}, ErrStale
	}
	// A single exact reviewed field supplies the full governed option population.
	// Chart selections/defaults are deliberately not cross-filtered into this read.
	scope := []exec.RelationScope{{Dataset: datasetID, Columns: []string{physical.Name}}}
	refs := definitionReferences(Definition{Source: binding.Source, Context: binding.Context, Topics: []TopicPin{pin}}, []topics.Published{p})
	if err := RequireReferences(e, Read, refs); err != nil {
		return authoringOptionResolution{}, err
	}
	return authoringOptionResolution{filterOptionResolution: filterOptionResolution{binding: binding, relation: relation, physical: physical, semantic: *column, publications: []topics.Published{p}, scope: scope}, topic: pin, dataset: datasetID, dimension: dimensionID, refs: refs, blocks: []AuthoringOptionBlock{}}, nil
}

func (s *Authoring) resolveAuthoringOption(ctx context.Context, e identity.Envelope, target AuthoringOptionTarget) (authoringOptionResolution, error) {
	if err := RequireAuthoringOptionTarget(e, target); err != nil {
		return authoringOptionResolution{}, err
	}
	if d := target.Dataset; d != nil {
		return s.resolveAuthoringOptionDimension(ctx, e, d.Topic, d.Dataset, d.Dimension)
	}
	r := target.Report
	snapshot, err := s.documents.repo.ReadDocument(ctx, e, "report", r.Report, DocumentReference{Revision: r.Revision}, Read, false)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if snapshot.State.Archived || snapshot.Revision.Digest != r.Digest || r.Policy == "private_preview" && (snapshot.Revision.Actor != e.User() || snapshot.State.DraftRevision != r.Revision) || r.Policy == "published" && (snapshot.PublishedAt == nil || snapshot.State.PublishedRevision != r.Revision) {
		return authoringOptionResolution{}, ErrStale
	}
	definition, err := ProjectStoredDocument(snapshot.Revision.Raw, "report")
	if err != nil {
		return authoringOptionResolution{}, err
	}
	if definition.SchemaVersion != PagedDocumentVersion {
		return authoringOptionResolution{}, unsupportedPreparation("private_option_pages_required")
	}
	canvas, err := SelectReportCanvas(definition, r.Page)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	var filter *ReportFilter
	for i := range canvas.Definition.Filters {
		if canvas.Definition.Filters[i].Parameter.Name == r.Filter {
			filter = &canvas.Definition.Filters[i]
		}
	}
	if filter == nil || filter.Parameter.Dimension == nil || !slices.Contains([]string{"dimension_value", "dimension_set"}, filter.Parameter.Type) {
		return authoringOptionResolution{}, unsupportedPreparation("private_option_filter_unsupported")
	}
	var out authoringOptionResolution
	for _, w := range canvas.Definition.Widgets {
		for _, b := range w.Bindings {
			if b.Filter != r.Filter {
				continue
			}
			if w.Block == nil || !slices.Contains([]string{"", "published", "private_preview"}, w.Block.Policy) || r.Policy == "published" && w.Block.Policy == "private_preview" {
				return authoringOptionResolution{}, unsupportedPreparation("private_option_binding_unsupported")
			}
			if w.Block.Revision < 1 {
				return authoringOptionResolution{}, unsupportedPreparation("option_exact_block_revision_required")
			}
			actions := []Access{Read, Execute}
			if w.Block.Policy == "private_preview" {
				actions = append(actions, Preview)
			}
			for _, a := range actions {
				if err := Require(e, w.Block.Block, a); err != nil {
					return authoringOptionResolution{}, err
				}
			}
			block, err := s.documents.blocks.repo.ReadBlock(ctx, e, w.Block.Block, Reference{Revision: w.Block.Revision}, Execute)
			if err != nil {
				return authoringOptionResolution{}, err
			}
			if err := CheckDocumentBlockReference(e, *w.Block, block); err != nil {
				return authoringOptionResolution{}, err
			}
			pins, err := AuthoringRuleAbsence(block.Revision)
			if err != nil || len(pins) != 1 {
				return authoringOptionResolution{}, unsupportedPreparation("private_option_origin_unsupported")
			}
			if err := s.documents.blocks.checkAuthoringRuleAbsence(ctx, e, block.Revision); err != nil {
				return authoringOptionResolution{}, err
			}
			// Arbitrary copied private definitions do not gain a broader option
			// population merely by declaring a dimension with a familiar name.
			if _, _, err := s.documents.blocks.resolveDefinitions(ctx, e, block.Revision.Definition, true); err != nil {
				return authoringOptionResolution{}, err
			}
			var parameter *Parameter
			for i := range block.Revision.Definition.Parameters {
				if block.Revision.Definition.Parameters[i].Name == b.Parameter {
					parameter = &block.Revision.Definition.Parameters[i]
				}
			}
			if parameter == nil || parameter.Type != filter.Parameter.Type || parameter.Dimension == nil || *parameter.Dimension != *filter.Parameter.Dimension {
				return authoringOptionResolution{}, unsupportedPreparation("private_option_dimension_mismatch")
			}
			pin := pins[0]
			if parameter.Dimension.Topic != pin.Topic || parameter.Dimension.Version != pin.Version {
				return authoringOptionResolution{}, ErrStale
			}
			publication, err := s.documents.blocks.topics.Read(ctx, e, pin.Topic, pin.Version)
			if err != nil {
				return authoringOptionResolution{}, err
			}
			dataset := ""
			for _, d := range publication.Definition.Dimensions {
				if d.ID == parameter.Dimension.Dimension {
					dataset = d.Field.Dataset
				}
			}
			resolved, err := s.resolveAuthoringOptionDimension(ctx, e, pin, dataset, parameter.Dimension.Dimension)
			if err != nil {
				return authoringOptionResolution{}, err
			}
			if block.Revision.Definition.Source != resolved.binding.Source || block.Revision.Definition.Context != resolved.binding.Context {
				return authoringOptionResolution{}, ErrStale
			}
			if out.dataset == "" {
				out = resolved
			} else if optionResolutionDigest(out) != optionResolutionDigest(resolved) {
				return authoringOptionResolution{}, unsupportedPreparation("private_option_population_mismatch")
			}
			policy := w.Block.Policy
			if policy == "" {
				policy = "published"
			}
			out.blocks = append(out.blocks, AuthoringOptionBlock{Block: w.Block.Block, Revision: w.Block.Revision, Digest: block.Revision.Digest, Policy: policy})
		}
	}
	if len(out.blocks) == 0 {
		return authoringOptionResolution{}, unsupportedPreparation("private_option_unbound")
	}
	out.blocks, err = canonicalAuthoringOptionBlocks(out.blocks)
	if err != nil {
		return authoringOptionResolution{}, err
	}
	return out, nil
}

func canonicalAuthoringOptionBlocks(in []AuthoringOptionBlock) ([]AuthoringOptionBlock, error) {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b AuthoringOptionBlock) int {
		if order := strings.Compare(a.Block, b.Block); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Revision, b.Revision); order != 0 {
			return order
		}
		if order := strings.Compare(a.Digest, b.Digest); order != 0 {
			return order
		}
		return strings.Compare(a.Policy, b.Policy)
	})
	for i := 1; i < len(out); i++ {
		if out[i-1].Block == out[i].Block && out[i-1].Revision == out[i].Revision && out[i-1].Digest != out[i].Digest {
			return nil, ErrStale
		}
	}
	return slices.Compact(out), nil
}

func optionResolutionDigest(r authoringOptionResolution) string {
	return digest([]any{r.binding, r.topic, r.dataset, r.dimension, r.physical, r.semantic, r.scope})
}
