package exec

import "testing"

func TestPostgresFunctionSignatureNativeAST(t *testing.T) {
	for _, expr := range []string{"sum()", "lower(1)", "abs(true)", "round(1::float8,2)", "round(1,2.5)", "rank()", "row_number(1) OVER ()", "lower('a') OVER ()", "sum(DISTINCT id) OVER ()", "lag(id,1,'a'::text) FILTER (WHERE active) OVER ()", "nth_value(id,1.5) OVER ()", "count(DISTINCT *)", "abs(1,2)", "pg_catalog.lower(1)", "lower(count(*))", "char_length('a'::bytea)"} {
		if _, _, err := resolveFixture("SELECT " + expr + " FROM analytics.sales"); err == nil {
			t.Errorf("invalid signature accepted: %s", expr)
		}
	}
	for _, expr := range []string{"sum(amount)", "sum(amount) FILTER (WHERE active)", "count(*) OVER ()", "rank() OVER (ORDER BY id)", "lag(id,1,NULL) OVER (ORDER BY id)", "round(1.25,1)", "round(1::float8)", "substring('abc',1,2)", "substring('abc','a.*')", "abs(-1)", "sum(amount ORDER BY id)", "date_trunc('month',created_at)", "pg_catalog.lower('a')"} {
		if _, raw, err := resolveFixture("SELECT " + expr + " FROM analytics.sales"); err != nil {
			t.Errorf("valid signature rejected: %s: %v %s", expr, err, raw)
		}
	}
}
