package charts

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
)

// PresentationVersion pins a display-only extension independently from mapping
// and build versions. An absent extension preserves historical bytes.
const PresentationVersion = 1

// ErrPresentationNoop means an edit leaves the exact saved presentation unchanged.
var ErrPresentationNoop = errors.New("charts: presentation unchanged")

// PresentationField is a closed set of controls that native renderers consume.
type PresentationField string

const (
	PresentationDisplayLabel   PresentationField = "display_label"
	PresentationFractionDigits PresentationField = "fraction_digits"
)

// ColumnPresentation stores changed fields only, in canonical mapping-column
// order. It never changes a column's identity, semantics or calculation inputs.
type ColumnPresentation struct {
	Version int                     `json:"version" jsonschema:"enum=1"`
	Columns []ColumnDisplayOverride `json:"columns"`
}

// ColumnDisplayOverride targets a canonical bound ID, never a name or position.
// Pointers distinguish omitted/inherited values from explicit zero/empty text.
type ColumnDisplayOverride struct {
	Column         string  `json:"column"`
	DisplayLabel   *string `json:"display_label,omitempty"`
	FractionDigits *int    `json:"fraction_digits,omitempty" jsonschema:"minimum=0,maximum=20"`
}

// ColumnPresentationSet contains presentation only, not a caller-owned Format.
type ColumnPresentationSet struct {
	DisplayLabel   *string `json:"display_label,omitempty"`
	FractionDigits *int    `json:"fraction_digits,omitempty" jsonschema:"minimum=0,maximum=20"`
}

// ColumnPresentationEdit applies set/reset atomically to one existing column.
// A field cannot be set and reset in the same edit.
type ColumnPresentationEdit struct {
	Column string                 `json:"column"`
	Set    *ColumnPresentationSet `json:"set,omitempty"`
	Reset  []PresentationField    `json:"reset,omitempty"`
}

// PresentationPatch is a purpose-specific mutation, with no binding or data fields.
type PresentationPatch struct {
	Version int                      `json:"version" jsonschema:"enum=1"`
	Edits   []ColumnPresentationEdit `json:"edits"`
}

// PresentationCapabilities lists only controls used by this selected output.
// It grants no access and describes no controls for other result-schema columns.
type PresentationCapabilities struct {
	Version int                            `json:"version"`
	Columns []ColumnPresentationCapability `json:"columns"`
}

// ColumnPresentationCapability scopes each control to a real native display role.
// Display labels are table headers, never legend/axis/KPI role captions.
type ColumnPresentationCapability struct {
	Column string              `json:"column"`
	Role   string              `json:"role"`
	Fields []PresentationField `json:"fields"`
}

func clonePresentation(p *ColumnPresentation) *ColumnPresentation {
	if p == nil {
		return nil
	}
	out := *p
	out.Columns = slices.Clone(p.Columns)
	for i := range out.Columns {
		c := &out.Columns[i]
		if c.DisplayLabel != nil {
			v := *c.DisplayLabel
			c.DisplayLabel = &v
		}
		if c.FractionDigits != nil {
			v := *c.FractionDigits
			c.FractionDigits = &v
		}
	}
	return &out
}

func presentationRole(m Mapping, id string) string {
	b := m.Bindings
	// Legacy KPI HTML has no KPIResult consumer. Do not offer an inert control.
	if m.Kind == KPI && m.Version != DisplayVersion {
		return ""
	}
	if m.Kind == Table {
		if !slices.Contains(b.Columns, id) {
			return ""
		}
		if m.Version == DisplayVersion {
			if m.Table == nil {
				return ""
			}
			for _, c := range m.Table.Columns {
				if c.Column == id && c.Visible {
					return "table_column"
				}
			}
			return ""
		}
		return "table_column"
	}
	if b.Value == id {
		return "value"
	}
	if slices.Contains(b.Values, id) {
		return "values"
	}
	if m.Kind == KPI {
		if b.Target == id {
			return "target"
		}
		return ""
	}
	if b.X == id {
		return "x"
	}
	if b.Y == id {
		return "y"
	}
	if b.Category == id {
		return "category"
	}
	return ""
}

