package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/rendering/bffexample"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase32(t *testing.T) {
	t.Run("AC01", phase32Authority)
	t.Run("AC02", phase32BFF)
	t.Run("AC03", phase32StaticWorker)
	t.Run("AC04", phase32Injection)
	t.Run("AC05", phase32ProcessLimits)
	t.Run("AC06", phase32ExactFidelity)
	t.Run("AC07", phase32Lifecycle)
	t.Run("AC08", phase32ExportMatrix)
}

type phase32Viewer struct {
	mu    sync.Mutex
	calls int
	value reporting.DeliveryViewResult
	err   error
	fn    func(reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error)
}

func (v *phase32Viewer) View(_ context.Context, _ identity.Envelope, in reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls++
	if v.fn != nil {
		return v.fn(in)
	}
	return v.value, v.err
}
func phase32View() reporting.DeliveryViewResult {
	return reporting.DeliveryViewResult{
		Locale: "en", Timezone: "UTC",
		Summary:    reporting.DeliveryRunSummary{Kind: "block", Run: "run", Target: reporting.DeliveryTarget{Kind: "block", ID: "block", Revision: 1}, Expires: time.Now().Add(time.Hour)},
		PageBounds: reporting.ViewerPage{Limit: 2, Total: 2},
		Output: &reporting.ViewerOutput{
			ID: "table", Kind: "table", State: "succeeded", RetainedDigest: strings.Repeat("a", 64),
			Table: &reporting.ViewerTable{
				Columns: []charts.Column{{ID: "amount", Name: "amount", DisplayLabel: "Amount <exact>", Type: "decimal", Format: charts.Format{FractionDigits: 2, Currency: "USD"}}, {ID: "note", Name: "note", Type: "text"}},
				Rows: [][]charts.Cell{
					{{Value: "12345678901234567890.12"}, {Value: "null ≠ 0"}},
					{{Null: true}, {Value: "<script>bad</script>"}},
				},
			},
		},
	}
}
func phase32Request(format string) rendering.Request {
	return rendering.Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "table", Limit: 2}, Format: format, Theme: "light", Width: 800, Height: 420}
}
func phase32Actor(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "user", "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func phase32Options() rendering.Options {
	return rendering.Options{WorkerVersion: "worker-v1", ThemeVersion: "theme-v1", MaxTime: 5 * time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 4 << 20, MaxOutputBytes: 4 << 20, MaxConcurrent: 2, Retention: time.Hour}
}

func phase32Authority(t *testing.T) {
	v := &phase32Viewer{value: phase32View()}
	s, _ := rendering.New(v, 1<<20)
	in := phase32Request("html")
	denied := phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:other")
	if _, err := s.Export(t.Context(), denied, in); err == nil || v.calls != 0 {
		t.Fatal("denial crossed retained boundary", err, v.calls)
	}
	allowed := phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run")
	if _, err := s.Export(t.Context(), allowed, in); err != nil || v.calls != 1 {
		t.Fatal(err, v.calls)
	}
}

func phase32BFF(t *testing.T) {
	var gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(rendering.Rendition{MediaType: "text/html; charset=utf-8", Content: "<p>sealed</p>"})
	}))
	defer up.Close()
	h, err := bffexample.New(up.URL, up.Client(), func(context.Context) (string, error) { return "pengui-server-token", nil }, []string{"https://console.example"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(mustJSON32(t, phase32Request("html"))))
	req.Header.Set("Authorization", "Bearer browser-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || gotAuth != "Bearer pengui-server-token" || strings.Contains(rec.Body.String(), "token") || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://console.example") {
		t.Fatal(rec.Code, gotAuth, rec.Header(), rec.Body.String())
	}
	registry, _ := reportingapi.DeliveryRegistry(true, true, true)
	for _, d := range registry.Definitions() {
		if strings.Contains(strings.ToLower(d.Path+d.ID), "bootstrap") || strings.Contains(strings.ToLower(d.Path+d.ID), "embedtoken") {
			t.Fatal("local embed authority advertised", d.ID)
		}
	}
}

