package reporting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

func amountDefinition(t *testing.T) Definition {
	t.Helper()
	d := contractDefinition()
	d.ExpectedSchema = append(d.ExpectedSchema, exec.Field{Name: "looks_like_revenue", Type: "integer", NativeType: "int8", Encoding: "string"})
	d.Outputs[0].Mapping.Columns = append(d.Outputs[0].Mapping.Columns, charts.Column{ID: "counter", Name: "looks_like_revenue", Type: "integer", Role: "measure", Provenance: charts.Provenance{Version: 1}})
	d.Outputs[0].Mapping.Bindings.Columns = append(d.Outputs[0].Mapping.Bindings.Columns, "counter")
	if err := seedAmountDeclarations(&d, []nlqexec.CapturedAmountCompleteness{{Metric: "sales:measure:known", ValueColumn: 0, UnknownCountMetric: "sales:kpi:unknown", UnknownCountColumn: 1}}); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReviewedAmountDeclarationBindings(t *testing.T) {
	d := amountDefinition(t)
	if err := validateAmountDeclarations(d); err != nil {
		t.Fatal(err)
	}
	if len(d.Outputs[0].AmountCompleteness) != 2 || d.Outputs[0].AmountCompleteness[1].Role != "unknown_count" {
		t.Fatal("count alias changed its role")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Definition)
	}{
		{"ordinal swap", func(d *Definition) { d.AmountCompleteness[0].ValueColumn = 1 }},
		{"alias tamper", func(d *Definition) { d.AmountCompleteness[0].UnknownCountField.Name = "n" }},
		{"counter type", func(d *Definition) { d.ExpectedSchema[1].Type = "text" }},
		{"drop disclosure", func(d *Definition) { d.Outputs[0].AmountCompleteness = nil }},
		{"count as currency", func(d *Definition) { d.Outputs[0].Mapping.Columns[1].Format.Currency = "USD" }},
		{"role swap", func(d *Definition) { d.Outputs[0].AmountCompleteness[1].Role = "amount" }},
		{"wrong declaration", func(d *Definition) { d.Outputs[0].AmountCompleteness[0].Declaration = "foreign" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := clone(d)
			tc.mutate(&bad)
			if validateAmountDeclarations(bad) == nil {
				t.Fatal("tamper accepted")
			}
		})
	}
	removed := clone(d)
	removed.AmountCompleteness = nil
	if retainAmountDeclarations(d, removed) == nil {
		t.Fatal("whole-definition edit dropped obligations")
	}
	// Selecting the count as a separately reviewed KPI remains supported.
	count := clone(d)
	count.Outputs[0].Kind = "kpi"
	count.Outputs[0].Mapping.Kind = charts.KPI
	count.Outputs[0].Mapping.Bindings = charts.Bindings{Value: "counter"}
	count.Outputs[0].AmountCompleteness = count.Outputs[0].AmountCompleteness[1:]
	if err := validateAmountDeclarations(count); err != nil {
		t.Fatal("explicit count output rejected", err)
	}
	legacy, _ := json.Marshal(contractDefinition())
	if strings.Contains(string(legacy), "amount_completeness") {
		t.Fatal("nil extension changed historical wire")
	}
}

func TestReviewedAmountResultDisclosure(t *testing.T) {
	d := amountDefinition(t)
	o := d.Outputs[0]
	for _, tc := range []struct {
		name, count, status string
		truncated           bool
	}{
		{"zero", "0", "complete", false}, {"positive", "2", "incomplete", false}, {"exact large", "9007199254740993", "incomplete", false}, {"null", "null", "unknown", false}, {"negative", "-1", "unknown", false}, {"fraction", "1.5", "unknown", false}, {"expanded counter bound", "1e9999", "unknown", false}, {"truncated", "0", "unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := exec.Result{Schema: clone(d.ExpectedSchema), Outcome: "succeeded", Rows: [][]json.RawMessage{{json.RawMessage("null"), json.RawMessage(tc.count)}}}
			if tc.truncated {
				r.Truncation = "row_limit"
			}
			got := reviewedAmountDisclosures(d, "digest", o, r)
			if len(got) != 2 || got[0].Evidence != "reviewed_definition" || got[0].Result.Status != tc.status || got[1].Role != "unknown_count" || got[1].Unit != "count" {
				t.Fatalf("disclosure %#v", got)
			}
			if tc.truncated && len(got[0].Result.Rows) != 0 {
				t.Fatal("truncated rows certified")
			}
		})
	}
	empty := reviewedAmountDisclosures(d, "digest", o, exec.Result{Schema: d.ExpectedSchema, Outcome: "empty"})
	if empty[0].Result.Status != "complete" {
		t.Fatal("empty population not represented")
	}
	wrong := clone(d.ExpectedSchema)
	wrong[0], wrong[1] = wrong[1], wrong[0]
	got := reviewedAmountDisclosures(d, "digest", o, exec.Result{Schema: wrong, Outcome: "succeeded"})
	if got[0].Result.Status != "unknown" {
		t.Fatal("ordinal/schema drift certified")
	}
}