func presentationFields(m Mapping, c Column) []PresentationField {
	role := presentationRole(m, c.ID)
	fields := []PresentationField{}
	if role == "table_column" {
		fields = append(fields, PresentationDisplayLabel)
	}
	if role != "" && numeric(c.Type) && c.Format.Percent == "" {
		fields = append(fields, PresentationFractionDigits)
	}
	return fields
}

// PresentationCapabilitiesFor validates only saved metadata; it never obtains
// values, invokes a model or changes the selected mapping.
func PresentationCapabilitiesFor(ctx context.Context, m Mapping, limits Limits) (PresentationCapabilities, error) {
	if err := validatePresentationMapping(ctx, m, limits); err != nil {
		return PresentationCapabilities{}, err
	}
	out := PresentationCapabilities{Version: PresentationVersion, Columns: []ColumnPresentationCapability{}}
	for _, c := range m.Columns {
		if fields := presentationFields(m, c); len(fields) > 0 {
			out.Columns = append(out.Columns, ColumnPresentationCapability{Column: c.ID, Role: presentationRole(m, c.ID), Fields: fields})
		}
	}
	return out, nil
}

func validatePresentationMapping(ctx context.Context, m Mapping, limits Limits) error {
	return ValidateMapping(ctx, Data{Version: Version, Columns: m.Columns, Rows: [][]Cell{}, Completeness: Completeness{Status: "complete_result"}}, m, limits)
}

func validatePresentation(m Mapping, limits Limits) error {
	p := m.Presentation
	if p == nil {
		return nil
	}
	if p.Version != PresentationVersion || len(p.Columns) == 0 {
		return ErrInvalid
	}
	if len(p.Columns) > len(m.Columns) || len(p.Columns) > limits.MaxColumns {
		return ErrLimit
	}
	previous := -1
	for _, o := range p.Columns {
		index := columnIndex(m.Columns, o.Column)
		if index <= previous || o.DisplayLabel == nil && o.FractionDigits == nil {
			return ErrInvalid
		}
		previous = index
		c := m.Columns[index]
		fields := presentationFields(m, c)
		if o.DisplayLabel != nil && (!slices.Contains(fields, PresentationDisplayLabel) || !text(*o.DisplayLabel, 256) || *o.DisplayLabel == c.DisplayLabel) {
			return ErrInvalid
		}
		if o.FractionDigits != nil && (!slices.Contains(fields, PresentationFractionDigits) || *o.FractionDigits < 0 || *o.FractionDigits > 20 || !c.Format.PreservePrecision && *o.FractionDigits == c.Format.FractionDigits) {
			return ErrInvalid
		}
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return ErrInvalid
	}
	if len(encoded) > limits.MaxBytes {
		return ErrLimit
	}
	return nil
}

// ProjectPresentationColumns derives exact renderer metadata from the saved
// mapping, including table visibility. Frozen readers can require full equality
// with this result; semantic-field exceptions are neither needed nor permitted.
func ProjectPresentationColumns(ctx context.Context, m Mapping, limits Limits) ([]Column, error) {
	if err := validatePresentationMapping(ctx, m, limits); err != nil {
		return nil, err
	}
	return projectPresentationColumns(m), ctx.Err()
}

func projectPresentationColumns(m Mapping) []Column {
	columns := make([]Column, 0, len(m.Columns))
	for _, c := range m.Columns {
		if m.Kind == Table && m.Version == DisplayVersion && presentationRole(m, c.ID) == "" {
			continue
		}
		if m.Presentation != nil {
			for _, o := range m.Presentation.Columns {
				if o.Column != c.ID {
					continue
				}
				if o.DisplayLabel != nil {
					c.DisplayLabel = *o.DisplayLabel
				}
				if o.FractionDigits != nil {
					c.Format.FractionDigits = *o.FractionDigits
					c.Format.PreservePrecision = false
				}
				break
			}
		}
		columns = append(columns, c)
	}
	return columns
}

