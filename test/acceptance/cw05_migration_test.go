package acceptance

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/test/support"
)

// TestCW05DisplayIntentConstraint runs migration 043 in PostgreSQL and exercises
// its actual CHECK boundary. Go remains the semantic validator; storage rejects
// malformed v3 shapes even when a writer bypasses it.
func TestCW05DisplayIntentConstraint(t *testing.T) {
	ctx := context.Background()
	dsn := support.Database(t)
	raw := upgradeFixture(t, dsn)
	db := support.Open(t, dsn)
	if err := db.Check(ctx); err != nil {
		t.Fatal("apply migration 043", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=42 AND name='migrations/042_learning_templates.sql'`) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=43 AND name='migrations/043_reporting_display_intent.sql'`) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations WHERE version=44 AND name='migrations/044_reporting_question_assessments.sql'`) != 1 {
		t.Fatal("migrations 043-044 did not preserve the populated upgrade sequence")
	}
	sql(t, raw, `CREATE TEMP TABLE cw05_display_constraint(definition jsonb CHECK(chartworks.reporting_display_intent_valid(definition)))`)

	column := `{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"items","currency":"","percent":"","fraction_digits":2},"provenance":{"version":1,"source":"","source_revision":0,"topic":"","topic_version":"","semantic_id":""}}`
	validTable := `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[` + column + `],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`
	validKPI := `{"outputs":[{"kind":"kpi","mapping":{"version":3,"kind":"kpi","columns":[` + column + `],"bindings":{"value":"amount"},"kpi":{"value_row":"last","comparison_mode":"none","show_delta":true,"show_percent_delta":true,"show_target_difference":false,"sparkline":false,"thresholds":[]}}}]}`
	withThreshold := func(value string) string {
		threshold := `"thresholds":[{"operator":"gte","value":"` + value + `","state":"reviewed"}]`
		return strings.Replace(validKPI, `"thresholds":[]`, threshold, 1)
	}
	for _, definition := range []string{`{"outputs":[{"kind":"table","mapping":{"version":2}}]}`, validTable, validKPI} {
		if _, err := raw.Exec(ctx, `INSERT INTO cw05_display_constraint VALUES($1::jsonb)`, definition); err != nil {
			t.Fatal("valid or legacy display intent rejected", err)
		}
	}
	for _, value := range []string{"1e3", "-2.5E-4", "1e00000", "1e+4096", strings.Repeat("1", 4096)} {
		if _, err := raw.Exec(ctx, `INSERT INTO cw05_display_constraint VALUES($1::jsonb)`, withThreshold(value)); err != nil {
			t.Fatal("Go-compatible scientific threshold rejected", value[:min(len(value), 32)], err)
		}
	}

	invalid := []struct {
		name       string
		definition string
	}{
		{"table options missing", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[` + column + `],"bindings":{"columns":["amount"]},"table":{}}}]}`},
		{"table bindings missing", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[` + column + `],"bindings":{},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"table has no visible column", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[` + column + `],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":false}],"page_size":100,"show_totals":true}}}]}`},
		{"table page size exceeds bound", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[` + column + `],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":1001,"show_totals":true}}}]}`},
		{"kpi options missing", `{"outputs":[{"kind":"kpi","mapping":{"version":3,"kind":"kpi","columns":[` + column + `],"bindings":{"value":"amount"},"kpi":{}}}]}`},
		{"kpi value binding missing", `{"outputs":[{"kind":"kpi","mapping":{"version":3,"kind":"kpi","columns":[` + column + `],"bindings":{},"kpi":{"value_row":"last","comparison_mode":"none","show_delta":true,"show_percent_delta":true,"show_target_difference":false,"sparkline":false,"thresholds":[]}}}]}`},
		{"column format missing", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"unit exceeds bound", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"12345678901234567890123456789012345678901234567890123456789012345","currency":"","percent":"","fraction_digits":2}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"currency enum malformed", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"","currency":"usd","percent":"","fraction_digits":2}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"percent enum malformed", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"","currency":"","percent":"ratio","fraction_digits":2}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"invalid currency percent combination", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"","currency":"USD","percent":"whole","fraction_digits":2}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"fraction digits exceed bound", `{"outputs":[{"kind":"table","mapping":{"version":3,"kind":"table","columns":[{"id":"amount","name":"amount","type":"decimal","role":"measure","grain":"","aggregation":"sum","format":{"unit":"","currency":"","percent":"","fraction_digits":21}}],"bindings":{"columns":["amount"]},"table":{"columns":[{"column":"amount","visible":true}],"page_size":100,"show_totals":true}}}]}`},
		{"threshold exponent above bound", withThreshold("1e4097")},
		{"threshold negative exponent below bound", withThreshold("1e-4097")},
		{"threshold exponent text too long", withThreshold("1e000000")},
		{"threshold signed exponent text too long", withThreshold("1e+04096")},
		{"threshold malformed exponent", withThreshold("1e+")},
		{"threshold text exceeds bound", withThreshold(strings.Repeat("1", 4097))},
	}
	for _, tc := range invalid {
		if _, err := raw.Exec(ctx, `INSERT INTO cw05_display_constraint VALUES($1::jsonb)`, tc.definition); err == nil {
			t.Fatal("migration constraint accepted malformed v3 intent", tc.name)
		}
	}
}
