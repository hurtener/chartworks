package charts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func presentationPointer[T any](value T) *T { return &value }
func presentationWire(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}
func presentationPatch(edits ...charts.ColumnPresentationEdit) charts.PresentationPatch {
	return charts.PresentationPatch{Version: charts.PresentationVersion, Edits: edits}
}
func presentationDigits(id string, digits int) charts.ColumnPresentationEdit {
	return charts.ColumnPresentationEdit{Column: id, Set: &charts.ColumnPresentationSet{FractionDigits: &digits}}
}
func presentationTable(t *testing.T) (charts.Data, charts.Mapping) {
	t.Helper()
	d := displayData()
	m, err := charts.BindDisplay(t.Context(), d, charts.Table, charts.Bindings{Columns: []string{"period", "actual", "baseline"}}, []charts.Order{}, charts.DefaultOptions(), nil, &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "period", Visible: true}, {Column: "actual", Visible: true}, {Column: "baseline", Visible: false}}, PageSize: 25, ShowTotals: true}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return d, m
}
func presentationBuild(t *testing.T, d charts.Data, m charts.Mapping) charts.Output {
	t.Helper()
	out, err := charts.Build(t.Context(), d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func assertPresentationOnly(t *testing.T, before, after charts.Output) {
	t.Helper()
	after.Columns = before.Columns
	after.Mapping.Presentation = before.Mapping.Presentation
	if !reflect.DeepEqual(before, after) {
		t.Fatal("display-only edit changed canonical metadata, exact values, nullness, calculations, geometry or membership")
	}
}

func TestPresentationPatchDetachedCanonicalOrderResetAndNilness(t *testing.T) {
	d, m := presentationTable(t)
	m.Bindings.Values = []string{}
	m.Bindings.Hierarchy = []string{}
	before, source := presentationWire(t, m), presentationWire(t, d)
	patch := presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{DisplayLabel: presentationPointer(""), FractionDigits: presentationPointer(0)}}, charts.ColumnPresentationEdit{Column: "period", Set: &charts.ColumnPresentationSet{DisplayLabel: presentationPointer("Accounting month")}})
	changed, err := charts.ApplyPresentationPatch(t.Context(), m, patch, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version != m.Version || len(changed.Presentation.Columns) != 2 || changed.Presentation.Columns[0].Column != "period" || changed.Presentation.Columns[1].Column != "actual" {
		t.Fatal("overlay lost canonical mapping order/version", changed)
	}
	canonical := changed
	canonical.Presentation = nil
	if !reflect.DeepEqual(canonical, m) || !bytes.Equal(presentationWire(t, m), before) || !bytes.Equal(presentationWire(t, d), source) {
		t.Fatal("canonical input or nilness changed")
	}
	built := presentationBuild(t, d, changed)
	base := presentationBuild(t, d, m)
	assertPresentationOnly(t, base, built)
	if built.Columns[1].DisplayLabel != "" || built.Columns[1].Format.FractionDigits != 0 || built.Mapping.Columns[1].DisplayLabel != "Revenue" || built.Mapping.Columns[1].Format.FractionDigits != 2 {
		t.Fatal("zero/empty override lost or baseline mutated")
	}
	projected, err := charts.ProjectPresentationColumns(t.Context(), changed, charts.Defaults())
	if err != nil || !reflect.DeepEqual(projected, built.Columns) {
		t.Fatal("frozen projection differs from native build", err)
	}
	*patch.Edits[0].Set.FractionDigits = 19
	*built.Mapping.Presentation.Columns[1].FractionDigits = 18
	built.Columns[1].Format.Unit = "mutated"
	if *changed.Presentation.Columns[1].FractionDigits != 0 || changed.Columns[1].Format.Unit != "" {
		t.Fatal("overlay pointers or renderer metadata alias caller input")
	}
	var roundtrip charts.Mapping
	if err := json.Unmarshal(presentationWire(t, changed), &roundtrip); err != nil || !bytes.Equal(presentationWire(t, roundtrip), presentationWire(t, changed)) {
		t.Fatal("presentation roundtrip", err)
	}
	reset, err := charts.ApplyPresentationPatch(t.Context(), changed, presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Reset: []charts.PresentationField{charts.PresentationDisplayLabel, charts.PresentationFractionDigits}}, charts.ColumnPresentationEdit{Column: "period", Reset: []charts.PresentationField{charts.PresentationDisplayLabel}}), charts.Defaults())
	if err != nil || reset.Presentation != nil || !reflect.DeepEqual(reset, m) || !bytes.Equal(presentationWire(t, reset), before) {
		t.Fatal("last reset did not restore nil extension and exact canonical bytes", err)
	}
	if _, err = charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits("actual", 2)), charts.Defaults()); !errors.Is(err, charts.ErrPresentationNoop) {
		t.Fatal("canonical no-op accepted", err)
	}
	if _, err = charts.ApplyPresentationPatch(t.Context(), changed, presentationPatch(presentationDigits("actual", 0)), charts.Defaults()); !errors.Is(err, charts.ErrPresentationNoop) {
		t.Fatal("identical edit accepted", err)
	}
	if _, err = charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Reset: []charts.PresentationField{charts.PresentationFractionDigits}}), charts.Defaults()); !errors.Is(err, charts.ErrPresentationNoop) {
		t.Fatal("absent reset created presentation", err)
	}
}

