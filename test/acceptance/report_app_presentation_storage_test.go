package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
)

// These exercise the actual PostgreSQL CHECK, including SQL NULL semantics and
// malformed input. Exact renderer roles/no-op normalization remain native rules.
func TestReportAppPresentationStorageConstraint(t *testing.T) {
	ctx := t.Context()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	if err := db.Check(ctx); err != nil {
		t.Fatal("apply presentation migration", err)
	}
	raw := support.Raw(t, dsn)
	if count(t, raw, `SELECT count(*) FROM pg_constraint WHERE conrelid='chartworks.block_revisions'::regclass AND conname='block_presentation_check' AND convalidated`) != 1 {
		t.Fatal("immutable block constraint absent or unvalidated")
	}
	sql(t, raw, `CREATE TEMP TABLE presentation_constraint(definition jsonb CONSTRAINT presentation_check CHECK(chartworks.reporting_presentation_valid(definition)))`)
	base := func(version int) map[string]any {
		return map[string]any{
			"version": version, "kind": "table",
			"columns": []any{
				map[string]any{"id": "id", "type": "integer", "format": map[string]any{"percent": ""}},
				map[string]any{"id": "amount", "type": "decimal", "format": map[string]any{"percent": ""}},
			},
			"bindings":     map[string]any{"columns": []any{"id", "amount"}},
			"table":        map[string]any{"columns": []any{map[string]any{"column": "id", "visible": true}, map[string]any{"column": "amount", "visible": true}}},
			"presentation": map[string]any{"version": 1, "columns": []any{map[string]any{"column": "amount", "display_label": "Displayed amount", "fraction_digits": 2}}},
		}
	}
	presentation := func(m map[string]any) map[string]any { return m["presentation"].(map[string]any) }
	item := func(m map[string]any) map[string]any { return presentation(m)["columns"].([]any)[0].(map[string]any) }
	canonical := func(m map[string]any) map[string]any { return m["columns"].([]any)[1].(map[string]any) }
	format := func(m map[string]any) map[string]any { return canonical(m)["format"].(map[string]any) }
	tests := []struct {
		name   string
		valid  bool
		change func(map[string]any)
	}{
		{"valid", true, func(map[string]any) {}},
		{"legacy omission", true, func(m map[string]any) { delete(m, "presentation") }},
		{"empty label and zero digits", true, func(m map[string]any) { item(m)["display_label"], item(m)["fraction_digits"] = "", 0 }},
		{"maximum digits", true, func(m map[string]any) { item(m)["fraction_digits"] = 20 }},
		{"unicode label byte bound", true, func(m map[string]any) { item(m)["display_label"] = strings.Repeat("é", 128) }},
		{"label only text", true, func(m map[string]any) { delete(item(m), "fraction_digits"); canonical(m)["type"] = "text" }},
		{"digits only", true, func(m map[string]any) { delete(item(m), "display_label") }},
		{"integer format", true, func(m map[string]any) { canonical(m)["type"] = "integer" }},
		{"number format", true, func(m map[string]any) { canonical(m)["type"] = "number" }},
		{"presentation null", false, func(m map[string]any) { m["presentation"] = nil }},
		{"presentation array", false, func(m map[string]any) { m["presentation"] = []any{} }},
		{"presentation scalar", false, func(m map[string]any) { m["presentation"] = true }},
		{"unknown presentation version", false, func(m map[string]any) { presentation(m)["version"] = 2 }},
		{"missing presentation version", false, func(m map[string]any) { delete(presentation(m), "version") }},
		{"null presentation version", false, func(m map[string]any) { presentation(m)["version"] = nil }},
		{"string presentation version", false, func(m map[string]any) { presentation(m)["version"] = "1" }},
		{"unknown presentation field", false, func(m map[string]any) { presentation(m)["currency"] = "EUR" }},
		{"empty overlay columns", false, func(m map[string]any) { presentation(m)["columns"] = []any{} }},
		{"null overlay columns", false, func(m map[string]any) { presentation(m)["columns"] = nil }},
		{"missing overlay columns", false, func(m map[string]any) { delete(presentation(m), "columns") }},
		{"object overlay columns", false, func(m map[string]any) { presentation(m)["columns"] = map[string]any{} }},
		{"null overlay entry", false, func(m map[string]any) { presentation(m)["columns"] = []any{nil} }},
		{"unknown overlay column", false, func(m map[string]any) { item(m)["column"] = "other" }},
		{"null overlay column", false, func(m map[string]any) { item(m)["column"] = nil }},
		{"missing overlay column", false, func(m map[string]any) { delete(item(m), "column") }},
		{"empty override", false, func(m map[string]any) { delete(item(m), "display_label"); delete(item(m), "fraction_digits") }},
		{"semantic field injection", false, func(m map[string]any) { item(m)["type"] = "number" }},
		{"duplicate overlay column", false, func(m map[string]any) { presentation(m)["columns"] = []any{item(m), item(m)} }},
		{"noncanonical overlay order", false, func(m map[string]any) {
			presentation(m)["columns"] = []any{item(m), map[string]any{"column": "id", "display_label": "ID"}}
		}},
		{"null label", false, func(m map[string]any) { item(m)["display_label"] = nil }},
		{"numeric label", false, func(m map[string]any) { item(m)["display_label"] = 1 }},
		{"label exceeds byte bound", false, func(m map[string]any) { item(m)["display_label"] = strings.Repeat("é", 129) }},
		{"non-table label", false, func(m map[string]any) { m["kind"] = "kpi" }},
		{"missing table kind", false, func(m map[string]any) { delete(m, "kind") }},
		{"null digits", false, func(m map[string]any) { item(m)["fraction_digits"] = nil }},
		{"string digits", false, func(m map[string]any) { item(m)["fraction_digits"] = "2" }},
		{"malformed string digits", false, func(m map[string]any) { item(m)["fraction_digits"] = "not-a-number" }},
		{"fractional digits", false, func(m map[string]any) { item(m)["fraction_digits"] = 1.5 }},
		{"negative digits", false, func(m map[string]any) { item(m)["fraction_digits"] = -1 }},
		{"excess digits", false, func(m map[string]any) { item(m)["fraction_digits"] = 21 }},
		{"nonnumeric digits target", false, func(m map[string]any) { canonical(m)["type"] = "text" }},
		{"missing numeric type", false, func(m map[string]any) { delete(canonical(m), "type") }},
		{"null numeric type", false, func(m map[string]any) { canonical(m)["type"] = nil }},
		{"fraction percent", false, func(m map[string]any) { format(m)["percent"] = "fraction" }},
		{"whole percent", false, func(m map[string]any) { format(m)["percent"] = "whole" }},
		{"missing percent", false, func(m map[string]any) { delete(format(m), "percent") }},
		{"null percent", false, func(m map[string]any) { format(m)["percent"] = nil }},
		{"unknown mapping version", false, func(m map[string]any) { m["version"] = 4 }},
		{"missing mapping version", false, func(m map[string]any) { delete(m, "version") }},
		{"null mapping version", false, func(m map[string]any) { m["version"] = nil }},
		{"string mapping version", false, func(m map[string]any) { m["version"] = "1" }},
		{"missing canonical columns", false, func(m map[string]any) { delete(m, "columns") }},
		{"null canonical columns", false, func(m map[string]any) { m["columns"] = nil }},
		{"empty canonical columns", false, func(m map[string]any) { m["columns"] = []any{} }},
		{"duplicate canonical target", false, func(m map[string]any) { m["columns"] = []any{canonical(m), canonical(m)} }},
		{"table unbound column", false, func(m map[string]any) { m["bindings"] = map[string]any{"columns": []any{"id"}} }},
		{"table absent bindings", false, func(m map[string]any) { delete(m, "bindings") }},
		{"table malformed bindings", false, func(m map[string]any) { m["bindings"] = map[string]any{"columns": "amount"} }},
	}
	for _, version := range []int{1, 2, 3} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("v%d/%s", version, tc.name), func(t *testing.T) {
				m := base(version)
				tc.change(m)
				definition, err := json.Marshal(map[string]any{"outputs": []any{map[string]any{"kind": "table", "mapping": m}}})
				if err != nil {
					t.Fatal(err)
				}
				var valid bool
				if err := raw.QueryRow(ctx, `SELECT chartworks.reporting_presentation_valid($1::jsonb)`, definition).Scan(&valid); err != nil || valid != tc.valid {
					t.Fatal("storage validation must return a fail-closed boolean", valid, err)
				}
				_, err = raw.Exec(ctx, `INSERT INTO presentation_constraint VALUES($1::jsonb)`, definition)
				if tc.valid {
					if err != nil {
						t.Fatal("valid structural shape rejected", err)
					}
				} else {
					var pgerr *pgconn.PgError
					if !errors.As(err, &pgerr) || pgerr.Code != "23514" || pgerr.ConstraintName != "presentation_check" {
						t.Fatal("expected actual CHECK rejection", err)
					}
				}
			})
		}
	}
	for _, tc := range []struct {
		name  string
		value any
	}{{"missing", nil}, {"wrong shape", "columns"}, {"hidden", []any{map[string]any{"column": "amount", "visible": false}}}, {"string visibility", []any{map[string]any{"column": "amount", "visible": "true"}}}} {
		t.Run("v3 visibility/"+tc.name, func(t *testing.T) {
			m := base(3)
			m["table"] = map[string]any{"columns": tc.value}
			definition, err := json.Marshal(map[string]any{"outputs": []any{map[string]any{"mapping": m}}})
			if err != nil {
				t.Fatal(err)
			}
			var valid bool
			if err := raw.QueryRow(ctx, `SELECT chartworks.reporting_presentation_valid($1::jsonb)`, definition).Scan(&valid); err != nil || valid {
				t.Fatal("invisible table override accepted", err)
			}
		})
	}
	for _, size := range []int{256, 257} {
		t.Run(fmt.Sprintf("column bound %d", size), func(t *testing.T) {
			m := base(3)
			columns, bindings, visibility, overrides := []any{}, []any{}, []any{}, []any{}
			for i := range size {
				id := fmt.Sprintf("c%d", i)
				columns = append(columns, map[string]any{"id": id, "type": "decimal", "format": map[string]any{"percent": ""}})
				bindings = append(bindings, id)
				visibility = append(visibility, map[string]any{"column": id, "visible": true})
				overrides = append(overrides, map[string]any{"column": id, "fraction_digits": 2})
			}
			m["columns"], m["bindings"], m["table"] = columns, map[string]any{"columns": bindings}, map[string]any{"columns": visibility}
			for _, overlay := range [][]any{overrides[:1], overrides} {
				presentation(m)["columns"] = overlay
				definition, err := json.Marshal(map[string]any{"outputs": []any{map[string]any{"mapping": m}}})
				if err != nil {
					t.Fatal(err)
				}
				var valid bool
				if err := raw.QueryRow(ctx, `SELECT chartworks.reporting_presentation_valid($1::jsonb)`, definition).Scan(&valid); err != nil || valid != (size == 256) {
					t.Fatal("canonical/overlay column bound", valid, err)
				}
			}
		})
	}
}
