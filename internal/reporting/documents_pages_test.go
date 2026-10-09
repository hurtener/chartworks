package reporting

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"reflect"
	"testing"
)

func pagedDocumentFixture() DocumentDefinition {
	d := documentFixture()
	first := clone(d.Widgets[0])
	first.ID = "first"
	second := clone(first)
	second.ID = "second"
	d.SchemaVersion = PagedDocumentVersion
	d.Widgets = nil
	d.ReportPages = []ReportPage{{ID: "overview", Title: "Overview", Widgets: []Widget{first}}, {ID: "detail", Title: "Detail", Locale: "es-AR", Timezone: "America/Argentina/Buenos_Aires", Widgets: []Widget{second}}}
	return d
}
func TestReportPagesDefinitionAndIsolation(t *testing.T) {
	limits := config.DefaultReportingComposition()
	d := pagedDocumentFixture()
	if err := ValidateDocument("report", d, limits, false); err != nil {
		t.Fatal(err)
	}
	leaves := ReportCanvases(d)
	if len(leaves) != 2 || leaves[0].ID != "overview" || leaves[1].Definition.Locale != "es-AR" || leaves[1].Definition.Timezone != "America/Argentina/Buenos_Aires" {
		t.Fatal(leaves)
	}
	leaves[0].Definition.Widgets[0].Text.Text = "changed"
	if reflect.DeepEqual(leaves[0].Definition.Widgets, d.ReportPages[0].Widgets) {
		t.Fatal("projection aliases immutable page")
	}
	if _, err := SelectReportCanvas(d, ""); err == nil {
		t.Fatal("paged selector guessed a page")
	}
	if _, err := SelectReportCanvas(d, "missing"); err == nil {
		t.Fatal("missing page accepted")
	}
	empty := clone(d)
	empty.ReportPages = append(empty.ReportPages, ReportPage{ID: "blank", Title: "Blank", Widgets: []Widget{}})
	if err := ValidateDocument("report", empty, limits, false); err != nil {
		t.Fatal("empty private canvas requires synthetic content", err)
	}
	mutations := map[string]func(*DocumentDefinition){
		"duplicate page":                func(d *DocumentDefinition) { d.ReportPages[1].ID = d.ReportPages[0].ID },
		"duplicate widget across pages": func(d *DocumentDefinition) { d.ReportPages[1].Widgets[0].ID = d.ReportPages[0].Widgets[0].ID },
		"mixed root content":            func(d *DocumentDefinition) { d.Widgets = clone(d.ReportPages[0].Widgets) },
		"legacy version inline pages":   func(d *DocumentDefinition) { d.SchemaVersion = DocumentVersion },
		"unknown version":               func(d *DocumentDefinition) { d.SchemaVersion = 4 },
		"missing widgets":               func(d *DocumentDefinition) { d.ReportPages[0].Widgets = nil },
		"no pages":                      func(d *DocumentDefinition) { d.ReportPages = nil },
		"bad locale":                    func(d *DocumentDefinition) { d.ReportPages[0].Locale = "not-real" },
		"bad zone":                      func(d *DocumentDefinition) { d.ReportPages[0].Timezone = "not-real" },
		"unsafe page id":                func(d *DocumentDefinition) { d.ReportPages[0].ID = "../page" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := clone(d)
			mutate(&bad)
			if ValidateDocument("report", bad, limits, false) == nil {
				t.Fatal("invalid pages accepted")
			}
		})
	}
	small := limits
	small.MaxWidgets = 1
	if ValidateDocument("report", d, small, false) == nil {
		t.Fatal("widget budget multiplied by page count")
	}
	small = limits
	small.MaxPages = 1
	if ValidateDocument("report", d, small, false) == nil {
		t.Fatal("page budget ignored")
	}
	if manualDocument(DocumentDefinition{SchemaVersion: PagedDocumentVersion, ReportPages: []ReportPage{{Widgets: []Widget{{Kind: "query"}}}}}) == nil {
		t.Fatal("manual nested query bypassed")
	}
}
func TestReportPagesLegacyProjectionAndDigest(t *testing.T) {
	d := documentFixture()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	hash := DocumentDigest(raw)
	projected, err := ProjectStoredDocument(raw, "report")
	if err != nil {
		t.Fatal(err)
	}
	leaves := ReportCanvases(projected)
	if len(leaves) != 1 || leaves[0].ID != "main" || projected.SchemaVersion != DocumentVersion || len(projected.ReportPages) != 0 || DocumentDigest(raw) != hash || string(raw) != string(before) {
		t.Fatal("legacy definition automatically upgraded")
	}
	marshaled, _ := json.Marshal(projected)
	if string(raw) != string(marshaled) {
		t.Fatal("historical flat wire shape changed")
	}
	if DashboardCanvasID("section", d, "main") != "section" {
		t.Fatal("legacy dashboard page renamed")
	}
	paged := pagedDocumentFixture()
	if DashboardCanvasID("section", paged, "overview") == DashboardCanvasID("other", paged, "overview") || DashboardCanvasID("section", paged, "overview") == DashboardCanvasID("section", paged, "detail") {
		t.Fatal("nested page identities collide")
	}
}
func TestReportPagesScheduledSelection(t *testing.T) {
	d := pagedDocumentFixture()
	dispatch := &jobs.ReportingDispatch{Target: jobs.ReportingTarget{Type: "report", ID: "report", Locale: d.Locale, Timezone: d.Timezone}}
	got, err := scheduledDefinition(d, dispatch)
	if err != nil || !reflect.DeepEqual(got, d) {
		t.Fatal("schedule dropped or changed pages", got, err)
	}
	dispatch.Target.Type = "saved_question"
	if _, err := scheduledDefinition(d, dispatch); err == nil {
		t.Fatal("unqualified saved question selector accepted")
	}
}
