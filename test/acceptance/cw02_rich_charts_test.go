package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

// TestCW02RichCharts traces rich mappings through actual HTTP, immutable
// publication and retained Apps delivery. Phase31 AC04 separately drives the
// same rich fixture shapes through the real browser component.
func TestCW02RichCharts(t *testing.T) {
	t.Run("closed-http", testCW02ChartHTTP)
	t.Run("saved-lifecycle", testCW02SavedChartLifecycle)
}
func testCW02ChartHTTP(t *testing.T) {
	f := newChartHTTP(t, nil, nil)
	catalog, err := f.client.ChartCatalog(t.Context())
	if err != nil || !reflect.DeepEqual(catalog.MappingVersions, []int{1, 2, 3}) || len(catalog.Kinds) != 14 {
		t.Fatal("versioned catalog", catalog, err)
	}
	for _, fixture := range chartfixtures.RichCases() {
		t.Run(fixture.Name, func(t *testing.T) {
			out, err := f.client.SpecifyChart(t.Context(), cw.ChartSpecifyRequest{Data: fixture.Data, Kind: fixture.Kind, Bindings: fixture.Bindings, Order: fixture.Order, Options: charts.DefaultOptions()})
			if err != nil || out.Output.Version != 2 {
				t.Fatal("real SDK/HTTP rich specification", err)
			}
			mapping := phase27Copy(t, out.Output.Mapping)
			replayed, err := f.client.BuildChart(t.Context(), cw.ChartBuildRequest{Data: fixture.Data, Mapping: mapping})
			if err != nil || !reflect.DeepEqual(replayed, out) {
				t.Fatal("wire save/read/build changed values or repeated slots", err)
			}
			data := phase27Copy(t, fixture.Data)
			data.Columns[0].Provenance.SourceRevision++
			_, err = f.client.BuildChart(t.Context(), cw.ChartBuildRequest{Data: data, Mapping: mapping})
			chartStatus(t, err, http.StatusConflict)
		})
	}
	// Unknown fields and an object disguised as a repeated column are rejected
	// by the actual closed HTTP decoder, not only by a struct constructor.
	fixture := chartfixtures.RichCases()[0]
	for _, extra := range []string{`"values":["revenue",{"column":"quantity"}]`, `"values":["revenue","quantity"],"expression":"unreviewed"`} {
		body := `{"data":` + chartJSON(t, fixture.Data) + `,"kind":"line","bindings":{"category":"day",` + extra + `},"order":[],"options":` + chartJSON(t, charts.DefaultOptions()) + `}`
		req := httptest.NewRequest(http.MethodPost, "/v1/charts/specify", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.bearer)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatal("open repeated-binding decoder", response.Code)
		}
	}
	// Metadata-only optional ranking sees the intent classification and sealed
	// descriptors. Neither raw question nor exact labels/pins/rows may escape.
	ranker := &cw02RankCapture{}
	ranked := newChartHTTP(t, ranker, func(o *chartservice.Options) { o.RankEnabled = true })
	for _, candidate := range chartfixtures.RichCases() {
		if candidate.Name == "bubble_series" {
			out, err := ranked.client.SelectChart(t.Context(), cw.ChartSelectRequest{Data: candidate.Data, Rank: true, Intent: "Show bubble geometry SYNTHETIC_QUESTION_CANARY"})
			if err != nil || out.Provenance.Ranking != "not_applicable" || ranker.calls.Load() != 0 || out.Selection.Selected.Mapping.Bindings.Size != "size" {
				t.Fatal("a single suitable shape must not invoke ranking", out.Provenance, err)
			}
			// Repeated categorical tuples correctly excluded comparisons above.
			// Distinct synthetic labels now permit several honest alternatives;
			// only this case should reach the optional gateway ranker.
			for i := range candidate.Data.Rows {
				candidate.Data.Rows[i][3].Value += " / " + candidate.Data.Rows[i][0].Value
			}
			out, err = ranked.client.SelectChart(t.Context(), cw.ChartSelectRequest{Data: candidate.Data, Rank: true, Intent: "Show bubble geometry SYNTHETIC_QUESTION_CANARY"})
			query, metadata := ranker.snapshot()
			if err != nil || out.Provenance.Ranking != "gateway_ranked" || query != "bubble" || ranker.calls.Load() != 1 {
				t.Fatal("bounded optional rank", out.Provenance, err)
			}
			found := false
			for _, c := range append([]charts.Candidate{out.Selection.Selected}, out.Selection.Alternatives...) {
				if c.Mapping.Kind == charts.Scatter && c.Mapping.Bindings.Size == "size" {
					found = true
				}
			}
			if !found || out.Selection.Evidence.Intent != "bubble" {
				t.Fatal("bubble was excluded before model sealing")
			}
			for _, forbidden := range []string{"SYNTHETIC_QUESTION_CANARY", "North", "literal", "Population", "warehouse", "metrics", "1.0000000000000001"} {
				if strings.Contains(metadata, forbidden) {
					t.Fatal("private chart input reached optional ranker")
				}
			}
		}
	}
}

