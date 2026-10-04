package reporting

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// These are retained semantic/type dispositions, not grants or source reads.
// Empty Kinds plus Reason keeps unsupported reviewed dimensions visible.
type AuthoringFilterCapability struct {
	Dimension       string   `json:"dimension"`
	Kinds           []string `json:"kinds"`
	Supported       bool     `json:"supported"`
	Reason          string   `json:"reason,omitempty"`
	DefaultRequired bool     `json:"default_required"`
	MaxSetSize      int      `json:"max_set_size,omitempty"`
	OptionLookup    bool     `json:"option_lookup"`
	DateBounds      string   `json:"date_bounds,omitempty"`
}

func authoringFilterCapabilities(p topics.Published, dataset topics.Dataset, binding exec.Binding, disposition string, optionsAvailable bool) []AuthoringFilterCapability {
	out := []AuthoringFilterCapability{}
	for _, dimension := range p.Definition.Dimensions {
		capability := AuthoringFilterCapability{Dimension: dimension.ID, Kinds: []string{}, DefaultRequired: true, Reason: disposition}
		if capability.Reason == "" {
			capability.Reason = dimensionUnsupported(dimension, dataset.ID)
		}
		if capability.Reason == "" {
			var column *semantics.Column
			for i := range dataset.Columns {
				if dataset.Columns[i].ID == dimension.Field.ID {
					column = &dataset.Columns[i]
				}
			}
			if column == nil {
				capability.Reason = "filter_type_unsupported"
			} else {
				_, physical, found := filterColumn(binding, dataset.ID, column.SourceName)
				if !found || physical.NativeType != column.NativeType || physical.Category != column.Category || physical.Nullable != column.Nullable {
					capability.Reason = "source_binding_changed"
				} else if authoringTextColumn(*column) {
					capability.Kinds = []string{"select", "multi_select"}
					capability.MaxSetSize = DimensionSetCapacity
					capability.OptionLookup = optionsAvailable
				} else if authoringDateColumn(*column) && dimension.Role == semantics.DimensionTemporal {
					capability.Kinds = []string{"date_range"}
					capability.DateBounds = "start_inclusive_end_exclusive_date_only"
				} else {
					capability.Reason = "filter_type_unsupported"
				}
			}
		}
		capability.Supported = capability.Reason == ""
		out = append(out, capability)
	}
	return out
}
