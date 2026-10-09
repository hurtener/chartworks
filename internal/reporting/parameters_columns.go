package reporting

import (
	"slices"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// ColumnReference identifies a physical column in one exact registered schema.
// It is not a reviewed dimension or an authority grant. Type is checked against
// that schema on every definition resolution. Calendar/Timezone govern temporal
// input only; report/session timezone can never silently change its meaning.
type ColumnReference struct {
	SourceDataset SourceDatasetPin `json:"source_dataset"`
	Name          string           `json:"name"`
	Type          string           `json:"type" jsonschema:"enum=text,enum=identifier,enum=integer,enum=number,enum=boolean,enum=date,enum=timestamp,enum=instant"`
	Calendar      string           `json:"calendar,omitempty"`
	Timezone      string           `json:"timezone,omitempty"`
}

// ScalarRange has inclusive lower and exclusive upper bounds, both exact text.
// Dates use YYYY-MM-DD. Timestamps use civil ISO seconds without an offset;
// instants resolve those civil values in the reference's explicit IANA zone.
// Ambiguous/nonexistent civil times are rejected, never guessed.
type ScalarRange struct {
	Start        string `json:"start"`
	EndExclusive string `json:"end_exclusive"`
}

func columnFilterParameter(p Parameter) bool {
	return slices.Contains([]string{"column_value", "column_set", "column_range"}, p.Type)
}

func columnFilterType(c semantics.Column) string {
	switch c.NativeType {
	case "int2", "int4", "int8", "smallint", "integer", "bigint":
		return "integer"
	}
	return authoringColumnKind(c)
}

func (c ColumnReference) valid() bool {
	if !c.SourceDataset.valid() || c.Name == "" || !text(c.Name, 256) || !slices.Contains([]string{"text", "identifier", "integer", "number", "boolean", "date", "timestamp", "instant"}, c.Type) {
		return false
	}
	if slices.Contains([]string{"date", "timestamp", "instant"}, c.Type) {
		if c.Calendar != "gregorian" {
			return false
		}
		if c.Type == "instant" {
			_, err := namedZone(c.Timezone)
			return err == nil
		}
		return c.Timezone == ""
	}
	return c.Calendar == "" && c.Timezone == ""
}

func validateColumnFilterDeclaration(p Parameter) error {
	if !columnFilterParameter(p) || p.Column == nil || !p.Column.valid() || p.Dimension != nil || !p.Required || p.Default == nil || p.ListLength != 0 || p.Min != "" || p.Max != "" || len(p.Enum) != 0 {
		return ErrInvalid
	}
	temporal := slices.Contains([]string{"date", "timestamp", "instant"}, p.Column.Type)
	if p.Type == "column_range" && !temporal && p.Column.Type != "number" && p.Column.Type != "integer" || p.Type != "column_range" && temporal {
		return ErrInvalid
	}
	_, err := resolveColumnFilter(p, *p.Default)
	return err
}

func columnFilterScalar(c ColumnReference, value string) (exec.Parameter, error) {
	kind := c.Type
	switch kind {
	case "text":
		kind = "dimension_value" // shared scalar codec, no semantic reference
	case "identifier":
		// PostgreSQL uuid canonical form only. Bind as text with inferred source type.
		if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || len(strings.ReplaceAll(value, "-", "")) != 32 || strings.Trim(strings.ReplaceAll(value, "-", ""), "0123456789abcdef") != "" {
			return exec.Parameter{}, ErrInvalid
		}
		kind = "dimension_value"
	case "timestamp", "instant":
		wall, err := time.Parse("2006-01-02T15:04:05.999999", value)
		if err != nil || wall.Year() < 1 || wall.Year() > 9999 || wall.Format("2006-01-02T15:04:05.999999") != value {
			return exec.Parameter{}, ErrInvalid
		}
		if kind == "timestamp" {
			return exec.Parameter{Kind: "text", Value: value}, nil
		}
		zone, err := namedZone(c.Timezone)
		if err != nil {
			return exec.Parameter{}, err
		}
		instant, err := civil(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), zone, "reject")
		if err != nil {
			return exec.Parameter{}, err
		}
		return exec.Parameter{Kind: "text", Value: instant.UTC().Format(time.RFC3339Nano)}, nil
	}
	return scalar(Parameter{Type: kind}, value)
}

