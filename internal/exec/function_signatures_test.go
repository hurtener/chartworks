package exec

import "testing"

func TestPostgresFunctionSignatureNativeAST(t *testing.T) {
	for _, expr := range []string{"sum()", "lower(1)", "abs(true)", "round(1::float8,2)", "round(avg(1::float8),2)", "round(1,2.5)", "rank()", "row_number(1) OVER ()", "lower('a') OVER ()", "sum(DISTINCT id) OVER ()", "lag(id,1,'a'::text) FILTER (WHERE active) OVER ()", "nth_value(id,1.5) OVER ()", "count(DISTINCT *)", "abs(1,2)", "pg_catalog.lower(1)", "lower(count(*))", "char_length('a'::bytea)"} {
		if _, _, err := resolveFixture("SELECT " + expr + " FROM analytics.sales"); err == nil {
			t.Errorf("invalid signature accepted: %s", expr)
		}
	}
	for _, expr := range []string{"sum(amount)", "sum(amount) FILTER (WHERE active)", "count(*) OVER ()", "rank() OVER (ORDER BY id)", "lag(id,1,NULL) OVER (ORDER BY id)", "round(1.25,1)", "round(avg(1),2)", "round(1::float8)", "substring('abc',1,2)", "substring('abc','a.*')", "abs(-1)", "sum(amount ORDER BY id)", "date_trunc('month',created_at)", "pg_catalog.lower('a')"} {
		if _, raw, err := resolveFixture("SELECT " + expr + " FROM analytics.sales"); err != nil {
			t.Errorf("valid signature rejected: %s: %v %s", expr, err, raw)
		}
	}
}

func TestPostgresSpecialExpressionNativeAST(t *testing.T) {
	for _, expr := range []string{"coalesce(1,TRUE)", "greatest(1,TRUE)", "least(1,TRUE)", "nullif(1,TRUE)", "coalesce(NULL,1,'x'::text)", "greatest(NULL,1,'x'::text)", "least(NULL,1,'x'::text)"} {
		if _, _, err := resolveFixture("SELECT " + expr + " AS checked FROM analytics.sales"); err == nil {
			t.Errorf("incompatible special expression accepted: %s", expr)
		}
	}
	for _, expr := range []string{"coalesce(1,2.5)", "coalesce(NULL,1,2.5)", "greatest(NULL,1,2.5)", "least(NULL,1,2.5)", "nullif(1,2.5)", "coalesce(NULL,'a'::text)", "nullif(id,0)", "greatest(amount,0)", "coalesce('1',1)"} {
		if _, raw, err := resolveFixture("SELECT " + expr + " AS checked FROM analytics.sales"); err != nil {
			t.Errorf("valid special expression rejected: %s %v %s", expr, err, raw)
		}
	}
}
