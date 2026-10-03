package exec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
)

func learningDomainFixture(t *testing.T, dialect, sql string, parameters []Parameter) (Plan, Binding, identity.Envelope) {
	t.Helper()
	b := parserBinding()
	b.Dialect = dialect
	b.Relations[0].Columns = append(b.Relations[0].Columns, Column{Name: "day", NativeType: "date", Safe: true}, Column{Name: "moment", NativeType: "timestamp without time zone", Safe: true}, Column{Name: "instant", NativeType: "timestamp with time zone", Safe: true}, Column{Name: "key", NativeType: "uuid", Safe: true})
	if dialect == "mysql" {
		b.Catalog = "analytics"
		for i := range b.Relations[0].Columns {
			c := &b.Relations[0].Columns[i]
			switch c.Name {
			case "moment":
				c.NativeType = "datetime(6)"
			case "instant":
				c.NativeType = "timestamp(6)"
			case "key":
				c.NativeType = "varchar(36)"
			}
		}
	}
	if dialect != "postgres" && dialect != "mysql" {
		types := map[string]map[string]string{
			"sqlserver":  {"moment": "datetime2(7)", "instant": "datetimeoffset(7)", "key": "uniqueidentifier"},
			"bigquery":   {"moment": "DATETIME", "instant": "TIMESTAMP", "key": "STRING"},
			"snowflake":  {"moment": "TIMESTAMP_NTZ(9)", "instant": "TIMESTAMP_TZ(9)", "key": "VARCHAR"},
			"databricks": {"moment": "TIMESTAMP_NTZ", "instant": "TIMESTAMP", "key": "STRING"},
		}
		for i := range b.Relations[0].Columns {
			c := &b.Relations[0].Columns[i]
			if typ := types[dialect][c.Name]; typ != "" {
				c.NativeType = typ
			}
		}
	}
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:*"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{binding: b}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	p, err := validator.Validate(t.Context(), e, Request{Source: b.Source, Context: b.Context, SQL: sql, Parameters: parameters})
	if err != nil {
		t.Fatal("native validation", sql, err)
	}
	return p, b, e
}
func TestSQLRecoveryLearningNativeParameterDomains(t *testing.T) {
	for _, tc := range []struct {
		sql     string
		domains []string
	}{
		{`SELECT id FROM analytics.sales WHERE day=$1`, []string{"date"}},
		{`SELECT id FROM analytics.sales WHERE moment>$1 AND instant<$2`, []string{"timestamp", "timestamptz"}},
		{`SELECT id FROM analytics.sales WHERE key=$1`, []string{"uuid"}},
		{`SELECT $1::date AS d, CAST($2 AS timestamp) AS t, $3::timestamptz AS z, $4::uuid AS u FROM analytics.sales`, []string{"date", "timestamp", "timestamptz", "uuid"}},
		{`SELECT id FROM analytics.sales WHERE day=$1::date OR day=$1::date`, []string{"date"}},
		{`SELECT id FROM analytics.sales WHERE name=$1`, []string{""}},
		{`SELECT $1::timestamp(3) AS t, $2::timestamptz(6) AS z FROM analytics.sales`, []string{"timestamp", "timestamptz"}},
		{`WITH x AS (SELECT $1::date AS d FROM analytics.sales) SELECT d FROM x`, []string{"date"}},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			params := make([]Parameter, len(tc.domains))
			for i := range params {
				params[i] = Parameter{Kind: "text", Value: "PRIVATE_VALUES_DO_NOT_ESTABLISH_DOMAINS"}
			}
			p, b, e := learningDomainFixture(t, "postgres", tc.sql, params)
			domains, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, tc.domains) {
				t.Fatal(domains, err)
			}
			p.candidate.parameters[0].Value = "different"
			again, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, again) {
				t.Fatal("value-driven domain", again, err)
			}
		})
	}
}
func TestSQLRecoveryLearningDomainConflictsAndOpaqueCustody(t *testing.T) {
	for _, sql := range []string{`SELECT $1::date AS a,$1::timestamp AS b FROM analytics.sales`, `SELECT $1::date AS a,$1 AS b FROM analytics.sales`, `SELECT id FROM analytics.sales WHERE day=$1 OR name=$1`, `SELECT $1::date AS a FROM analytics.sales WHERE lower($1)='x'`} {
		p, b, e := learningDomainFixture(t, "postgres", sql, []Parameter{{Kind: "text", Value: "private"}})
		if _, err := p.LearningParameterDomains(t.Context(), e, b); !errors.Is(err, ErrUnsupported) {
			t.Fatal("ambiguous domain accepted", sql, err)
		}
	}
	p, b, e := learningDomainFixture(t, "postgres", `SELECT $1::date AS d FROM analytics.sales`, []Parameter{{Kind: "text", Value: "private"}})
	if _, err := (Plan{}).LearningParameterDomains(t.Context(), e, b); !errors.Is(err, ErrBinding) {
		t.Fatal("unsealed proof", err)
	}
	raw, _ := json.Marshal(p)
	var retained Plan
	if json.Unmarshal(raw, &retained) == nil {
		t.Fatal("retained plan executable")
	}
	if _, err := retained.LearningParameterDomains(t.Context(), e, b); err == nil {
		t.Fatal("retained domain proof")
	}
	bad := b.Clone()
	bad.Revision++
	if _, err := p.LearningParameterDomains(t.Context(), e, bad); err == nil {
		t.Fatal("changed source proof")
	}
	foreign, err := identity.FromVerified("other", "actor", "session", []string{"sources.query"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.LearningParameterDomains(t.Context(), foreign, b); err == nil {
		t.Fatal("foreign authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.LearningParameterDomains(ctx, e, b); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := p.LearningParameterDomains(nil, e, b); err == nil {
		t.Fatal("nil context")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			domains, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || domains[0] != "date" {
				t.Error(domains, err)
			} else {
				domains[0] = "changed"
			}
		})
	}
	wg.Wait()
}
func TestSQLRecoveryLearningMySQLParameterDomains(t *testing.T) {
	for _, tc := range []struct {
		sql     string
		domains []string
	}{
		{`SELECT id FROM analytics.sales WHERE day=?`, []string{"date"}},
		{`SELECT id FROM analytics.sales WHERE moment>? AND instant<?`, []string{"timestamp", "timestamp"}},
		{`SELECT CAST(? AS DATE) AS d FROM analytics.sales`, []string{"date"}},
		{`SELECT CAST(? AS DATETIME) AS d FROM analytics.sales`, []string{"timestamp"}},
		{`SELECT id FROM analytics.sales WHERE name=? AND day>?`, []string{"", "date"}},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			parameters := make([]Parameter, len(tc.domains))
			for i := range parameters {
				parameters[i] = Parameter{Kind: "text", Value: "not a domain hint"}
			}
			p, b, e := learningDomainFixture(t, "mysql", tc.sql, parameters)
			domains, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, tc.domains) {
				t.Fatal(domains, err)
			}
		})
	}
}

