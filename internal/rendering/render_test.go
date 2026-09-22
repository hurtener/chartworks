package rendering

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

func newTestService(v Viewer, maxBytes int) (*Service, error) {
	return NewManaged(v, NewMemoryRepository(), LocalProcessor{MaxBytes: maxBytes}, maxBytes, Options{WorkerVersion: "test", ThemeVersion: "test", MaxTime: time.Second, MaxMemoryBytes: 64 << 20, MaxInputBytes: maxBytes, MaxOutputBytes: maxBytes, MaxConcurrent: 1, MaxWidgets: 100, Retention: time.Hour, Isolation: "development"})
}

type fixtureViewer struct {
	value reporting.DeliveryViewResult
	calls int
}

func (f *fixtureViewer) View(context.Context, identity.Envelope, reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
	f.calls++
	return f.value, nil
}
func authority(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	scopes = append(scopes, "cw.run.export:run")
	e, err := identity.FromVerified("tenant", "user", "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func tableView() reporting.DeliveryViewResult {
	return reporting.DeliveryViewResult{Locale: "en-US", Timezone: "UTC", PageBounds: reporting.ViewerPage{Offset: 0, Limit: 2, Total: 2}, Output: &reporting.ViewerOutput{
		ID: "table", Kind: "table", State: "succeeded", RetainedDigest: strings.Repeat("a", 64),
		Table: &reporting.ViewerTable{
			Columns: []charts.Column{{ID: "name", Name: "name", DisplayLabel: "Display <name>", Type: "text"}},
			Rows:    [][]charts.Cell{{{Value: "=2+2"}}, {{Value: "<script>alert(1)</script>"}}},
		},
	}}
}

func TestStaticExportsUseRetainedValuesAndEscapeContent(t *testing.T) {
	f := &fixtureViewer{value: tableView()}
	s, err := newTestService(f, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	e := authority(t, "reporting.read", "reporting.export")
	base := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 100}, Theme: "light", Width: 800, Height: 420}
	base.Format = "csv"
	csv, err := s.Export(context.Background(), e, base)
	if err != nil || !strings.Contains(csv.Content, "'=2+2") || csv.SourceDigest != strings.Repeat("a", 64) {
		t.Fatalf("unsafe CSV: %+v %v", csv, err)
	}
	base.Format = "html"
	page, err := s.Export(context.Background(), e, base)
	if err != nil || strings.Contains(page.Content, "<script>") || !strings.Contains(page.Content, "&lt;script&gt;") || !strings.Contains(page.Content, "Display &lt;name&gt;") {
		t.Fatalf("unsafe HTML: %s %v", page.Content, err)
	}
	if f.calls != 2 {
		t.Fatal("unexpected retained reads", f.calls)
	}
}

func TestExportRequiresBothCurrentReadAndExportAuthority(t *testing.T) {
	f := &fixtureViewer{value: tableView()}
	s, _ := newTestService(f, 1<<20)
	in := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 10}, Format: "json", Theme: "dark", Width: 640, Height: 320}
	for _, scopes := range [][]string{{"reporting.read"}, {"reporting.export"}} {
		if _, err := s.Export(context.Background(), authority(t, scopes...), in); err == nil {
			t.Fatal("partial authority accepted")
		}
	}
	foreign, err := identity.FromVerified("tenant", "user", "session", []string{"reporting.read", "reporting.export", "cw.run.export:other"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export(context.Background(), foreign, in); err == nil {
		t.Fatal("foreign run export reach accepted")
	}
	if f.calls != 0 {
		t.Fatal("denied export reached retained storage")
	}
}

func TestClosedFormatterMatchesReviewedDisplayIntent(t *testing.T) {
	decimal := charts.Column{Type: "decimal", Format: charts.Format{FractionDigits: 2, Locale: "es-AR", CurrencySymbol: "US$"}}
	if got := formatCell(charts.Cell{Value: "1234.567"}, decimal, "UTC"); got != "1.234,57 US$" {
		t.Fatal("decimal format", got)
	}
	date := charts.Column{Type: "temporal", Format: charts.Format{DatePattern: "date_short", Locale: "es-AR"}}
	if got := formatCell(charts.Cell{Value: "2026-09-22"}, date, "UTC"); got != "22/09/2026" {
		t.Fatal("date format", got)
	}
	date.Format.DatePattern = "year_month"
	if got := formatCell(charts.Cell{Value: "2026-09"}, date, "UTC"); got != "09/2026" {
		t.Fatal("year/month format", got)
	}
	date.Format.DatePattern, date.Format.Locale = "date_long", "en-US"
	if got := formatCell(charts.Cell{Value: "2026-09-22"}, date, "UTC"); got != "September 22, 2026" {
		t.Fatal("long date format", got)
	}
	date.Format.DatePattern, date.Format.Locale = "date_short", "es-AR"
	if got := formatCell(charts.Cell{Value: "2026-09-22T01:30:00Z"}, date, "America/Argentina/Buenos_Aires"); got != "21/09/2026" {
		t.Fatal("report timezone boundary", got)
	}
	date.Format.DatePattern, date.Format.Locale = "datetime_short", "en-US"
	for _, instant := range []string{"2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"} {
		if got := formatCell(charts.Cell{Value: instant}, date, "America/New_York"); got != "Nov 01, 2026 01:30" {
			t.Fatal("DST report timezone", instant, got)
		}
	}
	percent := charts.Column{Type: "decimal", Format: charts.Format{Percent: "fraction"}}
	if got := formatCell(charts.Cell{Value: "0.125"}, percent, "UTC"); got != "12.5%" {
		t.Fatal("percent format", got)
	}
}

func TestStaticKPIContainsActualFormattedContentWithoutScript(t *testing.T) {
	column := charts.Column{ID: "actual", Name: "actual", DisplayLabel: "Ingresos", Type: "decimal", Format: charts.Format{Currency: "USD", CurrencySymbol: "US$", Locale: "es-AR", FractionDigits: 2}}
	chart := &charts.Output{Version: charts.DisplayVersion, Kind: charts.KPI, State: "ready", Columns: []charts.Column{column}, Mapping: charts.Mapping{Version: charts.DisplayVersion, Kind: charts.KPI, Columns: []charts.Column{column}, Bindings: charts.Bindings{Value: "actual"}, Options: charts.Options{Title: "Ingresos"}}, KPIResult: &charts.KPIResult{Value: charts.Value{Exact: "1234.567"}, PercentDelta: &charts.Value{Exact: "18.18"}}}
	f := &fixtureViewer{value: reporting.DeliveryViewResult{Output: &reporting.ViewerOutput{ID: "kpi", Kind: "kpi", State: "succeeded", RetainedDigest: strings.Repeat("b", 64), Chart: chart}}}
	s, _ := newTestService(f, 1<<20)
	in := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "kpi"}, Format: "html", Theme: "light", Width: 800, Height: 420}
	page, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), in)
	if err != nil || !strings.Contains(page.Content, "1.234,57 US$") || !strings.Contains(page.Content, "18.18%") || strings.Contains(page.Content, "<script") {
		t.Fatal("static KPI content", page.Content, err)
	}
	in.Format = "svg"
	image, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), in)
	if err != nil || !strings.Contains(image.Content, "<svg") || !strings.Contains(image.Content, "1.234,57 US$") || strings.Contains(image.Content, "<script") {
		t.Fatal("static KPI SVG", image.Content, err)
	}
}

