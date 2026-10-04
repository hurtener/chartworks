package acceptance

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
	nethtml "golang.org/x/net/html"
)

type phase32PageExpectation struct {
	id, container string
	page          reporting.ReportPage
	cells         []string
}

type phase32PagesCase struct {
	name, kind, run string
	private         bool
	pages           []phase32PageExpectation
}

type phase32PagesFixture struct {
	f      *phase30Fixture
	actor  identity.Envelope
	scopes []string
	cases  []phase32PagesCase
}

// This independently runnable domain check shares the exact manifests consumed
// by AC03/v3-pages. It does not qualify the isolated renderer or replace AC03.
func TestPhase32ReportPagesManifests(t *testing.T) {
	phase32NewPagesFixture(t)
}

func phase32NewPagesFixture(t *testing.T) phase32PagesFixture {
	t.Helper()
	f := newPhase30Fixture(t, false)
	d := f.domain
	ctx := t.Context()
	block := phase27Copy(t, d.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
	block.Outputs = block.Outputs[:1]
	for i := range block.Outputs[0].Mapping.Columns {
		column := &block.Outputs[0].Mapping.Columns[i]
		if column.Name == "amount" {
			column.Format.FractionDigits = 3
			column.Format.Locale = "en-US"
		}
	}
	d.block(t, "renderer-page-source", block)
	definition := phase29Text("Retained inline pages")
	definition.SchemaVersion, definition.Widgets = reporting.PagedDocumentVersion, nil
	expected := []phase32PageExpectation{}
	for i, id := range []string{"overview", "detail"} {
		text := phase27Copy(t, phase29Text("").Widgets[0])
		text.ID, text.Text.Format, text.Text.Text = "note-"+id, "plain", "Retained "+id+" <note>"
		text.Presentation.Title = "Note " + id
		chart := phase29BlockWidget("chart-"+id, "renderer-page-source", 1, "table-main")
		chart.Block.Revision = 1
		chart.Block.Limits = &reporting.QueryLimits{MaxRows: 10, MaxBytes: 64 << 10, TimeoutMillis: 5000, QueryAttempts: 1}
		chart.Grid.Height = 8
		chart.Presentation.Title = "Values " + id
		chart.Bindings = []reporting.FilterBinding{{Filter: "minimum", Parameter: "minimum"}}
		page := reporting.ReportPage{ID: id, Title: "Page " + id + " <retained>", Locale: "en-US", Timezone: "UTC", Widgets: []reporting.Widget{text, chart}, Filters: []reporting.ReportFilter{{Label: "Minimum", Parameter: reporting.Parameter{Name: "minimum", Type: "integer", Default: &reporting.Value{Literal: fmt.Sprint(i + 1)}}}}}
		cells := []string{"1", "9,007,199,254,740,993.125", "2", "5.500"}
		if i == 1 {
			page.Timezone = "America/Argentina/Buenos_Aires"
			cells = []string{"2", "5.500"}
		}
		definition.ReportPages = append(definition.ReportPages, page)
		expected = append(expected, phase32PageExpectation{id: id, page: page, cells: cells})
	}
	state := d.report(t, "renderer-paged-report", definition, false)
	preview, err := d.compositions.Admit(ctx, d.execute, "report", state.ID, reporting.CompositionRequest{Key: "renderer-private-pages", Reference: reporting.DocumentReference{Revision: state.DraftRevision}, Preview: true})
	if err != nil || !preview.Private || preview.QueryGroups != 2 {
		t.Fatal("private inline-page admission", preview, err)
	}
	if completed, err := d.compositions.Run(ctx, d.execute, preview.ID, false); err != nil || !completed.Complete {
		t.Fatal("private inline-page execution", completed, err)
	}
	// The old preview must retain actor/session custody after publication. The
	// dashboard and scheduled occurrence use the explicitly published revision.
	state = phase29Publish(t, d.documents, d.author, state)
	dashboard := phase29Text("Nested retained pages")
	dashboard.Widgets = nil
	nestedPages := []phase32PageExpectation{}
	for _, container := range []string{"first-group", "second-group"} {
		dashboard.Pages = append(dashboard.Pages, reporting.DocumentPage{ID: container, Title: container, Report: state.ID, Revision: state.PublishedRevision})
		for _, want := range expected {
			want.id = reporting.DashboardCanvasID(container, definition, want.page.ID)
			want.container = container
			nestedPages = append(nestedPages, want)
		}
	}
	dashboardState, err := d.documents.Create(ctx, d.author, "dashboard", "renderer-paged-dashboard", dashboard)
	if err != nil {
		t.Fatal(err)
	}
	dashboardState = phase29Publish(t, d.documents, d.author, dashboardState)
	nested, err := d.compositions.Admit(ctx, d.execute, "dashboard", dashboardState.ID, reporting.CompositionRequest{Key: "renderer-nested-pages"})
	if err != nil {
		t.Fatal(err)
	}
	if completed, err := d.compositions.Run(ctx, d.execute, nested.ID, false); err != nil || !completed.Complete {
		t.Fatal("nested inline-page execution", completed, err)
	}
	job := f.submit(t, "renderer-page-occurrence", phase30Target("report", state.ID))
	if err := f.queue.RunOnce(ctx); err != nil {
		t.Fatal("queued inline-page execution", err)
	}
	if done := f.get(t, job.ID); done.State != "succeeded" || done.ManifestHash != job.ManifestHash || done.Delivery == nil || done.Delivery.Catalog != "available" {
		t.Fatal("scheduled inline-page receipt", done)
	}
	cases := []phase32PagesCase{{"private-report", "report", preview.ID, true, expected}, {"nested-dashboard", "dashboard", nested.ID, false, nestedPages}, {"scheduled-report", "report", job.ID, false, expected}}
	scopes := []string{"reporting.read", "reporting.preview", "reporting.export", "cw.report.read:" + state.ID, "cw.report.preview:" + state.ID, "cw.execution_context.use:" + d.base.Context}
	for _, tc := range cases {
		scopes = append(scopes, "cw.run.read:"+tc.run, "cw.run.export:"+tc.run)
	}
	actor := phase27Actor(t, d.f, d.execute.User(), scopes)
	for _, tc := range cases {
		// Inspect the real immutable storage pin separately from the public
		// delivery checks. Execution recovery correctly belongs to the original
		// actor/session, which is the broker executor for the scheduled case.
		var body []byte
		var digest string
		if err := support.Raw(t, d.f.f.dsn).QueryRow(ctx, `SELECT p.manifest,c.manifest_digest FROM chartworks.composition_run_payloads p JOIN chartworks.composition_runs c USING(tenant_id,operation_id) WHERE p.tenant_id=$1 AND p.operation_id=$2`, actor.Tenant(), tc.run).Scan(&body, &digest); err != nil {
			t.Fatal(tc.name, "persisted manifest", err)
		}
		sealed, err := reporting.DecodeCompositionManifest(body, digest)
		if err != nil || sealed.Version != reporting.PagedCompositionVersion || sealed.Private != tc.private || len(sealed.Pages) != len(tc.pages) {
			t.Fatal(tc.name, "complete paged manifest", err)
		}
		view, err := f.delivery.View(ctx, actor, reporting.DeliveryViewRequest{Kind: tc.kind, Run: tc.run, Limit: 10})
		if err != nil || view.Summary.Private != tc.private || len(view.Pages) != len(tc.pages) {
			t.Fatal(tc.name, "complete retained page projection", view, err)
		}
		for i, want := range tc.pages {
			page := sealed.Pages[i]
			if page.ID != want.id || page.Report != state.ID || page.Revision != state.PublishedRevision || page.ReportPage != want.page.ID || page.ContainerPage != want.container || page.Locale != want.page.Locale || page.Timezone != want.page.Timezone || len(page.Widgets) != len(want.page.Widgets) {
				t.Fatal(tc.name, "exact page pin/settings", i, page)
			}
			for j, widget := range page.Widgets {
				if !reflect.DeepEqual(widget.Definition, want.page.Widgets[j]) {
					t.Fatal(tc.name, "widget changed in retained manifest", i, j)
				}
			}
			selected, err := f.delivery.View(ctx, actor, reporting.DeliveryViewRequest{Kind: tc.kind, Run: tc.run, Page: want.id, Widget: want.page.Widgets[1].ID, Limit: 10})
			if err != nil || selected.Output == nil || selected.Output.Table == nil || len(selected.Output.Table.Rows)*2 != len(want.cells) || selected.Timezone != want.page.Timezone {
				t.Fatal(tc.name, "same-dataset page filter/settings isolation", i, selected, err)
			}
		}
	}
	return phase32PagesFixture{f: f, actor: actor, scopes: scopes, cases: cases}
}

// AC03 uses the real PostgreSQL composition, delivery, schedule and rendition
// stores. Every static output crosses the normal namespace/cgroup Process;
// phase32Options is unchanged and there is no local-processor fallback.
func phase32ReportPages(t *testing.T) {
	p := phase32NewPagesFixture(t)
	d := p.f.domain
	process, err := rendering.NewProcess(mustExecutable32(t), phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	service, err := rendering.NewManaged(p.f.delivery, d.f.f.db, process, 4<<20, phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	queries, lookups, models := d.attemptCount(t), d.f.f.lookups.Load(), d.f.model.requests.Load()
	for _, tc := range p.cases {
		t.Run(tc.name, func(t *testing.T) {
			request := rendering.Request{View: reporting.DeliveryViewRequest{Kind: tc.kind, Run: tc.run, Limit: 10}, Full: true, Format: "html", Theme: "light", Width: 800, Height: 420}
			for _, format := range []string{"html", "svg"} {
				request.Format = format
				rendered, err := service.Generate(t.Context(), p.actor, request)
				if err != nil || rendered.ID == "" || rendered.Digest == "" || rendered.SourceDigest == "" {
					t.Fatal(format, "isolated full-page rendering", err)
				}
				if format == "html" {
					phase32AssertPagesHTML(t, rendered.Content, tc.pages)
				} else {
					if rendered.Height != len(tc.pages)*408 || strings.Count(rendered.Content, `data-kind="table"`) != len(tc.pages) {
						t.Fatal("SVG lost page extent or table", rendered.Height)
					}
					for _, id := range []string{"overview", "detail"} {
						if strings.Count(rendered.Content, "Page "+id+" &lt;retained&gt;") != len(tc.pages)/2 || strings.Count(rendered.Content, "Retained "+id+" &lt;note&gt;") != len(tc.pages)/2 {
							t.Fatal("SVG lost page title/text", id)
						}
					}
					if strings.Count(rendered.Content, "9,007,199,254,740,993.125") != len(tc.pages)/2 || strings.Count(rendered.Content, "5.500") != len(tc.pages) {
						t.Fatal("SVG changed exact page-local values")
					}
				}
				record, err := d.f.f.db.ReadRendition(t.Context(), p.actor.Tenant(), rendered.ID)
				if err != nil || record.Private != tc.private || record.Actor != p.actor.User() || record.Session != p.actor.Session() {
					t.Fatal("rendition custody changed", err)
				}
				if retained, err := service.Read(t.Context(), p.actor, rendering.ReadRequest{ID: rendered.ID}); err != nil || retained.Content != rendered.Content || retained.Digest != rendered.Digest {
					t.Fatal("retained rendition changed", err)
				}
				deniedScopes := slices.DeleteFunc(slices.Clone(p.scopes), func(scope string) bool { return scope == "cw.execution_context.use:"+d.base.Context })
				denied := phase27Actor(t, d.f, p.actor.User(), deniedScopes)
				if output, err := service.Read(t.Context(), denied, rendering.ReadRequest{ID: rendered.ID}); err == nil || output.Content != "" {
					t.Fatal("withdrawn context exposed retained bytes", err)
				}
				if tc.private {
					other := phase27Actor(t, d.f, "renderer-other-reader", p.scopes)
					if output, err := service.Read(t.Context(), other, rendering.ReadRequest{ID: rendered.ID}); err == nil || output.Content != "" {
						t.Fatal("later publication expanded private rendition audience", err)
					}
					if output, err := service.Generate(t.Context(), other, request); err == nil || output.Content != "" {
						t.Fatal("foreign actor rendered private pages", err)
					}
					withoutPreview := phase27Actor(t, d.f, p.actor.User(), slices.DeleteFunc(slices.Clone(p.scopes), func(scope string) bool { return strings.HasPrefix(scope, "cw.report.preview:") }))
					if output, err := service.Read(t.Context(), withoutPreview, rendering.ReadRequest{ID: rendered.ID}); err == nil || output.Content != "" {
						t.Fatal("published report reach exposed an old private rendition", err)
					}
				}
			}
			for _, format := range []string{"png", "pdf", "csv", "json"} {
				request.Format = format
				if output, err := service.Generate(t.Context(), p.actor, request); !errors.Is(err, rendering.ErrInvalid) || output.Content != "" || output.ID != "" {
					t.Fatal("unsupported full output silently flattened pages", format, err)
				}
			}
			request.Format = "html"
			boundedOptions := phase32Options()
			boundedOptions.MaxWidgets = 3
			bounded, err := rendering.NewManaged(p.f.delivery, d.f.f.db, process, 4<<20, boundedOptions)
			if err != nil {
				t.Fatal(err)
			}
			if output, err := bounded.Generate(t.Context(), p.actor, request); !errors.Is(err, reporting.ErrBudget) || output.Content != "" || output.ID != "" {
				t.Fatal("aggregate widget cap was applied per page", err)
			}
			noRead := phase27Actor(t, d.f, p.actor.User(), []string{"reporting.read", "reporting.export", "cw.run.export:" + tc.run, "cw.execution_context.use:" + d.base.Context})
			if output, err := service.Generate(t.Context(), noRead, request); err == nil || output.Content != "" {
				t.Fatal("export reach became artifact read authority", err)
			}
			noExport := phase27Actor(t, d.f, p.actor.User(), slices.DeleteFunc(slices.Clone(p.scopes), func(scope string) bool { return scope == "cw.run.export:"+tc.run }))
			if output, err := service.Generate(t.Context(), noExport, request); err == nil || output.Content != "" {
				t.Fatal("artifact reach became export authority", err)
			}
		})
	}
	if d.attemptCount(t) != queries || d.f.f.lookups.Load() != lookups || d.f.model.requests.Load() != models {
		t.Fatal("rendering or rendition reads queried a source/model")
	}
}

func phase32AssertPagesHTML(t *testing.T, content string, expected []phase32PageExpectation) {
	t.Helper()
	document, err := nethtml.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	var elements func(*nethtml.Node, string) []*nethtml.Node
	elements = func(node *nethtml.Node, tag string) []*nethtml.Node {
		out := []*nethtml.Node{}
		if node.Type == nethtml.ElementNode && node.Data == tag {
			out = append(out, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			out = append(out, elements(child, tag)...)
		}
		return out
	}
	var text func(*nethtml.Node) string
	text = func(node *nethtml.Node) string {
		if node.Type == nethtml.TextNode {
			return node.Data
		}
		out := ""
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			out += text(child)
		}
		return out
	}
	attribute := func(node *nethtml.Node, name string) string {
		for _, value := range node.Attr {
			if value.Key == name {
				return value.Val
			}
		}
		return ""
	}
	pages := elements(document, "section")
	if len(pages) != len(expected) || len(elements(document, "article")) != len(expected)*2 || len(elements(document, "script")) != 0 || strings.Count(content, "<!doctype html>") != 1 {
		t.Fatal("full HTML lost pages/widgets or requires a script")
	}
	for i, want := range expected {
		page := pages[i]
		headings := elements(page, "h1")
		if attribute(page, "data-page") != want.id || len(headings) != 1 || text(headings[0]) != want.page.Title {
			t.Fatal("HTML page coordinate/title/order", i)
		}
		widgets := elements(page, "article")
		if len(widgets) != len(want.page.Widgets) {
			t.Fatal("HTML changed per-page widget count", i)
		}
		for j, definition := range want.page.Widgets {
			widget := widgets[j]
			titles := elements(widget, "h2")
			style := fmt.Sprintf("grid-column:%d/span %d;grid-row:%d/span %d", definition.Grid.Column+1, definition.Grid.Width, definition.Grid.Row+2, definition.Grid.Height)
			if widget.Parent != page || attribute(widget, "data-widget") != definition.ID || attribute(widget, "style") != style || len(titles) != 1 || text(titles[0]) != definition.Presentation.Title {
				t.Fatal("HTML changed widget ownership/title/geometry", i, j)
			}
			if definition.Text != nil {
				if !strings.Contains(text(widget), definition.Text.Text) {
					t.Fatal("HTML lost retained note", i)
				}
				continue
			}
			cells := []string{}
			for _, cell := range elements(widget, "td") {
				cells = append(cells, text(cell))
			}
			if len(elements(widget, "table")) != 1 || !reflect.DeepEqual(cells, want.cells) {
				t.Fatal("HTML changed exact page-local values", i, cells)
			}
		}
	}
}
