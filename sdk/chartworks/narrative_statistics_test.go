package chartworks_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestStatisticalNarrativeSDKPreservesTypedCoordinates(t *testing.T) {
	value := sdk.BlockNarrativeFieldRef{Column: 1, Field: sdk.BlockSchemaField{Name: "amount", Type: "decimal", Encoding: "string", NativeType: "numeric"}}
	date := sdk.BlockNarrativeFieldRef{Column: 0, Field: sdk.BlockSchemaField{Name: "day", Type: "temporal", Encoding: "string", NativeType: "temporal"}}
	spec := sdk.BlockNarrative{PolicyVersion: sdk.BlockStatisticalNarrativePolicyVersion, SchemaVersion: sdk.BlockStatisticalNarrativeSchemaVersion, Reduction: "statistical_evidence", Statistics: []sdk.BlockNarrativeStatistic{{ID: "trend", Kind: "trend", Value: value, Time: &sdk.BlockNarrativeTimeOrder{Field: date, Meaning: "date"}}, {ID: "spread", Kind: "population_variance", Value: value}}}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var got sdk.BlockNarrative
	if err = json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, spec) {
		t.Fatal("statistical wire changed", err)
	}
	legacy, err := json.Marshal(sdk.BlockNarrative{PolicyVersion: sdk.BlockNarrativePolicyVersion})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "statistics") {
		t.Fatal("legacy wire acquired statistical declaration")
	}
}