func phase32StaticWorker(t *testing.T) {
	v := &phase32Viewer{value: phase32View()}
	p, err := rendering.NewProcess(mustExecutable32(t), phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	s, err := rendering.NewManaged(v, rendering.NewMemoryRepository(), p, 4<<20, phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Generate(t.Context(), phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run"), phase32Request("html"))
	if err != nil || strings.Contains(out.Content, "<script>") || !strings.Contains(out.Content, "12,345,678,901,234,567,890.12 USD") || !strings.Contains(out.Content, "&lt;script&gt;") {
		t.Fatal(err, out.Content)
	}
	if strings.Contains(out.Content, "<script src") || !strings.Contains(out.Content, "<table>") {
		t.Fatal("not genuine static content")
	}
	root := phase32View()
	root.Output = nil
	root.Summary.Kind = "report"
	root.Summary.Target.Kind = "report"
	root.Pages = []reporting.CompositionPageSummary{{ID: "main", Title: "Executive <page>", Widgets: []reporting.CompositionWidgetSummary{{ID: "w1", Kind: "block", State: "completed", Grid: reporting.GridCell{Column: 2, Row: 3, Width: 6, Height: 6}, Presentation: reporting.Presentation{Title: "Revenue <widget>"}}}}}
	compositionViewer := &phase32Viewer{fn: func(request reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
		if request.Widget == "" {
			return root, nil
		}
		return phase32View(), nil
	}}
	compositionService, _ := rendering.NewManaged(compositionViewer, rendering.NewMemoryRepository(), p, 4<<20, phase32Options())
	full := phase32Request("html")
	full.Full = true
	full.View.Kind = "report"
	full.View.Output = ""
	page, err := compositionService.Generate(t.Context(), phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run"), full)
	if err != nil || !strings.Contains(page.Content, "grid-column:3/span 6") || strings.Count(page.Content, "<!doctype html>") != 1 || !strings.Contains(page.Content, "Executive &lt;page&gt;") || !strings.Contains(page.Content, "Revenue &lt;widget&gt;") {
		t.Fatal("composition geometry", err, page.Content)
	}
	textViewer := &phase32Viewer{fn: func(request reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error) {
		if request.Widget == "" {
			return root, nil
		}
		return reporting.DeliveryViewResult{Text: &reporting.TextWidget{Format: "plain", Text: "Exact <text>"}}, nil
	}}
	textService, _ := rendering.NewManaged(textViewer, rendering.NewMemoryRepository(), p, 4<<20, phase32Options())
	full.Format = "svg"
	graphic, err := textService.Generate(t.Context(), phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run"), full)
	if err != nil || !strings.Contains(graphic.Content, `x="141"`) || !strings.Contains(graphic.Content, "Exact &lt;text&gt;") {
		t.Fatal("svg composition geometry", err, graphic.Content)
	}
}

func phase32Injection(t *testing.T) {
	view := phase32View()
	view.Output.Table.Columns[0].DisplayLabel = `<img src=x onerror=alert(1)>`
	v := &phase32Viewer{value: view}
	s, _ := rendering.New(v, 1<<20)
	out, err := s.Export(t.Context(), phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run"), phase32Request("html"))
	if err != nil || !strings.Contains(out.Content, "&lt;img") {
		t.Fatal(err, out.Content)
	}
	hostile := phase32Request("html")
	hostile.Theme = "url(https://example.invalid)"
	if _, err = s.Export(t.Context(), phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run"), hostile); err == nil {
		t.Fatal("external theme accepted")
	}
	sealed := []byte(`{"version":"chartworks-render-worker-v1","request":{"view":{"kind":"block","run":"run","page":"","widget":"","output":"table","offset":0,"limit":2},"format":"html","theme":"light","width":800,"height":420,"network_url":"https://example.invalid"},"view":{}}`)
	var worker bytes.Buffer
	if err := rendering.WorkerMain(bytes.NewReader(sealed), &worker, 1<<20, 1<<20, 1<<30); !errors.Is(err, rendering.ErrInvalid) || worker.Len() != 0 {
		t.Fatal("open worker input accepted", err, worker.String())
	}
}

func phase32ProcessLimits(t *testing.T) {
	dir := t.TempDir()
	crash := filepath.Join(dir, "crash")
	sleep := filepath.Join(dir, "sleep")
	large := filepath.Join(dir, "large")
	clean := filepath.Join(dir, "clean-env")
	if os.WriteFile(crash, []byte("#!/bin/sh\nexit 9\n"), 0700) != nil || os.WriteFile(sleep, []byte("#!/bin/sh\n/bin/sleep 2\n"), 0700) != nil {
		t.Fatal("script")
	}
	work := rendering.SealedWork{Version: rendering.WorkerProtocolVersion, Request: phase32Request("html"), View: phase32View()}
	t.Setenv("SECRET_CANARY", "must-not-cross")
	cleanScript := fmt.Sprintf("#!/bin/sh\n[ -z \"$SECRET_CANARY\" ] || exit 7\nexec %q --sealed-render-worker\n", mustExecutable32(t))
	if os.WriteFile(clean, []byte(cleanScript), 0700) != nil {
		t.Fatal("environment probe")
	}
	cleanProcess, _ := rendering.NewProcess(clean, phase32Options())
	if _, err := cleanProcess.Process(t.Context(), work); err != nil {
		t.Fatal("credential environment crossed worker boundary", err)
	}
	opts := phase32Options()
	p, _ := rendering.NewProcess(crash, opts)
	if _, err := p.Process(t.Context(), work); !errors.Is(err, rendering.ErrWorker) {
		t.Fatal(err)
	}
	opts.MaxTime = 100 * time.Millisecond
	p, _ = rendering.NewProcess(sleep, opts)
	if _, err := p.Process(t.Context(), work); !errors.Is(err, rendering.ErrTimeout) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p, _ = rendering.NewProcess(mustExecutable32(t), phase32Options())
	if _, err := p.Process(ctx, work); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if os.WriteFile(large, []byte("#!/bin/sh\n/usr/bin/head -c 4096 /dev/zero\n"), 0700) != nil {
		t.Fatal("large script")
	}
	opts = phase32Options()
	opts.MaxOutputBytes = 1024
	p, _ = rendering.NewProcess(large, opts)
	if _, err := p.Process(t.Context(), work); !errors.Is(err, rendering.ErrOutputLimit) {
		t.Fatal("output limit", err)
	}
	opts = phase32Options()
	opts.MaxConcurrent = 1
	p, _ = rendering.NewProcess(sleep, opts)
	started := make(chan struct{})
	done := make(chan struct{})
	blockCtx, stopBlock := context.WithCancel(t.Context())
	go func() { close(started); _, _ = p.Process(blockCtx, work); close(done) }()
	<-started
	time.Sleep(25 * time.Millisecond)
	if _, err := p.Process(t.Context(), work); !errors.Is(err, rendering.ErrBusy) {
		t.Fatal("concurrency limit", err)
	}
	stopBlock()
	<-done
}

func phase32ExactFidelity(t *testing.T) {
	v := &phase32Viewer{value: phase32View()}
	s, _ := rendering.New(v, 1<<20)
	actor := phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run")
	j, err := s.Export(t.Context(), actor, phase32Request("json"))
	if err != nil || !strings.Contains(j.Content, "12345678901234567890.12") || !strings.Contains(j.Content, "\"null\":true") {
		t.Fatal(err, j.Content)
	}
	h, err := s.Export(t.Context(), actor, phase32Request("html"))
	if err != nil || !strings.Contains(h.Content, "12,345,678,901,234,567,890.12 USD") || strings.Index(h.Content, "12,345") > strings.Index(h.Content, "null ≠ 0") {
		t.Fatal(err, h.Content)
	}
}

func phase32Lifecycle(t *testing.T) {
	repo := support.Open(t, support.Database(t))
	view := phase32View()
	view.Summary.Expires = time.Now().Add(150 * time.Millisecond)
	v := &phase32Viewer{value: view}
	opts := phase32Options()
	s, _ := rendering.NewManaged(v, repo, rendering.LocalProcessor{MaxBytes: 4 << 20}, 4<<20, opts)
	actor := phase32Actor(t, "reporting.read", "reporting.export", "reporting.retention", "cw.run.export:run", "cw.tenant.erase:tenant")
	a, err := s.Generate(t.Context(), actor, phase32Request("html"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Generate(t.Context(), actor, phase32Request("html"))
	if err != nil || a.ID != b.ID || a.Digest != b.Digest {
		t.Fatal("idempotency", err, a.ID, b.ID)
	}
	opts.ThemeVersion = "theme-v2"
	s2, _ := rendering.NewManaged(v, repo, rendering.LocalProcessor{MaxBytes: 4 << 20}, 4<<20, opts)
	c, err := s2.Generate(t.Context(), actor, phase32Request("html"))
	if err != nil || c.ID == a.ID {
		t.Fatal("version history collapsed", err, c.ID)
	}
	read, err := s.Read(t.Context(), actor, rendering.ReadRequest{ID: a.ID})
	if err != nil || read.Content == "" {
		t.Fatal(err)
	}
	time.Sleep(175 * time.Millisecond)
	expired, err := s.Expire(t.Context(), actor, rendering.ExpireRequest{Limit: 10})
	if err != nil || expired.Count < 2 {
		t.Fatal("retention coupling", expired, err)
	}
	if _, err = s.Read(t.Context(), actor, rendering.ReadRequest{ID: a.ID}); err == nil {
		t.Fatal("deleted rendition remained readable")
	}
}

func phase32ExportMatrix(t *testing.T) {
	v := &phase32Viewer{value: phase32View()}
	s, _ := rendering.New(v, 1<<20)
	actor := phase32Actor(t, "reporting.read", "reporting.export", "cw.run.export:run")
	for _, format := range []string{"json", "csv", "html"} {
		if _, err := s.Export(t.Context(), actor, phase32Request(format)); err != nil {
			t.Fatal(format, err)
		}
	}
	chartView := phase32View()
	chartView.Output.Table = nil
	column := charts.Column{ID: "amount", Name: "amount", Type: "decimal"}
	chartView.Output.Chart = &charts.Output{Version: charts.Version, Kind: charts.Bar, State: "ready", Columns: []charts.Column{column}, Mapping: charts.Mapping{Version: charts.Version, Kind: charts.Bar, Columns: []charts.Column{column}, Bindings: charts.Bindings{Value: "amount"}}, Points: []charts.Point{{Value: charts.Value{Exact: "10"}}}}
	v.value = chartView
	if svg, err := s.Export(t.Context(), actor, phase32Request("svg")); err != nil || !strings.Contains(svg.Content, "<svg") {
		t.Fatal("svg", err)
	}
	for _, format := range []string{"pdf", "png"} {
		if _, err := s.Export(t.Context(), actor, phase32Request(format)); !errors.Is(err, rendering.ErrInvalid) {
			t.Fatal("unsupported advertised", format, err)
		}
	}
	registry, err := reportingapi.DeliveryRegistry(true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range registry.Definitions() {
		ids[d.ID] = true
		if strings.Contains(strings.ToLower(d.Summary), "pdf") || strings.Contains(strings.ToLower(d.Summary), "png") {
			t.Fatal(d.ID)
		}
	}
	for _, id := range []string{"reportingExport", "reportingRenditionCreate", "reportingRenditionRead", "reportingRenditionList", "reportingRenditionExpire"} {
		if !ids[id] {
			t.Fatal("missing operation", id)
		}
	}
}

func mustJSON32(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func mustExecutable32(t *testing.T) string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
