package rendering

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
)

func retainedAmount() reporting.AmountDisclosure {
	return reporting.AmountDisclosure{Label: "Known <amount>", ValueField: "value", UnknownCountField: "counter", Evidence: "reviewed_definition", DefinitionDigest: strings.Repeat("a", 64), Declaration: "known", Role: "amount", RowsScope: "visible_source_rows", QueryOutcome: "succeeded", Result: nlqexec.AmountCompleteness{Policy: reporting.ReviewedAmountCompletenessPolicy, Metric: "sales:measure:known", ValueColumn: 1, UnknownCountMetric: "sales:kpi:unknown", UnknownCountColumn: 0, Status: "incomplete", Scope: "returned_query_rows", Rows: []nlqexec.AmountCompletenessRow{{Row: 0, Status: "complete", UnknownCount: "0"}, {Row: 2, Status: "incomplete", UnknownCount: "9007199254740993"}}}}
}

func TestAmountDisclosureAllStaticFormats(t *testing.T) {
	v := tableView()
	v.Output.AmountCompleteness = []reporting.AmountDisclosure{retainedAmount()}
	v.Output.Table.RowIndices = []int{2, 0}
	next := 2
	v.PageBounds.Total = 3
	v.PageBounds.Next = &next
	f := &fixtureViewer{value: v}
	s, err := newTestService(f, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	e := authority(t, "reporting.read", "reporting.export")
	for _, format := range []string{"json", "html", "svg", "csv"} {
		t.Run(format, func(t *testing.T) {
			r, err := s.Export(context.Background(), e, exportRequest(format))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(r.Content, "9007199254740993") {
				t.Fatal("exact counter lost")
			}
			if format == "html" || format == "svg" {
				if !strings.Contains(r.Content, "Known &lt;amount&gt;: incomplete") || !strings.Contains(r.Content, "reviewed definition") || !strings.Contains(r.Content, "returned query rows") || strings.Contains(r.Content, "Known <amount>") || !safeStatic(format, r.Content) {
					t.Fatal("unsafe or missing disclosure", r.Content)
				}
			}
			if format == "svg" && !strings.Contains(r.Content, `viewBox="0 0 800 420"`) {
				t.Fatal("footer escaped viewport")
			}
			if format == "csv" {
				rows, err := csv.NewReader(strings.NewReader(r.Content)).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 3 || rows[1][1] != "incomplete" || rows[1][2] != "9007199254740993" || rows[2][1] != "complete" || rows[2][2] != "0" {
					t.Fatal("CSV counter borrowed another sorted row", rows)
				}
				if rows[1][6] != "value" || rows[1][7] != "counter" || rows[1][8] != "count" {
					t.Fatal("CSV counter identity/unit missing", rows)
				}
			}
		})
	}
}

func TestAmountDisclosureRejectsForgeryAndOmission(t *testing.T) {
	good := retainedAmount()
	for _, change := range []func(*reporting.AmountDisclosure){func(d *reporting.AmountDisclosure) { d.Evidence = "unknown" }, func(d *reporting.AmountDisclosure) { d.Result.Status = "complete" }, func(d *reporting.AmountDisclosure) { d.Result.Rows[1].UnknownCount = "0.5" }, func(d *reporting.AmountDisclosure) { d.Unit = "USD" }} {
		d := good
		d.Result.Rows = append([]nlqexec.AmountCompletenessRow(nil), good.Result.Rows...)
		change(&d)
		if _, err := AmountDisclosureLines([]reporting.AmountDisclosure{d}); err == nil {
			t.Fatal("forged disclosure rendered")
		}
	}
	v := tableView()
	v.Output.AmountCompleteness = []reporting.AmountDisclosure{good}
	v.Output.Table.RowIndices = []int{0, 2}
	f := &fixtureViewer{value: v}
	s, _ := newTestService(f, 1<<20)
	request := exportRequest("svg")
	request.Width, request.Height = 320, 200
	if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), request); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("small viewport silently dropped disclosure", err)
	}
	d := good
	d.Truncation = "row_limit"
	d.Result.Status = "unknown"
	d.Result.Rows = nil
	lines, err := AmountDisclosureLines([]reporting.AmountDisclosure{d})
	if err != nil || !strings.Contains(strings.Join(lines, " "), "Unknown amount count (displayed rows): unknown") {
		t.Fatal("truncation became zero", lines, err)
	}
}

func TestEmptyCSVKeepsCoverageEnvelopeWithoutInventingRows(t *testing.T) {
	v := tableView()
	v.Output.Table.Rows = nil
	v.PageBounds = reporting.ViewerPage{Limit: 1}
	d := retainedAmount()
	d.QueryOutcome = "empty"
	d.Result.Status = "complete"
	d.Result.Rows = nil
	v.Output.AmountCompleteness = []reporting.AmountDisclosure{d}
	f := &fixtureViewer{value: v}
	s, _ := newTestService(f, 1<<20)
	r, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("csv"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(r.Content)).ReadAll()
	if err != nil || len(rows) != 1 {
		t.Fatal("CSV fabricated a data row", rows, err)
	}
	if len(r.Projection.AmountCoverage) != 1 || r.Projection.AmountCoverage[0].Evidence != "reviewed_definition" || r.Projection.AmountCoverage[0].Status != "complete" {
		t.Fatal("empty CSV lost its retained coverage envelope")
	}
	d.QueryOutcome = "truncated"
	d.Truncation = "bytes"
	d.Result.Status = "unknown"
	v.Output.AmountCompleteness = []reporting.AmountDisclosure{d}
	f.value = v
	r, err = s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("csv"))
	if err != nil || r.Projection.AmountCoverage[0].Status != "unknown" || r.Projection.AmountCoverage[0].Truncation != "bytes" {
		t.Fatal("empty truncated export certified coverage", err)
	}
}