func resolveColumnFilter(p Parameter, value Value) ([]exec.Parameter, error) {
	if p.Column == nil || !p.Column.valid() || value.Period != nil || value.DateRange != nil {
		return nil, ErrInvalid
	}
	switch p.Type {
	case "column_value":
		if value.Range != nil || len(value.Items) != 0 {
			return nil, ErrInvalid
		}
		bound, err := columnFilterScalar(*p.Column, value.Literal)
		return []exec.Parameter{bound}, err
	case "column_set":
		if value.Range != nil || value.Literal != "" || len(value.Items) < 1 || len(value.Items) > DimensionSetCapacity {
			return nil, ErrInvalid
		}
		items := slices.Clone(value.Items)
		slices.Sort(items)
		out := make([]exec.Parameter, DimensionSetCapacity)
		for i := range out {
			out[i].Kind = "null"
		}
		for i, item := range items {
			if i > 0 && item == items[i-1] {
				return nil, ErrInvalid
			}
			var err error
			out[i], err = columnFilterScalar(*p.Column, item)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	case "column_range":
		if value.Range == nil || value.Literal != "" || len(value.Items) != 0 {
			return nil, ErrInvalid
		}
		a, err := columnFilterScalar(*p.Column, value.Range.Start)
		b, endErr := columnFilterScalar(*p.Column, value.Range.EndExclusive)
		if err != nil || endErr != nil {
			return nil, ErrInvalid
		}
		kind := p.Column.Type
		if kind == "instant" {
			kind = "datetime"
		}
		var comparison int
		if kind == "timestamp" {
			// Canonical civil ISO timestamps compare chronologically as text.
			comparison = strings.Compare(a.Value, b.Value)
		} else {
			comparison, err = compareScalar(kind, a.Value, b.Value)
		}
		if err != nil || comparison >= 0 {
			return nil, ErrInvalid
		}
		return []exec.Parameter{a, b}, nil
	}
	return nil, ErrInvalid
}

// Resolve the persisted physical reference and verify it is in the definition's
// exact source origin or one of its reviewed datasets. SQL identity is checked
// separately by the common positive-predicate proof.
func resolveFilterColumn(d Definition, p Parameter, binding exec.Binding, publications []topics.Published) (semantics.Column, error) {
	if validateColumnFilterDeclaration(p) != nil || d.Source != binding.Source || d.Context != binding.Context {
		return semantics.Column{}, ErrInvalid
	}
	ref := p.Column
	relation, err := sourceDatasetRelation(ref.SourceDataset, binding)
	if err != nil {
		return semantics.Column{}, err
	}
	var column semantics.Column
	found := 0
	for _, actual := range relation.Columns {
		if actual.Name == ref.Name && actual.Safe {
			column = semantics.Column{SourceName: actual.Name, NativeType: actual.NativeType, Category: actual.Category, Nullable: actual.Nullable}
			found++
		}
	}
	if found != 1 || columnFilterType(column) != ref.Type {
		return semantics.Column{}, ErrStale
	}
	if d.SourceDataset != nil {
		if *d.SourceDataset != ref.SourceDataset {
			return semantics.Column{}, ErrInvalid
		}
		return column, nil
	}
	for _, publication := range publications {
		for _, dataset := range publication.Definition.Datasets {
			if dataset.ID != relation.ID || dataset.Source.Source != binding.Source || dataset.Source.Context != binding.Context || dataset.Source.SourceRevision != binding.Revision {
				continue
			}
			for _, reviewed := range dataset.Columns {
				if reviewed.SourceName == column.SourceName && reviewed.NativeType == column.NativeType && reviewed.Category == column.Category && reviewed.Nullable == column.Nullable {
					return column, nil
				}
			}
		}
	}
	return semantics.Column{}, ErrInvalid
}
