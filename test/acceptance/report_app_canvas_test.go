package acceptance

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// TestReportAppCanvas retains a real heading, KPI, temporal trend and table from
// approved source outputs. The manual app neither invents chart specifications
// nor turns retained reads, redraws or paging into source/model execution.
func TestReportAppCanvas(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	const blockID, reportID = "canvas-source", "canvas-report"
	// As in the CW02 saved-chart fixture, date-typed synthetic labels are
	// approved SQL projections of the already authorized id column.
	block := phase27Definition(t, f.f, f.blockAuthor, "SELECT CASE WHEN id=1 THEN '2026-01-02'::date ELSE '2026-01-03'::date END AS sale_date, amount FROM analytics.sales ORDER BY id")
	columns := phase27Copy(t, block.Outputs[0].Mapping.Columns)
	columns[0].Role, columns[0].DisplayLabel = "time", "Sale date"
	columns[0].Format = charts.Format{DatePattern: "date_medium", Locale: "en-US"}
	columns[1].Role, columns[1].DisplayLabel, columns[1].Aggregation = "measure", "Revenue", "sum"
	columns[1].Format = charts.Format{Unit: "revenue", Currency: "USD", FractionDigits: 3, Locale: "en-US"}
	order := []charts.Order{{Column: columns[0].ID, Direction: "asc"}}
	kpi := charts.Mapping{Version: charts.DisplayVersion, Kind: charts.KPI, Columns: phase27Copy(t, columns), Bindings: charts.Bindings{Category: columns[0].ID, Value: columns[1].ID}, Order: order, Options: charts.DefaultOptions(),
		KPI: &charts.KPIOptions{ValueRow: "first", ComparisonMode: "none", Sparkline: true, Thresholds: []charts.KPIThreshold{}}}
	trend := charts.Mapping{Version: charts.Version, Kind: charts.Line, Columns: phase27Copy(t, columns), Bindings: charts.Bindings{Category: columns[0].ID, Value: columns[1].ID}, Order: order, Options: charts.DefaultOptions()}
	table := charts.Mapping{Version: charts.DisplayVersion, Kind: charts.Table, Columns: phase27Copy(t, columns), Bindings: charts.Bindings{Columns: []string{columns[0].ID, columns[1].ID}}, Order: order, Options: charts.DefaultOptions(),
		Table: &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: columns[0].ID, Visible: true}, {Column: columns[1].ID, Visible: true}}, PageSize: 1, ShowTotals: true}}
	block.Outputs = []reporting.Output{{ID: "kpi-main", Kind: "kpi", Mapping: &kpi}, {ID: "trend-main", Kind: "chart", Mapping: &trend}, {ID: "table-main", Kind: "table", Mapping: &table}}
	f.block(t, blockID, block)
	snapshot, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, blockID, reporting.Reference{}, reporting.Read)
	if err != nil || snapshot.Revision.Number != 1 {
		t.Fatal("published source fixture", snapshot.Revision.Number, err)
	}
	service, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, f.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}

	// Only the preparation fixture uses its pre-existing administrative test
	// authority. Every manual report call below is signed with exact resources.
	authorScopes := []string{"reporting.read", "reporting.write", "reporting.preview", "cw.tenant.write:" + f.author.Tenant(),
		"cw.report.read:" + reportID, "cw.report.write:" + reportID, "cw.report.preview:" + reportID, "cw.block.read:" + blockID}
	for _, ref := range snapshot.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(authorScopes, scope) {
			authorScopes = append(authorScopes, scope)
		}
	}
	actor := func(scopes []string) identity.Envelope {
		t.Helper()
		for _, scope := range scopes {
			if strings.Contains(scope, "*") {
				t.Fatal("manual fixture widened signed authority", scope)
			}
		}
		return phase27Actor(t, f.f, f.author.User(), scopes)
	}
	author := actor(authorScopes)
	d := phase29Text("Approved revenue report")
	d.Widgets[0].Text = &reporting.TextWidget{Format: "markdown", Text: "# Revenue report\n\nApproved retained evidence"}
	for i, output := range block.Outputs {
		w := phase29BlockWidget([]string{"kpi", "trend", "table"}[i], blockID, i+1, output.ID)
		w.Block.Revision = snapshot.Revision.Number
		d.Widgets = append(d.Widgets, w)
	}
	d.Widgets[1].Grid = reporting.GridCell{Row: 1, Width: 4, Height: 2}
	d.Widgets[2].Grid = reporting.GridCell{Column: 4, Row: 1, Width: 8, Height: 2}
	d.Widgets[3].Grid = reporting.GridCell{Row: 3, Width: 12, Height: 3}
	beforeSource, beforeModel, beforeAttempts := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	state, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: reportID, Definition: d})
	if err != nil || state.PublishedRevision != 0 {
		t.Fatal("create private manual composition", state, err)
	}
	d.Widgets[1].Grid.Width, d.Widgets[2].Grid.Column, d.Widgets[2].Grid.Width = 3, 3, 9
	d.Widgets[2].Presentation = reporting.Presentation{Title: "Revenue trend", Subtitle: "Approved source values", Density: "comfortable"}
	state, err = service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: reportID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: d})
	if err != nil || state.DraftRevision != 2 {
		t.Fatal("save bounded layout", state, err)
	}
	saved, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID})
	if err != nil || !reflect.DeepEqual(saved.Definition, d) {
		t.Fatal("reopen changed saved grid, presentation or output references", err)
	}
	if f.f.f.lookups.Load() != beforeSource || f.f.model.requests.Load() != beforeModel || f.attemptCount(t) != beforeAttempts {
		t.Fatal("manual create/save/reopen executed source or model work")
	}

	executeScopes := append(slices.Clone(authorScopes), "reporting.execute", "sources.read", "sources.query", "topics.read",
		"cw.report.execute:"+reportID, "cw.block.execute:"+blockID, "cw.block.preview:"+blockID, "cw.source.query:"+block.Source)
	executor := actor(executeScopes)
	preview, err := service.Preview(ctx, executor, reporting.AuthoringPreviewRequest{Report: reportID, Key: "canvas-preview", Revision: state.DraftRevision})
	if err != nil || !preview.Private || preview.Complete || preview.Revision != state.DraftRevision {
		t.Fatal("explicit private admission", preview, err)
	}
	if f.attemptCount(t) != beforeAttempts || f.f.model.requests.Load() != beforeModel {
		t.Fatal("admission executed approved SQL or called a model")
	}
	sealed, err := f.f.f.db.ReadComposition(ctx, executor, preview.ID)
	if err != nil || len(sealed.Manifest.Pages) != 1 || len(sealed.Manifest.Pages[0].Widgets) != 4 {
		t.Fatal("sealed report shape", sealed, err)
	}
	// Grouping is a measured domain property: record its exact accepted groups
	// and compare physical attempts after execution, rather than inferring one
	// source call merely because three widgets point at the same block.
	t.Logf("same-block accepted query groups: %d", preview.QueryGroups)
	if preview.QueryGroups != len(sealed.Manifest.Groups) || preview.QueryGroups != 1 || !reflect.DeepEqual(sealed.Manifest.Groups[0].Outputs, []string{"kpi-main", "trend-main", "table-main"}) {
		t.Fatal("same-source selection did not preserve the expected output union", preview.QueryGroups, sealed.Manifest.Groups)
	}
	for i, w := range sealed.Manifest.Pages[0].Widgets {
		if w.Definition.Grid != d.Widgets[i].Grid || w.Definition.Presentation != d.Widgets[i].Presentation {
			t.Fatal("admission changed saved layout or presentation", i)
		}
		if i != 0 && (w.Definition.Block.Block != blockID || w.Definition.Block.Revision != snapshot.Revision.Number || !reflect.DeepEqual(w.Definition.Block.Outputs, d.Widgets[i].Block.Outputs)) {
			t.Fatal("admission changed exact selected output", i, w.Definition.Block)
		}
	}
	for _, missing := range []string{"cw.block.execute:" + blockID, "cw.execution_context.use:" + block.Context} {
		denied := actor(slices.DeleteFunc(slices.Clone(executeScopes), func(s string) bool { return s == missing }))
		beforeDeniedSource := f.f.f.lookups.Load()
		if _, err := service.Execute(ctx, denied, reporting.AuthoringExecuteRequest{Run: preview.ID}); err == nil {
			t.Fatal("missing exact dependency/context executed", missing)
		}
		if f.f.f.lookups.Load() != beforeDeniedSource || f.attemptCount(t) != beforeAttempts || f.f.model.requests.Load() != beforeModel {
			t.Fatal("denied execution reached source/model", missing)
		}
	}
	completed, err := service.Execute(ctx, executor, reporting.AuthoringExecuteRequest{Run: preview.ID})
	if err != nil || !completed.Private || !completed.Complete || completed.State != "completed" {
		t.Fatal("execute approved private report", completed, err)
	}
	attempts := f.attemptCount(t) - beforeAttempts
	t.Logf("same-block source execution attempts: %d", attempts)
	if attempts != preview.QueryGroups || f.f.model.requests.Load() != beforeModel {
		t.Fatal("approved report attempt accounting or zero-model invariant", attempts, preview.QueryGroups)
	}
	retained, err := f.f.f.db.ReadComposition(ctx, executor, preview.ID)
	if err != nil || len(retained.Results) != preview.QueryGroups {
		t.Fatal("retained group receipts", retained, err)
	}
	for _, result := range retained.Results {
		if result.Block == nil || !result.Block.Private || result.Block.Revision != snapshot.Revision.Number || result.Block.Block != blockID {
			t.Fatal("private exact published block provenance lost", result)
		}
	}

	// Execute authority cannot read the new private run. The host must refresh
	// the returned run ID, retaining exact report preview and actual context reach.
	if _, err := delivery.View(ctx, executor, reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: "main", Widget: "kpi", Output: "kpi-main"}); err == nil {
		t.Fatal("execution authority read a fresh run without an exact run grant")
	}
	readerScopes := []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + reportID, "cw.execution_context.use:" + block.Context}
	reader := actor(readerScopes)
	beforeRetainedSource := f.f.f.lookups.Load()
	view := func(widget, output string, offset int) reporting.DeliveryViewResult {
		t.Helper()
		out, err := delivery.View(ctx, reader, reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: "main", Widget: widget, Output: output, Offset: offset, Limit: 1})
		if err != nil || out.Selection.Run != preview.ID || out.Selection.Page != "main" || out.Selection.Widget != widget || out.Selection.Output != output || !out.Summary.Private {
			t.Fatal("retained exact output selection", widget, out, err)
		}
		return out
	}
	heading := view("intro", "", 0)
	if heading.Text == nil || *heading.Text != *d.Widgets[0].Text || len(heading.Pages) != 1 || len(heading.Pages[0].Widgets) != 4 {
		t.Fatal("retained heading or saved grid missing", heading)
	}
	for i, w := range heading.Pages[0].Widgets {
		if w.ID != d.Widgets[i].ID || w.Grid != d.Widgets[i].Grid || w.Presentation != d.Widgets[i].Presentation {
			t.Fatal("retained grid did not match saved canonical report", i, w)
		}
	}
	for i, widget := range []string{"kpi", "trend", "table"} {
		output := block.Outputs[i]
		payload, err := f.compositions.Widget(ctx, reader, preview.ID, "main", widget)
		if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].ID != output.ID || payload.Outputs[0].Kind != output.Kind || payload.Outputs[0].Chart == nil {
			t.Fatal("widget disclosed another output subset", widget, payload, err)
		}
		chart := payload.Outputs[0].Chart
		if !reflect.DeepEqual(chart.Mapping, *output.Mapping) || !reflect.DeepEqual(chart.Columns, columns) {
			t.Fatal("retained specification, units or exact values changed", widget, chart)
		}
		first := view(widget, output.ID, 0)
		if first.Output == nil || first.Output.ID != output.ID || first.Output.Kind != output.Kind || first.Output.RetainedDigest != payload.Outputs[0].Digest {
			t.Fatal("delivery changed retained output identity", first)
		}
		if widget == "table" {
			if len(chart.Rows) != 2 || chart.Rows[0][1].Value != "9007199254740993.125" || chart.Rows[1][1].Value != "5.500" {
				t.Fatal("retained table changed exact values", chart.Rows)
			}
			second := view(widget, output.ID, 1)
			if first.Output.Table == nil || second.Output.Table == nil || len(first.Output.Table.Rows) != 1 || len(second.Output.Table.Rows) != 1 || first.Output.Table.Rows[0][1].Value != "9007199254740993.125" || second.Output.Table.Rows[0][1].Value != "5.500" || first.PageBounds.Next == nil || *first.PageBounds.Next != 1 || second.PageBounds.Next != nil || first.PageBounds.Total != 2 || second.PageBounds.Total != 2 {
				t.Fatal("retained paging changed exact rows or page scope", first, second)
			}
			if !reflect.DeepEqual(first.Output.Table.Columns, columns) || !reflect.DeepEqual(first.Output.Table.Totals, second.Output.Table.Totals) || len(first.Output.Table.Totals) != 1 || first.Output.Table.Totals[0].Value.Value != "9007199254740998.625" {
				t.Fatal("paging changed units or recalculated whole-result total", first.Output.Table, second.Output.Table)
			}
		} else if first.Output.Chart == nil || !reflect.DeepEqual(*first.Output.Chart, *chart) {
			t.Fatal("delivery altered the approved KPI/trend", first)
		}
		if widget == "kpi" && (chart.KPIResult == nil || chart.KPIResult.Value.Exact != "9007199254740993.125" || len(chart.KPIResult.Sparkline) != 2 || chart.KPIResult.Sparkline[1].Exact != "5.500") {
			t.Fatal("KPI value or sparkline was reinterpreted", chart.KPIResult)
		}
		if widget == "trend" && (chart.Kind != charts.Line || len(chart.Points) != 2 || chart.Points[0].Category.Value != "2026-01-02" || chart.Points[1].Category.Value != "2026-01-03" || chart.Points[0].Value.Exact != "9007199254740993.125" || chart.Points[1].Value.Exact != "5.500") {
			t.Fatal("temporal trend lost approved coordinates or exact labels", chart.Points)
		}
		if redraw := view(widget, output.ID, 0); !reflect.DeepEqual(redraw, first) {
			t.Fatal("retained redraw changed stable data", widget)
		}
	}
	for _, missing := range readerScopes {
		denied := actor(slices.DeleteFunc(slices.Clone(readerScopes), func(s string) bool { return s == missing }))
		if out, err := delivery.View(ctx, denied, reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: "main", Widget: "trend", Output: "trend-main"}); err == nil || out.Output != nil || len(out.Pages) != 0 {
			t.Fatal("missing native action/run/preview/context disclosed payload", missing, out, err)
		}
	}
	for _, request := range []reporting.DeliveryViewRequest{
		{Kind: "report", Run: preview.ID, Page: "missing", Widget: "trend", Output: "trend-main"},
		{Kind: "report", Run: preview.ID, Page: "main", Widget: "missing", Output: "trend-main"},
		{Kind: "report", Run: preview.ID, Page: "main", Widget: "trend", Output: "missing"},
		{Kind: "report", Run: preview.ID, Page: "main", Widget: "trend", Output: "kpi-main"},
		{Kind: "report", Run: preview.ID, Page: "main", Widget: "intro", Output: "table-main"},
	} {
		if out, err := delivery.View(ctx, reader, request); err == nil || out.Output != nil || out.Text != nil || len(out.Pages) != 0 {
			t.Fatal("invalid page/widget/output path disclosed retained data", request, out, err)
		}
	}
	if f.f.f.lookups.Load() != beforeRetainedSource || f.attemptCount(t) != beforeAttempts+attempts || f.f.model.requests.Load() != beforeModel {
		t.Fatal("retained read/redraw/paging/denial executed source or model work")
	}
}
