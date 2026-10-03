package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// These are deliberately structural storage fixtures, not executable analytical
// claims. Native service qualification separately proves receipt reconstruction.
func TestScalarEntailmentPopulatedMigration(t *testing.T) {
	ctx := t.Context()
	dsn := support.Database(t)
	raw := upgradeFixture(t, dsn)
	migrations, err := postgres.Migrations()
	if err != nil || len(migrations) != 81 || migrations[80].Name != "migrations/081_nlq_scalar_predicate_entailment.sql" {
		t.Fatal("migration identity", err)
	}
	for _, m := range migrations[1:80] {
		sql(t, raw, m.SQL)
		sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, m.Version, m.Name, m.Checksum)
	}
	sql(t, raw, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale) VALUES('scalar-upgrade','actor','session','context','["topic"]','en')`)
	receipt := func(version int) map[string]any {
		if version == 0 {
			return nil
		}
		out := map[string]any{"version": fmt.Sprintf("analytical-metrics-v%d", version), "scope": exec.AnalyticalMetricScope, "contract": exec.Hash("retained-reviewed-contract"), "query": exec.AnalyticalQueryDigest("SELECT 1", nil), "metrics": []string{"topic:measure:amount"}}
		if version >= 4 {
			out["query_population"] = exec.AnalyticalQueryPopulationPolicy
		}
		if version >= 6 {
			out["intent"] = exec.AnalyticalIntentPolicy
		}
		if version >= 9 {
			out["outputs"] = []exec.AnalyticalOutput{{Metric: "topic:measure:amount", Column: 0}}
		}
		if version == 9 {
			out["scope"] = strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_scoped_singleton_populations")
		}
		if version >= 10 && version <= 12 {
			family := map[int]string{10: "independent_owned_grouped_populations", 11: "independent_selected_grouped_populations", 12: "independent_filtered_grouped_populations"}[version]
			out["scope"] = strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", family)
			out["grouping"] = []string{"topic:dimension:region"}
		}
		if version == 13 {
			out["scope"] = strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_entailed_scoped_singleton_populations")
			out["scalar_entailment"] = exec.AnalyticalScalarEntailmentPolicy
			out["scalar_entailment_coverage"] = exec.Hash("full-occurrence-proof")
		}
		return out
	}
	insert := func(id string, version int, r map[string]any, clarification map[string]any) error {
		var analytical, protected any
		if r != nil {
			analytical, _ = json.Marshal(r)
		}
		if clarification != nil {
			protected, _ = json.Marshal(clarification)
		}
		_, err := raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,analytical_version,analytical,clarification)
 VALUES('scalar-upgrade','actor','session',$1,'topic','["topic"]','["v1"]','[]','[]','{}','context','en','Reviewed scalar amount','{}','{}','SELECT 1','[]','{}','planned','[]','[]','[]',0,0,1,$2,$3::jsonb,$4::jsonb)`, id, version, analytical, protected)
		return err
	}
	for version := 0; version <= 12; version++ {
		if err := insert(fmt.Sprintf("%032x", version+1), version, receipt(version), nil); err != nil {
			t.Fatal("populate original receipt version", version, err)
		}
	}
	var before string
	if err := raw.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(q) ORDER BY query_id)::text FROM chartworks.nlq_queries q`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	upgraded := support.Open(t, dsn)
	if err := upgraded.Check(ctx); err != nil {
		t.Fatal("normal forward upgrade", err)
	}
	var after string
	if err := raw.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(q) ORDER BY query_id)::text FROM chartworks.nlq_queries q`).Scan(&after); err != nil || before != after {
		t.Fatal("forward migration changed retained v0-v12 rows", err)
	}
	for _, m := range migrations[:80] {
		var name, digest string
		if err := raw.QueryRow(ctx, `SELECT name,checksum FROM chartworks.schema_migrations WHERE version=$1`, m.Version).Scan(&name, &digest); err != nil || name != m.Name || digest != m.Checksum {
			t.Fatal("old migration changed", m.Version, err)
		}
	}
	binding := func() map[string]any {
		return map[string]any{"schema_version": 1, "base_sql": "SELECT reviewed scalar base", "base_parameters": nil, "binding": map[string]any{
			"schema_version": 6, "population_policy": exec.AnalyticalScalarEntailmentPolicy, "source_binding": exec.Hash("source"), "constraints": exec.Hash("constraints"), "statement": exec.Hash("statement"), "validation": map[string]any{"validated": true},
			"bindings":    []any{map[string]any{"population": "orders"}, map[string]any{"population": "refunds"}},
			"entailments": []any{map[string]any{"kind": "entailed_scalar_predicate", "resolution": exec.Hash("resolution"), "coverage": exec.Hash("coverage"), "occurrences": 6, "parameters": []int{}}},
		}}
	}
	if err := insert(fmt.Sprintf("%032x", 100), 13, receipt(13), binding()); err != nil {
		t.Fatal("closed new structural shape rejected", err)
	}
	cases := []struct {
		name    string
		version int
		mutate  func(map[string]any, map[string]any)
	}{
		{"v13_without_binding", 13, func(_ map[string]any, b map[string]any) { delete(b, "binding") }},
		{"v13_old_binding", 13, func(_ map[string]any, b map[string]any) { b["binding"].(map[string]any)["schema_version"] = 2 }},
		{"v13_without_effects", 13, func(_ map[string]any, b map[string]any) { delete(b["binding"].(map[string]any), "entailments") }},
		{"v13_empty_effects", 13, func(_ map[string]any, b map[string]any) { b["binding"].(map[string]any)["entailments"] = []any{} }},
		{"v13_wrong_policy", 13, func(_ map[string]any, b map[string]any) {
			b["binding"].(map[string]any)["population_policy"] = exec.AnalyticalScalarPopulationPolicy
		}},
		{"v13_without_coverage", 13, func(r map[string]any, _ map[string]any) { delete(r, "scalar_entailment_coverage") }},
		{"v13_old_receipt", 13, func(r map[string]any, _ map[string]any) { r["version"] = exec.AnalyticalScopedPopulationsVersion }},
		{"v13_grouped", 13, func(r map[string]any, _ map[string]any) { r["grouping"] = []string{"topic:dimension:region"} }},
		{"v13_missing_validation", 13, func(_ map[string]any, b map[string]any) { delete(b["binding"].(map[string]any), "validation") }},
		{"v12_new_binding", 12, func(_ map[string]any, _ map[string]any) {}},
		{"v9_new_receipt_fields", 9, func(r map[string]any, b map[string]any) {
			r["scalar_entailment"] = exec.AnalyticalScalarEntailmentPolicy
			b["binding"].(map[string]any)["schema_version"] = 2
			delete(b["binding"].(map[string]any), "entailments")
		}},
		{"legacy_empty_effect_field", 9, func(_ map[string]any, b map[string]any) {
			b["binding"].(map[string]any)["schema_version"] = 2
			b["binding"].(map[string]any)["entailments"] = []any{}
		}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, b := receipt(tc.version), binding()
			tc.mutate(r, b)
			if err := insert(fmt.Sprintf("%032x", 200+i), tc.version, r, b); err == nil {
				t.Fatal("mismatched analytical/binder family entered storage")
			}
		})
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.nlq_queries`) != 14 {
		t.Fatal("rejected insert left durable state")
	}
}
