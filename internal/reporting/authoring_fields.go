package reporting

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/jackc/pgx/v5"
)

const AuthoringFieldCompilerVersion = "typed-dataset-postgres-v3"

// The discriminator is explicit. A physical column is not a reviewed metric,
// even when their names happen to match. The legacy request remains unchanged
// when Fields is omitted, including its canonical custody digest.
type AuthoringFieldSelection struct {
	Mode       string                      `json:"mode" jsonschema:"enum=aggregate,enum=rows"`
	Dimensions []AuthoringGrouping         `json:"dimensions"`
	Measures   []AuthoringMeasureSelection `json:"measures"`
}

type AuthoringGrouping struct {
	Kind     string `json:"kind" jsonschema:"enum=column,enum=dimension"`
	Field    string `json:"field"`
	Grain    string `json:"grain,omitempty"`
	Calendar string `json:"calendar,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type AuthoringMeasureSelection struct {
	Kind        string `json:"kind" jsonschema:"enum=column,enum=measure,enum=count"`
	Field       string `json:"field,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
}

type AuthoringColumnCapability struct {
	Filters      *ColumnFilterCapability `json:"filters,omitempty"`
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	SourceName   string                  `json:"source_name"`
	NativeType   string                  `json:"native_type"`
	Type         string                  `json:"type"`
	Category     string                  `json:"category"`
	Nullable     bool                    `json:"nullable"`
	Supported    bool                    `json:"supported"`
	Reason       string                  `json:"reason,omitempty"`
	Aggregations []string                `json:"aggregations"`
	Grains       []string                `json:"grains"`
}

type AuthoringFieldCatalog struct {
	Compiler   string                        `json:"compiler"`
	Supported  bool                          `json:"supported"`
	Reason     string                        `json:"reason,omitempty"`
	MaxColumns int                           `json:"max_columns"`
	Columns    []AuthoringColumnCapability   `json:"columns"`
	Dimensions []AuthoringGroupingCapability `json:"dimensions"`
}

type AuthoringGroupingCapability struct {
	ID        string                    `json:"id"`
	Name      string                    `json:"name"`
	Column    string                    `json:"column"`
	Supported bool                      `json:"supported"`
	Reason    string                    `json:"reason,omitempty"`
	Temporal  *semantics.TemporalPolicy `json:"temporal,omitempty"`
}

func authoringGroupingCapabilities(p topics.Published, d topics.Dataset, columns []AuthoringColumnCapability) []AuthoringGroupingCapability {
	out := []AuthoringGroupingCapability{}
	for _, dimension := range p.Definition.Dimensions {
		item := AuthoringGroupingCapability{ID: dimension.ID, Name: dimension.Name, Column: dimension.Field.ID, Temporal: clone(dimension.Temporal)}
		switch {
		case dimension.Field.Kind != semantics.KindColumn || dimension.Field.Dataset != d.ID:
			item.Reason = "different_dataset"
		case len(dimension.Filters) > 0:
			item.Reason = "dimension_filters_unsupported"
		case !slices.ContainsFunc(columns, func(c AuthoringColumnCapability) bool { return c.ID == item.Column && c.Supported }):
			item.Reason = "field_type_unsupported"
		case dimension.Temporal != nil && dimension.Temporal.Calendar != "gregorian":
			item.Reason = "calendar_unsupported"
		default:
			item.Supported = true
		}
		out = append(out, item)
	}
	return out
}

