package reporting

import (
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestAuthoringFilterMetadata(t *testing.T) {
	_, p, d, b := preparationCompileFixture()
	d.Columns = append(d.Columns, semantics.Column{ID: "day", SourceName: "day", NativeType: "date", Category: "temporal"}, semantics.Column{ID: "timestamp", SourceName: "timestamp", NativeType: "timestamptz", Category: "temporal"})
	p.Definition.Datasets[0] = d
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "day", Role: semantics.DimensionTemporal, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "day"}}, semantics.Dimension{ID: "timestamp", Role: semantics.DimensionTemporal, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "timestamp"}})
	b.Relations[0].Columns = append(b.Relations[0].Columns, exec.Column{Name: "day", NativeType: "date", Category: "temporal", Safe: true}, exec.Column{Name: "timestamp", NativeType: "timestamptz", Category: "temporal", Safe: true})
	out := authoringFilterCapabilities(p, d, b, "", true)
	if len(out) != 3 || !out[0].Supported || !reflect.DeepEqual(out[0].Kinds, []string{"select", "multi_select"}) || out[0].MaxSetSize != 16 || !out[0].OptionLookup || !out[1].Supported || !reflect.DeepEqual(out[1].Kinds, []string{"date_range"}) || out[1].OptionLookup || out[1].DateBounds != "start_inclusive_end_exclusive_date_only" || out[2].Supported || out[2].Reason != "filter_type_unsupported" {
		t.Fatal(out)
	}
	for _, c := range authoringFilterCapabilities(p, d, b, "reviewed_rules_unsupported", true) {
		if c.Supported || len(c.Kinds) > 0 || c.OptionLookup || c.Reason != "reviewed_rules_unsupported" {
			t.Fatal(c)
		}
	}
	p.Definition.Dimensions[0].Temporal = &semantics.TemporalPolicy{Calendar: "gregorian"}
	if c := authoringFilterCapabilities(p, d, b, "", true)[0]; c.Supported || c.Reason != "temporal_policy_unsupported" {
		t.Fatal(c)
	}
	s, _, boundary, e, in := preparationServiceFixture(t)
	view, err := s.Dataset(t.Context(), e, AuthoringDatasetRequest{Topic: in.Intent.Topic, Dataset: in.Intent.Dataset})
	if err != nil || view.Compiler != AuthoringCompilerVersion || view.FilterCompiler != AuthoringFilteredCompilerVersion || len(view.FilterCapabilities) != 1 || boundary.physical.Load() != 0 || boundary.planning.Load() != 0 {
		t.Fatal("metadata executed source work", view, err)
	}
}
