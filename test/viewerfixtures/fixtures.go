// Package viewerfixtures executes the actual bundled viewer against synthetic
// outputs produced by the real chart engine. Missing browser tooling fails; it
// never silently substitutes static HTML inspection for component acceptance.
package viewerfixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/chartfixtures"
	reportviewer "github.com/hurtener/chartworks/web/report-viewer"
)

type chartCase struct {
	Kind         charts.Kind    `json:"kind"`
	Scenario     string         `json:"scenario"`
	Output       *charts.Output `json:"output,omitempty"`
	Error        string         `json:"error,omitempty"`
	ExpectedRows [][]*string    `json:"expected_rows"`
}

type fixture struct {
	Cases       []chartCase                      `json:"cases"`
	View        reporting.ReportingViewResult    `json:"view"`
	Description reporting.ReportingDescription   `json:"description"`
	Run         reporting.ReportingRunResult     `json:"run"`
	Table       charts.Output                    `json:"table"`
	Precision   charts.Output                    `json:"precision"`
	Percent     charts.Output                    `json:"percent"`
}

func expectedRows(out charts.Output) [][]*string {
	rows := [][]*string{}
	cell := func(v charts.Cell) *string {
		if v.Null { return nil }
		copy := v.Value
		return &copy
	}
	value := func(v charts.Value) *string {
		if v.Null { return nil }
		copy := v.Exact
		return &copy
	}
	if out.Kind == charts.Table {
		for _, source := range out.Rows {
			row := []*string{}
			for _, v := range source { row = append(row, cell(v)) }
			rows = append(rows, row)
		}
		return rows
	}
	b := out.Mapping.Bindings
	for _, p := range out.Points {
		row := []*string{}
		for _, c := range out.Columns {
			switch c.ID {
			case b.Category: row = append(row, cell(p.Category))
			case b.Series: row = append(row, cell(p.Series))
			case b.Parent: row = append(row, cell(p.Parent))
			case b.X: row = append(row, value(p.X))
			case b.Y: row = append(row, value(p.Y))
			case b.Value: row = append(row, value(p.Value))
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func build(t *testing.T, kind charts.Kind, edit func(*charts.Data)) charts.Output {
	t.Helper()
	d, bindings, order := chartfixtures.Fixture(kind, "binding")
	if edit != nil { edit(&d) }
	mapping, err := charts.Bind(context.Background(), d, kind, bindings, order, charts.DefaultOptions(), charts.Defaults())
	if err != nil { t.Fatal("actual fixture binding", err) }
	out, err := charts.Build(context.Background(), d, mapping, charts.Defaults())
	if err != nil { t.Fatal("actual fixture normalization", err) }
	return out
}

func produce(t *testing.T) fixture {
	t.Helper()
	out := fixture{Cases: []chartCase{}}
	for _, entry := range charts.Catalog() {
		for _, scenario := range chartfixtures.Cases() {
			g := chartfixtures.Produce(entry.Kind, scenario)
			c := chartCase{Kind: entry.Kind, Scenario: scenario, Output: g.Output, Error: g.Error, ExpectedRows: [][]*string{}}
			if g.Output != nil { c.ExpectedRows = expectedRows(*g.Output) }
			out.Cases = append(out.Cases, c)
		}
	}
	out.Table = build(t, charts.Table, nil)
	out.Precision = build(t, charts.Bar, func(d *charts.Data) {
		d.Rows[0][1].Value = "9007199254740993.0100"
		d.Rows[1][1].Value = "0.0200"
		d.Rows[2][1].Value = "-0.0001"
		d.Completeness = charts.Completeness{Status: "truncated", Reason: "rows"}
	})
	out.Percent = build(t, charts.KPI, func(d *charts.Data) {
		d.Rows[0][1].Value = "0.123456789123456789"
		d.Columns[1].Format.Percent = "fraction"
	})
	created := time.Now().UTC().Truncate(time.Second)
	target := reporting.DeliveryTarget{Kind: "block", ID: "viewer-fixture", Revision: 1}
	choices := []reporting.ViewerOutputChoice{{ID: "table-main", Kind: "table", Title: "Table one"}, {ID: "table-second", Kind: "table", Title: "Table two"}}
	filters := []reporting.ViewerFilter{{Page: "main", Label: "Minimum", Parameter: reporting.Parameter{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}}
	next := 2
	out.View = reporting.ReportingViewResult{
		Version: reporting.DeliveryVersion,
		Summary: reporting.ReportingRunSummary{Kind: "block", Run: "viewer-run", Target: target, State: "succeeded", Created: created, Expires: created.Add(time.Hour)},
		Selection: reporting.ReportingViewRequest{Kind: "block", Run: "viewer-run", Output: "table-main", Limit: 2},
		Locale: "en", Timezone: "UTC", Outputs: choices, Pages: []reporting.CompositionPageSummary{}, Filters: filters,
		Trust: &reporting.Trust{Publication: "published", Certification: "uncertified", Health: reporting.Health{Status: "healthy"}},
		Observed: &created,
		Output: &reporting.ViewerOutput{ID: "table-main", Kind: "table", State: "succeeded", RetainedDigest: "fixture",
			Table: &reporting.ViewerTable{Columns: out.Table.Columns, Rows: out.Table.Rows[:2], Totals: out.Table.Totals, Completeness: out.Table.Completeness, Warnings: out.Table.Warnings}},
		PageBounds: reporting.ViewerPage{Offset: 0, Limit: 2, Total: len(out.Table.Rows), Next: &next},
	}
	out.Description = reporting.ReportingDescription{Version: reporting.DeliveryVersion,
		Resource: reporting.ReportingResource{Target: target, Title: target.ID, Locale: "en"},
		Outputs: choices, Filters: filters, Pages: []reporting.CompositionPageSummary{}, Timezone: "UTC"}
	out.Run = reporting.ReportingRunResult{Version: reporting.DeliveryVersion, Kind: "block", Run: "viewer-new-run", State: "succeeded", Target: target}
	return out
}

// Run launches an actual browser for the named component suite. Phase31 calls
// this helper directly, so a green acceptance result necessarily exercised the
// component. CI uses Node 22+ and Chrome; CHARTWORKS_CHROME_BIN supports an
// explicitly installed Chromium/Chrome binary on other development platforms.
func Run(t *testing.T, suite string) {
	t.Helper()
	if suite != "charts" && suite != "interaction" && suite != "security" && suite != "all" { t.Fatal("unknown component suite") }
	_, file, _, ok := runtime.Caller(0)
	if !ok { t.Fatal("cannot resolve bundled component test") }
	script := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "web", "report-viewer", "component.test.mjs"))
	node, err := exec.LookPath("node")
	if err != nil { t.Fatal("Phase31 component acceptance requires Node 22+ and Chrome; no browser tests were skipped", err) }
	directory := t.TempDir()
	htmlPath, fixturePath := filepath.Join(directory,"resource.html"), filepath.Join(directory,"fixture.json")
	body, err := json.Marshal(produce(t))
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(htmlPath, []byte(reportviewer.HTML()), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(fixturePath, body, 0600); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, script, htmlPath, fixturePath, suite)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil { t.Fatalf("actual viewer component %s: %v\n%s\n%s", suite, err, stdout.String(), stderr.String()) }
	var receipt struct {
		Suite string `json:"suite"`
		Passes int `json:"passes"`
		Kinds int `json:"kinds"`
		Engine string `json:"engine"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil || receipt.Suite != suite || receipt.Passes < 10 || receipt.Engine != "Chromium actual bundled resource" || (suite == "charts" || suite == "all") && receipt.Kinds != 14 {
		t.Fatalf("missing component behavior receipt: %s %v", stdout.String(), err)
	}
	t.Logf("component %s: %d browser assertions; engine=%s", suite, receipt.Passes, receipt.Engine)
}