// Only actual physical types select available operations. Labels and semantic
// roles never coerce a numeric year or text period into a date.
func authoringColumnKind(c semantics.Column) string {
	native := c.NativeType
	// PostgreSQL format_type includes precision/scale and character length.
	// Remove only its numeric modifier syntax; the complete original native type
	// must still match the actual safe source column before compilation.
	if open := strings.IndexByte(native, '('); open > 0 {
		if close := strings.IndexByte(native, ')'); close > open+1 {
			modifier := native[open+1 : close]
			if strings.Trim(modifier, "0123456789,- ") == "" {
				native = native[:open] + native[close+1:]
			}
		}
	}
	switch native {
	case "int2", "int4", "int8", "smallint", "integer", "bigint", "numeric", "decimal", "float4", "float8", "real", "double precision":
		return "number"
	case "text", "varchar", "character varying", "bpchar", "char", "character":
		return "text"
	case "bool", "boolean":
		return "boolean"
	case "date":
		return "date"
	case "timestamp", "timestamp without time zone":
		return "timestamp"
	case "timestamptz", "timestamp with time zone":
		return "instant"
	case "uuid":
		return "identifier"
	}
	return ""
}

func authoringColumnAggregations(c semantics.Column) []string {
	kind := authoringColumnKind(c)
	if kind == "" {
		return []string{}
	}
	out := []string{"count", "distinct_count"}
	if kind == "number" {
		out = append(out, "sum", "average")
	}
	if kind == "number" {
		out = append(out, "minimum", "maximum")
	}
	return out
}

func authoringColumnGrains(c semantics.Column) []string {
	switch authoringColumnKind(c) {
	case "date":
		return []string{"day", "week", "month", "quarter", "year"}
	case "timestamp", "instant":
		return []string{"minute", "hour", "day", "week", "month", "quarter", "year"}
	}
	return []string{}
}

func authoringFieldCatalog(d topics.Dataset, binding exec.Binding, maxColumns int) *AuthoringFieldCatalog {
	out := &AuthoringFieldCatalog{Compiler: AuthoringFieldCompilerVersion, MaxColumns: maxColumns, Columns: []AuthoringColumnCapability{}}
	for _, c := range d.Columns {
		item := AuthoringColumnCapability{ID: c.ID, Name: c.Name, SourceName: c.SourceName, NativeType: c.NativeType, Type: authoringColumnKind(c), Category: c.Category, Nullable: c.Nullable, Aggregations: []string{}, Grains: []string{}}
		_, actual, found := filterColumn(binding, d.ID, c.SourceName)
		switch {
		case !found || actual.NativeType != c.NativeType || actual.Category != c.Category || actual.Nullable != c.Nullable:
			item.Reason = "source_binding_changed"
		case authoringColumnKind(c) == "":
			item.Reason = "field_type_unsupported"
		default:
			item.Supported = true
			item.Aggregations = authoringColumnAggregations(c)
			item.Grains = authoringColumnGrains(c)
			item.Filters = authoringColumnFilterCapability(c)
		}
		out.Columns = append(out.Columns, item)
	}
	return out
}

func authoringGroupExpression(c semantics.Column, in AuthoringGrouping, policy *semantics.TemporalPolicy) (string, error) {
	physical := pgx.Identifier{c.SourceName}.Sanitize()
	if in.Grain == "" {
		if in.Calendar != "" || in.Timezone != "" || policy != nil {
			return "", unsupportedPreparation("calendar_selection_required")
		}
		return physical, nil
	}
	if in.Calendar != "gregorian" || !slices.Contains(authoringColumnGrains(c), in.Grain) {
		return "", unsupportedPreparation("date_grouping_unsupported")
	}
	if policy != nil && (policy.Calendar != in.Calendar || policy.Timezone != in.Timezone || !slices.Contains(policy.Grains, semantics.TimeGrain(in.Grain))) {
		return "", unsupportedPreparation("reviewed_calendar_mismatch")
	}
	// A civil timestamp/date has no timezone. Instants require an explicit IANA
	// zone and use PostgreSQL's three-argument truncation, independent of session TZ.
	if authoringColumnKind(c) == "instant" {
		if in.Timezone == "" || len(in.Timezone) > 128 || in.Timezone == "Local" {
			return "", unsupportedPreparation("timezone_required")
		}
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return "", unsupportedPreparation("timezone_unsupported")
		}
		return "date_trunc('" + in.Grain + "', " + physical + ", '" + strings.ReplaceAll(in.Timezone, "'", "''") + "')", nil
	}
	if in.Timezone != "" {
		return "", unsupportedPreparation("civil_time_has_no_timezone")
	}
	if authoringColumnKind(c) == "date" {
		return "CAST(date_trunc('" + in.Grain + "', CAST(" + physical + " AS timestamp)) AS date)", nil
	}
	return "date_trunc('" + in.Grain + "', " + physical + ")", nil
}

