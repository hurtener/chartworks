package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestSQLRecoveryVocabularyPostgresNative(t *testing.T) {
	expressions := map[string]string{
		"count": "count(*)", "sum": "sum(amount)", "avg": "avg(amount)", "min": "min(amount)", "max": "max(amount)",
		"abs": "abs(amount)", "round": "round(amount)", "ceil": "ceil(amount)", "ceiling": "ceiling(amount)", "floor": "floor(amount)",
		"lower": "lower(name)", "upper": "upper(name)", "length": "length(name)", "char_length": "char_length(name)", "octet_length": "octet_length(name)",
		"trim": "trim(name)", "btrim": "btrim(name)", "ltrim": "ltrim(name)", "rtrim": "rtrim(name)",
		"substring": "substring(name,1,1)", "substr": "substr(name,1,1)", "replace": "replace(name,'a','b')", "like_escape": "pg_catalog.like_escape(name,'!')",
		"date_trunc": "date_trunc('month',created_at)", "date_part": "date_part('year',created_at)", "extract": "extract(year FROM created_at)",
		"row_number": "row_number() OVER (ORDER BY id)", "rank": "rank() OVER (ORDER BY id)", "dense_rank": "dense_rank() OVER (ORDER BY id)",
		"lag": "lag(id) OVER (ORDER BY id)", "lead": "lead(id) OVER (ORDER BY id)", "first_value": "first_value(id) OVER (ORDER BY id)",
		"last_value": "last_value(id) OVER (ORDER BY id)", "nth_value": "nth_value(id,1) OVER (ORDER BY id)", "ntile": "ntile(2) OVER (ORDER BY id)",
		"percent_rank": "percent_rank() OVER (ORDER BY id)", "cume_dist": "cume_dist() OVER (ORDER BY id)",
	}
	profile, _ := sqlpolicy.ForDialect("postgres")
	if len(expressions) != len(profile.Functions) {
		t.Fatal("native name fixture incomplete")
	}
	for _, name := range profile.Functions {
		expression, ok := expressions[name]
		if !ok {
			t.Fatal("no fixture for name", name)
		}
		t.Run(name, func(t *testing.T) {
			if _, _, err := resolveFixture("SELECT " + expression + " AS checked FROM analytics.sales"); err != nil {
				t.Fatal("advertised native form rejected", err)
			}
		})
	}
	for _, expr := range []string{"coalesce(amount,0)", "greatest(amount,0)", "least(amount,0)", "nullif(amount,0)", "current_date", "current_time", "current_timestamp", "localtime", "localtimestamp", "pg_catalog.abs(amount)", "amount::numeric"} {
		if _, _, err := resolveFixture("SELECT " + expr + " AS checked FROM analytics.sales"); err != nil {
			t.Fatal("special expression/type/value regression", expr, err)
		}
	}
	for _, expr := range []string{"pg_read_file('/etc/passwd')", "pg_sleep(1)", "public.abs(amount)", `"ABS"(amount)`, "coalesce(secret,0)", "amount::regclass", "current_user", "sum(custom)", "nextval('s')", "id OPERATOR(public.+) 1"} {
		if _, _, err := resolveFixture("SELECT " + expr + " AS checked FROM analytics.sales"); err == nil {
			t.Fatal("name profile widened native proof", expr)
		}
	}
}

func TestSQLRecoveryVocabularyWarehouseNative(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			adapter.binding = Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: dialect, Contract: "contract", Fingerprint: Hash("synthetic"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}}}}}
			adapter.explains = 0
			plan, err := validator.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: "SELECT sum(id) AS total FROM analytics.sales"})
			if err != nil || !plan.Receipt().Validated || adapter.explains != 1 {
				t.Fatal("shared positive function gate rejected", err)
			}
			for _, sql := range []string{"SELECT mystery(id) AS total FROM analytics.sales", "SELECT public.sum(id) AS total FROM analytics.sales", "SELECT sum(secret) AS total FROM analytics.sales", "SELECT sum(id) AS total FROM other.sales"} {
				adapter.explains = 0
				plan, err = validator.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: sql})
				if err == nil || plan.Receipt().Validated || adapter.explains != 0 {
					t.Fatal("name policy bypassed function/dependency proof", sql, err)
				}
			}
		})
	}
	// The vocabulary is not an identity or a source capability. Even a query
	// entirely inside its name gate must fail current signed authority first.
	e, err = identity.FromVerified("other-tenant", "actor", "session", []string{"sources.query"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter.explains = 0
	if p, err := validator.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: "SELECT sum(id) AS total FROM analytics.sales"}); err == nil || p.Receipt().Validated || adapter.explains != 0 {
		t.Fatal("profile substituted for identity")
	}
}

func TestSQLRecoveryVocabularyParameterSpelling(t *testing.T) {
	for _, d := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		profile, _ := sqlpolicy.ForDialect(d)
		expected := "?"
		switch profile.ParameterStyle {
		case "dollar-numbered":
			expected = "$2"
		case "at-p-numbered":
			expected = "@p2"
		case "positional-question-mark":
		default:
			t.Fatal("unknown advertised style")
		}
		if actual := businessPlaceholder(d, 2); actual != expected {
			t.Fatal("generation and binder disagree", d, actual, expected)
		}
		n, err := businessParameterIndex(expected, d, 2)
		if err != nil || n != 2 {
			t.Fatal("native marker parser disagrees", d, err)
		}
		for _, bad := range []string{"$0", "@p0", "$02", "@p02", "arbitrary", "?2"} {
			if _, err := businessParameterIndex(bad, d, 2); err == nil {
				t.Fatal("invalid index admitted", d, bad)
			}
		}
	}
	if _, err := sqlpolicy.ForDialect("private-unsupported"); !errors.Is(err, sqlpolicy.ErrDialect) || strings.Contains(err.Error(), "private-unsupported") {
		t.Fatal("unknown dialect echoed")
	}
}