func TestReviewedAmountRetainedIntegrity(t *testing.T) {
	d := amountDefinition(t)
	saved := d.Outputs[0]
	m := RunManifest{Revision: Revision{Definition: d, Digest: strings.Repeat("a", 64)}}
	m.Limits.MaxRows = 10
	r := exec.Result{Schema: d.ExpectedSchema, Outcome: "succeeded", Rows: [][]json.RawMessage{{json.RawMessage("8"), json.RawMessage("1")}}}
	out := RetainedOutput{State: "succeeded", AmountCompleteness: reviewedAmountDisclosures(d, m.Revision.Digest, saved, r)}
	if err := checkAmountDisclosures(m, saved, out, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RetainedOutput)
	}{
		{"drop", func(o *RetainedOutput) { o.AmountCompleteness = nil }},
		{"proof claim", func(o *RetainedOutput) { o.AmountCompleteness[0].Evidence = "analytical_receipt" }},
		{"old revision", func(o *RetainedOutput) { o.AmountCompleteness[0].DefinitionDigest = strings.Repeat("b", 64) }},
		{"hidden incomplete", func(o *RetainedOutput) { o.AmountCompleteness[0].Result.Status = "complete" }},
		{"wrong row", func(o *RetainedOutput) { o.AmountCompleteness[0].Result.Rows[0].Row = 1 }},
		{"nonintegral counter", func(o *RetainedOutput) { o.AmountCompleteness[0].Result.Rows[0].UnknownCount = "0.5" }},
		{"amount unit", func(o *RetainedOutput) { o.AmountCompleteness[1].Unit = "USD" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := clone(out)
			tc.mutate(&bad)
			if checkAmountDisclosures(m, saved, bad, false) == nil {
				t.Fatal("forged metadata accepted")
			}
		})
	}
}

func TestAmountDisclosurePagingUsesSourceRows(t *testing.T) {
	d := amountDefinition(t)
	r := exec.Result{Schema: d.ExpectedSchema, Outcome: "succeeded", Rows: [][]json.RawMessage{{json.RawMessage("2"), json.RawMessage("0")}, {json.RawMessage("1"), json.RawMessage("0")}, {json.RawMessage("2"), json.RawMessage("1")}}}
	original := reviewedAmountDisclosures(d, strings.Repeat("a", 64), d.Outputs[0], r)
	page := projectAmountRows(original, []int{0, 2, 1}, 1, 2)
	if len(page) != 2 || page[0].RowsScope != "visible_source_rows" || page[0].Result.Status != "incomplete" || len(page[0].Result.Rows) != 1 || page[0].Result.Rows[0].Row != 2 || page[0].Result.Rows[0].UnknownCount != "1" {
		t.Fatal("tie borrowed a different count", page)
	}
	later := projectAmountRows(original, []int{0, 2, 1}, 2, 3)
	if later[0].Result.Status != "incomplete" || later[0].Result.Rows[0].UnknownCount != "0" || later[0].Result.Rows[0].Row != 1 {
		t.Fatal("page rewrote query-wide scope", later)
	}
	if len(original[0].Result.Rows) != 3 {
		t.Fatal("projection mutated retained evidence")
	}
}

func TestAmountDisclosureHonorsReviewedTableVisibility(t *testing.T) {
	d := amountDefinition(t)
	o := &d.Outputs[0]
	o.Mapping.Version = charts.DisplayVersion
	o.Mapping.Table = &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "c0", Visible: true}, {Column: "counter", Visible: false}}, PageSize: 10}
	// A hidden companion need not become an additional visible metric. The
	// displayed amount still explicitly carries its required count disclosure.
	o.AmountCompleteness = o.AmountCompleteness[:1]
	if err := validateAmountDeclarations(d); err != nil {
		t.Fatal(err)
	}
	o.Mapping.Table.Columns[0].Visible = false
	o.Mapping.Table.Columns[1].Visible = true
	if validateAmountDeclarations(d) == nil {
		t.Fatal("hidden amount still claimed display authority")
	}
	o.AmountCompleteness = []AmountOutputBinding{{Declaration: d.AmountCompleteness[0].ID, Role: "unknown_count"}}
	if err := validateAmountDeclarations(d); err != nil {
		t.Fatal("explicit visible count rejected", err)
	}
}
