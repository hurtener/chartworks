package reporting

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func fieldSelectionFixture() (AuthoringDatasetIntent, topics.Published, topics.Dataset, exec.Binding) {
	columns := []semantics.Column{
		{ID: "specimen", SourceName: "specimen_code", Name: "Specimen", NativeType: "text", Category: "text"},
		{ID: "instrument", SourceName: "instrument_code", Name: "Instrument", NativeType: "text", Category: "text"},
		{ID: "observed", SourceName: "observed_on", Name: "Observed on", NativeType: "date", Category: "temporal"},
		{ID: "value", SourceName: "reading", Name: "Reading", NativeType: "numeric", Category: "numeric", Nullable: true},
		{ID: "instant", SourceName: "recorded_at", Name: "Recorded at", NativeType: "timestamptz", Category: "temporal"},
		{ID: "year", SourceName: "year_code", Name: "Year", NativeType: "int4", Category: "numeric"},
		{ID: "flag", SourceName: "accepted", Name: "Accepted", NativeType: "bool", Category: "boolean"},
	}
	d := topics.Dataset{ID: "observations", Name: "Observations", Source: topics.Binding{Source: "source", Context: "context", Dataset: "observations", SourceRevision: 1}, Columns: columns}
	p := topics.Published{Digest: strings.Repeat("a", 64), State: topics.State{Topic: "research", Version: "v1", Active: true}, Definition: topics.Definition{Topic: "research", Version: "v1", Datasets: []topics.Dataset{d}, Measures: []semantics.Measure{{ID: "mean", Name: "Mean reading", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "value"}, Aggregation: semantics.AggregationAverage}}}}
	r := exec.Relation{ID: d.ID, Schema: "workspace", Name: "observations"}
	for _, c := range columns {
		r.Columns = append(r.Columns, exec.Column{Name: c.SourceName, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Safe: true})
	}
	b := exec.Binding{Tenant: "tenant", Source: "source", Context: "context", Revision: 1, Dialect: "postgres", Contract: "postgres-read-v1", Fingerprint: strings.Repeat("f", 64), Relations: []exec.Relation{r}}
	f := &AuthoringFieldSelection{Mode: "aggregate", Dimensions: []AuthoringGrouping{{Kind: "column", Field: "specimen"}, {Kind: "column", Field: "instrument"}, {Kind: "column", Field: "observed", Grain: "month", Calendar: "gregorian"}}, Measures: []AuthoringMeasureSelection{{Kind: "measure", Field: "mean"}, {Kind: "column", Field: "value", Aggregation: "maximum"}, {Kind: "count"}}}
	in := AuthoringDatasetIntent{Topic: TopicPin{Topic: "research", Version: "v1", Digest: p.Digest}, Dataset: d.ID, Fields: f, Mapping: AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"group_1", "group_2", "group_3", "value_1", "value_2", "value_3"}}, Options: charts.DefaultOptions(), Table: &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "group_1", Visible: true}, {Column: "group_2", Visible: true}, {Column: "group_3", Visible: true}, {Column: "value_1", Visible: true}, {Column: "value_2", Visible: true}, {Column: "value_3", Visible: true}}, PageSize: 20}}}
	return in, p, d, b
}

func TestAuthoringFieldsMultiGroupMultiMeasure(t *testing.T) {
	in, p, d, b := fieldSelectionFixture()
	out, err := compileAuthoringDataset(in, p, d, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`avg("reading") AS "value_1"`, `max("reading") AS "value_2"`, `count(*) AS "value_3"`, `CAST(date_trunc('month', CAST("observed_on" AS timestamp)) AS date)`, `FROM "workspace"."observations"`, `GROUP BY "specimen_code", "instrument_code"`} {
		if !strings.Contains(out.SQL, fragment) {
			t.Fatal("missing operation", fragment, out.SQL)
		}
	}
	if len(out.Columns) != 6 || len(out.Dependencies) != 1 || !reflect.DeepEqual(out.Scope[0].Columns, []string{"instrument_code", "observed_on", "reading", "specimen_code"}) || AuthoringCompilerForIntent(in) != AuthoringFieldCompilerVersion {
		t.Fatal(out)
	}
	// The compiler never invents result types. The source-observed schema is what
	// will supply these in production; exercise the actual retained renderer seam.
	for i, typ := range []string{"text", "text", "temporal", "decimal", "decimal", "integer"} {
		if out.Columns[i].Type != "" {
			t.Fatal("compiler invented type")
		}
		out.Columns[i].Type = typ
	}
	data := charts.Data{Version: 1, Columns: out.Columns, Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}
	if _, err := charts.BindDisplay(context.Background(), data, charts.Table, in.Mapping.Bindings, nil, in.Mapping.Options, nil, in.Mapping.Table, charts.Defaults()); err != nil {
		t.Fatal("compiled selection cannot bind real table", err)
	}
}