func TestStaticExportClosedFormatsAndBounds(t *testing.T) {
	column := charts.Column{ID: "value", Name: "value", Type: "decimal"}
	chart := &charts.Output{Version: charts.Version, Kind: charts.Bar, State: "ready", Columns: []charts.Column{column}, Mapping: charts.Mapping{Version: charts.Version, Kind: charts.Bar, Columns: []charts.Column{column}, Options: charts.Options{Title: "A < B"}}, Points: []charts.Point{{Value: charts.Value{Exact: "42"}}}}
	f := &fixtureViewer{value: reporting.DeliveryViewResult{Output: &reporting.ViewerOutput{ID: "chart", Kind: "chart", State: "succeeded", RetainedDigest: strings.Repeat("c", 64), Chart: chart}}}
	s, _ := newTestService(f, 1<<20)
	base := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "chart"}, Theme: "dark", Width: 640, Height: 320}
	for _, format := range []string{"json", "html", "svg"} {
		base.Format = format
		got, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), base)
		if err != nil || got.Bytes == 0 || got.Digest == "" || got.Format != format {
			t.Fatal(format, got, err)
		}
		if format != "json" && (strings.Contains(got.Content, "<script") || strings.Contains(got.Content, "A < B")) {
			t.Fatal("unsafe chart text", got.Content)
		}
	}
	base.Format = "csv"
	if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), base); !errors.Is(err, ErrInvalid) {
		t.Fatal("chart exported as CSV", err)
	}
	f.value.Output = &reporting.ViewerOutput{ID: "narrative", Kind: "narrative", State: "succeeded", RetainedDigest: strings.Repeat("d", 64), Narrative: &reporting.NarrativeResult{Text: "<b>inert</b>"}}
	base.Format = "html"
	page, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), base)
	if err != nil || !strings.Contains(page.Content, "&lt;b&gt;inert&lt;/b&gt;") {
		t.Fatal("narrative escaping", page.Content, err)
	}
}

func TestRendererRejectsInvalidConstructionRequestAndArtifact(t *testing.T) {
	if _, err := New(nil, 1<<20); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	f := &fixtureViewer{value: tableView()}
	if _, err := New(f, 100); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	s, _ := newTestService(f, 1024)
	valid := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table"}, Format: "html", Theme: "light", Width: 640, Height: 320}
	for _, change := range []func(*Request){func(r *Request) { r.Format = "pdf" }, func(r *Request) { r.Theme = "remote" }, func(r *Request) { r.Width = 100 }, func(r *Request) { r.Height = 5000 }, func(r *Request) { r.View.Limit = 1001 }} {
		request := valid
		change(&request)
		if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), request); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid request accepted", request, err)
		}
	}
	f.value.Output.State = "running"
	if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), valid); !errors.Is(err, reporting.ErrIncomplete) {
		t.Fatal("incomplete artifact rendered", err)
	}
	f.value = tableView()
	s, _ = newTestService(f, 1024)
	f.value.Output.Table.Rows = append(f.value.Output.Table.Rows, []charts.Cell{{Value: strings.Repeat("x", 2048)}})
	f.value.PageBounds.Limit, f.value.PageBounds.Total = 3, 3
	if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), valid); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("output budget not enforced", err)
	}
}
