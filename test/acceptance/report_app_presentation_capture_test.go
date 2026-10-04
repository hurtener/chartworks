package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// This journey captures only real public DTOs. The source fixture and immutable
// definition custody remain native; the optional browser recording contains no
// source connection, SQL, identity envelope, or private operational record.
func TestReportAppPresentationCapture(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	const sourceID, tableID, kpiID, reportID = "presentation-source", "presentation-table-copy", "presentation-kpi-copy", "presentation-report"
	definition := phase27Definition(t, f.f, f.blockAuthor, "SELECT CASE WHEN id=1 THEN '2026-01-02'::date ELSE '2026-01-03'::date END AS sale_date, amount, CASE WHEN id=1 THEN 1.2345e-7::double precision ELSE 2.7e-7::double precision END AS tiny_amount FROM analytics.sales ORDER BY id")
	columns := phase27Copy(t, definition.Outputs[0].Mapping.Columns)
	columns[0].Role, columns[0].DisplayLabel = "time", "Sale date"
	columns[0].Format = charts.Format{DatePattern: "date_medium", Locale: "en-US"}
	columns[1].Role, columns[1].DisplayLabel, columns[1].Aggregation = "measure", "Revenue", "sum"
	columns[1].Format = charts.Format{Unit: "revenue", Currency: "USD", FractionDigits: 3, Locale: "en-US"}
	columns[2].Role, columns[2].DisplayLabel = "measure", "Small amount"
	columns[2].Format = charts.Format{Unit: "revenue", Currency: "USD", FractionDigits: 3, Locale: "en-US"}
	for i := range columns {
		columns[i].Provenance = charts.Provenance{Version: 1, Source: definition.Source, SourceRevision: f.f.pack.Datasets[0].Source.SourceRevision}
	}
	columns[1].Provenance.Topic, columns[1].Provenance.TopicVersion, columns[1].Provenance.SemanticID = definition.Topics[0].Topic, definition.Topics[0].Version, "revenue"
	table := charts.Mapping{Version: charts.DisplayVersion, Kind: charts.Table, Columns: phase27Copy(t, columns), Bindings: charts.Bindings{Columns: []string{columns[0].ID, columns[1].ID, columns[2].ID}}, Order: []charts.Order{{Column: columns[0].ID, Direction: "asc"}}, Options: charts.DefaultOptions(), Table: &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: columns[0].ID, Visible: true}, {Column: columns[1].ID, Visible: true}, {Column: columns[2].ID, Visible: true}}, PageSize: 100, ShowTotals: true}}
	kpi := charts.Mapping{Version: charts.DisplayVersion, Kind: charts.KPI, Columns: phase27Copy(t, []charts.Column{columns[0], columns[2]}), Bindings: charts.Bindings{Category: columns[0].ID, Value: columns[2].ID}, Order: []charts.Order{{Column: columns[0].ID, Direction: "desc"}}, Options: charts.DefaultOptions(), KPI: &charts.KPIOptions{ValueRow: "last", ComparisonMode: "previous_row", ShowDelta: true, ShowPercentDelta: true, Sparkline: true, Thresholds: []charts.KPIThreshold{}}}
	definition.Outputs = []reporting.Output{{ID: "table-main", Kind: "table", Mapping: &table}, {ID: "kpi-main", Kind: "kpi", Mapping: &kpi}}
	for _, output := range definition.Outputs {
		if err := charts.ValidateMapping(ctx, charts.Data{Version: charts.Version, Columns: output.Mapping.Columns, Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}, *output.Mapping, charts.Defaults()); err != nil {
			t.Fatal("native synthetic mapping", output.ID, err)
		}
	}
	f.block(t, sourceID, definition)
	sourceSnapshot, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{Revision: 1}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, f.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	// Each native operation receives a bounded exact resource envelope. The
	// setup helper's wildcard authority never reaches the manual authoring seam.
	scopesFor := func(blocks []string, report bool, execution bool) []string {
		scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "topics.read", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant()}
		if !report {
			scopes = append(scopes, "charts.bind", "reporting.validate", "sources.read", "sources.query", "cw.topic.write:"+sourceSnapshot.State.Topic, "cw.source.query:"+definition.Source)
		}
		if report {
			scopes = append(scopes, "cw.report.read:"+reportID, "cw.report.write:"+reportID, "cw.report.preview:"+reportID)
		}
		if execution {
			scopes = append(scopes, "reporting.execute", "sources.read", "sources.query", "cw.report.execute:"+reportID, "cw.source.query:"+definition.Source)
		}
		for _, id := range blocks {
			scopes = append(scopes, "cw.block.read:"+id, "cw.block.preview:"+id)
			if !report {
				scopes = append(scopes, "cw.block.write:"+id)
			}
			if execution {
				scopes = append(scopes, "cw.block.execute:"+id)
			}
		}
		for _, ref := range sourceSnapshot.References {
			scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
			if !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
			}
		}
		if len(scopes) > 32 {
			t.Fatal("scope ceiling", len(scopes))
		}
		for _, scope := range scopes {
			if strings.Contains(scope, "*") {
				t.Fatal("unbounded authoring scope")
			}
		}
		return scopes
	}
	actor := func(blocks []string, report, execution bool) identity.Envelope {
		return phase27Actor(t, f.f, f.author.User(), scopesFor(blocks, report, execution))
	}
	type counters struct {
		SourceReads   int   `json:"source_reads"`
		SourceLookups int64 `json:"source_lookups"`
		ModelCalls    int64 `json:"model_calls"`
	}
	measure := func() counters { return counters{f.attemptCount(t), f.f.f.lookups.Load(), f.f.model.requests.Load()} }
	delta := func(before counters) counters {
		after := measure()
		return counters{after.SourceReads - before.SourceReads, after.SourceLookups - before.SourceLookups, after.ModelCalls - before.ModelCalls}
	}
	noWork := func(before counters, name string) counters {
		t.Helper()
		d := delta(before)
		if d != (counters{}) {
			t.Fatal(name, "executed hidden source/model work", d)
		}
		return d
	}
	before := measure()
	sourceReadRequest := reporting.AuthoringBlockReadRequest{Block: sourceID, Revision: 1}
	source, err := service.ReadBlock(ctx, actor([]string{sourceID}, false, false), sourceReadRequest)
	if err != nil || source.Block.Evidence == nil || source.Block.Private {
		t.Fatal("published source capability read", err)
	}
	d := phase29Text("Presentation evidence")
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	tableWidget := phase29BlockWidget("table", sourceID, 0, "table-main")
	tableWidget.Grid.Height = 4
	tableWidget.Block.Revision = 1
	tableWidget.Block.Policy = "published"
	tableWidget.Presentation.Title = "Revenue table"
	kpiWidget := phase29BlockWidget("kpi", sourceID, 4, "kpi-main")
	kpiWidget.Grid.Height = 4
	kpiWidget.Block.Revision = 1
	kpiWidget.Block.Policy = "published"
	kpiWidget.Presentation.Title = "Revenue KPI"
	d.ReportPages = []reporting.ReportPage{{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{tableWidget, kpiWidget}}, {ID: "notes", Title: "Notes", Widgets: []reporting.Widget{{ID: "note", Kind: "text", Grid: reporting.GridCell{Width: 12, Height: 2}, Text: &reporting.TextWidget{Format: "plain", Text: "Synthetic notes remain unchanged during field formatting."}}}}}
	reportActor := actor([]string{sourceID}, true, true)
	initialCreateRequest := reporting.AuthoringCreateRequest{ID: reportID, Definition: d}
	state, err := service.Create(ctx, reportActor, initialCreateRequest)
	if err != nil {
		t.Fatal(err)
	}
	readRequest := reporting.AuthoringReadRequest{Report: reportID}
	initialReport, err := service.Read(ctx, reportActor, readRequest)
	if err != nil {
		t.Fatal(err)
	}
	draftsRequest := reporting.DraftListRequest{After: "", Limit: 40}
	initialDrafts, err := service.Drafts(ctx, reportActor, draftsRequest)
	if err != nil {
		t.Fatal(err)
	}
	capabilityRequest := reporting.AuthoringCapabilitiesRequest{Report: reportID}
	capabilities, err := service.Capabilities(ctx, reportActor, capabilityRequest)
	if err != nil || !capabilities.CanPreview {
		t.Fatal("manual capabilities", capabilities, err)
	}
	globalCapabilitiesRequest := reporting.AuthoringCapabilitiesRequest{}
	globalCapabilities, err := service.Capabilities(ctx, reportActor, globalCapabilitiesRequest)
	if err != nil {
		t.Fatal(err)
	}
	catalogRequest := reporting.DeliverySearchRequest{Kind: "report", Query: "", After: "", Limit: 40, Locale: "en-US"}
	catalog, err := delivery.Search(ctx, reportActor, catalogRequest)
	if err != nil || len(catalog.Items) != 0 {
		t.Fatal("empty unpublished report catalog", err)
	}
	initialCounts := noWork(before, "create/read/capabilities/catalog")
	stages := map[string]any{}
	explicitTotals := counters{}
	addExplicit := func(c counters) {
		explicitTotals.SourceReads += c.SourceReads
		explicitTotals.SourceLookups += c.SourceLookups
		explicitTotals.ModelCalls += c.ModelCalls
	}
	type stageResult struct {
		Block  reporting.AuthoringBlockView
		Report reporting.DocumentView
		Views  map[string]reporting.DeliveryViewResult
	}
	var baseline stageResult
	currentReport := initialReport
	stage := func(name string, block reporting.AuthoringBlockView, mutation any, mutationBefore counters, widget int, validate bool) stageResult {
		t.Helper()
		record := map[string]any{"block": block, "block_read_request": reporting.AuthoringBlockReadRequest{Block: block.Block.State.ID, Revision: block.Block.Revision}}
		counts := map[string]counters{"mutation": noWork(mutationBefore, name+" mutation")}
		if mutation != nil {
			record["mutation_request"] = mutation
			if !block.Block.Private || block.Block.Evidence != nil || block.Block.State.PublishedRevision != 0 || block.Block.ExecutionDigest != source.Block.ExecutionDigest {
				t.Fatal("copy/amend inherited approval or altered execution", name)
			}
			if block.Block.Source != source.Block.Source || block.Block.Context != source.Block.Context || !reflect.DeepEqual(block.Block.ExpectedSchema, source.Block.ExpectedSchema) || !reflect.DeepEqual(block.Block.Parameters, source.Block.Parameters) || !reflect.DeepEqual(block.Block.Topics, source.Block.Topics) || !reflect.DeepEqual(block.Block.AmountCompleteness, source.Block.AmountCompleteness) {
				t.Fatal("presentation changed canonical block metadata", name)
			}
			for i, output := range block.Block.Outputs {
				actual := phase27Copy(t, output)
				actual.Mapping.Presentation = nil
				if !reflect.DeepEqual(actual, source.Block.Outputs[i]) {
					t.Fatal("presentation changed canonical output metadata", name, output.ID)
				}
			}
			beforeSave := measure()
			next := phase27Copy(t, currentReport.Definition)
			next.ReportPages[0].Widgets[widget].Block = &reporting.BlockWidget{Block: block.Block.State.ID, Revision: block.Block.Revision, Digest: block.Block.Digest, Policy: "private_preview", Outputs: []string{[]string{"table-main", "kpi-main"}[widget]}}
			saveRequest := reporting.AuthoringSaveRequest{Report: reportID, ExpectedVersion: state.Version, Revision: currentReport.Revision, Definition: next}
			ids := []string{sourceID}
			for _, w := range next.ReportPages[0].Widgets {
				if !slices.Contains(ids, w.Block.Block) {
					ids = append(ids, w.Block.Block)
				}
			}
			reportActor = actor(ids, true, true)
			state, err = service.Save(ctx, reportActor, saveRequest)
			if err != nil {
				t.Fatal(name, "report save", err)
			}
			currentReport, err = service.Read(ctx, reportActor, readRequest)
			if err != nil || !reflect.DeepEqual(currentReport.Definition, next) || !reflect.DeepEqual(currentReport.Definition.ReportPages[1], initialReport.Definition.ReportPages[1]) {
				t.Fatal(name, "report reopen", err)
			}
			record["report_save_request"], record["report_state"] = saveRequest, state
			counts["save_reopen"] = noWork(beforeSave, name+" save/reopen")
		}
		record["report"], record["report_read_request"] = currentReport, readRequest
		drafts, err := service.Drafts(ctx, reportActor, draftsRequest)
		if err != nil {
			t.Fatal(err)
		}
		record["drafts"] = drafts
		if validate {
			vBefore := measure()
			request := reporting.AuthoringBlockValidateRequest{Block: block.Block.State.ID, ExpectedVersion: block.Block.State.Version, Revision: block.Block.Revision, Digest: block.Block.Digest, Arguments: []reporting.Argument{}, Resolution: reporting.Resolution{At: time.Now().UTC().Truncate(time.Second).Add(123 * time.Millisecond), Timezone: "UTC"}}
			validation, err := service.ValidateBlock(ctx, actor([]string{block.Block.State.ID}, false, false), request)
			if err != nil {
				t.Fatal(name, "explicit validation", err)
			}
			counts["explicit_validation"] = delta(vBefore)
			addExplicit(counts["explicit_validation"])
			if counts["explicit_validation"].SourceReads != 1 || counts["explicit_validation"].ModelCalls != 0 {
				t.Fatal("validation accounting", counts)
			}
			block, err = service.ReadBlock(ctx, actor([]string{block.Block.State.ID}, false, false), reporting.AuthoringBlockReadRequest{Block: block.Block.State.ID, Revision: block.Block.Revision})
			if err != nil || block.Block.Evidence == nil || block.Block.Evidence.DefinitionDigest != block.Block.Digest || block.Block.Evidence.Revision != block.Block.Revision {
				t.Fatal("validation pins", err)
			}
			record["validation_request"], record["validation"], record["validated_block"] = request, validation, block
		} else {
			record["validated_block"] = block
		}
		previewBefore := measure()
		previewRequest := reporting.AuthoringPreviewRequest{Report: reportID, Revision: currentReport.Revision, Key: "presentation-browser-" + name, Pages: []reporting.PageInput{}, Resolution: reporting.Resolution{At: time.Now().UTC().Truncate(time.Second).Add(123 * time.Millisecond), Timezone: "UTC"}}
		preview, err := service.Preview(ctx, reportActor, previewRequest)
		if err != nil || !preview.Private || preview.Complete {
			t.Fatal(name, "private admission", err)
		}
		counts["preview_admission"] = noWork(previewBefore, name+" preview admission")
		executeRequest := reporting.AuthoringExecuteRequest{Run: preview.ID}
		complete, err := service.Execute(ctx, reportActor, executeRequest)
		if err != nil || !complete.Private || !complete.Complete || complete.State != "completed" {
			t.Fatal(name, "explicit private execution", complete, err)
		}
		counts["explicit_preview_execution"] = delta(previewBefore)
		addExplicit(counts["explicit_preview_execution"])
		if counts["explicit_preview_execution"].SourceReads != preview.QueryGroups || preview.QueryGroups < 1 || counts["explicit_preview_execution"].ModelCalls != 0 {
			t.Fatal("preview accounting", counts)
		}
		reader := phase27Actor(t, f.f, f.author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + reportID, "cw.execution_context.use:" + definition.Context})
		retainedBefore := measure()
		rootRequest := reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Offset: 0, Limit: 100}
		root, err := delivery.View(ctx, reader, rootRequest)
		if err != nil || !root.Summary.Private {
			t.Fatal("retained root", err)
		}
		requests := map[string]reporting.DeliveryViewRequest{}
		views := map[string]reporting.DeliveryViewResult{}
		for _, w := range []struct{ name, page, output string }{{"table", "analysis", "table-main"}, {"kpi", "analysis", "kpi-main"}, {"note", "notes", ""}} {
			key := w.name
			if key == "note" {
				key = "notes"
			}
			request := reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: w.page, Widget: w.name, Output: w.output, Offset: 0, Limit: 100}
			view, err := delivery.View(ctx, reader, request)
			if err != nil || !view.Summary.Private || view.Selection.Run != preview.ID {
				t.Fatal(name, "retained output", key, err)
			}
			redraw, err := delivery.View(ctx, reader, request)
			if err != nil || !reflect.DeepEqual(redraw, view) {
				t.Fatal("retained redraw", err)
			}
			requests[key], views[key] = request, view
		}
		if views["table"].Output == nil || views["table"].Output.Table == nil || views["kpi"].Output == nil || views["kpi"].Output.Chart == nil || views["notes"].Text == nil {
			t.Fatal("public retained DTO shape")
		}
		tbl, metric := views["table"].Output.Table, views["kpi"].Output.Chart
		if len(tbl.Rows) != 2 || tbl.Rows[0][1].Value != "9007199254740993.125" || tbl.Rows[1][1].Value != "5.500" || len(tbl.Totals) != 1 || tbl.Totals[0].Value.Value != "9007199254740998.625" || metric.KPIResult == nil || !strings.Contains(metric.KPIResult.Value.Exact, "e-") || metric.KPIResult.PercentDelta == nil || metric.KPIResult.PercentDelta.Exact == "" || metric.KPIResult.Comparison == nil || metric.KPIResult.Comparison.Exact == "" || metric.KPIResult.Delta == nil || metric.KPIResult.Delta.Exact == "" || views["notes"].Text.Text != initialReport.Definition.ReportPages[1].Widgets[0].Text.Text {
			t.Fatal("retained exact values or notes", name)
		}
		if name != "source" {
			original := baseline.Views["kpi"].Output.Chart
			if !reflect.DeepEqual(metric.KPIResult, original.KPIResult) || !reflect.DeepEqual(metric.Mapping.Columns, original.Mapping.Columns) || !reflect.DeepEqual(tbl.Rows, baseline.Views["table"].Output.Table.Rows) || !reflect.DeepEqual(tbl.Totals, baseline.Views["table"].Output.Table.Totals) {
				t.Fatal("display edit changed canonical data/derived-percent", name)
			}
			for _, c := range metric.Columns {
				prior := columns[slices.IndexFunc(columns, func(column charts.Column) bool { return column.ID == c.ID })]
				prior.Format.FractionDigits = c.Format.FractionDigits
				if !reflect.DeepEqual(c, prior) {
					t.Fatal("KPI changed units/provenance", name)
				}
			}
			for i, c := range tbl.Columns {
				prior := columns[i]
				prior.DisplayLabel = c.DisplayLabel
				prior.Format.FractionDigits = c.Format.FractionDigits
				if !reflect.DeepEqual(c, prior) {
					t.Fatal("table changed canonical units/provenance", name)
				}
			}
		}
		after, err := service.Read(ctx, reportActor, readRequest)
		if err != nil || !reflect.DeepEqual(after, currentReport) {
			t.Fatal("preview changed report", err)
		}
		counts["retained_read_redraw_reopen"] = noWork(retainedBefore, name+" retained reads")
		record["preview_request"], record["preview"], record["execute_request"], record["complete"] = previewRequest, preview, executeRequest, complete
		record["view_root_request"], record["view_root"], record["view_requests"], record["views"], record["counts"], record["report_after_preview"] = rootRequest, root, requests, views, counts, after
		stages[name] = record
		return stageResult{block, currentReport, views}
	}
	baseline = stage("source", source, nil, measure(), 0, false)
	label, digits, scientificDigits := "Displayed total", 2, 0
	patch := charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: columns[1].ID, Set: &charts.ColumnPresentationSet{DisplayLabel: &label, FractionDigits: &digits}}, {Column: columns[2].ID, Set: &charts.ColumnPresentationSet{FractionDigits: &scientificDigits}}}}
	copyRequest := reporting.AuthoringBlockPresentationCopyRequest{Block: sourceID, NewBlock: tableID, ExpectedVersion: source.Block.State.Version, Revision: source.Block.Revision, Digest: source.Block.Digest, Output: "table-main", Presentation: patch}
	mutationBefore := measure()
	copied, err := service.CopyBlockPresentation(ctx, actor([]string{sourceID, tableID}, false, false), copyRequest)
	if err != nil {
		t.Fatal(err)
	}
	copiedStage := stage("copied_table", copied, copyRequest, mutationBefore, 0, true)
	// Capture the native HTTP error projection rather than inventing a browser
	// conflict shape. The duplicate target is rejected before any source work.
	errorBefore := measure()
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.AuthoringHandler(f.f.f.token.verifier, service, http.NotFoundHandler()))
	raw, err := json.Marshal(copyRequest)
	if err != nil {
		t.Fatal(err)
	}
	claims := f.f.f.token.claims(f.author.Tenant(), f.author.User(), scopesFor([]string{sourceID, tableID}, false, false))
	claims["session"] = "phase27-session"
	response := callProtected(t, handler, "POST", reportingapi.AuthoringPath+"block_copy", f.f.f.token.sign(t, claims, nil), string(raw), map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusConflict {
		t.Fatal("native duplicate-copy conflict", response.Code, response.Body.String())
	}
	var conflict any
	if err := json.Unmarshal(response.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	bindings, err := reportingapi.AuthoringMCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.New(f.f.f.token.verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	claims["aud"] = "chartworks:mcp"
	claims["scopes"] = append(scopesFor([]string{sourceID, tableID}, false, false), "mcp.use")
	bearer := f.f.f.token.sign(t, claims, nil)
	client, err := mcp.Client(func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	mcpConflict, err := client.CallTool(ctx, "reporting_authoring_block_copy_v1", raw)
	if err != nil || mcpConflict == nil || !mcpConflict.IsError {
		t.Fatal("native MCP conflict", err)
	}
	conflictCounts := noWork(errorBefore, "duplicate copy")
	label2, digits2 := "Revised display", 0
	amendment := reporting.AuthoringBlockPresentationRequest{Block: tableID, ExpectedVersion: copiedStage.Block.Block.State.Version, Revision: copiedStage.Block.Block.Revision, Digest: copiedStage.Block.Block.Digest, Output: "table-main", Presentation: charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: columns[1].ID, Set: &charts.ColumnPresentationSet{DisplayLabel: &label2, FractionDigits: &digits2}}}}}
	mutationBefore = measure()
	amended, err := service.PatchBlockPresentation(ctx, actor([]string{tableID}, false, false), amendment)
	if err != nil {
		t.Fatal(err)
	}
	amendedStage := stage("amended_table", amended, amendment, mutationBefore, 0, true)
	reset := reporting.AuthoringBlockPresentationRequest{Block: tableID, ExpectedVersion: amendedStage.Block.Block.State.Version, Revision: amendedStage.Block.Block.Revision, Digest: amendedStage.Block.Block.Digest, Output: "table-main", Presentation: charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: columns[1].ID, Reset: []charts.PresentationField{charts.PresentationDisplayLabel, charts.PresentationFractionDigits}}, {Column: columns[2].ID, Reset: []charts.PresentationField{charts.PresentationFractionDigits}}}}}
	mutationBefore = measure()
	resetBlock, err := service.PatchBlockPresentation(ctx, actor([]string{tableID}, false, false), reset)
	if err != nil || resetBlock.Block.Digest != source.Block.Digest || resetBlock.Block.Outputs[0].Mapping.Presentation != nil {
		t.Fatal("reset exact inheritance", err)
	}
	stage("reset_table", resetBlock, reset, mutationBefore, 0, true)
	kpiCopyRequest := reporting.AuthoringBlockPresentationCopyRequest{Block: sourceID, NewBlock: kpiID, ExpectedVersion: source.Block.State.Version, Revision: source.Block.Revision, Digest: source.Block.Digest, Output: "kpi-main", Presentation: charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: columns[2].ID, Set: &charts.ColumnPresentationSet{FractionDigits: &digits2}}}}}
	mutationBefore = measure()
	kpiCopy, err := service.CopyBlockPresentation(ctx, actor([]string{sourceID, kpiID}, false, false), kpiCopyRequest)
	if err != nil {
		t.Fatal(err)
	}
	stage("formatted_kpi", kpiCopy, kpiCopyRequest, mutationBefore, 1, true)
	sourceAfter, err := service.ReadBlock(ctx, actor([]string{sourceID}, false, false), sourceReadRequest)
	if err != nil || !reflect.DeepEqual(sourceAfter, source) {
		t.Fatal("source metadata mutated", err)
	}
	if delta(before) != explicitTotals {
		t.Fatal("unaccounted source/model work outside explicit validation and preview", delta(before), explicitTotals)
	}
	if delta(before).ModelCalls != 0 {
		t.Fatal("presentation journey invoked model")
	}
	// The scanner is part of the acceptance assertion even when export is off.
	capture := map[string]any{"capture": map[string]any{"version": 1, "test": "TestReportAppPresentationCapture", "backend": "real PostgreSQL synthetic source", "source_base": "3806aba74b5d2df2cd4dbfeb4ff30f6ab0d337e5", "normalizations": []string{}, "total_counts": delta(before)}, "source_block_read_request": sourceReadRequest, "source_block": source, "source_block_after": sourceAfter, "initial_create_request": initialCreateRequest, "initial_report": initialReport, "initial_report_read_request": readRequest, "initial_drafts": initialDrafts, "drafts_request": draftsRequest, "capabilities_request": capabilityRequest, "capabilities": capabilities, "global_capabilities_request": globalCapabilitiesRequest, "global_capabilities": globalCapabilities, "published_catalog_request": catalogRequest, "published_catalog": catalog, "initial_counts": initialCounts, "stages": stages, "response_contract": map[string]any{"errors": map[string]any{"duplicate_copy": map[string]any{"request": copyRequest, "status": response.Code, "body": conflict, "mcp": mcpConflict, "counts": conflictCounts}}}}
	wire, err := json.MarshalIndent(capture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationPublicCapture(t, wire, definition.SQL)
	if path := os.Getenv("CHARTWORKS_PRESENTATION_DTO_PATH"); path != "" {
		if err := os.WriteFile(path, wire, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("native public capture: %d bytes, counts %+v", len(wire), delta(before))
}

// The closed public QueryLimits shape has a scalar query_attempts ceiling.
// That metadata is distinct from private RunSnapshot query_attempts records.
func presentationPublicCaptureError(wire []byte, sourceSQL string) error {
	const maxBytes, maxNodes, maxDepth = 1 << 20, 60000, 64
	if len(wire) > maxBytes || sourceSQL != "" && strings.Contains(string(wire), sourceSQL) {
		return errors.New("public capture size/SQL bound")
	}
	var decoded any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		return err
	}
	nodes := 0
	var inspect func(any, int, string) error
	inspect = func(value any, depth int, parent string) error {
		nodes++
		if nodes > maxNodes || depth > maxDepth {
			return errors.New("public capture recursion bound")
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				switch strings.ToLower(key) {
				case "query_attempts":
					integer := func(name string, min, max float64, zero bool) bool {
						n, ok := v[name].(float64)
						return ok && ((zero && n == 0) || (n >= min && n <= max)) && n == float64(int(n))
					}
					if parent != "query_limits" || len(v) != 4 || !integer("max_rows", 0, 10000, false) || !integer("max_bytes", 1024, 4<<20, true) || !integer("timeout_ms", 1000, 60000, true) || !integer("query_attempts", 0, 3, false) {
						return errors.New("private attempt record or invalid public query limit")
					}
				case "sql", "statement", "statements", "attempt", "attempts", "remote", "manifest", "session", "token", "credential", "credentials", "authorization", "password", "dsn", "connection_string", "native_pid", "control_handle", "execution_proof", "envelope", "secret", "secrets", "api_key", "access_token", "refresh_token", "bearer", "tokens", "native_control", "source_control", "request_control", "operation_record", "lease", "fence":
					return errors.New("private custody in public DTO capture")
				}
				if err := inspect(child, depth+1, key); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := inspect(child, depth+1, parent); err != nil {
					return err
				}
			}
		case string:
			lower := strings.ToLower(v)
			for _, prefix := range []string{"postgres://", "postgresql://", "bearer ", "-----begin private key"} {
				if strings.Contains(lower, prefix) {
					return errors.New("private value in public DTO capture")
				}
			}
		}
		return nil
	}
	return inspect(decoded, 0, "")
}

