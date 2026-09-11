package reporting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
)

func documentFixture() DocumentDefinition {
	return DocumentDefinition{SchemaVersion: DocumentVersion, Locale: "en-US", Timezone: "UTC",
		Metadata: []DocumentMetadata{{Locale: "en-US", Title: "Synthetic report"}},
		Widgets: []Widget{{ID: "note", Kind: "text", Grid: GridCell{Width: 12, Height: 1}, Text: &TextWidget{Format: "markdown", Text: "# Evidence\n\n**Bounded** text."}}}}
}

func TestDocumentCanonicalJSON(t *testing.T) {
	a := json.RawMessage(`{"schema_version":2,"metadata":{"title":"Evidence","count":9007199254740993}}`)
	b := json.RawMessage(`{ "metadata": {"count":9007199254740993, "title":"Evidence"}, "schema_version": 2 }`)
	if DocumentDigest(a) == "" || DocumentDigest(a) != DocumentDigest(b) {
		t.Fatal("JSONB key ordering or exact integer preservation changed the hash")
	}
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`, `{"a":`, strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34)} {
		if DocumentDigest(json.RawMessage(raw)) != "" {
			t.Fatalf("ambiguous or unbounded JSON accepted: %.32s", raw)
		}
	}
}

func TestDocumentDefinition(t *testing.T) {
	limits := config.DefaultReportingComposition()
	if err := ValidateDocument("report", documentFixture(), limits, false); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*DocumentDefinition){
		func(d *DocumentDefinition) { d.SchemaVersion = 99 },
		func(d *DocumentDefinition) { d.Widgets[0].Kind = "script" },
		func(d *DocumentDefinition) { d.Widgets[0].Block = &BlockWidget{Block: "hidden"} },
		func(d *DocumentDefinition) { d.Widgets[0].Grid.Width = 13 },
		func(d *DocumentDefinition) { d.Widgets[0].Text.Text = "<script>alert(1)</script>" },
		func(d *DocumentDefinition) { d.Widgets[0].Text.Text = "&lt;img src=x&gt;" },
		func(d *DocumentDefinition) { d.Widgets[0].Text.Text = "[click](javascript:alert(1))" },
		func(d *DocumentDefinition) { d.Widgets[0].Text.Text = strings.Repeat("x", limits.MaxTextBytes+1) },
		func(d *DocumentDefinition) { d.Widgets[0].Presentation.Density = "javascript" },
		func(d *DocumentDefinition) { d.Widgets = append(d.Widgets, d.Widgets[0]) },
		func(d *DocumentDefinition) { d.Widgets = append(d.Widgets, Widget{ID: "overlap", Kind: "text", Grid: d.Widgets[0].Grid, Text: &TextWidget{Format: "plain", Text: "note"}}) },
		func(d *DocumentDefinition) { d.Locale = "fr-FR" },
		func(d *DocumentDefinition) { d.Timezone = "not-a-timezone" },
		func(d *DocumentDefinition) { d.Filters = []ReportFilter{{Parameter: Parameter{Name: "unused", Type: "integer"}}} },
	}
	for i, mutate := range mutations {
		d := documentFixture()
		mutate(&d)
		if err := ValidateDocument("report", d, limits, false); err == nil {
			t.Fatalf("unsafe or inconsistent definition %d accepted", i)
		}
	}
	dashboard := documentFixture()
	dashboard.Widgets = nil
	if ValidateDocument("dashboard", dashboard, limits, false) == nil || ValidateDocument("dashboard", dashboard, limits, true) != nil {
		t.Fatal("zero pages must be a valid redacted view, not an authored dashboard")
	}
}

func TestDocumentLegacyProjection(t *testing.T) {
	d := documentFixture()
	w := d.Widgets[0]
	w.Grid = GridCell{}
	d.SchemaVersion, d.Widgets = 1, nil
	d.Sections = []LegacySection{{ID: "intro", Title: "Introduction", Widgets: []Widget{w}}}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	before := string(raw)
	projected, err := ProjectStoredDocument(raw, "report")
	if err != nil || len(projected.Widgets) != 2 || projected.Widgets[0].Text.Text != "Introduction" || projected.Widgets[1].Section != "intro" || projected.Widgets[1].Grid.Row != 1 || projected.SchemaVersion != DocumentVersion {
		t.Fatal("legacy meaning was not projected", projected, err)
	}
	projected.Widgets[1].Text.Text = "changed"
	if string(raw) != before || DocumentDigest(raw) == "" {
		t.Fatal("projection mutated historical bytes")
	}
}

func TestDocumentParameterPrecedence(t *testing.T) {
	value := func(s string) *Value { return &Value{Literal: s} }
	parameters := []Parameter{{Name: "n", Type: "integer", Default: value("1"), Min: "1", Max: "9"}}
	d := documentFixture()
	d.Defaults = []Argument{{Name: "n", Value: *value("2")}}
	d.Filters = []ReportFilter{{Parameter: Parameter{Name: "minimum", Type: "integer", Default: value("4")}}}
	w := Widget{Block: &BlockWidget{Block: "block"}, Literals: []Argument{{Name: "n", Value: *value("3")}}, Bindings: []FilterBinding{{Filter: "minimum", Parameter: "n"}}, Overrides: []string{"n"}}
	resolution := Resolution{At: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), Timezone: "UTC"}
	resolved, err := ResolveWidgetParameters(parameters, d, w, nil, nil, resolution)
	if err != nil || resolved.Parameters[0].Value != "4" || resolved.Values[0].Provenance != "filter:minimum" {
		t.Fatal("filter default precedence", resolved, err)
	}
	resolved, err = ResolveWidgetParameters(parameters, d, w, []Argument{{Name: "minimum", Value: *value("5")}}, []Argument{{Name: "n", Value: *value("6")}}, resolution)
	if err != nil || resolved.Parameters[0].Value != "6" || resolved.Values[0].Provenance != "invocation_override" {
		t.Fatal("explicit override precedence", resolved, err)
	}
	for _, bad := range [][]Argument{{{Name: "n", Value: *value("1 OR TRUE")}}, {{Name: "unknown", Value: *value("6")}}, {{Name: "n", Value: *value("5")}, {Name: "n", Value: *value("6")}}} {
		if _, err := ResolveWidgetParameters(parameters, d, w, nil, bad, resolution); err == nil {
			t.Fatal("invalid override accepted", bad)
		}
	}
	w.Overrides = nil
	if _, err := ResolveWidgetParameters(parameters, d, w, nil, []Argument{{Name: "n", Value: *value("6")}}, resolution); err == nil {
		t.Fatal("undeclared invocation override accepted")
	}
	d.Filters[0].Parameter.Type = "number"
	if ValidateWidgetBindings(parameters, d, w) == nil {
		t.Fatal("filter-to-parameter type mismatch accepted")
	}
}

func FuzzDocumentProjection(f *testing.F) {
	f.Add([]byte(`{"schema_version":99}`))
	raw, _ := json.Marshal(documentFixture())
	f.Add(raw)
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := string(raw)
		d, err := ProjectStoredDocument(raw, "report")
		if string(raw) != before {
			t.Fatal("input mutated")
		}
		if err == nil && (d.SchemaVersion != DocumentVersion || len(d.Widgets) == 0 || len(d.Widgets) > 100) {
			t.Fatal("unbounded or incomplete successful projection")
		}
	})
}
