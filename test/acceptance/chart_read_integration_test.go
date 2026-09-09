package acceptance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/sourceapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

// This complements the six catalog/HTTP acceptance cases with the actual
// PostgreSQL -> validated executor -> HTTP SDK -> pure output path. Missing real
// database configuration is a failure, never a skipped end-to-end claim.
func TestChartsFromQualifiedReadExecution(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "chart-read-source")
	executor := readExecutor(t, f, nil)
	service, err := chartservice.New(config.DefaultCharts().ServiceOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	readRegistry, err := sourceapi.ExecutionAPIRegistry()
	if err != nil {
		t.Fatal(err)
	}
	chartRegistry, err := chartapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := api.Compose(readRegistry, chartRegistry)
	if err != nil {
		t.Fatal(err)
	}
	h := api.Guard(f.token.verifier, registry, chartapi.Handler(f.token.verifier, service, sourceapi.ExecutionHandler(f.token.verifier, f.validator, executor, http.NotFoundHandler())))
	server := httptest.NewServer(h)
	defer server.Close()
	scopes := append(f.e.Scopes(), "charts.read", "charts.select", "charts.bind", "cw.tenant.read:"+f.e.Tenant())
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), scopes), nil)
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ExecuteRead(context.Background(), source.ID, cw.ReadExecutionRequest{Context: source.ContextID, SQL: `SELECT name,amount,9007199254740993::bigint AS big FROM analytics.sales ORDER BY id`, Execution: cw.ReadExecutionOptions{Operation: "chart-real-read", Number: 1}})
	if err != nil || result.Result == nil || result.Attempt.Status != "succeeded" {
		t.Fatalf("real read did not succeed: %+v %v", result.Attempt, err)
	}
	d, err := cw.ChartDataFromReadResult(context.Background(), *result.Result, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	d.Columns[1].Aggregation = "sum"
	lookups := f.lookups.Load()
	out, err := client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Bar, Bindings: charts.Bindings{Category: "c0", Value: "c1"}, Options: charts.DefaultOptions()})
	if err != nil || out.Output.Points[0].Value.Exact != "9007199254740993.125" || out.Output.Totals[0].Value.Value != "9007199254740998.625" {
		t.Fatalf("native precision lost: %+v %v", out, err)
	}
	table, err := client.SpecifyChart(context.Background(), cw.ChartSpecifyRequest{Data: d, Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"c2", "c1", "c0"}}, Options: charts.DefaultOptions()})
	if err != nil || table.Output.Columns[0].ID != "c2" || table.Output.Rows[0][0].Value != "9007199254740993" {
		t.Fatal("native integer or projected column order changed", err)
	}
	if _, err = client.BuildChart(context.Background(), cw.ChartBuildRequest{Data: d, Mapping: out.Output.Mapping}); err != nil {
		t.Fatal(err)
	}
	if f.lookups.Load() != lookups || f.writeLookups.Load() != 0 {
		t.Fatal("drawing reaccessed a source/credential")
	}
}