func assertPresentationPublicCapture(t *testing.T, wire []byte, sourceSQL string) {
	t.Helper()
	if err := presentationPublicCaptureError(wire, sourceSQL); err != nil {
		t.Fatal(err)
	}
}

func TestPresentationPublicDTOBoundary(t *testing.T) {
	valid := `{"query_limits":{"max_rows":1000,"max_bytes":1048576,"timeout_ms":60000,"query_attempts":3}}`
	if err := presentationPublicCaptureError([]byte(valid), ""); err != nil {
		t.Fatal("public query ceiling rejected", err)
	}
	for _, raw := range []string{
		`{"query_attempts":[]}`, `{"query_attempts":{"id":"private"}}`, `{"query_attempts":3}`,
		`{"query_limits":{"max_rows":1000,"max_bytes":1048576,"timeout_ms":60000,"query_attempts":[]}}`,
		`{"query_limits":{"max_rows":1000,"max_bytes":1048576,"timeout_ms":60000,"query_attempts":{"id":"private"}}}`,
		`{"query_limits":{"max_rows":1000,"max_bytes":1048576,"timeout_ms":60000,"query_attempts":4}}`,
		`{"query_limits":{"max_rows":1000,"max_bytes":1048576,"timeout_ms":60000,"query_attempts":3,"remote":"private"}}`,
		`{"nested":[{"session":"private"}]}`, `{"nested":{"authorization":"private"}}`,
		`{"safe_name":"postgres://private"}`, `{"safe_name":"Bearer private"}`,
		strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65),
		"[" + strings.Repeat("null,", 60000) + "null]",
		`"` + strings.Repeat("x", 1<<20) + `"`,
	} {
		if err := presentationPublicCaptureError([]byte(raw), ""); err == nil {
			t.Fatal("unsafe capture admitted")
		}
	}
	if err := presentationPublicCaptureError([]byte(`{"safe_name":"SELECT synthetic"}`), "SELECT synthetic"); err == nil {
		t.Fatal("SQL value escaped")
	}
}