func TestSQLRecoveryLearningDomainNullKeepsLegacyKind(t *testing.T) {
	p, b, e := learningDomainFixture(t, "postgres", `SELECT $1::date AS d,$1::uuid AS u FROM analytics.sales`, []Parameter{{Kind: "null"}})
	domains, err := p.LearningParameterDomains(t.Context(), e, b)
	if err != nil || !reflect.DeepEqual(domains, []string{""}) {
		t.Fatal("null kind became a text domain", domains, err)
	}
}

// These use actual native parser/validator boundaries with a recording adapter.
// They are not SQL Server/BigQuery/Snowflake/Databricks live-engine passes.
func TestSQLRecoveryLearningWarehouseDomainMatrix(t *testing.T) {
	for _, tc := range []struct {
		dialect, sql string
		domains      []string
	}{
		{"sqlserver", `SELECT CAST(@p1 AS date) AS d,CAST(@p2 AS datetime2(3)) AS t,CAST(@p3 AS datetimeoffset) AS z,CAST(@p4 AS uniqueidentifier) AS u FROM analytics.sales`, []string{"date", "timestamp", "timestamptz", "uuid"}},
		{"bigquery", `SELECT CAST(@p1 AS DATE) AS d,CAST(@p2 AS DATETIME) AS t,CAST(@p3 AS TIMESTAMP) AS z FROM analytics.sales`, []string{"date", "timestamp", "timestamptz"}},
		{"snowflake", `SELECT CAST(? AS DATE) AS d,CAST(? AS TIMESTAMP_NTZ) AS t,CAST(? AS TIMESTAMP_TZ) AS z FROM analytics.sales`, []string{"date", "timestamp", "timestamptz"}},
		{"databricks", `SELECT CAST(:p1 AS DATE) AS d,CAST(:p2 AS TIMESTAMP_NTZ) AS t,CAST(:p3 AS TIMESTAMP) AS z FROM analytics.sales`, []string{"date", "timestamp", "timestamptz"}},
		{"sqlserver", `SELECT id FROM analytics.sales WHERE day = @p2 AND moment > @p1 AND instant < @p3`, []string{"timestamp", "date", "timestamptz"}},
		{"bigquery", `SELECT id FROM analytics.sales WHERE day = @p2 AND moment > @p1 AND instant < @p3`, []string{"timestamp", "date", "timestamptz"}},
		{"snowflake", `SELECT id FROM analytics.sales WHERE day=? AND moment>? AND instant<?`, []string{"date", "timestamp", "timestamptz"}},
		{"databricks", `SELECT id FROM analytics.sales WHERE day=:p1 AND moment>:p2 AND instant<:p3`, []string{"date", "timestamp", "timestamptz"}},
	} {
		t.Run(tc.dialect+tc.sql, func(t *testing.T) {
			parameters := make([]Parameter, len(tc.domains))
			kinds := make([]string, len(parameters))
			for i := range parameters {
				parameters[i] = Parameter{Kind: "text", Value: "private source value is not a type hint"}
				kinds[i] = "text"
			}
			p, b, e := learningDomainFixture(t, tc.dialect, tc.sql, parameters)
			domains, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, tc.domains) {
				t.Fatal(domains, err)
			}
			schema, _ := exampleparams.New(kinds)
			schema, err = schema.WithDomains(domains)
			if err != nil {
				t.Fatal(err)
			}
			values, err := schema.ProbeValues()
			if err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				parameters[i].Value = value
			}
			probePlan, b, e := learningDomainFixture(t, tc.dialect, tc.sql, parameters)
			again, err := probePlan.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, again) {
				t.Fatal("fixed probe/native AST domain disagreement", again, err)
			}
		})
	}
}
func TestSQLRecoveryLearningWarehouseDomainConflicts(t *testing.T) {
	for _, dialect := range []string{"sqlserver", "bigquery"} {
		cast := "DATETIME"
		if dialect == "sqlserver" {
			cast = "datetime2"
		}
		sql := `SELECT CAST(@p1 AS DATE) AS d,CAST(@p1 AS ` + cast + `) AS t FROM analytics.sales`
		p, b, e := learningDomainFixture(t, dialect, sql, []Parameter{{Kind: "text", Value: "2001-01-01"}})
		if _, err := p.LearningParameterDomains(t.Context(), e, b); !errors.Is(err, ErrUnsupported) {
			t.Fatal("repeated incompatible domain", dialect, err)
		}
	}
	for _, tc := range []struct {
		dialect, native string
		want            string
	}{
		{"sqlserver", "timestamp", ""}, {"sqlserver", "rowversion", ""}, {"sqlserver", "datetimeoffset(7)", "timestamptz"},
		{"snowflake", "TIMESTAMP", ""}, {"snowflake", "TIMESTAMP_LTZ(9)", "timestamptz"}, {"snowflake", "TIMESTAMP_NTZ(9)", "timestamp"},
		{"bigquery", "TIMESTAMP", "timestamptz"}, {"bigquery", "DATETIME", "timestamp"}, {"databricks", "TIMESTAMP", "timestamptz"},
		{"mysql", "datetime(7)", ""}, {"sqlserver", "datetime2(8)", ""}, {"snowflake", "timestamp_ntz(99)", ""},
		{"sqlserver", "vendor.datetime2", ""}, {"bigquery", "uuid", ""},
	} {
		if got := learningNativeDomain(tc.dialect, tc.native); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	for _, dialect := range []string{"sqlserver", "bigquery"} {
		p := learningDomainProof{binding: Binding{Dialect: dialect}}
		for _, name := range []string{"@p01", "@p0", "@p65", "@other", "p1"} {
			if _, ok := p.warehouseNamedParameter(map[string]any{"column": map[string]any{"name": map[string]any{"name": name}}}); ok {
				t.Fatal("noncanonical named marker", name)
			}
		}
		if _, ok := p.warehouseNamedParameter(map[string]any{"column": map[string]any{"name": map[string]any{"name": "@p1", "quoted": true}}}); ok {
			t.Fatal("quoted column acquired parameter role")
		}
	}
}

func TestSQLRecoveryLearningWarehouseDomainScopedCasts(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		marker := "?"
		if dialect == "databricks" {
			marker = ":p1"
		}
		next := "?"
		if dialect == "databricks" {
			next = ":p2"
		}
		if dialect == "sqlserver" || dialect == "bigquery" {
			marker = "@p1"
			next = "@p2"
		}
		for _, sql := range []string{
			`WITH bounded AS (SELECT CAST(` + marker + ` AS DATE) AS d FROM analytics.sales) SELECT d FROM bounded`,
			`SELECT d FROM (SELECT CAST(` + marker + ` AS DATE) AS d FROM analytics.sales) AS bounded`,
			`WITH bounded AS (SELECT CAST(` + marker + ` AS DATE) AS d FROM analytics.sales) SELECT CAST(` + next + ` AS DATE) AS e FROM bounded`,
		} {
			t.Run(dialect+sql, func(t *testing.T) {
				n := 1
				if strings.Contains(sql, " AS e ") {
					n = 2
				}
				parameters := make([]Parameter, n)
				want := make([]string, n)
				for i := range parameters {
					parameters[i] = Parameter{Kind: "text", Value: "2000-01-02"}
					want[i] = "date"
				}
				p, b, e := learningDomainFixture(t, dialect, sql, parameters)
				got, err := p.LearningParameterDomains(t.Context(), e, b)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatal(got, err)
				}
			})
		}
	}
}

