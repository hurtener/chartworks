package semantics

import (
	"reflect"
	"testing"
)

func TestEnhancementCalendarCanonicalizationIsClosedAndImmutable(t *testing.T) {
	pack := testPack()
	pack.Datasets[0].Columns = append(pack.Datasets[0].Columns, Column{ID: "occurred", SourceName: "occurred", Name: "Order date", NativeType: "date", Category: "temporal"})
	model, err := Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	original := model.Digest()
	before := model.Pack()
	canonicalDigest := ""
	for _, calendar := range EnhancementCalendarNames() {
		t.Run(calendar, func(t *testing.T) {
			policy := &TemporalPolicy{Calendar: calendar, Timezone: "UTC", Grains: []TimeGrain{GrainDay, GrainMonth, GrainQuarter, GrainYear}}
			proposal := Enhancement{Dataset: "orders", Column: "occurred", Kind: EnhancementDimension, Name: "Order date", Role: DimensionTemporal, Temporal: policy}
			changed, err := ApplyEnhancements(model, "v2", []Enhancement{proposal})
			if err != nil {
				t.Fatal(err)
			}
			if model.Digest() != original || !reflect.DeepEqual(model.Pack(), before) || policy.Calendar != calendar || proposal.Temporal != policy {
				t.Fatal("authoring mutated caller or prior model")
			}
			found := false
			for _, dimension := range changed.Pack().Dimensions {
				if dimension.Field.ID == "occurred" {
					found = dimension.Temporal != nil && dimension.Temporal.Calendar == "gregorian" && dimension.Temporal.Timezone == "UTC" && reflect.DeepEqual(dimension.Temporal.Grains, policy.Grains)
				}
			}
			if !found {
				t.Fatal("canonical calendar or preserved temporal meaning missing")
			}
			if canonicalDigest == "" {
				canonicalDigest = changed.Digest()
			} else if changed.Digest() != canonicalDigest {
				t.Fatal("display spelling changed canonical semantic identity")
			}
			policy.Calendar = "caller_changed"
			policy.Grains[0] = GrainHour
			for _, dimension := range changed.Pack().Dimensions {
				if dimension.Field.ID == "occurred" && (dimension.Temporal.Calendar != "gregorian" || dimension.Temporal.Grains[0] != GrainDay) {
					t.Fatal("compiled result shares caller state")
				}
			}
		})
	}
	for _, calendar := range []string{"", "fiscal", "iso8601", "gregORian", "gregorian ", " Gregorian"} {
		_, err := ApplyEnhancements(model, "v2", []Enhancement{{Dataset: "orders", Column: "occurred", Kind: EnhancementDimension, Name: "Order date", Role: DimensionTemporal, Temporal: &TemporalPolicy{Calendar: calendar, Grains: []TimeGrain{GrainMonth}}}})
		if err == nil || validationCode(t, err) != CodeInvalidValue || model.Digest() != original {
			t.Fatal("unknown calendar was interpreted or prior model changed")
		}
	}
	names := EnhancementCalendarNames()
	names[0] = "foreign"
	if EnhancementCalendarNames()[0] != "gregorian" {
		t.Fatal("producer vocabulary is mutable")
	}
}
