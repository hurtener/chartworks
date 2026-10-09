package reporting

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func columnParameter(kind, typ string, value Value) Parameter {
	_, _, d, binding := fieldSelectionFixture()
	ref := &ColumnReference{SourceDataset: SourceDatasetPin{Source: binding.Source, Context: binding.Context, Dataset: d.ID, SourceRevision: binding.Revision, SchemaDigest: exec.Hash(binding.Relations[0])}, Name: "field", Type: typ}
	if typ == "date" || typ == "timestamp" || typ == "instant" {
		ref.Calendar = "gregorian"
	}
	if typ == "instant" {
		ref.Timezone = "America/New_York"
	}
	return Parameter{Name: "filter", Type: kind, Required: true, Default: &value, Column: ref}
}

func TestColumnFilterValues(t *testing.T) {
	cases := []struct {
		typ, kind string
		value     Value
		want      []exec.Parameter
	}{
		{"text", "column_value", Value{Literal: "x' OR TRUE --"}, []exec.Parameter{{Kind: "text", Value: "x' OR TRUE --"}}},
		{"text", "column_value", Value{}, []exec.Parameter{{Kind: "text", Value: ""}}},
		{"integer", "column_value", Value{Literal: "9007199254740993"}, []exec.Parameter{{Kind: "integer", Value: "9007199254740993"}}},
		{"number", "column_range", Value{Range: &ScalarRange{"9007199254740993.125", "9007199254740993.126"}}, []exec.Parameter{{Kind: "number", Value: "9007199254740993.125"}, {Kind: "number", Value: "9007199254740993.126"}}},
		{"boolean", "column_value", Value{Literal: "false"}, []exec.Parameter{{Kind: "boolean", Value: "false"}}},
		{"identifier", "column_value", Value{Literal: "23c3e803-7893-4f00-b181-c8323930d653"}, []exec.Parameter{{Kind: "text", Value: "23c3e803-7893-4f00-b181-c8323930d653"}}},
		{"date", "column_range", Value{Range: &ScalarRange{"2024-02-29", "2024-03-01"}}, []exec.Parameter{{Kind: "text", Value: "2024-02-29"}, {Kind: "text", Value: "2024-03-01"}}},
		{"timestamp", "column_range", Value{Range: &ScalarRange{"2026-03-08T00:00:00", "2026-03-09T00:00:00"}}, []exec.Parameter{{Kind: "text", Value: "2026-03-08T00:00:00"}, {Kind: "text", Value: "2026-03-09T00:00:00"}}},
		{"instant", "column_range", Value{Range: &ScalarRange{"2026-03-08T00:00:00", "2026-03-09T00:00:00"}}, []exec.Parameter{{Kind: "text", Value: "2026-03-08T05:00:00Z"}, {Kind: "text", Value: "2026-03-09T04:00:00Z"}}},
	}
	for _, tc := range cases {
		t.Run(tc.typ+"/"+tc.kind+"/"+tc.value.Literal, func(t *testing.T) {
			p := columnParameter(tc.kind, tc.typ, tc.value)
			for _, zone := range []string{"UTC", "Pacific/Apia"} {
				r := testResolution()
				r.Timezone = zone
				got, err := ResolveParameters([]Parameter{p}, nil, r)
				if err != nil || !reflect.DeepEqual(got.Parameters, tc.want) {
					t.Fatalf("%#v %v", got, err)
				}
			}
		})
	}
	for _, typ := range []string{"text", "integer", "number", "boolean", "identifier"} {
		value := map[string]string{"text": "a", "integer": "9007199254740993", "number": "1.00001", "boolean": "true", "identifier": "23c3e803-7893-4f00-b181-c8323930d653"}[typ]
		p := columnParameter("column_set", typ, Value{Items: []string{value}})
		got, err := ResolveParameters([]Parameter{p}, nil, testResolution())
		if err != nil || len(got.Parameters) != 16 || got.Parameters[0].Value != value {
			t.Fatal(typ, got, err)
		}
		for _, bound := range got.Parameters[1:] {
			if bound.Kind != "null" {
				t.Fatal("unbounded set")
			}
		}
	}
}