func TestAuthoringFieldsHaveNoBusinessNameDependence(t *testing.T) {
	in, p, d, b := fieldSelectionFixture()
	original, err := compileAuthoringDataset(in, p, d, b)
	if err != nil {
		t.Fatal(err)
	}
	// Rename every physical column and all display metadata while keeping stable
	// typed identities. The selected operators, scopes and positional outputs must
	// be identical under that renaming, without any domain-specific fallback.
	replacements := []string{}
	for i := range d.Columns {
		old := d.Columns[i].SourceName
		name := "field_" + d.Columns[i].ID
		replacements = append(replacements, `"`+old+`"`, `"`+name+`"`)
		d.Columns[i].SourceName, d.Columns[i].Name = name, "Unrelated "+d.Columns[i].ID
		b.Relations[0].Columns[i].Name = name
	}
	p.Definition.Datasets[0] = d
	renamed, err := compileAuthoringDataset(in, p, d, b)
	if err != nil || strings.NewReplacer(replacements...).Replace(original.SQL) != renamed.SQL {
		t.Fatal("query depends on business names", err)
	}
	for i := range original.Columns {
		if original.Columns[i].ID != renamed.Columns[i].ID || original.Columns[i].Aggregation != renamed.Columns[i].Aggregation || original.Columns[i].Role != renamed.Columns[i].Role {
			t.Fatal("name changed analytical behavior")
		}
	}
}

func TestAuthoringFieldsTypeAndPolicyRejections(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AuthoringDatasetIntent, *topics.Published, *topics.Dataset, *exec.Binding)
	}{
		{"numeric year is not a date", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions[2].Field = "year"
		}},
		{"text is not numeric", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Measures[1].Field = "specimen"
		}},
		{"boolean sum", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Measures[1] = AuthoringMeasureSelection{Kind: "column", Field: "flag", Aggregation: "sum"}
		}},
		{"reviewed measure cannot change aggregation", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Measures[0].Aggregation = "sum"
		}},
		{"reviewed filters retained", func(_ *AuthoringDatasetIntent, p *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			p.Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "mandatory"}}
		}},
		{"unsafe physical column", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) {
			b.Relations[0].Columns[3].Safe = false
		}},
		{"source revision drift", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) { b.Revision++ }},
		{"physical type drift", func(_ *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, b *exec.Binding) {
			b.Relations[0].Columns[3].NativeType = "text"
		}},
		{"ambiguous legacy intent", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Measure = "mean"
		}},
		{"duplicate measure", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Measures[1] = in.Fields.Measures[0]
		}},
		{"row aggregation", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Mode = "rows"
		}},
		{"SQL as a field", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions[0].Field = "specimen);delete from other"
		}},
		{"SQL as aggregation", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Measures[1].Aggregation = "sum);select"
		}},
		{"column budget", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions = make([]AuthoringGrouping, 257)
		}},
		{"unknown calendar", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions[2].Calendar = "unregistered"
		}},
		{"civil date cannot shift timezone", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions[2].Timezone = "UTC"
		}},
		{"instant requires timezone", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions[2].Field = "instant"
		}},
		{"empty structured selection", func(in *AuthoringDatasetIntent, _ *topics.Published, _ *topics.Dataset, _ *exec.Binding) {
			in.Fields.Dimensions = nil
			in.Fields.Measures = nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, p, d, b := fieldSelectionFixture()
			tc.change(&in, &p, &d, &b)
			if _, err := compileAuthoringDataset(in, p, d, b); err == nil {
				t.Fatal("invalid selection admitted")
			}
		})
	}
}

func TestAuthoringFieldsRowsCountAndExplicitCalendar(t *testing.T) {
	in, p, d, b := fieldSelectionFixture()
	in.Fields.Mode, in.Fields.Measures = "rows", nil
	in.Fields.Dimensions[2].Grain, in.Fields.Dimensions[2].Calendar = "", ""
	out, err := compileAuthoringDataset(in, p, d, b)
	if err != nil || strings.Contains(out.SQL, "GROUP BY") || strings.Contains(out.SQL, "date_trunc") || len(out.Columns) != 3 {
		t.Fatal(out, err)
	}
	in.Fields.Mode, in.Fields.Dimensions, in.Fields.Measures = "aggregate", nil, []AuthoringMeasureSelection{{Kind: "count"}}
	out, err = compileAuthoringDataset(in, p, d, b)
	if err != nil || !strings.Contains(out.SQL, "count(*)") || len(out.Scope[0].Columns) != 1 || out.Columns[0].Provenance.Source != b.Source || out.Columns[0].Provenance.Topic != "" || out.Columns[0].Provenance.SemanticID != "" {
		t.Fatal(out, err)
	}
	in.Fields.Dimensions = []AuthoringGrouping{{Kind: "column", Field: "instant", Grain: "day", Calendar: "gregorian", Timezone: "America/Argentina/Buenos_Aires"}}
	out, err = compileAuthoringDataset(in, p, d, b)
	if err != nil || !strings.Contains(out.SQL, `date_trunc('day', "recorded_at", 'America/Argentina/Buenos_Aires')`) {
		t.Fatal(out, err)
	}
	for _, zone := range []string{"Local", "Bad/Zone", "UTC');select"} {
		in.Fields.Dimensions[0].Timezone = zone
		if _, err := compileAuthoringDataset(in, p, d, b); err == nil {
			t.Fatal(zone)
		}
	}
}

