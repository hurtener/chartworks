package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

type chartRankEngine struct {
	phase21Engine
	calls            atomic.Int64
	unexpected       atomic.Int64
	mode             string
	started, release chan struct{}
	cancel           context.CancelFunc
	expire           func()
}

func (g *chartRankEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	g.unexpected.Add(1)
	return gateway.Generated{}, gateway.ErrDisabled
}
func (g *chartRankEngine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	g.unexpected.Add(1)
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (g *chartRankEngine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	g.unexpected.Add(1)
	return gateway.Ranked{}, gateway.ErrDisabled
}

func (g *chartRankEngine) VisualRank(ctx context.Context, call gateway.Call, budget *gateway.Budget, intent string, c gateway.Candidates) (gateway.Ranked, error) {
	if !c.Valid(call) {
		return gateway.Ranked{}, gateway.ErrInput
	}
	for _, item := range c.Items() {
		if strings.Contains(item.Text, "fixture") || strings.Contains(item.Text, "Alpha") || strings.Contains(item.Text, "3.250") {
			return gateway.Ranked{}, errors.New("private fixture reached ranker")
		}
	}
	if g.mode == "budget" {
		return gateway.Ranked{}, budget.Reserve(call, 1<<20)
	}
	if err := budget.Reserve(call, 200); err != nil {
		return gateway.Ranked{}, err
	}
	g.calls.Add(1)
	in, out, cost := 11, 5, 0.002
	r := gateway.Ranked{Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "visual_rank", Provider: "recorded", RequestedModel: "fixture-model", Attempts: 1, InputTokens: &in, OutputTokens: &out, CostUSD: &cost}}}}
	for i := len(c.Items()) - 1; i >= 0; i-- {
		score := float64(i)
		r.Items = append(r.Items, gateway.RankedItem{ID: c.Items()[i].ID, Score: &score})
	}
	switch g.mode {
	case "failure":
		r.Receipt.Calls[0].InputTokens = nil
		r.Receipt.Calls[0].OutputTokens = nil
		r.Receipt.Calls[0].CostUSD = nil
		return r, gateway.ErrUnavailable
	case "warning":
		r.Receipt.Warning = "rank_unavailable"
	case "duplicate":
		r.Items[1] = r.Items[0]
	case "foreign":
		r.Items[0].ID = "invented"
	case "missing":
		r.Items = r.Items[:1]
	case "nan":
		n := math.NaN()
		r.Items[0].Score = &n
	case "receipt":
		r.Receipt.Calls = nil
	case "cancel":
		g.cancel()
	case "expire":
		g.expire()
	case "block":
		close(g.started)
		select {
		case <-g.release:
		case <-ctx.Done():
			return r, ctx.Err()
		}
	}
	return r, nil
}

type chartHTTPFixture struct {
	token   *tokenFixture
	service *chartservice.Service
	handler http.Handler
	client  *cw.Client
	bearer  string
}