func TestSQLRecoveryLearningDomainSourceAliasesNeverSupplyLineage(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		marker := "?"
		if dialect == "databricks" {
			marker = ":p1"
		}
		if dialect == "postgres" {
			marker = "$1"
		}
		if dialect == "sqlserver" || dialect == "bigquery" {
			marker = "@p1"
		}
		sql := `WITH sales AS (SELECT name AS day FROM analytics.sales) SELECT day FROM sales WHERE day = ` + marker
		p, b, e := learningDomainFixture(t, dialect, sql, []Parameter{{Kind: "text", Value: "2001-01-01"}})
		domains, err := p.LearningParameterDomains(t.Context(), e, b)
		if err != nil || !reflect.DeepEqual(domains, []string{""}) {
			t.Fatal("CTE borrowed same-named physical date column", dialect, domains, err)
		}
	}
	p := learningDomainProof{binding: parserBinding()}
	scope := learningDomainScope{}
	p.pgSources(map[string]any{"JoinExpr": map[string]any{"alias": map[string]any{"Alias": map[string]any{"aliasname": "renamed"}}, "larg": map[string]any{"RangeVar": map[string]any{"schemaname": "analytics", "relname": "sales"}}}}, scope)
	if len(scope) != 1 || scope["renamed"] != nil {
		t.Fatal("joined output alias borrowed input lineage")
	}
}

