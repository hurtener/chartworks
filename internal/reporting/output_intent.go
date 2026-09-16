package reporting

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

// ErrNarrativePolicy rejects unsupported narrative-policy mappings explicitly.
var ErrNarrativePolicy = fmt.Errorf("%w: narrative policy requires explicit supported mapping", ErrInvalid)

func narrativeLocale(value string) bool {
	return value == "en" || value == "es" || strings.HasPrefix(value, "en-") || strings.HasPrefix(value, "es-")
}

// OutputMetadata is inert localized display text, never data or SQL authority.
type OutputMetadata struct {
	Locale      string `json:"locale"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// OutputIntent is authored on an output, independently of block metadata. Order
// is explicit and unique within a definition; execution request order is separate.
type OutputIntent struct {
	Metadata        []OutputMetadata `json:"metadata"`
	Enabled         bool             `json:"enabled"`
	DefaultSelected bool             `json:"default_selected"`
	DisplayOrder    int              `json:"display_order" jsonschema:"minimum=0,maximum=63"`
}

// OutputSelectionError is a content-free typed rejection. It deliberately does
// not echo unknown/private output identifiers or imply that a label grants reach.
type OutputSelectionError struct{ Code string }

func (e *OutputSelectionError) Error() string { return "reporting: " + e.Code }
func (e *OutputSelectionError) Unwrap() error {
	if e.Code == "output_not_selected" {
		return ErrIncomplete
	}
	return ErrInvalid
}

func selectionError(code string) error { return &OutputSelectionError{Code: code} }

// SelectionErrorCode returns the registered rejection code, or empty for other errors.
func SelectionErrorCode(err error) string {
	var selected *OutputSelectionError
	if errors.As(err, &selected) {
		return selected.Code
	}
	return ""
}

// OutputChoice is a metadata-only, revision-bound selector. Omitted and disabled
// choices remain visible only behind the same signed/private read eligibility.
type OutputChoice struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	Intent   OutputIntent `json:"intent"`
	Selected bool         `json:"selected"`
	State    string       `json:"state" jsonschema:"enum=selected,enum=omitted,enum=disabled"`
	Code     string       `json:"code,omitempty"`
}

// OutputSelection records the accepted interpretation of nil versus [] and the
// exact execution sequence. Choices are display-ordered, not execution-ordered.
type OutputSelection struct {
	DefinitionVersion int            `json:"definition_version" jsonschema:"enum=1,enum=2"`
	Version           int            `json:"version" jsonschema:"enum=2"`
	Mode              string         `json:"mode" jsonschema:"enum=legacy_all,enum=defaults,enum=explicit"`
	Requested         []string       `json:"requested"`
	Selected          []string       `json:"selected"`
	Choices           []OutputChoice `json:"choices"`
}

func legacyIntent(d Definition, o Output, position int) OutputIntent {
	out := OutputIntent{Enabled: true, DefaultSelected: true, DisplayOrder: position, Metadata: []OutputMetadata{}}
	for _, m := range d.Metadata {
		// Stable kind labels disambiguate legacy outputs without inventing new IDs.
		kind := map[string]string{"table": "Table", "chart": "Chart", "kpi": "Indicator", "narrative": "Narrative"}[o.Kind]
		if m.Locale == "es" || strings.HasPrefix(m.Locale, "es-") {
			kind = map[string]string{"table": "Tabla", "chart": "Gráfico", "kpi": "Indicador", "narrative": "Narrativa"}[o.Kind]
		}
		if kind == "" {
			kind = o.Kind
		}
		title := kind + " · " + o.ID
		if m.Title != "" {
			title = m.Title + " — " + title
		}
		// Legacy titles may already use the full field bound. Preserve their exact
		// text as description rather than silently truncating a localized name.
		if len(title) > 256 {
			title = kind + " · " + o.ID
		}
		out.Metadata = append(out.Metadata, OutputMetadata{Locale: m.Locale, DisplayName: title, Description: m.Description})
	}
	if len(out.Metadata) == 0 {
		out.Metadata = []OutputMetadata{{Locale: "en", DisplayName: o.ID}}
	}
	return out
}

func intentValid(in OutputIntent, limits config.Reporting) bool {
	if in.DisplayOrder < 0 || in.DisplayOrder > 63 || len(in.Metadata) == 0 || len(in.Metadata) > limits.MaxLocales {
		return false
	}
	seen := map[string]bool{}
	for _, m := range in.Metadata {
		if !locale(m.Locale) || seen[m.Locale] || strings.TrimSpace(m.DisplayName) == "" || !text(m.DisplayName, 256) || !text(m.Description, 4096) {
			return false
		}
		seen[m.Locale] = true
	}
	return true
}

// LocalizedOutput follows exact tag, language, then authored first-locale order.
// It is pure and deterministic; no model, environment locale or map ordering.
func LocalizedOutput(in OutputIntent, requested string) OutputMetadata {
	for _, m := range in.Metadata {
		if m.Locale == requested {
			return m
		}
	}
	base := strings.Split(requested, "-")[0]
	for _, m := range in.Metadata {
		if strings.Split(m.Locale, "-")[0] == base {
			return m
		}
	}
	if len(in.Metadata) > 0 {
		return in.Metadata[0]
	}
	return OutputMetadata{}
}

// MigrateDefinition returns a detached v2 authoring draft with documented v1
// defaults. Persist it only through ordinary CAS edit/validation/publication.
// No publication, attestation, source authority, or historical hash is changed.
func MigrateDefinition(d Definition) (Definition, error) {
	if d.SchemaVersion != SchemaVersion && d.SchemaVersion != CurrentSchemaVersion {
		return Definition{}, ErrInvalid
	}
	d = clone(d)
	if d.SchemaVersion == CurrentSchemaVersion {
		// Migration is not repair: absent v2 intent or policy ceilings must not
		// be filled using v1 defaults and silently change authored selection.
		for _, output := range d.Outputs {
			if output.Intent == nil {
				return Definition{}, ErrInvalid
			}
			if output.Narrative != nil && boundedNarrativePolicy(*output.Narrative) != nil {
				return Definition{}, ErrNarrativePolicy
			}
		}
		return d, nil
	}
	for i := range d.Outputs {
		if d.Outputs[i].Intent == nil {
			v := legacyIntent(d, d.Outputs[i], i)
			d.Outputs[i].Intent = &v
		}
		if n := d.Outputs[i].Narrative; n != nil {
			if n.SchemaVersion != "grounded-narrative-v1" || !narrativeLocale(n.Locale) || n.PolicyVersion != "" && n.PolicyVersion != NarrativePolicyVersion {
				return Definition{}, ErrNarrativePolicy
			}
			switch n.Instructions {
			case "evidence_only", "Summarize only the returned evidence", "Describe the displayed evidence without inventing values.":
				n.Instructions = "evidence_only"
			default:
				return Definition{}, ErrNarrativePolicy
			}
			if n.MaxClaims == 0 {
				n.MaxClaims = 32
			}
			n.PolicyVersion = NarrativePolicyVersion
			if err := boundedNarrativePolicy(*n); err != nil {
				return Definition{}, err
			}
		}
	}
	d.SchemaVersion = CurrentSchemaVersion
	return d, nil
}

// OutputDefinitions projects legacy intent without changing stored definitions.
func OutputDefinitions(d Definition) []Output {
	out := clone(d.Outputs)
	for i := range out {
		if out[i].Intent == nil {
			in := legacyIntent(d, out[i], i)
			out[i].Intent = &in
		}
	}
	return out
}

// ResolveOutputSelection preserves v1 empty-means-all and explicit execution
// order. V2 nil means defaults; [] is rejected, never an implicit query. Defaults
// skip disabled outputs; explicitly requesting a disabled output rejects the run.
func ResolveOutputSelection(d Definition, requested []string) ([]Output, OutputSelection, error) {
	var snapshot OutputSelection
	if d.SchemaVersion != SchemaVersion && d.SchemaVersion != CurrentSchemaVersion || len(d.Outputs) == 0 || len(d.Outputs) > 64 || len(requested) > 64 {
		return nil, snapshot, ErrInvalid
	}
	if d.SchemaVersion == CurrentSchemaVersion {
		for _, output := range d.Outputs {
			if output.Intent == nil {
				return nil, snapshot, ErrInvalid
			}
		}
	}
	all := OutputDefinitions(d)
	available := map[string]Output{}
	order := map[int]bool{}
	for _, o := range all {
		if !identity.Identifier(o.ID) {
			return nil, snapshot, ErrInvalid
		}
		if _, exists := available[o.ID]; exists {
			return nil, snapshot, selectionError("output_duplicate")
		}
		if o.Intent == nil || order[o.Intent.DisplayOrder] {
			return nil, snapshot, ErrInvalid
		}
		order[o.Intent.DisplayOrder] = true
		available[o.ID] = o
	}
	snapshot = OutputSelection{DefinitionVersion: d.SchemaVersion, Version: 2, Mode: "explicit", Requested: clone(requested), Selected: []string{}, Choices: []OutputChoice{}}
	if len(requested) == 0 {
		if d.SchemaVersion == SchemaVersion {
			snapshot.Mode = "legacy_all"
		} else {
			if requested != nil {
				return nil, OutputSelection{}, selectionError("output_selection_empty")
			}
			snapshot.Mode = "defaults"
		}
	}
	selected := map[string]bool{}
	out := []Output{}
	if snapshot.Mode == "explicit" {
		for _, id := range requested {
			if selected[id] {
				return nil, OutputSelection{}, selectionError("output_duplicate")
			}
			o, ok := available[id]
			if !ok {
				return nil, OutputSelection{}, selectionError("output_unknown")
			}
			if !o.Intent.Enabled {
				return nil, OutputSelection{}, selectionError("output_disabled")
			}
			selected[id] = true
			out = append(out, o)
		}
	} else {
		if d.SchemaVersion == CurrentSchemaVersion {
			slices.SortStableFunc(all, func(a, b Output) int { return a.Intent.DisplayOrder - b.Intent.DisplayOrder })
		}
		for _, o := range all {
			if o.Intent.Enabled && (snapshot.Mode == "legacy_all" || o.Intent.DefaultSelected) {
				selected[o.ID] = true
				out = append(out, o)
			}
		}
	}
	for _, o := range out {
		snapshot.Selected = append(snapshot.Selected, o.ID)
	}
	for _, o := range all {
		choice := OutputChoice{ID: o.ID, Kind: o.Kind, Intent: clone(*o.Intent), Selected: selected[o.ID], State: "omitted", Code: "not_selected"}
		if !o.Intent.Enabled {
			choice.State = "disabled"
			choice.Code = "output_disabled"
		} else if selected[o.ID] {
			choice.State = "selected"
			choice.Code = ""
		} else if snapshot.Mode == "defaults" {
			choice.Code = "not_default"
		}
		snapshot.Choices = append(snapshot.Choices, choice)
	}
	slices.SortStableFunc(snapshot.Choices, func(a, b OutputChoice) int { return a.Intent.DisplayOrder - b.Intent.DisplayOrder })
	if len(out) == 0 {
		return nil, snapshot, selectionError("output_selection_empty")
	}
	return out, snapshot, nil
}
