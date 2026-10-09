package reporting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestPhysicalOptionExactResultAndSearch(t *testing.T) {
	for _, tc := range []struct{ column, wire, raw, value string }{
		{"integer", "integer", `"9007199254740993"`, "9007199254740993"},
		{"number", "decimal", `"9007199254740993.125"`, "9007199254740993.125"},
		{"number", "number", `1.25`, "1.25"},
		{"boolean", "boolean", `false`, "false"},
		{"text", "text", `""`, ""},
		{"identifier", "text", `"23c3e803-7893-4f00-b181-c8323930d653"`, "23c3e803-7893-4f00-b181-c8323930d653"},
	} {
		p := columnParameter("column_value", tc.column, Value{Literal: tc.value})
		value, err := physicalOptionValue(*p.Column, exec.Field{Type: tc.wire}, json.RawMessage(tc.raw))
		expected, _ := json.Marshal(tc.value)
		if err != nil || string(value) != string(expected) {
			t.Fatal(tc, string(value), err)
		}
		r := authoringOptionResolution{column: p.Column}
		a, err := optionCursorParameter(r, value)
		b, wantErr := columnFilterScalar(*p.Column, tc.value)
		if err != nil || wantErr != nil || a != b {
			t.Fatal("cursor lost native value", a, b, err, wantErr)
		}
		if tc.column != "text" {
			got, err := optionSearchParameter(r, tc.value)
			if err != nil || got != b {
				t.Fatal("search coerced value", got, err)
			}
		}
	}
	for _, tc := range []struct{ column, wire, raw string }{
		{"number", "decimal", `"NaN"`}, {"integer", "number", `9007199254740993`}, {"boolean", "text", `"false"`}, {"identifier", "text", `"bad"`}, {"text", "text", `null`},
	} {
		p := columnParameter("column_value", tc.column, Value{})
		if _, err := physicalOptionValue(*p.Column, exec.Field{Type: tc.wire}, json.RawMessage(tc.raw)); err == nil {
			t.Fatal("bad result accepted", tc)
		}
	}
	_, _, _, binding := preparationCompileFixture()
	r := authoringOptionResolution{filterOptionResolution: filterOptionResolution{binding: binding, relation: binding.Relations[0], physical: binding.Relations[0].Columns[0]}, column: columnParameter("column_value", "integer", Value{Literal: "1"}).Column}
	statement, err := authoringOptionStatement(r, true, true, 3)
	if err != nil || strings.Contains(statement, "LOWER") || !strings.Contains(statement, " = $1") || !strings.Contains(statement, " > $2") || !strings.Contains(statement, "LIMIT 3") {
		t.Fatal(statement, err)
	}
	if _, err := optionSearchParameter(r, "1 OR TRUE"); err == nil {
		t.Fatal("invalid exact search")
	}
}

func TestPhysicalOptionTargetUnionAndLegacyBytes(t *testing.T) {
	pin := columnParameter("column_value", "text", Value{}).Column.SourceDataset
	topic := TopicPin{Topic: "topic", Version: "v1", Digest: strings.Repeat("a", 64)}
	old := AuthoringDatasetOptionTarget{NewBlock: "chart", Topic: topic, Dataset: pin.Dataset, Dimension: "dimension"}
	wire, _ := json.Marshal(old)
	expected := `{"new_block":"chart","topic":{"topic":"topic","version":"v1","digest":"` + topic.Digest + `"},"dataset":"` + pin.Dataset + `","dimension":"dimension"}`
	if !old.valid() || string(wire) != expected {
		t.Fatal("legacy bytes", string(wire))
	}
	raw := AuthoringDatasetOptionTarget{NewBlock: "chart", SourceDataset: &pin, Dataset: pin.Dataset, Column: "field"}
	if !raw.valid() {
		t.Fatal("raw target rejected")
	}
	for _, mutate := range []func(*AuthoringDatasetOptionTarget){func(d *AuthoringDatasetOptionTarget) { d.Topic = topic }, func(d *AuthoringDatasetOptionTarget) { d.Dimension = "dimension" }, func(d *AuthoringDatasetOptionTarget) { d.Column = "" }, func(d *AuthoringDatasetOptionTarget) { d.Dataset = "other" }} {
		d := clone(raw)
		mutate(&d)
		if d.valid() {
			t.Fatal("mixed or changed origin accepted")
		}
	}
	r := AuthoringOptionRecord{Target: AuthoringOptionTarget{Dataset: &raw}, Source: pin.Source, Context: pin.Context, Dataset: pin.Dataset, SourceRevision: pin.SourceRevision, Column: columnParameter("column_value", "text", Value{}).Column}
	if !r.ValidOrigin() {
		t.Fatal("record rejected")
	}
	r.Column.SourceDataset.Context = "other"
	if r.ValidOrigin() {
		t.Fatal("record context widened")
	}
}