type cw02RankCapture struct {
	chartRankEngine
	mu              sync.Mutex
	query, metadata string
}

func (g *cw02RankCapture) VisualRank(ctx context.Context, call gateway.Call, budget *gateway.Budget, intent string, candidates gateway.Candidates) (gateway.Ranked, error) {
	g.mu.Lock()
	g.query = intent
	for _, candidate := range candidates.Items() {
		g.metadata += candidate.Text + "\n"
	}
	g.mu.Unlock()
	return g.chartRankEngine.VisualRank(ctx, call, budget, intent, candidates)
}

func (g *cw02RankCapture) snapshot() (string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.query, g.metadata
}

func testCW02SavedChartLifecycle(t *testing.T) {
	f := newPhase31Fixture(t, false)
	domain := f.domain
	ctx := t.Context()
	// One ordinary validated query fans out to all rich variants. All values are
	// synthetic projections of the already authorized fixture relation.
	sql := `SELECT CASE WHEN id=1 THEN '2026-01-01'::date ELSE '2026-01-02'::date END AS day,
		id::text AS category, CASE WHEN id=1 THEN 'North' ELSE 'South' END AS region,
		'A'::text AS country, 'Device'::text AS product, amount AS revenue,
		CASE WHEN id=1 THEN NULL::bigint ELSE 9223372036854775807::bigint END AS quantity,
		id::numeric AS exposure, amount AS response, (id*id)::bigint AS population
		FROM analytics.sales ORDER BY id`
	definition := phase27Definition(t, domain.f, domain.blockAuthor, sql)
	data := charts.Data{Version: 1, Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}
	for _, original := range definition.Outputs[0].Mapping.Columns {
		column := original
		column.ID = column.Name
		column.Provenance.Source = definition.Source
		column.Provenance.SourceRevision = domain.f.pack.Datasets[0].Source.SourceRevision
		switch column.Name {
		case "day":
			column.Grain = "day"
		case "revenue", "response":
			column.Role, column.Aggregation, column.Format.Currency = "measure", "sum", "USD"
			column.Provenance.Topic, column.Provenance.TopicVersion, column.Provenance.SemanticID = definition.Topics[0].Topic, definition.Topics[0].Version, "amount"
		case "quantity", "population":
			column.Role, column.Aggregation, column.Format.Unit = "measure", "sum", "items"
		case "exposure":
			column.Role = "measure"
		}
		data.Columns = append(data.Columns, column)
	}
	definition.Outputs = nil
	bind := func(name string, kind charts.Kind, b charts.Bindings) {
		t.Helper()
		mapping, err := charts.Bind(ctx, data, kind, b, nil, charts.DefaultOptions(), charts.Defaults())
		if err != nil {
			t.Fatal("declared rich fixture", name, err)
		}
		outputKind := "chart"
		if kind == charts.Table {
			outputKind = "table"
		}
		definition.Outputs = append(definition.Outputs, reporting.Output{ID: name, Kind: outputKind, Mapping: &mapping})
	}
	for _, kind := range []charts.Kind{charts.Line, charts.Area, charts.Bar, charts.ColumnChart, charts.GroupedBar} {
		category := "category"
		if kind == charts.Line || kind == charts.Area {
			category = "day"
		}
		bind(string(kind)+"-multi", kind, charts.Bindings{Category: category, Values: []string{"revenue", "quantity"}})
		bind(string(kind)+"-split", kind, charts.Bindings{Category: category, Values: []string{"revenue", "quantity"}, Series: "region"})
	}
	bind("bubble", charts.Scatter, charts.Bindings{X: "exposure", Y: "response", Size: "population", Series: "region"})
	bind("hierarchy", charts.Treemap, charts.Bindings{Hierarchy: []string{"region", "country", "product"}, Value: "revenue"})
	bind("legacy-table", charts.Table, charts.Bindings{Columns: []string{"category", "revenue", "quantity"}})
	// Exercise the shared rich bindings with the current authored output intent,
	// rather than relying on the legacy empty-selection projection.
	definition, err := cw.MigrateBlockDefinition(definition)
	if err != nil {
		t.Fatal("migrate rich outputs to the current authored definition", err)
	}
	registry, err := reportingapi.Registry(true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	verifier := domain.f.f.token.verifier
	handler := api.Guard(verifier, registry, reportingapi.Handler(verifier, domain.blocks, http.NotFoundHandler()))
	server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, handler))
	t.Cleanup(server.Close)
	bearer := phase27Token(t, domain.f, domain.blockAuthor.User(), domain.blockAuthor.Session(), phase27Scopes(domain.blockAuthor.Tenant()))
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateBlock(ctx, cw.BlockCreateRequest{ID: "cw02-rich", Definition: definition})
	if err != nil {
		t.Fatal("persist rich definition through actual SDK and closed API", err)
	}
	read, err := client.ReadBlock(ctx, created.State.ID, cw.BlockReference{Draft: true})
	if err != nil || !reflect.DeepEqual(read.Outputs, definition.Outputs) {
		t.Fatal("PostgreSQL/SDK roundtrip lost repeated binding pins", err)
	}
	validation, err := client.ValidateBlock(ctx, created.State.ID, cw.BlockValidateRequest{ExpectedVersion: created.State.Version})
	if err != nil {
		t.Fatal("real execution validates all saved rich shapes", err)
	}
	_, err = client.PublishBlock(ctx, created.State.ID, cw.BlockPublishRequest{ExpectedVersion: validation.State.Version, Evidence: validation.Evidence.ID})
	if err != nil {
		t.Fatal("immutable publication", err)
	}
	published, err := client.ReadBlock(ctx, created.State.ID, cw.BlockReference{})
	if err != nil || !reflect.DeepEqual(published.Outputs, definition.Outputs) {
		t.Fatal("published mapping compatibility", err)
	}
	beforeModels, beforeAttempts := domain.f.model.requests.Load(), domain.attemptCount(t)
	request := phase31Request("block", created.State.ID, "cw02-rich-run")
	// V2 uses omission for authored defaults; the legacy helper sends an
	// explicit empty selection, which the current contract correctly rejects.
	request.Outputs = nil
	run, err := f.service.Run(ctx, domain.execute, request)
	if err != nil || run.Run == "" || run.State != "succeeded" {
		t.Fatalf("actual v2 rich reporting run: %+v %v", run, err)
	}
	if domain.attemptCount(t) != beforeAttempts+1 || domain.f.model.requests.Load() != beforeModels {
		t.Fatal("rich fanout must use one query and no model")
	}
	sealed, err := domain.f.f.db.ReadFrozenRun(ctx, domain.execute, run.Run, false)
	if err != nil {
		t.Fatal("read actual sealed reuse identity", err)
	}
	m := sealed.Manifest
	reuseKey := func(buildVersion int) string {
		t.Helper()
		wire, err := json.Marshal([]any{reporting.FrozenVersion, buildVersion, m.Tenant, m.Block, m.Revision.Digest,
			m.Outputs, m.Resolved.Parameters, m.Resolved.Timezone, m.Locale, readexec.Hash(m.Binding), m.Private, "",
			m.Policy, m.Trust, m.Model, "reporting-output-policy-v2", m.Selection, m.QueryLimits, m.ResultPolicy, m.Limits})
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(wire)
		return hex.EncodeToString(hash[:])
	}
	if m.ReuseKey != reuseKey(charts.BuildVersion) || m.ReuseKey == reuseKey(charts.Version) {
		t.Fatal("frozen reuse must distinguish the current builder from old scalar transport version")
	}
	reader := f.reader(t, created.State.ID, "")
	beforeSource := domain.f.f.lookups.Load()
	viewerClient := f.client(t, f.scopes, false)
	for _, saved := range definition.Outputs {
		retained, err := domain.runs.Output(ctx, reader, run.Run, saved.ID)
		if err != nil || retained.Chart == nil || !reflect.DeepEqual(retained.Chart.Mapping, *saved.Mapping) {
			t.Fatal("actual retained mapping/pins", saved.ID, err)
		}
		rebuilt, err := domain.runs.RebuildOutput(ctx, reader, run.Run, saved.ID)
		if err != nil || rebuilt.Digest != retained.Digest || !reflect.DeepEqual(rebuilt.Chart, retained.Chart) {
			t.Fatal("retained build changed exact values", saved.ID, err)
		}
		view, err := viewerClient.ViewReporting(ctx, cw.ReportingViewRequest{Kind: "block", Run: run.Run, Output: saved.ID, Limit: 1})
		if err != nil || view.Output == nil {
			t.Fatal("actual Apps provider/client consumer", saved.ID, err)
		}
		if saved.ID == "legacy-table" {
			if retained.Chart.Version != 1 || view.Output.Table == nil || len(view.Output.Table.Rows) != 1 || view.PageBounds.Next == nil {
				t.Fatal("old scalar mapping/table paging regressed")
			}
			continue
		}
		if view.Output.Chart == nil || !reflect.DeepEqual(view.Output.Chart, retained.Chart) || len(view.Output.Chart.RowIndices) != 2 {
			t.Fatal("rich shape narrowed in the actual viewer consumer", saved.ID)
		}
		chart := view.Output.Chart
		if saved.ID == "line-multi" && (len(chart.Series) != 2 || chart.Transformation.MissingPoints != 1 || chart.Series[0].Format.Currency != "USD" || chart.Series[1].Format.Unit != "items") {
			t.Fatal("saved multi-unit/missing-value intent lost")
		}
		if saved.ID == "hierarchy" && (len(chart.Hierarchy) != 6 || chart.Hierarchy[2].Depth != 2) {
			t.Fatal("saved hierarchy was flattened")
		}
		if saved.ID == "bubble" && (len(chart.Points) != 2 || chart.Points[0].Size.Exact != "1" || chart.Points[1].Size.Exact != "4") {
			t.Fatal("saved numeric size channel lost")
		}
		// Incompatible metadata cannot be smuggled into the unchanged saved
		// mapping; the rebuild needs a separately reviewed replacement.
		changed := charts.Data{Version: 1, Columns: phase27Copy(t, chart.Columns), Rows: phase27Copy(t, chart.Rows), Completeness: chart.Completeness}
		changed.Columns[0].Provenance.SourceRevision++
		if _, err := charts.Build(ctx, changed, *saved.Mapping, charts.Defaults()); !errors.Is(err, charts.ErrMappingChanged) {
			t.Fatal("saved source revision drift silently rebound", err)
		}
	}
	if domain.f.f.lookups.Load() != beforeSource || domain.f.model.requests.Load() != beforeModels || domain.attemptCount(t) != beforeAttempts+1 {
		t.Fatal("saved output selection/read/rebuild performed source or model work")
	}
	bad := phase27Copy(t, definition)
	bad.Outputs[0].Mapping.Columns[0].Provenance.SourceRevision++
	_, err = client.CreateBlock(ctx, cw.BlockCreateRequest{ID: "cw02-invalid-origin", Definition: bad})
	chartStatus(t, err, http.StatusBadRequest)
	t.Run("stored-integrity", func(t *testing.T) {
		testCW02StoredChartIntegrity(t, f, client, definition)
	})
	// Every JSON body has now crossed actual persistence, closed HTTP, SDK,
	// publication, frozen build and retained Apps view, not just Go field checks.
	wire, err := json.Marshal(published)
	if err != nil || !strings.Contains(string(wire), `"hierarchy":["region","country","product"]`) || strings.Contains(string(wire), "SELECT CASE") {
		t.Fatal("published projection changed bindings or exposed SQL", err)
	}
}