func TestColumnFilterRejectsTypeAndCalendarAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		typ, kind string
		value     Value
	}{
		{"integer", "column_value", Value{Literal: "1.5"}}, {"integer", "column_value", Value{Literal: "01"}},
		{"number", "column_value", Value{Literal: "NaN"}}, {"boolean", "column_value", Value{Literal: "yes"}},
		{"identifier", "column_value", Value{Literal: "not-a-uuid"}},
		{"date", "column_range", Value{Range: &ScalarRange{"2023-02-29", "2023-03-01"}}},
		{"date", "column_range", Value{Range: &ScalarRange{"2026-01-01", "2026-01-01"}}},
		{"timestamp", "column_range", Value{Range: &ScalarRange{"2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"}}},
		{"instant", "column_range", Value{Range: &ScalarRange{"2026-03-08T02:30:00", "2026-03-09T00:00:00"}}},
		{"instant", "column_range", Value{Range: &ScalarRange{"2026-11-01T01:30:00", "2026-11-02T00:00:00"}}},
		{"text", "column_range", Value{Range: &ScalarRange{"a", "b"}}},
		{"date", "column_value", Value{Literal: "2026-01-01"}},
		{"text", "column_value", Value{Literal: "a", Items: []string{"a"}}},
		{"text", "column_set", Value{Items: []string{"a", "a"}}},
		{"text", "column_set", Value{Items: make([]string, 17)}},
	} {
		if _, err := ResolveParameters([]Parameter{columnParameter(tc.kind, tc.typ, tc.value)}, nil, testResolution()); err == nil {
			t.Fatalf("accepted %#v", tc)
		}
	}
	p := columnParameter("column_range", "instant", Value{Range: &ScalarRange{"2026-03-08T00:00:00", "2026-03-09T00:00:00"}})
	for _, mutate := range []func(*Parameter){
		func(p *Parameter) { p.Column.Timezone = "" }, func(p *Parameter) { p.Column.Timezone = "Local" }, func(p *Parameter) { p.Column.Calendar = "" }, func(p *Parameter) { p.Column.Calendar = "invented" },
		func(p *Parameter) {
			p.Dimension = &DimensionReference{Topic: "topic", Version: "v1", Dimension: "field"}
		}, func(p *Parameter) { p.Required = false }, func(p *Parameter) { p.Default = nil },
		func(p *Parameter) { p.Column.SourceDataset.SchemaDigest = "" }, func(p *Parameter) { p.Type = "datetime" }, func(p *Parameter) { p.Default.DateRange = &DateRange{"2026-01-01", "2026-02-01"} },
	} {
		bad := clone(p)
		mutate(&bad)
		if validateDeclarations([]Parameter{bad}, 1) == nil {
			t.Fatal("bad column reference accepted", bad)
		}
	}
	legacy := Parameter{Name: "x", Type: "integer", Default: &Value{Literal: "1"}}
	raw, _ := json.Marshal(legacy)
	if string(raw) != `{"name":"x","type":"integer","required":false,"default":{"literal":"1"}}` {
		t.Fatal("legacy canonical bytes changed", string(raw))
	}
	legacy.Default.Range = &ScalarRange{"1", "2"}
	if validateDeclarations([]Parameter{legacy}, 1) == nil {
		t.Fatal("legacy accepted new union member")
	}
}

