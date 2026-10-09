package reporting

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/jackc/pgx/v5"
)

const AuthoringCompilerVersion = "reviewed-dataset-postgres-v1"

// Stable field selections and one exact origin; no SQL, client schema or rows.
type AuthoringDatasetIntent struct {
	SourceDataset *SourceDatasetPin        `json:"source_dataset,omitempty"`
	Fields        *AuthoringFieldSelection `json:"fields,omitempty"`
	Filters       []AuthoringDatasetFilter `json:"filters,omitempty"`
	Topic         TopicPin                 `json:"topic,omitempty"`
	Dataset       string                   `json:"dataset"`
	Dimensions    []string                 `json:"dimensions"`
	Measure       string                   `json:"measure"`
	Mapping       AuthoringChartMapping    `json:"mapping"`
}
type AuthoringDatasetRequest struct {
	SourceDataset *SourceDatasetPin `json:"source_dataset,omitempty"`
	Topic         TopicPin          `json:"topic,omitempty"`
	Dataset       string            `json:"dataset"`
}
type AuthoringSemanticField struct {
	ID          string `json:"id"`
	Binding     string `json:"binding"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Aggregation string `json:"aggregation,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}
type AuthoringDatasetView struct {
	SourceDataset      *SourceDatasetPin           `json:"source_dataset,omitempty"`
	Fields             *AuthoringFieldCatalog      `json:"fields,omitempty"`
	FilterCompiler     string                      `json:"filter_compiler,omitempty"`
	FilterCapabilities []AuthoringFilterCapability `json:"filter_capabilities,omitempty"`
	Compiler           string                      `json:"compiler"`
	Topic              TopicPin                    `json:"topic"`
	Dataset            string                      `json:"dataset"`
	Source             string                      `json:"source"`
	Context            string                      `json:"context"`
	SourceRevision     int64                       `json:"source_revision"`
	Dialect            string                      `json:"dialect"`
	Dimensions         []AuthoringSemanticField    `json:"dimensions"`
	Measures           []AuthoringSemanticField    `json:"measures"`
	ChartKinds         []charts.Kind               `json:"chart_kinds"`
	Limits             []string                    `json:"limitations"`
	Supported          bool                        `json:"supported"`
	Reason             string                      `json:"reason,omitempty"`
}
type authoringUnsupported struct{ code string }

func (e *authoringUnsupported) Error() string  { return "reporting: " + e.code }
func (e *authoringUnsupported) Unwrap() error  { return ErrInvalid }
func unsupportedPreparation(code string) error { return &authoringUnsupported{code} }
func manualChartKinds() []charts.Kind {
	return []charts.Kind{charts.Bar, charts.ColumnChart, charts.Line, charts.Area, charts.Pie, charts.Donut, charts.KPI, charts.Table}
}
func semanticBinding(kind, id string) string { return kind + "_" + exec.Hash([]string{kind, id})[:24] }
func dimensionUnsupported(d semantics.Dimension, dataset string) string {
	if d.Field.Kind != semantics.KindColumn || d.Field.Dataset != dataset {
		return "different_dataset"
	}
	if len(d.Filters) != 0 {
		return "dimension_filters_unsupported"
	}
	if d.Temporal != nil {
		return "temporal_policy_unsupported"
	}
	return ""
}
func measureUnsupported(m semantics.Measure, dataset string) string {
	if m.Field.Kind != semantics.KindColumn || m.Field.Dataset != dataset {
		return "different_dataset"
	}
	if len(m.Filters) != 0 {
		return "measure_filters_unsupported"
	}
	if m.Completeness != nil {
		return "amount_completeness_unsupported"
	}
	if !slices.Contains([]semantics.Aggregation{semantics.AggregationSum, semantics.AggregationAverage, semantics.AggregationMinimum, semantics.AggregationMaximum, semantics.AggregationCount, semantics.AggregationDistinctCount}, m.Aggregation) {
		return "aggregation_unsupported"
	}
	return ""
}
func datasetUnsupported(p topics.Published) string {
	if p.Definition.GroupDomain != nil {
		return "group_domain_policy_unsupported"
	}
	if p.Definition.GroupedPopulation != nil {
		return "grouped_population_unsupported"
	}
	if len(p.Definition.Datasets) > 0 {
		base := p.Definition.Datasets[0].Source
		for _, d := range p.Definition.Datasets {
			if d.Source.Source != base.Source || d.Source.Context != base.Context {
				return "multiple_source_contexts_unsupported"
			}
		}
	}
	return ""
}
func (s *Authoring) datasetPublication(ctx context.Context, e identity.Envelope, in AuthoringDatasetRequest) (*Service, topics.Published, topics.Dataset, error) {
	var p topics.Published
	var dataset topics.Dataset
	if in.SourceDataset != nil {
		return s.sourceDataset(ctx, e, in)
	}
	if ctx == nil || !identity.Identifier(in.Topic.Topic) || !identity.Identifier(in.Topic.Version) || !hashValid(in.Topic.Digest) || !identity.Identifier(in.Dataset) {
		return nil, p, dataset, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return nil, p, dataset, err
	}
	if err := access.Require(e, "topics.read", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: in.Topic.Topic}); err != nil {
		return nil, p, dataset, err
	}
	blocks, err := s.blockService()
	if err != nil {
		return nil, p, dataset, err
	}
	p, err = blocks.topics.Read(ctx, e, in.Topic.Topic, in.Topic.Version)
	if err != nil {
		return nil, p, dataset, err
	}
	if p.Definition.Topic != in.Topic.Topic || p.Definition.Version != in.Topic.Version || p.State.Topic != in.Topic.Topic || p.State.Version != in.Topic.Version || p.Digest != in.Topic.Digest || !p.State.Active || p.State.Archived {
		return nil, p, dataset, ErrStale
	}
	found := 0
	for _, item := range p.Definition.Datasets {
		if item.ID == in.Dataset {
			dataset = item
			found++
		}
	}
	if found != 1 || dataset.Source.Dataset != dataset.ID {
		return nil, p, dataset, ErrInvalid
	}
	return blocks, p, dataset, nil
}