func TestSQLRecoveryLearningAdditionalNativeDomains(t *testing.T) {
	for _, tc := range []struct{ dialect, cast, domain string }{
		{"postgres", "time", "time"}, {"postgres", "time(3)", "time"}, {"postgres", "timetz", "timetz"}, {"postgres", "timetz(6)", "timetz"},
		{"postgres", "interval", "interval"}, {"postgres", "interval day", "interval"}, {"postgres", "interval day to second(3)", "interval"}, {"postgres", "json", "json"}, {"postgres", "jsonb", "jsonb"},
		{"mysql", "TIME", "time"}, {"mysql", "JSON", "json"}, {"sqlserver", "TIME(7)", "time"}, {"bigquery", "TIME", "time"}, {"snowflake", "TIME(9)", "time"},
	} {
		t.Run(tc.dialect+tc.cast, func(t *testing.T) {
			marker := "?"
			if tc.dialect == "databricks" {
				marker = ":p1"
			}
			if tc.dialect == "postgres" {
				marker = "$1"
			}
			if tc.dialect == "sqlserver" || tc.dialect == "bigquery" {
				marker = "@p1"
			}
			sql := `SELECT CAST(` + marker + ` AS ` + tc.cast + `) AS value FROM analytics.sales`
			schema, _ := exampleparams.New([]string{"text"})
			schema, err := schema.WithDomains([]string{tc.domain})
			if err != nil {
				t.Fatal(err)
			}
			probes, _ := schema.ProbeValues()
			p, b, e := learningDomainFixture(t, tc.dialect, sql, []Parameter{{Kind: "text", Value: probes[0]}})
			domains, err := p.LearningParameterDomains(t.Context(), e, b)
			if err != nil || !reflect.DeepEqual(domains, []string{tc.domain}) {
				t.Fatal("native public-probe domain", domains, err)
			}
		})
	}
	for _, sql := range []string{`SELECT $1::time AS a,$1::timetz AS b FROM analytics.sales`, `SELECT $1::interval AS a,$1::text AS b FROM analytics.sales`, `SELECT $1::json AS a,$1::jsonb AS b FROM analytics.sales`} {
		p, b, e := learningDomainFixture(t, "postgres", sql, []Parameter{{Kind: "text", Value: "a private scalar cannot choose a domain"}})
		if _, err := p.LearningParameterDomains(t.Context(), e, b); !errors.Is(err, ErrUnsupported) {
			t.Fatal("cross-domain repeated slot", sql, err)
		}
	}
	for _, tc := range []struct{ dialect, typ string }{{"sqlserver", "json"}, {"bigquery", "json"}, {"snowflake", "json"}, {"databricks", "time"}, {"databricks", "json"}, {"mysql", "enum"}} {
		if got := learningNativeDomain(tc.dialect, tc.typ); got != "" {
			t.Fatal("unqualified native domain", tc, got)
		}
	}
}

type learningDomainWaitContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *learningDomainWaitContext) Err() error {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Err()
}
func TestSQLRecoveryLearningDomainNativeAdmissionCancellation(t *testing.T) {
	p, b, e := learningDomainFixture(t, "postgres", `SELECT $1::date AS d FROM analytics.sales`, []Parameter{{Kind: "text", Value: "2001-01-01"}})
	for range cap(learningDomainNativeSlots) {
		learningDomainNativeSlots <- struct{}{}
	}
	defer func() {
		for range cap(learningDomainNativeSlots) {
			<-learningDomainNativeSlots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := &learningDomainWaitContext{Context: ctx, entered: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := p.LearningParameterDomains(waiting, e, b); done <- err }()
	<-waiting.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("queued parse ignored cancellation", err)
	}
	if len(learningDomainNativeSlots) != cap(learningDomainNativeSlots) {
		t.Fatal("queued proof consumed another admission")
	}
}