func TestPresentationCapabilitiesOnlyConsumedRoles(t *testing.T) {
	_, table := presentationTable(t)
	d := displayData()
	kpi, err := charts.BindDisplay(t.Context(), d, charts.KPI, charts.Bindings{Category: "period", Value: "actual", Comparison: "baseline", Target: "target"}, nil, charts.DefaultOptions(), &charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowTargetDifference: true, Sparkline: true}, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		m      charts.Mapping
		want   map[string]string
		labels bool
	}{{"table", table, map[string]string{"period": "table_column", "actual": "table_column"}, true}, {"kpi", kpi, map[string]string{"actual": "value", "target": "target"}, false}}
	for _, name := range []string{"line_two_units", "bubble_series", "hierarchy_three_levels"} {
		f := richFixture(t, name)
		m, _ := richBuild(t, f)
		want := map[string]string{}
		for _, c := range m.Columns {
			switch c.ID {
			case m.Bindings.Value:
				want[c.ID] = "value"
			case m.Bindings.X:
				want[c.ID] = "x"
			case m.Bindings.Y:
				want[c.ID] = "y"
			default:
				if slices.Contains(m.Bindings.Values, c.ID) {
					want[c.ID] = "values"
				}
			}
		}
		cases = append(cases, struct {
			name   string
			m      charts.Mapping
			want   map[string]string
			labels bool
		}{name, m, want, false})
	}
	_, legacyKPI := bind(t, charts.KPI)
	cases = append(cases, struct {
		name   string
		m      charts.Mapping
		want   map[string]string
		labels bool
	}{"legacy_kpi", legacyKPI, map[string]string{}, false})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caps, err := charts.PresentationCapabilitiesFor(t.Context(), tc.m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			if caps.Version != 1 || len(caps.Columns) != len(tc.want) {
				t.Fatal("incorrect capabilities", caps)
			}
			for _, c := range caps.Columns {
				if tc.want[c.Column] != c.Role || slices.Contains(c.Fields, charts.PresentationDisplayLabel) != tc.labels {
					t.Fatal("inert or mislabeled control", c)
				}
			}
		})
	}
	// Numeric categories are admitted and consumed by pointLabel, while series
	// and hierarchy path captions remain raw and cannot claim precision control.
	pd, pb, po := chartfixtures.Fixture(charts.Bar, "binding")
	pd.Columns[0].Type = "integer"
	pd.Columns[0].Role = "dimension"
	for i := range pd.Rows {
		pd.Rows[i][0].Value = []string{"1", "2", "3"}[i]
	}
	numericCategory, err := charts.Bind(t.Context(), pd, charts.Bar, pb, po, charts.DefaultOptions(), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	caps, err := charts.PresentationCapabilitiesFor(t.Context(), numericCategory, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Columns[0].Column != "category" || caps.Columns[0].Role != "category" || !slices.Contains(caps.Columns[0].Fields, charts.PresentationFractionDigits) {
		t.Fatal("consumed numeric category control missing", caps)
	}
}

func TestPresentationNativeSemanticAndBoundRejections(t *testing.T) {
	d, m := presentationTable(t)
	invalid := []charts.PresentationPatch{
		{}, {Version: 2, Edits: []charts.ColumnPresentationEdit{presentationDigits("actual", 3)}}, presentationPatch(),
		presentationPatch(presentationDigits("missing", 3)), presentationPatch(presentationDigits("target", 3)), presentationPatch(presentationDigits("baseline", 3)), presentationPatch(presentationDigits("period", 3)),
		presentationPatch(presentationDigits("actual", -1)), presentationPatch(presentationDigits("actual", 21)), presentationPatch(presentationDigits("actual", 3), presentationDigits("actual", 4)),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual"}), presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Reset: []charts.PresentationField{"unit"}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Reset: []charts.PresentationField{charts.PresentationDisplayLabel, charts.PresentationDisplayLabel}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{FractionDigits: presentationPointer(3)}, Reset: []charts.PresentationField{charts.PresentationFractionDigits}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{FractionDigits: presentationPointer(3)}, Reset: []charts.PresentationField{}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{DisplayLabel: presentationPointer(strings.Repeat("x", 257))}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{DisplayLabel: presentationPointer("bad\x00label")}}),
		presentationPatch(charts.ColumnPresentationEdit{Column: "actual", Set: &charts.ColumnPresentationSet{DisplayLabel: presentationPointer(string([]byte{0xff}))}}),
	}
	for i, p := range invalid {
		if _, err := charts.ApplyPresentationPatch(t.Context(), m, p, charts.Defaults()); err == nil {
			t.Fatalf("invalid patch %d accepted", i)
		}
	}
	for _, percentage := range []string{"fraction", "whole"} {
		pm := m
		pm.Columns = slices.Clone(m.Columns)
		pm.Columns[1].Format.Currency = ""
		pm.Columns[1].Format.CurrencySymbol = ""
		pm.Columns[1].Format.Percent = percentage
		if _, err := charts.ApplyPresentationPatch(t.Context(), pm, presentationPatch(presentationDigits("actual", 3)), charts.Defaults()); err == nil {
			t.Fatal("percent precision falsely supported", percentage)
		}
	}
	valid, err := charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits("actual", 3)), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*charts.Mapping){
		func(m *charts.Mapping) { m.Presentation.Version = 2 }, func(m *charts.Mapping) { m.Presentation.Columns = nil }, func(m *charts.Mapping) { m.Presentation.Columns[0].Column = "target" }, func(m *charts.Mapping) {
			m.Presentation.Columns = append(m.Presentation.Columns, m.Presentation.Columns[0])
		}, func(m *charts.Mapping) { m.Presentation.Columns[0].FractionDigits = nil }, func(m *charts.Mapping) { *m.Presentation.Columns[0].FractionDigits = 2 }, func(m *charts.Mapping) { *m.Presentation.Columns[0].FractionDigits = 21 }, func(m *charts.Mapping) {
			m.Presentation.Columns = append(m.Presentation.Columns, charts.ColumnDisplayOverride{Column: "period", DisplayLabel: presentationPointer("late")})
		},
	} {
		var bad charts.Mapping
		if err := json.Unmarshal(presentationWire(t, valid), &bad); err != nil {
			t.Fatal(err)
		}
		mutate(&bad)
		if err := charts.ValidateMapping(t.Context(), d, bad, charts.Defaults()); err == nil {
			t.Fatal("invalid stored extension accepted", bad.Presentation)
		}
	}
	// The overlay must never weaken complete saved-column equality.
	for _, mutate := range []func(*charts.Column){func(c *charts.Column) { c.Name = "elsewhere" }, func(c *charts.Column) { c.Format.FractionDigits = 4 }, func(c *charts.Column) { c.Format.Unit = "kg" }, func(c *charts.Column) { c.Format.Currency = "EUR" }, func(c *charts.Column) { c.Role = "measure" }, func(c *charts.Column) { c.Provenance.Source = "other"; c.Provenance.SourceRevision = 1 }} {
		bad := valid
		bad.Columns = slices.Clone(valid.Columns)
		mutate(&bad.Columns[1])
		if err := charts.ValidateMapping(t.Context(), d, bad, charts.Defaults()); err == nil {
			t.Fatal("canonical pin mismatch accepted")
		}
	}
	legacyData, legacyKPI := bind(t, charts.KPI)
	legacyKPI.Presentation = &charts.ColumnPresentation{Version: 1, Columns: []charts.ColumnDisplayOverride{{Column: legacyKPI.Bindings.Value, FractionDigits: presentationPointer(20)}}}
	if _, err := charts.Build(t.Context(), legacyData, legacyKPI, charts.Defaults()); err == nil {
		t.Fatal("legacy KPI accepted unsupported presentation injection")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := charts.ApplyPresentationPatch(ctx, m, presentationPatch(presentationDigits("actual", 3)), charts.Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if _, err := charts.ApplyPresentationPatch(nil, m, presentationPatch(presentationDigits("actual", 3)), charts.Defaults()); !errors.Is(err, charts.ErrInvalid) {
		t.Fatal("nil context accepted", err)
	}
}

func TestPresentationAllNativeBuildPathsPreserveEvidence(t *testing.T) {
	for _, kind := range charts.Catalog() {
		if kind.Kind == charts.KPI {
			continue
		}
		t.Run(string(kind.Kind), func(t *testing.T) {
			d, m := bind(t, kind.Kind)
			base := presentationBuild(t, d, m)
			caps, err := charts.PresentationCapabilitiesFor(t.Context(), m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for _, cap := range caps.Columns {
				if !slices.Contains(cap.Fields, charts.PresentationFractionDigits) {
					continue
				}
				var digits int
				for _, c := range m.Columns {
					if c.ID == cap.Column {
						digits = (c.Format.FractionDigits + 1) % 21
					}
				}
				changed, err := charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits(cap.Column, digits)), charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				assertPresentationOnly(t, base, presentationBuild(t, d, changed))
				a, err := charts.BuildWithSourceRows(t.Context(), d, m, charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				b, err := charts.BuildWithSourceRows(t.Context(), d, changed, charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				assertPresentationOnly(t, a, b)
			}
		})
	}
	for _, f := range chartfixtures.RichCases() {
		t.Run(f.Name, func(t *testing.T) {
			m, base := richBuild(t, f)
			caps, err := charts.PresentationCapabilitiesFor(t.Context(), m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for _, cap := range caps.Columns {
				if !slices.Contains(cap.Fields, charts.PresentationFractionDigits) {
					continue
				}
				var digits int
				for _, c := range m.Columns {
					if c.ID == cap.Column {
						digits = (c.Format.FractionDigits + 1) % 21
					}
				}
				changed, err := charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits(cap.Column, digits)), charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				assertPresentationOnly(t, base, presentationBuild(t, f.Data, changed))
			}
		})
	}
}

func TestPresentationKPIExactPrecisionNeverFeedsBack(t *testing.T) {
	for _, raw := range []string{"9007199254740993.12345678901234567890", "1.23456789012345678901", "2.5", "-2.5", "-0.000", "0", "1.23456789e5", "1.23456789e-5", "NULL"} {
		for _, digits := range []int{0, 20} {
			t.Run(raw+"/"+strconv.Itoa(digits), func(t *testing.T) {
				d := displayData()
				d.Rows = d.Rows[:1]
				d.Rows[0][1] = charts.Cell{Value: raw}
				if raw == "NULL" {
					d.Rows[0][1] = charts.Cell{Null: true}
				}
				kpi := &charts.KPIOptions{ValueRow: "first", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, ShowTargetDifference: true, Thresholds: []charts.KPIThreshold{{Operator: "gte", Value: "0", State: "nonnegative"}}}
				m, err := charts.BindDisplay(t.Context(), d, charts.KPI, charts.Bindings{Value: "actual", Comparison: "baseline", Target: "target"}, nil, charts.DefaultOptions(), kpi, nil, charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				base := presentationBuild(t, d, m)
				changed, err := charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits("actual", digits)), charts.Defaults())
				if err != nil {
					t.Fatal(err)
				}
				assertPresentationOnly(t, base, presentationBuild(t, d, changed))
				d.Rows[0][2].Value = "0"
				zeroBase := presentationBuild(t, d, changed)
				if zeroBase.KPIResult.PercentDelta != nil {
					t.Fatal("zero baseline fabricated percent delta")
				}
			})
		}
	}
}

func TestPresentationConcurrentDetachedReuse(t *testing.T) {
	d, m := presentationTable(t)
	patch := presentationPatch(presentationDigits("actual", 3))
	changed, err := charts.ApplyPresentationPatch(t.Context(), m, patch, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	want := presentationWire(t, presentationBuild(t, d, changed))
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := charts.Build(t.Context(), d, changed, charts.Defaults())
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(want, presentationWire(t, out)) {
				t.Error("concurrent output drift")
			}
			*out.Mapping.Presentation.Columns[0].FractionDigits = 17
			out.Columns[1].DisplayLabel = "local"
		}()
	}
	wg.Wait()
	if *changed.Presentation.Columns[0].FractionDigits != 3 || *patch.Edits[0].Set.FractionDigits != 3 {
		t.Fatal("concurrent output aliases source")
	}
}

func TestPresentationRebindNeverRetargetsAnOverride(t *testing.T) {
	d, m := presentationTable(t)
	for i := range d.Columns {
		if d.Columns[i].ID == "actual" {
			d.Columns[i].Provenance.Topic = "topic"
			d.Columns[i].Provenance.TopicVersion = "v1"
			d.Columns[i].Provenance.SemanticID = "actual"
			m.Columns[1] = d.Columns[i]
		}
	}
	changed, err := charts.ApplyPresentationPatch(t.Context(), m, presentationPatch(presentationDigits("actual", 3)), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := charts.Rebind(t.Context(), d, changed, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	*proposal.Mapping.Presentation.Columns[0].FractionDigits = 4
	if *changed.Presentation.Columns[0].FractionDigits != 3 {
		t.Fatal("proposal aliases stored overlay")
	}
	for i := range d.Columns {
		if d.Columns[i].ID == "actual" {
			d.Columns[i].ID = "new_actual"
			d.Columns[i].Name = "new_actual"
		}
	}
	if _, err := charts.Rebind(t.Context(), d, changed, charts.Defaults()); err == nil {
		t.Fatal("overlay silently retargeted after semantic rebind")
	}
}