// Dataset reads retained metadata only, including retained source dialect.
// Unsupported reviewed concepts remain visible with explicit dispositions.
func (s *Authoring) Dataset(ctx context.Context, e identity.Envelope, in AuthoringDatasetRequest) (AuthoringDatasetView, error) {
	blocks, p, d, err := s.datasetPublication(ctx, e, in)
	if err != nil {
		return AuthoringDatasetView{}, err
	}
	out := AuthoringDatasetView{SourceDataset: clone(in.SourceDataset), Compiler: AuthoringCompilerVersion, Topic: in.Topic, Dataset: d.ID, Source: d.Source.Source, Context: d.Source.Context, SourceRevision: d.Source.SourceRevision, Dimensions: []AuthoringSemanticField{}, Measures: []AuthoringSemanticField{}, ChartKinds: manualChartKinds(), Limits: []string{"PostgreSQL field selection supports multiple groupings and measures within the advertised column and result budgets.", "Each chart uses one dataset. Only advertised field, aggregation, calendar and filter capabilities are supported; joins and arbitrary expressions are not inferred.", "Physical columns do not imply reviewed meaning. Reviewed metrics retain their definitions and policy restrictions.", "Prepare reads actual schema. Create remains private and unvalidated; Validate is a separate explicit read."}}
	var filterBinding exec.Binding
	out.Reason = datasetUnsupported(p)
	if out.Reason == "" && in.SourceDataset == nil {
		out.Reason, err = blocks.authoringRulesDisposition(ctx, e, in.Topic)
	}
	if err != nil {
		return AuthoringDatasetView{}, err
	}
	if blocks.sources == nil {
		if out.Reason == "" {
			out.Reason = "preparation_unavailable"
		}
	} else {
		binding, err := blocks.sources.ContextBinding(ctx, e, d.Source.Source, d.Source.Context)
		if err != nil {
			return AuthoringDatasetView{}, err
		}
		filterBinding = binding
		out.Dialect = binding.Dialect
		if binding.Source != d.Source.Source || binding.Context != d.Source.Context || binding.Revision != d.Source.SourceRevision {
			out.Reason = "source_binding_changed"
		} else if binding.Dialect != "postgres" {
			out.Reason = "dialect_unsupported"
		}
	}
	out.Supported = out.Reason == ""
	for _, dimension := range p.Definition.Dimensions {
		reason := dimensionUnsupported(dimension, d.ID)
		out.Dimensions = append(out.Dimensions, AuthoringSemanticField{ID: dimension.ID, Binding: semanticBinding("d", dimension.ID), Name: dimension.Name, Role: string(dimension.Role), Supported: reason == "", Reason: reason})
	}
	for _, measure := range p.Definition.Measures {
		reason := measureUnsupported(measure, d.ID)
		out.Measures = append(out.Measures, AuthoringSemanticField{ID: measure.ID, Binding: semanticBinding("m", measure.ID), Name: measure.Name, Role: "measure", Aggregation: string(measure.Aggregation), Unit: measure.Unit, Supported: reason == "", Reason: reason})
	}
	out.Fields = authoringFieldCatalog(d, filterBinding, blocks.limits.MaxSchemaColumns)
	out.Fields.Dimensions = authoringGroupingCapabilities(p, d, out.Fields.Columns)
	out.Fields.Supported, out.Fields.Reason = out.Supported, out.Reason
	if out.Fields.Supported && !slices.ContainsFunc(out.Fields.Columns, func(field AuthoringColumnCapability) bool { return field.Supported }) {
		out.Fields.Supported, out.Fields.Reason = false, "no_supported_column"
	}
	if out.Supported && !slices.ContainsFunc(out.Measures, func(field AuthoringSemanticField) bool { return field.Supported }) {
		out.Supported, out.Reason = false, "no_supported_measure"
	}
	out.FilterCompiler = AuthoringFilteredCompilerVersion
	_, optionsAvailable := blocks.repo.(AuthoringOptionRepository)
	out.FilterCapabilities = authoringFilterCapabilities(p, d, filterBinding, out.Fields.Reason, optionsAvailable && blocks.CanValidate())
	return out, ctx.Err()
}