func newChartHTTP(t *testing.T, engine gateway.Engine, edit func(*chartservice.Options)) chartHTTPFixture {
	t.Helper()
	f := newTokenFixture(t)
	opts := config.DefaultCharts().ServiceOptions()
	opts.MaxConcurrent = 64
	if edit != nil {
		edit(&opts)
	}
	service, err := chartservice.New(opts, engine)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := chartapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	h := api.Guard(f.verifier, registry, chartapi.Handler(f.verifier, service, http.NotFoundHandler()))
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	bearer := f.sign(t, f.claims("chart-tenant", "reader", []string{"charts.read", "charts.select", "charts.bind", "cw.tenant.read:chart-tenant"}), nil)
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	return chartHTTPFixture{f, service, h, client, bearer}
}
func chartJSON(t testing.TB, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func chartStatus(t *testing.T, err error, status int) {
	t.Helper()
	var failure *cw.StatusError
	if !errors.As(err, &failure) || failure.Status != status {
		t.Fatalf("status want %d: %v", status, err)
	}
}
func TestPhase20(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newChartHTTP(t, nil, nil)
		d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
		catalog, err := f.client.ChartCatalog(context.Background())
		if err != nil || len(catalog.Kinds) != 14 || catalog.RankingConfigured {
			t.Fatalf("catalog: %+v %v", catalog, err)
		}
		want, err := charts.Select(context.Background(), d, charts.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 8; i++ {
			out, err := f.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d})
			if err != nil || !reflect.DeepEqual(out.Selection, want) || out.Provenance.Ranking != "not_requested" || len(out.Provenance.Receipt.Calls) != 0 {
				t.Fatalf("non deterministic HTTP selection: %v", err)
			}
		}
		for _, c := range append([]charts.Candidate{want.Selected}, want.Alternatives...) {
			if _, e := charts.Build(context.Background(), d, c.Mapping, charts.Defaults()); e != nil {
				t.Fatal("invalid selected slots", e)
			}
		}
		if len(want.Alternatives) > 3 || want.Fallback {
			t.Fatal("bounded suitable set")
		}
		d.Columns = d.Columns[:1]
		for i := range d.Rows {
			d.Rows[i] = d.Rows[i][:1]
		}
		fallback, err := f.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d})
		if err != nil || !fallback.Selection.Fallback || fallback.Selection.Selected.Mapping.Kind != charts.Table || fallback.Selection.Selected.Reason != "table_fallback" {
			t.Fatal("unlabeled fallback", err)
		}
		d, b, o := chartfixtures.Fixture(charts.Bar, "binding")
		b.Value = "missing"
		_, err = f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Bar, Bindings: b, Order: o, Options: charts.DefaultOptions()})
		if err == nil {
			t.Fatal("missing slot accepted")
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newChartHTTP(t, nil, nil)
		d, b, o := chartfixtures.Fixture(charts.Line, "binding")
		d.Columns[1].Format = charts.Format{Currency: "USD", Unit: "revenue", FractionDigits: 3}
		out, err := f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Line, Bindings: b, Order: o, Options: charts.DefaultOptions()})
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range out.Output.Mapping.Columns {
			if c != d.Columns[i] {
				t.Fatal("metadata lost")
			}
		}
		if out.Output.Columns[0].Grain != "day" || out.Output.Columns[1].Provenance.TopicVersion != "v1" || out.Input != "caller_supplied" {
			t.Fatal("provenance/grain missing")
		}
		d.Columns[1].Format = charts.Format{Percent: "fraction", FractionDigits: 3}
		out, err = f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Line, Bindings: b, Order: o, Options: charts.DefaultOptions()})
		if err != nil || out.Output.Columns[1].Format.Percent != "fraction" || len(out.Output.Totals) != 0 {
			t.Fatal("percentage treated as additive", err)
		}
		if strings.Contains(chartJSON(t, out), "native_type") {
			t.Fatal("driver UI metadata escaped")
		}
	})
	t.Run("AC03", func(t *testing.T) {
		chartfixtures.Verify(t)
		f := newChartHTTP(t, nil, nil)
		for _, kind := range charts.Catalog() {
			t.Run("http_"+string(kind.Kind), func(t *testing.T) {
				d, b, o := chartfixtures.Fixture(kind.Kind, "order")
				in := cw.ChartSpecifyRequest{Data: d, Kind: kind.Kind, Bindings: b, Order: o, Options: charts.DefaultOptions()}
				out, err := f.client.SpecifyChart(context.Background(), in)
				if err != nil || out.Output.Kind != kind.Kind {
					t.Fatalf("explicit chart not served: %v", err)
				}
				saved, err := f.client.BuildChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: out.Output.Mapping})
				if err != nil || !reflect.DeepEqual(saved, out) {
					t.Fatal("saved output drift", err)
				}
			})
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newChartHTTP(t, nil, nil)
		wire := `{"schema":[{"name":"category","type":"text","encoding":"string"},{"name":"amount","type":"decimal","encoding":"string"},{"name":"count","type":"integer","encoding":"string"}],"rows":[["A","9007199254740993.125","9007199254740993"],["B","0.005","2"]],"outcome":"truncated","truncation":"rows"}`
		var result cw.ReadResult
		if json.Unmarshal([]byte(wire), &result) != nil {
			t.Fatal("read fixture")
		}
		d, err := cw.ChartDataFromReadResult(context.Background(), result, charts.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		d.Columns[1].Aggregation = "sum"
		d.Columns[2].Aggregation = "sum"
		out, err := f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"c0", "c1", "c2"}}, Options: charts.DefaultOptions()})
		if err != nil {
			t.Fatal(err)
		}
		if out.Output.Rows[0][1].Value != "9007199254740993.125" || out.Output.Rows[0][2].Value != "9007199254740993" || out.Output.Totals[0].Value.Value != "9007199254740993.130" || out.Output.Totals[1].Value.Value != "9007199254740995" {
			t.Fatalf("precision lost %+v", out.Output)
		}
		for _, total := range out.Output.Totals {
			if total.Scope != "returned_rows" {
				t.Fatal("truncated full-source total")
			}
		}
		bars, err := f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Bar, Bindings: charts.Bindings{Category: "c0", Value: "c1"}, Options: charts.DefaultOptions()})
		if err != nil || bars.Output.Points[0].Value.Exact != "9007199254740993.125" || !bars.Output.Points[0].Value.Approximate {
			t.Fatal("exact label and geometry not separated", err)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		engine := &chartRankEngine{}
		f := newChartHTTP(t, engine, func(o *chartservice.Options) { o.RankEnabled = true })
		d, b, o := chartfixtures.Fixture(charts.Bar, "binding")
		out, err := f.client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Bar, Bindings: b, Order: o, Options: charts.DefaultOptions()})
		if err != nil {
			t.Fatal(err)
		}
		original := chartJSON(t, out.Output.Mapping)
		d.Columns[1].ID = "amount"
		d.Columns[1].Name = "Amount"
		d.Columns[1].Provenance.TopicVersion = "v2"
		_, err = f.client.BuildChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: out.Output.Mapping})
		chartStatus(t, err, 409)
		proposal, err := f.client.RebindChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: out.Output.Mapping})
		if err != nil || proposal.Status != "review_required" || proposal.Mapping.Bindings.Value != "amount" || len(proposal.Changes) != 1 || original != chartJSON(t, out.Output.Mapping) {
			t.Fatal("unsafe rebind", err)
		}
		if _, err = f.client.BuildChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: proposal.Mapping}); err != nil {
			t.Fatal(err)
		}
		d.Columns[1].Format.Unit = "different"
		_, err = f.client.RebindChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: out.Output.Mapping})
		if err == nil {
			t.Fatal("incompatible units rebound")
		}
		if engine.calls.Load() != 0 || engine.unexpected.Load() != 0 {
			t.Fatal("frozen definition invoked ranker")
		}
	})
	t.Run("AC06", func(t *testing.T) {
		verifyChartHTTPNegatives(t)
		verifyChartRanker(t)
		verifyChartConcurrency(t)
		verifyChartDecodeAdmission(t)
		verifyChartBifrost(t)
	})
}