func TestAuthoringFieldsMetadataAndLegacyDigest(t *testing.T) {
	_, _, d, b := fieldSelectionFixture()
	catalog := authoringFieldCatalog(d, b, 32)
	if catalog.MaxColumns != 32 || len(catalog.Columns) != 7 || len(catalog.Columns[5].Grains) != 0 || !slices.Contains(catalog.Columns[3].Aggregations, "average") || slices.Contains(catalog.Columns[0].Aggregations, "sum") {
		t.Fatal(catalog)
	}
	b.Relations[0].Columns[0].Safe = false
	if item := authoringFieldCatalog(d, b, 32).Columns[0]; item.Supported || item.Reason == "" {
		t.Fatal(item)
	}
	in, _, _, _ := preparationCompileFixture()
	encoded, err := json.Marshal(in)
	if err != nil || strings.Contains(string(encoded), `"fields"`) || AuthoringCompilerForIntent(in) != AuthoringCompilerVersion {
		t.Fatal("legacy request changed", err)
	}
}

func TestAuthoringFieldsConfiguredCapacityBeforeRead(t *testing.T) {
	s, repo, boundary, actor, in := preparationServiceFixture(t)
	s.documents.blocks.limits.MaxSchemaColumns = 2
	in.Intent.Dimensions, in.Intent.Measure = nil, ""
	in.Intent.Fields = &AuthoringFieldSelection{Mode: "aggregate", Dimensions: []AuthoringGrouping{{Kind: "column", Field: "region"}}, Measures: []AuthoringMeasureSelection{{Kind: "column", Field: "amount", Aggregation: "sum"}, {Kind: "count"}}}
	out, err := s.PrepareDatasetChart(t.Context(), actor, in)
	if err != nil || out.Status != "unsupported" || out.Code != "field_capacity_exceeded" || boundary.physical.Load() != 0 || boundary.planning.Load() != 0 || len(repo.records) != 0 {
		t.Fatal("configured budget did not reject before work", out, err)
	}
}

func TestAuthoringFieldsNativeTypeModifiers(t *testing.T) {
	for _, tc := range []struct{ native, kind string }{
		{"numeric(30,3)", "number"}, {"timestamp(6) with time zone", "instant"},
		{"timestamp(3) without time zone", "timestamp"}, {"character varying(64)", "text"},
		{"date", "date"}, {"numeric_money", ""}, {"jsonb", ""},
	} {
		if got := authoringColumnKind(semantics.Column{NativeType: tc.native}); got != tc.kind {
			t.Errorf("%s: got %s, want %s", tc.native, got, tc.kind)
		}
	}
}

func TestAuthoringFieldsLongPhysicalLabel(t *testing.T) {
	in, p, d, b := fieldSelectionFixture()
	d.Columns[3].Name = strings.Repeat("é", 128)
	p.Definition.Datasets[0] = d
	out, err := compileAuthoringDataset(in, p, d, b)
	if err != nil || out.Columns[4].DisplayLabel != d.Columns[3].Name || out.Columns[4].Aggregation != "maximum" {
		t.Fatal("valid field label was lost or over-expanded", out, err)
	}
}

func TestAuthoringFieldsDoNotRequirePredefinedMeasures(t *testing.T) {
	s, _, boundary, actor, in := preparationServiceFixture(t)
	boundary.publication.Definition.Measures = nil
	view, err := s.Dataset(t.Context(), actor, AuthoringDatasetRequest{Topic: in.Intent.Topic, Dataset: in.Intent.Dataset})
	if err != nil || view.Supported || view.Reason != "no_supported_measure" || view.Fields == nil || !view.Fields.Supported || boundary.physical.Load() != 0 {
		t.Fatal("physical fields depend on a predefined measure", view, err)
	}
	for _, capability := range view.FilterCapabilities {
		if capability.Reason == "no_supported_measure" {
			t.Fatal("physical-field filter disabled by legacy measure restriction", capability)
		}
	}
}