type authoringCompiled struct {
	Parameters   []Parameter
	SQL          string
	Columns      []charts.Column
	Scope        []exec.RelationScope
	Dependencies []Dependency
}

func compileAuthoringDataset(in AuthoringDatasetIntent, p topics.Published, dataset topics.Dataset, binding exec.Binding) (authoringCompiled, error) {
	var out authoringCompiled
	if in.SourceDataset != nil {
		if in.Topic != (TopicPin{}) || in.Fields == nil || in.SourceDataset.Dataset != in.Dataset {
			return out, ErrInvalid
		}
		if _, err := sourceDatasetRelation(*in.SourceDataset, binding); err != nil {
			return out, err
		}
	}
	if binding.Dialect != "postgres" {
		return out, unsupportedPreparation("dialect_unsupported")
	}
	if !binding.Valid() || binding.Source != dataset.Source.Source || binding.Context != dataset.Source.Context || binding.Revision != dataset.Source.SourceRevision || in.Dataset != dataset.ID || in.Topic.Topic != p.Definition.Topic || in.Topic.Version != p.Definition.Version || in.Topic.Digest != p.Digest {
		return out, ErrStale
	}
	if reason := datasetUnsupported(p); reason != "" {
		return out, unsupportedPreparation(reason)
	}
	if in.Fields != nil {
		return compileAuthoringFields(in, p, dataset, binding)
	}
	if len(in.Dimensions) > 2 || !identity.Identifier(in.Measure) || !slices.Contains(manualChartKinds(), in.Mapping.Kind) || in.Mapping.Intent != nil {
		return out, unsupportedPreparation("chart_shape_unsupported")
	}
	var relation *exec.Relation
	for i := range binding.Relations {
		if binding.Relations[i].ID == dataset.ID {
			relation = &binding.Relations[i]
		}
	}
	if relation == nil {
		return out, ErrStale
	}
	columns := map[string]semantics.Column{}
	for _, c := range dataset.Columns {
		if _, exists := columns[c.ID]; exists {
			return out, ErrInvalid
		}
		columns[c.ID] = c
	}
	selected := map[string]bool{}
	resolve := func(ref semantics.Reference) (semantics.Column, error) {
		column, ok := columns[ref.ID]
		if !ok || ref.Kind != semantics.KindColumn || ref.Dataset != dataset.ID {
			return column, ErrInvalid
		}
		found := 0
		for _, actual := range relation.Columns {
			if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
				found++
			}
		}
		if found != 1 {
			return column, ErrStale
		}
		selected[column.SourceName] = true
		return column, nil
	}
	projection, grouping := []string{}, []string{}
	seen := map[string]bool{}
	for _, id := range in.Dimensions {
		if !identity.Identifier(id) || seen[id] {
			return out, ErrInvalid
		}
		seen[id] = true
		var d *semantics.Dimension
		for i := range p.Definition.Dimensions {
			if p.Definition.Dimensions[i].ID == id {
				if d != nil {
					return out, ErrInvalid
				}
				d = &p.Definition.Dimensions[i]
			}
		}
		if d == nil {
			return out, ErrInvalid
		}
		if reason := dimensionUnsupported(*d, dataset.ID); reason != "" {
			return out, unsupportedPreparation(reason)
		}
		c, err := resolve(d.Field)
		if err != nil {
			return out, err
		}
		alias := semanticBinding("d", id)
		physical := pgx.Identifier{c.SourceName}.Sanitize()
		projection = append(projection, physical+" AS "+pgx.Identifier{alias}.Sanitize())
		grouping = append(grouping, physical)
		role := "dimension"
		if d.Role == semantics.DimensionTemporal {
			role = "time"
		}
		if d.Role == semantics.DimensionIdentifier {
			role = "identifier"
		}
		out.Columns = append(out.Columns, charts.Column{ID: alias, Name: alias, DisplayLabel: d.Name, Role: role, Provenance: charts.Provenance{Version: 1, Source: binding.Source, SourceRevision: binding.Revision, Topic: in.Topic.Topic, TopicVersion: in.Topic.Version, SemanticID: id}})
	}
	var measure *semantics.Measure
	for i := range p.Definition.Measures {
		if p.Definition.Measures[i].ID == in.Measure {
			if measure != nil {
				return out, ErrInvalid
			}
			measure = &p.Definition.Measures[i]
		}
	}
	if measure == nil {
		return out, ErrInvalid
	}
	if reason := measureUnsupported(*measure, dataset.ID); reason != "" {
		return out, unsupportedPreparation(reason)
	}
	c, err := resolve(measure.Field)
	if err != nil {
		return out, err
	}
	op := map[semantics.Aggregation]string{semantics.AggregationSum: "sum", semantics.AggregationAverage: "avg", semantics.AggregationMinimum: "min", semantics.AggregationMaximum: "max", semantics.AggregationCount: "count", semantics.AggregationDistinctCount: "count"}[measure.Aggregation]
	argument := pgx.Identifier{c.SourceName}.Sanitize()
	if measure.Aggregation == semantics.AggregationDistinctCount {
		argument = "DISTINCT " + argument
	}
	alias := semanticBinding("m", measure.ID)
	projection = append(projection, op+"("+argument+") AS "+pgx.Identifier{alias}.Sanitize())
	out.Columns = append(out.Columns, charts.Column{ID: alias, Name: alias, DisplayLabel: measure.Name, Role: "measure", Aggregation: string(measure.Aggregation), Format: charts.Format{Unit: measure.Unit}, Provenance: charts.Provenance{Version: 1, Source: binding.Source, SourceRevision: binding.Revision, Topic: in.Topic.Topic, TopicVersion: in.Topic.Version, SemanticID: measure.ID}})
	out.SQL = "SELECT " + strings.Join(projection, ", ") + " FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize()
	predicates, parameters, err := compileAuthoringFilters(in, p, resolve)
	if err != nil {
		return out, err
	}
	out.Parameters = parameters
	if len(predicates) > 0 {
		out.SQL += " WHERE " + strings.Join(predicates, " AND ")
	}
	if len(grouping) > 0 {
		out.SQL += " GROUP BY " + strings.Join(grouping, ", ") + " ORDER BY " + strings.Join(grouping, ", ")
	}
	names := []string{}
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	out.Scope = []exec.RelationScope{{Dataset: dataset.ID, Columns: names}}
	out.Dependencies, err = deriveDependencies(binding, out.Scope, []string{dataset.ID})
	return out, err
}
