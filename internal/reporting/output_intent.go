package reporting

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

// OutputLocalized is plain text attached to one stable output, not its block.
type OutputLocalized struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// OutputIntent is required in definition v2. Display order is explicit and
// independent from the order in which the output objects were serialized.
type OutputIntent struct {
	Metadata        []OutputLocalized `json:"metadata"`
	Enabled         bool              `json:"enabled"`
	DefaultSelected bool              `json:"default_selected"`
	DisplayOrder    int               `json:"display_order"`
}

// OutputDescriptor conveys metadata only, never authority to values or SQL.
type OutputDescriptor struct {
	ID     string       `json:"id"`
	Kind   string       `json:"kind"`
	Intent OutputIntent `json:"intent"`
}

// OutputSelection preserves selected, omitted and disabled choices separately
// from the execution status of a selected output.
type OutputSelection struct {
	OutputDescriptor
	State string `json:"state" jsonschema:"enum=selected,enum=omitted,enum=disabled"`
}

// SelectionError is a content-free typed failure for an invalid explicit choice.
type SelectionError struct{ Code string }

func (e *SelectionError) Error() string { return fmt.Sprintf("reporting: output selection %s", e.Code) }
func (e *SelectionError) Unwrap() error { return ErrInvalid }
func selectionError(code string) error  { return &SelectionError{Code: code} }

// DefinitionCanonicalVersion leaves legacy hashes and published bytes unchanged.
func DefinitionCanonicalVersion(d Definition) string {
	if d.SchemaVersion == CurrentSchemaVersion {
		return "block-definition-v2"
	}
	if d.SchemaVersion == SchemaVersion {
		return CanonicalizationVersion
	}
	return ""
}

func intentValid(in OutputIntent, maxLocales int) bool {
	if in.DisplayOrder < 0 || in.DisplayOrder > 1000000 || len(in.Metadata) == 0 || len(in.Metadata) > maxLocales {
		return false
	}
	seen := map[string]bool{}
	for _, m := range in.Metadata {
		if !locale(m.Locale) || seen[m.Locale] || strings.TrimSpace(m.Name) == "" || !text(m.Name, 256) || !text(m.Description, 4096) {
			return false
		}
		seen[m.Locale] = true
	}
	return true
}

func validateOutputPolicies(d Definition, limits config.Reporting) error {
	if d.QueryLimits != nil && (!d.QueryLimits.Valid() || d.SchemaVersion != CurrentSchemaVersion) {
		return ErrInvalid
	}
	orders := map[int]bool{}
	for _, field := range d.ExpectedSchema {
		if field.Sensitivity != "" && field.Sensitivity != semantics.LiteralSensitive && field.Sensitivity != semantics.LiteralNonSensitive {
			return ErrInvalid
		}
		if d.SchemaVersion == SchemaVersion && field.Sensitivity != "" {
			return ErrInvalid
		}
	}
	for _, output := range d.Outputs {
		if d.SchemaVersion == SchemaVersion {
			if output.Intent != nil || output.Narrative != nil && output.Narrative.MaxClaims != 0 {
				return ErrInvalid
			}
			continue
		}
		if output.Intent == nil || !intentValid(*output.Intent, limits.MaxLocales) || orders[output.Intent.DisplayOrder] {
			return ErrInvalid
		}
		orders[output.Intent.DisplayOrder] = true
		if n := output.Narrative; n != nil {
			if n.Instructions != "" || n.MaxClaims < 1 || n.MaxClaims > 32 || !supportedNarrativeLocale(n.Locale) || n.SchemaVersion != "grounded-narrative-v1" {
				return ErrInvalid
			}
		}
	}
	return nil
}