func TestAuthoringColumnFiltersCompilerAndIdentity(t *testing.T) {
	for _, physical := range []bool{false, true} {
		t.Run(map[bool]string{false: "topic", true: "source"}[physical], func(t *testing.T) {
			in, p, d, b := fieldSelectionFixture()
			in.Fields = &AuthoringFieldSelection{Mode: "aggregate", Measures: []AuthoringMeasureSelection{{Kind: "count"}}}
			in.Mapping = AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: "value_1"}, Options: charts.DefaultOptions()}
			if physical {
				in.Topic = TopicPin{}
				in.SourceDataset = &SourceDatasetPin{Source: b.Source, Context: b.Context, Dataset: d.ID, SourceRevision: b.Revision, SchemaDigest: exec.Hash(b.Relations[0])}
				p = topics.Published{}
			}
			in.Filters = []AuthoringDatasetFilter{
				{Column: "specimen", Kind: "multi_select", Default: Value{Items: []string{"beta", "alpha"}}},
				{Column: "value", Kind: "range", Default: Value{Range: &ScalarRange{"1.125", "2.125"}}},
				{Column: "flag", Kind: "select", Default: Value{Literal: "true"}},
				{Column: "instant", Kind: "range", Calendar: "gregorian", Timezone: "America/New_York", Default: Value{Range: &ScalarRange{"2026-03-08T00:00:00", "2026-03-09T00:00:00"}}},
			}
			compiled, err := compileAuthoringDataset(in, p, d, b)
			if err != nil || len(compiled.Parameters) != 4 || strings.Contains(compiled.SQL, "alpha") {
				t.Fatal(compiled, err)
			}
			def := Definition{SchemaVersion: CurrentSchemaVersion, Source: b.Source, Context: b.Context, SourceDataset: in.SourceDataset, Topics: []TopicPin{in.Topic}, SQL: compiled.SQL, Parameters: compiled.Parameters}
			if physical {
				def.Topics = nil
			}
			if err := validateBoundedFilterSemantics(t.Context(), def, b, []topics.Published{p}); err != nil {
				t.Fatal(err)
			}
			if in.Filters[0].Default.Items[0] != "beta" {
				t.Fatal("compiler changed request")
			}
			for _, mutate := range []func(*Definition){
				func(d *Definition) { d.Parameters[0].Column.Name = "specimen_code" },
				func(d *Definition) { d.Parameters[0].Column.Type = "text" },
				func(d *Definition) { d.Parameters[0].Column.SourceDataset.SourceRevision++ },
				func(d *Definition) { d.Parameters[0].Column.SourceDataset.Context = "other" },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, `"accepted" = $1`, `"specimen_code" = $1`, 1) },
				func(d *Definition) { d.SQL += " OR TRUE" },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, " = $1", " <> $1", 1) },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, "count(*)", "count(*), $1", 1) },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, " IN (", " NOT IN (", 1) },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, " >= $2", " > $2", 1) },
				func(d *Definition) { d.SQL = strings.Replace(d.SQL, "$1", "CAST($1 AS boolean)", 1) },
			} {
				bad := clone(def)
				mutate(&bad)
				if validateBoundedFilterSemantics(t.Context(), bad, b, []topics.Published{p}) == nil {
					t.Fatal("forged predicate or reference", bad.SQL)
				}
			}
			bad := b
			bad.Relations = clone(b.Relations)
			bad.Relations[0].Columns[0].Safe = false
			if validateBoundedFilterSemantics(t.Context(), def, bad, []topics.Published{p}) == nil {
				t.Fatal("schema drift")
			}
			if !physical {
				removed := clone(p)
				removed.Definition.Datasets[0].Columns = nil
				if validateBoundedFilterSemantics(t.Context(), def, b, []topics.Published{removed}) == nil {
					t.Fatal("column outside reviewed dataset")
				}
			}
			filter := clone(def.Parameters[0])
			if !compatibleFilter(filter, def.Parameters[0]) {
				t.Fatal("matching filter")
			}
			filter.Column.SourceDataset.Context = "other"
			if compatibleFilter(filter, def.Parameters[0]) {
				t.Fatal("cross-context filter binding")
			}
		})
	}
}