func compileAuthoringFields(in AuthoringDatasetIntent, p topics.Published, dataset topics.Dataset, binding exec.Binding) (authoringCompiled, error) {
	var out authoringCompiled
	f := in.Fields
	if f == nil || len(in.Dimensions) != 0 || in.Measure != "" || !slices.Contains([]string{"aggregate", "rows"}, f.Mode) || len(f.Dimensions)+len(f.Measures) == 0 || len(f.Dimensions)+len(f.Measures) > 256 || f.Mode == "rows" && (len(f.Measures) != 0 || in.Mapping.Kind != charts.Table) || !slices.Contains(manualChartKinds(), in.Mapping.Kind) || in.Mapping.Intent != nil {
		return out, unsupportedPreparation("field_selection_unsupported")
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
		c, ok := columns[ref.ID]
		if !ok || !identity.Identifier(ref.ID) || ref.Kind != semantics.KindColumn || ref.Dataset != dataset.ID {
			return c, ErrInvalid
		}
		matches := 0
		for _, actual := range relation.Columns {
			if actual.Name == c.SourceName && actual.NativeType == c.NativeType && actual.Category == c.Category && actual.Nullable == c.Nullable && actual.Safe {
				matches++
			}
		}
		if matches != 1 {
			return c, ErrStale
		}
		if authoringColumnKind(c) == "" {
			return c, unsupportedPreparation("field_type_unsupported")
		}
		selected[c.SourceName] = true
		return c, nil
	}
	projection, grouping := []string{}, []string{}
	seen := map[string]bool{}
	for i, d := range f.Dimensions {
		if seen[digest(d)] {
			return out, ErrInvalid
		}
		seen[digest(d)] = true
		ref := semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: d.Field}
		label, role := "", "dimension"
		var policy *semantics.TemporalPolicy
		switch d.Kind {
		case "column":
		case "dimension":
			found := 0
			for _, dimension := range p.Definition.Dimensions {
				if dimension.ID != d.Field {
					continue
				}
				found++
				if dimension.Field.Dataset != dataset.ID || dimension.Field.Kind != semantics.KindColumn || len(dimension.Filters) != 0 {
					return out, unsupportedPreparation("reviewed_dimension_unsupported")
				}
				ref, label, policy = dimension.Field, dimension.Name, dimension.Temporal
				if dimension.Role == semantics.DimensionIdentifier {
					role = "identifier"
				}
			}
			if found != 1 {
				return out, ErrInvalid
			}
		default:
			return out, ErrInvalid
		}
		c, err := resolve(ref)
		if err != nil {
			return out, err
		}
		if label == "" {
			label = c.Name
		}
		if slices.Contains([]string{"date", "timestamp", "instant"}, authoringColumnKind(c)) {
			role = "time"
		}
		if f.Mode == "rows" && (d.Grain != "" || policy != nil) {
			return out, unsupportedPreparation("rows_require_physical_values")
		}
		expression, err := authoringGroupExpression(c, d, policy)
		if err != nil {
			return out, err
		}
		alias := "group_" + strconv.Itoa(i+1)
		projection = append(projection, expression+" AS "+pgx.Identifier{alias}.Sanitize())
		grouping = append(grouping, expression)
		provenance := charts.Provenance{Version: 1, Source: binding.Source, SourceRevision: binding.Revision}
		if in.Topic.Topic != "" {
			provenance.Topic, provenance.TopicVersion, provenance.SemanticID = in.Topic.Topic, in.Topic.Version, d.Field
		}
		out.Columns = append(out.Columns, charts.Column{ID: alias, Name: alias, DisplayLabel: label, Role: role, Grain: d.Grain, Format: charts.Format{PreservePrecision: slices.Contains([]string{"integer", "decimal", "number"}, authoringColumnKind(c))}, Provenance: provenance})
	}
	seen = map[string]bool{}
	for i, m := range f.Measures {
		if seen[digest(m)] {
			return out, ErrInvalid
		}
		seen[digest(m)] = true
		label, unit, aggregation, expression, semanticID := "", "", m.Aggregation, "", m.Field
		ref := semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: m.Field}
		switch m.Kind {
		case "count":
			if m.Field != "" || m.Aggregation != "" {
				return out, ErrInvalid
			}
			label, aggregation, expression = "Row count", "count", "count(*)"
		case "column":
		case "measure":
			if m.Aggregation != "" {
				return out, unsupportedPreparation("reviewed_aggregation_locked")
			}
			found := 0
			for _, measure := range p.Definition.Measures {
				if measure.ID != m.Field {
					continue
				}
				found++
				if reason := measureUnsupported(measure, dataset.ID); reason != "" {
					return out, unsupportedPreparation(reason)
				}
				ref, label, unit, aggregation = measure.Field, measure.Name, measure.Unit, string(measure.Aggregation)
			}
			if found != 1 {
				return out, ErrInvalid
			}
		default:
			return out, ErrInvalid
		}
		if expression == "" {
			c, err := resolve(ref)
			if err != nil {
				return out, err
			}
			if !slices.Contains(authoringColumnAggregations(c), aggregation) {
				return out, unsupportedPreparation("aggregation_type_unsupported")
			}
			if label == "" {
				label = c.Name
				if annotated := label + " · " + aggregation; len(annotated) <= 256 {
					label = annotated
				}
			}
			argument := pgx.Identifier{c.SourceName}.Sanitize()
			if aggregation == "distinct_count" {
				argument = "DISTINCT " + argument
			}
			op := map[string]string{"sum": "sum", "average": "avg", "minimum": "min", "maximum": "max", "count": "count", "distinct_count": "count"}[aggregation]
			expression = op + "(" + argument + ")"
		}
		alias := "value_" + strconv.Itoa(i+1)
		projection = append(projection, expression+" AS "+pgx.Identifier{alias}.Sanitize())
		provenance := charts.Provenance{Version: 1, Source: binding.Source, SourceRevision: binding.Revision}
		// count(*) is an explicit source aggregate, not a reviewed semantic field.
		// Inventing a semantic ID here would misrepresent its published meaning.
		if m.Kind != "count" && in.Topic.Topic != "" {
			provenance.Topic, provenance.TopicVersion, provenance.SemanticID = in.Topic.Topic, in.Topic.Version, semanticID
		}
		out.Columns = append(out.Columns, charts.Column{ID: alias, Name: alias, DisplayLabel: label, Role: "measure", Aggregation: aggregation, Format: charts.Format{Unit: unit, PreservePrecision: aggregation != "count" && aggregation != "distinct_count"}, Provenance: provenance})
	}
	out.SQL = "SELECT " + strings.Join(projection, ", ") + " FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize()
	predicates, parameters, err := compileAuthoringFilters(in, p, binding, resolve)
	if err != nil {
		return out, err
	}
	out.Parameters = parameters
	if len(predicates) > 0 {
		out.SQL += " WHERE " + strings.Join(predicates, " AND ")
	}
	if f.Mode == "aggregate" && len(grouping) > 0 {
		out.SQL += " GROUP BY " + strings.Join(grouping, ", ") + " ORDER BY " + strings.Join(grouping, ", ")
	}
	// The validator requires a nonempty restrictive projection even for count(*).
	// Bind one safe advertised column as evidence of the exact relation; its values
	// are not projected, and no column is invented when the dataset has none.
	if len(selected) == 0 {
		for _, c := range dataset.Columns {
			if _, err := resolve(semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: c.ID}); err == nil {
				break
			}
		}
	}
	if len(selected) == 0 {
		return out, unsupportedPreparation("no_supported_column")
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
