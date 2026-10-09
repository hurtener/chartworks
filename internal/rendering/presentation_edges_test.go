package rendering

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func TestPresentationTableNativePrecisionEdges(t *testing.T) {
	for _, tc := range []struct {
		name, exact, want string
		digits            int
	}{
		{name: "positive_half", exact: "2.5", digits: 0, want: "3"},
		{name: "plus_and_leading_zeroes", exact: "+0002.500", digits: 0, want: "3"},
		{name: "negative_half", exact: "-2.5", digits: 0, want: "-3"},
		{name: "exact_unsigned_zero", exact: "0.000", digits: 0, want: "0"},
		{name: "exact_positive_zero", exact: "+0.000", digits: 0, want: "0"},
		{name: "exact_negative_zero", exact: "-0.000", digits: 0, want: "0"},
		{name: "rounded_negative_zero", exact: "-0.0001", digits: 0, want: "-0"},
		{name: "exponent", exact: "1.234567890123456789e3", digits: 2, want: "1,234.57"},
		{name: "above_binary_integer_precision", exact: "9007199254740993.12567", digits: 2, want: "9,007,199,254,740,993.13"},
		{name: "twenty_places", exact: "0.123456789012345678905", digits: 20, want: "0.12345678901234567891"},
		{name: "browser_large_integer_tie", exact: "9007199254740993.125", digits: 2, want: "9,007,199,254,740,993.13"},
		{name: "browser_negative_tie", exact: "-2.345", digits: 2, want: "-2.35"},
		{name: "browser_plus_leading_zeroes", exact: "+00012.500", digits: 2, want: "12.50"},
		{name: "browser_exact_negative_zero", exact: "-0.000", digits: 2, want: "0.00"},
		{name: "browser_rounded_negative_zero", exact: "-0.0001", digits: 2, want: "-0.00"},
		{name: "browser_positive_exponent", exact: "1.2345e3", digits: 2, want: "1,234.50"},
		{name: "browser_negative_exponent", exact: "12345e-4", digits: 3, want: "1.235"},
		{name: "browser_twenty_place_negative_tie", exact: "-5e-21", digits: 20, want: "-0.00000000000000000001"},
		{name: "browser_zero_place_exponent", exact: "1e3", digits: 0, want: "1,000"},
		{name: "browser_leading_zeroes", exact: "0000", digits: 0, want: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, m := presentationSingleColumnTable(t, tc.exact, charts.Format{FractionDigits: 6})
			base := presentationBuild(t, d, m)
			patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "value", Set: &charts.ColumnPresentationSet{FractionDigits: &tc.digits}}}}, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			built := presentationBuild(t, d, patched)
			presentationAssertCanonical(t, base, built)
			if built.Rows[0][0].Value != tc.exact || d.Rows[0][0].Value != tc.exact {
				t.Fatal("formatting replaced canonical numeric text")
			}
			out := presentationRetainedViewer(t, built)
			presentationAssertStatic(t, out, "light", tc.want)
			scene, _ := presentationRaster(t, out, "light")
			presentationAssertSceneText(t, scene, tc.want)
			wire, err := renderCSV(out, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			records, err := csv.NewReader(bytes.NewReader(wire)).ReadAll()
			csvValue := tc.want
			if strings.HasPrefix(csvValue, "-") {
				csvValue = "'" + csvValue
			}
			if err != nil || len(records) != 2 || len(records[1]) != 1 || records[1][0] != csvValue {
				t.Fatal("CSV precision or formula neutralization diverged", records, err)
			}
		})
	}
}

func TestPresentationTableLabelsEscapeAndKeepPercentSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, percent, exact, label, csvLabel, want string
	}{
		{name: "formula_fraction", percent: "fraction", exact: "0.125", label: "=2+2", csvLabel: "'=2+2", want: "12.5%"},
		{name: "markup_whole", percent: "whole", exact: "12.500", label: "<script>alert(1)</script>", csvLabel: "<script>alert(1)</script>", want: "12.500%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, m := presentationSingleColumnTable(t, tc.exact, charts.Format{Percent: tc.percent, FractionDigits: 3})
			base := presentationBuild(t, d, m)
			patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "value", Set: &charts.ColumnPresentationSet{DisplayLabel: &tc.label}}}}, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			built := presentationBuild(t, d, patched)
			presentationAssertCanonical(t, base, built)
			if built.Columns[0].Format != base.Columns[0].Format || built.Rows[0][0].Value != tc.exact {
				t.Fatal("label-only overlay changed percent representation")
			}
			out := presentationRetainedViewer(t, built)
			presentationAssertStatic(t, out, "light", tc.label, tc.want)
			scene, _ := presentationRaster(t, out, "light")
			presentationAssertSceneText(t, scene, tc.label, tc.want)
			wire, err := renderCSV(out, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			records, err := csv.NewReader(bytes.NewReader(wire)).ReadAll()
			if err != nil || len(records) != 2 || len(records[0]) != 1 || len(records[1]) != 1 || records[0][0] != tc.csvLabel || records[1][0] != tc.want {
				t.Fatal("CSV header safety or percent semantics diverged", records, err)
			}
		})
	}
}

func presentationSingleColumnTable(t *testing.T, exact string, format charts.Format) (charts.Data, charts.Mapping) {
	t.Helper()
	d := charts.Data{Version: charts.Version, Columns: []charts.Column{{ID: "value", Name: "value", DisplayLabel: "Value", Type: "decimal", Role: "measure", Format: format, Provenance: charts.Provenance{Version: 1}}}, Rows: [][]charts.Cell{{{Value: exact}}}, Completeness: charts.Completeness{Status: "complete_result"}}
	m, err := charts.Bind(t.Context(), d, charts.Table, charts.Bindings{Columns: []string{"value"}}, nil, charts.DefaultOptions(), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return d, m
}