func validatePresentationEdit(edit ColumnPresentationEdit) error {
	if !identifier(edit.Column) || edit.Set == nil && len(edit.Reset) == 0 {
		return ErrInvalid
	}
	if edit.Set != nil && edit.Set.DisplayLabel == nil && edit.Set.FractionDigits == nil {
		return ErrInvalid
	}
	if len(edit.Reset) > 2 || edit.Reset != nil && len(edit.Reset) == 0 {
		return ErrInvalid
	}
	seen := map[PresentationField]bool{}
	for _, f := range edit.Reset {
		if f != PresentationDisplayLabel && f != PresentationFractionDigits || seen[f] {
			return ErrInvalid
		}
		seen[f] = true
		if edit.Set != nil && (f == PresentationDisplayLabel && edit.Set.DisplayLabel != nil || f == PresentationFractionDigits && edit.Set.FractionDigits != nil) {
			return ErrInvalid
		}
	}
	return nil
}

// ApplyPresentationPatch returns a fully detached mapping with only its optional
// presentation extension changed. Resetting the last field removes the extension.
// Canonical-equivalent sets normalize to inheritance; exact no-ops are rejected.
func ApplyPresentationPatch(ctx context.Context, m Mapping, patch PresentationPatch, limits Limits) (Mapping, error) {
	if err := validatePresentationMapping(ctx, m, limits); err != nil {
		return Mapping{}, err
	}
	if patch.Version != PresentationVersion || len(patch.Edits) == 0 {
		return Mapping{}, ErrInvalid
	}
	if len(patch.Edits) > len(m.Columns) || len(patch.Edits) > limits.MaxColumns {
		return Mapping{}, ErrLimit
	}
	out := cloneMapping(m)
	// Unlike a build's historical clone, a purpose-specific edit must preserve
	// even empty omitempty binding slices in the caller's saved definition.
	out.Bindings.Columns = slices.Clone(m.Bindings.Columns)
	out.Bindings.Values = slices.Clone(m.Bindings.Values)
	out.Bindings.Hierarchy = slices.Clone(m.Bindings.Hierarchy)
	current := map[string]ColumnDisplayOverride{}
	if out.Presentation != nil {
		for _, o := range out.Presentation.Columns {
			current[o.Column] = o
		}
	}
	seen := map[string]bool{}
	for _, edit := range patch.Edits {
		if err := ctx.Err(); err != nil {
			return Mapping{}, err
		}
		if err := validatePresentationEdit(edit); err != nil {
			return Mapping{}, err
		}
		index := columnIndex(m.Columns, edit.Column)
		if index < 0 || seen[edit.Column] {
			return Mapping{}, ErrInvalid
		}
		seen[edit.Column] = true
		c := m.Columns[index]
		fields := presentationFields(m, c)
		o := current[edit.Column]
		o.Column = edit.Column
		for _, f := range edit.Reset {
			if !slices.Contains(fields, f) {
				return Mapping{}, ErrInvalid
			}
			if f == PresentationDisplayLabel {
				o.DisplayLabel = nil
			} else {
				o.FractionDigits = nil
			}
		}
		if edit.Set != nil {
			if v := edit.Set.DisplayLabel; v != nil {
				if !slices.Contains(fields, PresentationDisplayLabel) || !text(*v, 256) {
					return Mapping{}, ErrInvalid
				}
				value := *v
				o.DisplayLabel = &value
				if value == c.DisplayLabel {
					o.DisplayLabel = nil
				}
			}
			if v := edit.Set.FractionDigits; v != nil {
				if !slices.Contains(fields, PresentationFractionDigits) || *v < 0 || *v > 20 {
					return Mapping{}, ErrInvalid
				}
				value := *v
				o.FractionDigits = &value
				if value == c.Format.FractionDigits && !c.Format.PreservePrecision {
					o.FractionDigits = nil
				}
			}
		}
		if o.DisplayLabel == nil && o.FractionDigits == nil {
			delete(current, edit.Column)
		} else {
			current[edit.Column] = o
		}
	}
	out.Presentation = nil
	if len(current) > 0 {
		out.Presentation = &ColumnPresentation{Version: PresentationVersion, Columns: make([]ColumnDisplayOverride, 0, len(current))}
		for _, c := range m.Columns {
			if o, ok := current[c.ID]; ok {
				out.Presentation.Columns = append(out.Presentation.Columns, o)
			}
		}
	}
	if err := validatePresentation(out, limits); err != nil {
		return Mapping{}, err
	}
	if reflect.DeepEqual(out.Presentation, m.Presentation) {
		return Mapping{}, ErrPresentationNoop
	}
	return out, ctx.Err()
}