func verifyChartHTTPNegatives(t *testing.T) {
	t.Helper()
	g := &chartRankEngine{}
	f := newChartHTTP(t, g, func(o *chartservice.Options) { o.RankEnabled = true })
	d, b, o := chartfixtures.Fixture(charts.Bar, "binding")
	in := cw.ChartSpecifyRequest{Data: d, Kind: charts.Bar, Bindings: b, Order: o, Options: charts.DefaultOptions()}
	body := chartJSON(t, in)
	for _, scopes := range [][]string{nil, {"charts.bind"}, {"charts.bind", "cw.tenant.read:other"}, {"charts.select", "cw.tenant.read:chart-tenant"}, {"charts.bind", "cw.source.read:fixture", "cw.execution_context.read:fixture"}} {
		token := f.token.sign(t, f.token.claims("chart-tenant", "reader", scopes), nil)
		r := callProtected(t, f.handler, "POST", "/v1/charts/specify", token, "{bad", nil)
		want := 404
		if !containsChartScope(scopes, "charts.bind") {
			want = 403
		}
		if r.Code != want {
			t.Fatalf("denial before body/resource input: want %d got %d %s", want, r.Code, r.Body.String())
		}
	}
	for _, test := range []struct {
		path, token, body string
		extra             map[string]string
		want              int
	}{
		{"/v1/charts/specify", "", body, map[string]string{"Cookie": "access_token=" + f.bearer}, 401},
		{"/v1/charts/specify?token=" + f.bearer, "", body, nil, 401},
		{"/v1/charts/specify?tenant=other", f.bearer, body, nil, 400},
		{"/v1/charts/specify", f.bearer, body, map[string]string{"Content-Type": "text/plain"}, 400},
		{"/v1/charts/specify", f.bearer, body, map[string]string{"Content-Encoding": "gzip"}, 400},
		{"/v1/charts/specify", f.bearer, strings.Replace(body, `"title":""`, `"title":"https://remote.invalid/a"`, 1), nil, 400},
		{"/v1/charts/specify", f.bearer, strings.Replace(body, `"title":""`, `"formatter":"evil()","title":""`, 1), nil, 400},
		{"/v1/charts/specify", f.bearer, strings.Replace(body, `"value":"3.250"`, `"value":null`, 1), nil, 400},
		{"/v1/charts/specify", f.bearer, body + body, nil, 400},
		{"/v1/charts/specify", f.bearer, strings.Repeat(" ", chartapi.MaxBodyBytes+1), nil, 413},
	} {
		r := callProtected(t, f.handler, "POST", test.path, test.token, test.body, test.extra)
		if r.Code != test.want {
			t.Fatalf("bad HTTP input want %d got %d: %.200s", test.want, r.Code, r.Body.String())
		}
	}
	for _, verb := range []string{"HEAD", "TRACE"} {
		r := callProtected(t, f.handler, verb, "/v1/charts/catalog", f.bearer, "", nil)
		if r.Code != 405 || r.Body.Len() != 0 {
			t.Fatal("method response contradicts schema")
		}
	}
	if r := callProtected(t, f.handler, "GET", "/v1/charts/catalog", f.bearer, "x", nil); r.Code != 400 {
		t.Fatal("GET body accepted")
	}
	limits := newChartHTTP(t, nil, func(o *chartservice.Options) { o.Limits.MaxRows = 1 })
	_, err := limits.client.SpecifyChart(context.Background(), in)
	chartStatus(t, err, 413)
	if g.calls.Load() != 0 || g.unexpected.Load() != 0 {
		t.Fatal("denial invoked model")
	}
}
func verifyChartRanker(t *testing.T) {
	t.Helper()
	d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
	baseline, err := charts.Select(context.Background(), d, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "failure", "warning", "duplicate", "foreign", "missing", "nan", "receipt", "budget", "disabled"} {
		t.Run("rank_"+mode, func(t *testing.T) {
			g := &chartRankEngine{mode: mode}
			f := newChartHTTP(t, g, func(o *chartservice.Options) { o.RankEnabled = mode != "disabled" })
			out, err := f.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d, Rank: true, Intent: "compare categories"})
			if err != nil {
				t.Fatal(err)
			}
			want := "invalid_response"
			switch mode {
			case "success":
				want = "gateway_ranked"
			case "disabled":
				want = "disabled"
			case "failure", "warning", "budget":
				want = "rules_preserved"
			}
			if out.Provenance.Ranking != want {
				t.Fatalf("rank %s: %+v", mode, out.Provenance)
			}
			if mode == "success" {
				if out.Selection.Selected.Mapping.Kind != baseline.Alternatives[len(baseline.Alternatives)-1].Mapping.Kind || len(out.Provenance.Ranked) == 0 || out.Provenance.ReservedCalls != 1 {
					t.Fatal("rank not applied/provenance missing")
				}
			} else if !reflect.DeepEqual(out.Selection, baseline) {
				t.Fatal("failed rank changed suitability set")
			}
			if mode == "failure" && (len(out.Provenance.Receipt.Calls) != 1 || out.Provenance.Receipt.Calls[0].CostUSD != nil) {
				t.Fatal("failed charge missing/zero fabricated")
			}
			if (mode == "budget" || mode == "disabled") && g.calls.Load() != 0 {
				t.Fatal("unadmitted inference")
			}
		})
	}
	g := &chartRankEngine{mode: "cancel"}
	f := newChartHTTP(t, g, func(o *chartservice.Options) { o.RankEnabled = true })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.cancel = cancel
	e := f.token.envelope(t, "chart-tenant", "reader", "charts.select", "cw.tenant.read:chart-tenant")
	out, err := f.service.Select(ctx, e, chartservice.SelectRequest{Data: d, Rank: true})
	var failure *chartservice.Failure
	if !errors.Is(err, context.Canceled) || !errors.As(err, &failure) || len(failure.Receipt.Calls) != 1 || !reflect.DeepEqual(out, chartservice.SelectionResult{}) {
		t.Fatal("late cancellation lost receipt or exposed result", err)
	}
	expired := &chartRankEngine{mode: "expire"}
	ef := newChartHTTP(t, expired, func(o *chartservice.Options) { o.RankEnabled = true })
	expired.expire = func() { ef.token.clock.Add(3600) }
	_, err = ef.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d, Rank: true})
	var status *cw.StatusError
	if !errors.As(err, &status) || status.Status != 401 || status.Receipt == nil || len(status.Receipt.Calls) != 1 {
		t.Fatal("expired result did not preserve SDK usage receipt", err)
	}
	// Core service enforcement is independent of HTTP and rejects zero authority.
	if _, err = f.service.Select(context.Background(), identity.Envelope{}, chartservice.SelectRequest{Data: d, Rank: true}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("service auth bypass", err)
	}
}
func verifyChartConcurrency(t *testing.T) {
	t.Helper()
	d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
	g := &chartRankEngine{mode: "block", started: make(chan struct{}), release: make(chan struct{})}
	f := newChartHTTP(t, g, func(o *chartservice.Options) { o.RankEnabled = true; o.MaxConcurrent = 1 })
	e := f.token.envelope(t, "chart-tenant", "reader", "charts.select", "charts.read", "cw.tenant.read:chart-tenant")
	done := make(chan error, 1)
	go func() {
		_, err := f.service.Select(context.Background(), e, chartservice.SelectRequest{Data: d, Rank: true})
		done <- err
	}()
	select {
	case <-g.started:
	case <-time.After(5 * time.Second):
		t.Fatal("rank not reached")
	}
	if _, err := f.service.Catalog(context.Background(), e); !errors.Is(err, chartservice.ErrBusy) {
		t.Fatal("concurrency queue unbounded", err)
	}
	close(g.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Catalog(context.Background(), e); err != nil {
		t.Fatal("admission leak", err)
	}
	h := newChartHTTP(t, nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := h.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d})
			if err != nil {
				t.Error(err)
				return
			}
			out.Selection.Selected.Mapping.Columns[0].Name = "local mutation"
		}()
	}
	wg.Wait()
	after, err := h.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d})
	if err != nil || after.Selection.Selected.Mapping.Columns[0].Name == "local mutation" {
		t.Fatal("shared output state", err)
	}
}

func containsChartScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// HTTP admission is bounded before reading attacker-sized JSON, not just before
// the pure transformation. Direct service admission is tested independently.
func verifyChartDecodeAdmission(t *testing.T) {
	f := newChartHTTP(t, nil, func(o *chartservice.Options) { o.MaxConcurrent = 1 })
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	entered := make(chan struct{})
	body := &chartSignaledReader{Reader: reader, entered: entered}
	request := httptest.NewRequest("POST", "/v1/charts/select", body)
	request.Header.Set("Authorization", "Bearer "+f.bearer)
	request.Header.Set("Content-Type", "application/json")
	done := make(chan int, 1)
	go func() { w := httptest.NewRecorder(); f.handler.ServeHTTP(w, request); done <- w.Code }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decoder not reached")
	}
	response := callProtected(t, f.handler, "POST", "/v1/charts/select", f.bearer, "{not JSON}", nil)
	if response.Code != 429 {
		t.Fatalf("decode queue unbounded: %d", response.Code)
	}
	_ = writer.Close()
	if code := <-done; code != 400 {
		t.Fatalf("short body: %d", code)
	}
	if response = callProtected(t, f.handler, "GET", "/v1/charts/catalog", f.bearer, "", nil); response.Code != 200 {
		t.Fatal("decode admission leaked")
	}
}