func selectOutputIntent(all []Output, selected []string) ([]Output, error) {
	if len(all) == 0 || len(all) > 64 || len(selected) > 64 {
		return nil, selectionError("invalid")
	}
	available := map[string]Output{}
	versioned := all[0].Intent != nil
	for _, output := range all {
		if !identity.Identifier(output.ID) {
			return nil, selectionError("invalid")
		}
		if _, exists := available[output.ID]; exists {
			return nil, selectionError("duplicate_definition")
		}
		if (output.Intent != nil) != versioned {
			return nil, selectionError("mixed_version")
		}
		available[output.ID] = output
	}
	if versioned && selected != nil && len(selected) == 0 {
		return nil, selectionError("empty")
	}
	wanted := map[string]bool{}
	for _, id := range selected {
		if wanted[id] {
			return nil, selectionError("duplicate")
		}
		output, exists := available[id]
		if !exists {
			return nil, selectionError("unknown")
		}
		if output.Intent != nil && !output.Intent.Enabled {
			return nil, selectionError("disabled")
		}
		wanted[id] = true
	}
	out := []Output{}
	for _, output := range all {
		include := wanted[output.ID]
		if len(selected) == 0 {
			include = !versioned || output.Intent.Enabled && output.Intent.DefaultSelected
		}
		if include {
			out = append(out, clone(output))
		}
	}
	if len(out) == 0 {
		return nil, selectionError("no_defaults")
	}
	if versioned {
		slices.SortFunc(out, func(a, b Output) int {
			if a.Intent.DisplayOrder < b.Intent.DisplayOrder {
				return -1
			}
			if a.Intent.DisplayOrder > b.Intent.DisplayOrder {
				return 1
			}
			return strings.Compare(a.ID, b.ID)
		})
	}
	return out, nil
}

// effectiveOutputs projects deterministic defaults; it never edits stored bytes.
func effectiveOutputs(d Definition) []Output {
	out := clone(d.Outputs)
	for i := range out {
		if out[i].Intent != nil {
			continue
		}
		metadata := []OutputLocalized{}
		for _, m := range d.Metadata {
			metadata = append(metadata, OutputLocalized{Locale: m.Locale, Name: out[i].ID})
		}
		if len(metadata) == 0 {
			metadata = append(metadata, OutputLocalized{Locale: "en", Name: out[i].ID})
		}
		out[i].Intent = &OutputIntent{Metadata: metadata, Enabled: true, DefaultSelected: true, DisplayOrder: i}
	}
	return out
}
func outputDescriptors(d Definition) []OutputDescriptor {
	out := make([]OutputDescriptor, 0, len(d.Outputs))
	for _, o := range effectiveOutputs(d) {
		out = append(out, OutputDescriptor{ID: o.ID, Kind: o.Kind, Intent: *o.Intent})
	}
	slices.SortFunc(out, func(a, b OutputDescriptor) int {
		if a.Intent.DisplayOrder < b.Intent.DisplayOrder {
			return -1
		}
		if a.Intent.DisplayOrder > b.Intent.DisplayOrder {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
func selectionSnapshot(d Definition, selected []Output) []OutputSelection {
	wanted := map[string]bool{}
	for _, o := range selected {
		wanted[o.ID] = true
	}
	out := []OutputSelection{}
	for _, o := range outputDescriptors(d) {
		state := "omitted"
		if !o.Intent.Enabled {
			state = "disabled"
		} else if wanted[o.ID] {
			state = "selected"
		}
		out = append(out, OutputSelection{OutputDescriptor: o, State: state})
	}
	return out
}
func selectionMode(d Definition, selected []string) string {
	if d.SchemaVersion == SchemaVersion && len(selected) == 0 {
		return "legacy_all"
	}
	if selected == nil {
		return "default"
	}
	return "explicit"
}
func attachOutputIntent(d Definition, selected []Output) []Output {
	all := map[string]*OutputIntent{}
	for _, o := range effectiveOutputs(d) {
		all[o.ID] = o.Intent
	}
	out := clone(selected)
	for i := range out {
		out[i].Intent = clone(all[out[i].ID])
	}
	return out
}

// LocalizedOutput selects exact locale, language, then first authored locale.
// The fallback is presentation only and never changes the stable identifier.
func LocalizedOutput(in OutputIntent, requested string) OutputLocalized {
	for _, m := range in.Metadata {
		if m.Locale == requested {
			return m
		}
	}
	language := strings.Split(requested, "-")[0]
	for _, m := range in.Metadata {
		if strings.Split(m.Locale, "-")[0] == language {
			return m
		}
	}
	if len(in.Metadata) > 0 {
		return in.Metadata[0]
	}
	return OutputLocalized{}
}

const CurrentSchemaVersion = 2