type chartSignaledReader struct {
	io.Reader
	entered chan struct{}
	once    sync.Once
}

func (r *chartSignaledReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	return r.Reader.Read(p)
}

func verifyChartBifrost(t *testing.T) {
	g := newGatewayFixture(t, nil)
	d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
	selected, err := charts.Select(context.Background(), d, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	list := append([]charts.Candidate{selected.Selected}, selected.Alternatives...)
	order := []string{}
	for i := len(list) - 1; i >= 0; i-- {
		order = append(order, string(list[i].Mapping.Kind))
	}
	content := chartJSON(t, map[string]any{"order": order})
	response := chartJSON(t, map[string]any{"id": "recorded-chart-ranking", "object": "chat.completion", "model": "model-visual_rank", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
	g.mode.Store("chat_raw:" + response)
	f := newChartHTTP(t, g.engine, func(o *chartservice.Options) { o.RankEnabled = true })
	out, err := f.client.SelectChart(context.Background(), cw.ChartSelectRequest{Data: d, Rank: true, Intent: "compare categories"})
	if err != nil || out.Provenance.Ranking != "gateway_ranked" || string(out.Selection.Selected.Mapping.Kind) != order[0] || g.requests.Load() != 1 {
		t.Fatalf("actual Bifrost selection: %+v %v requests=%d", out.Provenance, err, g.requests.Load())
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.requestBodies) != 1 || !strings.Contains(g.requestBodies[0], `"response_format"`) || strings.Contains(g.requestBodies[0], "Alpha") || strings.Contains(g.requestBodies[0], "3.250") || strings.Contains(g.requestBodies[0], "sales") {
		t.Fatal("gateway structured request/data minimization failed")
	}
}
