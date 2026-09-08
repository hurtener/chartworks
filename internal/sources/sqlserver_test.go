package sources

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

type sqlServerFixtureRows struct {
	values [][]any
	index  int
	err    error
	closed bool
}

func (r *sqlServerFixtureRows) Columns() []query.Column { return nil }
func (r *sqlServerFixtureRows) Next() bool {
	if r.closed || r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}
func (r *sqlServerFixtureRows) Values() ([]any, error) {
	if r.index == 0 || r.index > len(r.values) {
		return nil, errors.New("no row")
	}
	return r.values[r.index-1], nil
}
func (r *sqlServerFixtureRows) Err() error   { return r.err }
func (r *sqlServerFixtureRows) Close() error { r.closed = true; return nil }

type sqlServerFixtureSession struct {
	mutate string
	calls  int
	locks  int
}

func (s *sqlServerFixtureSession) Query(_ context.Context, q *query.Query) (query.RowStream, error) {
	s.calls++
	var rows [][]any
	switch {
	case q.Query == sqlServerIdentitySQL:
		rows = [][]any{{"16.0.1000.6", "synthetic-reader", "synthetic-reader", "analytics"}}
	case strings.Contains(q.Query, "fn_my_permissions"):
		rows = [][]any{{"SELECT"}, {"VIEW DEFINITION"}}
		if len(q.Args) == 1 && q.Args[0] == "DATABASE" {
			rows = append(rows, []any{"VIEW ANY COLUMN MASTER KEY DEFINITION"}, []any{"VIEW ANY COLUMN ENCRYPTION KEY DEFINITION"})
		}
		if s.mutate == "writer" {
			rows = append(rows, []any{"INSERT"})
		}
	case q.Query == sqlServerTableSQL:
		rows = [][]any{{int64(42), time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), int64(0), false, false, int64(0)}}
		if s.mutate == "ddl" && s.locks > 0 {
			rows[0][0] = int64(43)
		}
		if s.mutate == "rls" {
			rows[0][5] = int64(1)
		}
	case q.Query == sqlServerColumnsSQL:
		rows = [][]any{{"id", "bigint", int64(8), int64(19), int64(0), false, int64(1), false, int64(0), nil, false, false}}
		if s.mutate == "computed" {
			rows[0][7] = true
		}
		if s.mutate == "column" {
			rows[0][0] = "other"
		}
		if s.mutate == "custom" {
			rows[0][10] = true
		}
	case strings.HasPrefix(q.Query, "SELECT TOP (1) 1 AS chartworks_lock"):
		s.locks++
		rows = [][]any{{int64(1)}}
	default:
		return nil, errors.New("unexpected synthetic statement")
	}
	return &sqlServerFixtureRows{values: rows}, nil
}
func sqlServerFixtureConfig() config.SourceConnection {
	return config.SourceConnection{Tenant: "tenant", ID: "native", Dialect: "sqlserver", Version: "1", Relations: []config.SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{"id"}}}}
}
func TestSQLServerContext(t *testing.T) {
	c := sqlServerFixtureConfig()
	session := &sqlServerFixtureSession{}
	binding, err := inspectSQLServerContext(context.Background(), session, c, "source", 1, "localhost:1433/analytics")
	if err != nil || !binding.Valid() || session.locks != 1 || binding.Dialect != "sqlserver" || binding.Catalog != "analytics" || len(binding.Relations) != 1 {
		t.Fatalf("context: %+v %v", binding, err)
	}
	again, err := inspectSQLServerContext(context.Background(), &sqlServerFixtureSession{}, c, "source", 1, "localhost:1433/analytics")
	if err != nil || readexec.Hash(binding) != readexec.Hash(again) {
		t.Fatal("stable native evidence changed")
	}
	if _, err := inspectSQLServerContext(context.Background(), &sqlServerFixtureSession{mutate: "ddl"}, c, "source", 1, "localhost:1433/analytics"); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("DDL between metadata and lock did not invalidate context")
	}
	for _, mutation := range []string{"writer", "rls", "computed", "column", "custom"} {
		t.Run(mutation, func(t *testing.T) {
			s := &sqlServerFixtureSession{mutate: mutation}
			if _, err := inspectSQLServerContext(context.Background(), s, c, "source", 1, "localhost:1433/analytics"); err == nil || s.locks != 0 {
				t.Fatalf("unsupported context admitted: %s %v", mutation, err)
			}
		})
	}
}

func TestSQLServerPermissionsAllowDefaultDatabaseKeyMetadata(t *testing.T) {
	defaults := [][]any{{"CONNECT"}, {"VIEW ANY COLUMN MASTER KEY DEFINITION"}, {"VIEW ANY COLUMN ENCRYPTION KEY DEFINITION"}}
	if !sqlServerPermissions(defaults, "DATABASE") {
		t.Fatal("SQL Server default database key-metadata permissions were rejected")
	}
	if sqlServerPermissions(defaults, "SERVER") || sqlServerPermissions(append(defaults, []any{"ALTER ANY COLUMN MASTER KEY"}), "DATABASE") {
		t.Fatal("database metadata permissions widened another permission scope")
	}
}

const sqlServerFixturePlan = `<ShowPlanXML xmlns="http://schemas.microsoft.com/sqlserver/2004/07/showplan"><BatchSequence><Batch><Statements><StmtSimple StatementType="SELECT" StatementSubTreeCost="0.25"><QueryPlan><RelOp><Object Database="[analytics]" Schema="[analytics]" Table="[sales]"/></RelOp></QueryPlan></StmtSimple></Statements></Batch></BatchSequence></ShowPlanXML>`

func TestSQLServerNativePlan(t *testing.T) {
	binding := readexec.Binding{Relations: []readexec.Relation{{Schema: "analytics", Name: "sales"}}}
	cost, err := inspectSQLServerPlan([]byte(sqlServerFixturePlan), binding, "analytics")
	if err != nil || cost != 0.25 {
		t.Fatalf("native plan: %g %v", cost, err)
	}
	for _, document := range []string{"<broken", strings.Replace(sqlServerFixturePlan, "[sales]", "[other]", 1), strings.Replace(sqlServerFixturePlan, "Database=\"[analytics]\"", "Database=\"[foreign]\"", 1), strings.Replace(sqlServerFixturePlan, "StatementType=\"SELECT\"", "StatementType=\"UPDATE\"", 1), strings.Replace(sqlServerFixturePlan, "0.25", "NaN", 1), strings.Replace(sqlServerFixturePlan, "<QueryPlan>", "<QueryPlan><UserDefinedFunction/>", 1)} {
		if _, err := inspectSQLServerPlan([]byte(document), binding, "analytics"); err == nil {
			t.Fatal("unsupported native plan accepted")
		}
	}
}

func TestSQLServerPlanningSQL(t *testing.T) {
	statement := "SELECT id FROM analytics.sales WHERE id > @p1"
	if got, err := sqlServerPlanningSQL(statement, nil); err != nil || got != statement {
		t.Fatal("unparameterized SQL changed", err)
	}
	for _, test := range []struct {
		value  any
		native string
	}{
		{nil, "nvarchar(1)"}, {int64(3), "bigint"}, {true, "bit"},
		{"text", "nvarchar(4)"}, {"a😀", "nvarchar(3)"}, {"", "nvarchar(max)"},
		{strings.Repeat("x", 4001), "nvarchar(max)"},
	} {
		got, err := sqlServerPlanningSQL(statement, []any{test.value})
		if err != nil || got != "DECLARE @p1 "+test.native+";\n"+statement {
			t.Fatal("native parameter declaration changed", err)
		}
	}
	value := "synthetic'; DELETE FROM analytics.sales; --"
	got, err := sqlServerPlanningSQL(statement, []any{int64(0), value, false})
	if err != nil || !strings.HasPrefix(got, "DECLARE @p1 bigint, @p2 nvarchar(") || !strings.HasSuffix(got, ", @p3 bit;\n"+statement) || strings.Contains(got, value) {
		t.Fatal("planning lost parameter order or interpolated a value", err)
	}
	if _, err := sqlServerPlanningSQL(statement, []any{float64(1)}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("unregistered native parameter type admitted", err)
	}
}

func TestSQLServerValues(t *testing.T) {
	columns := []query.Column{{Name: "n", DatabaseType: "BIGINT"}, {Name: "d", DatabaseType: "DECIMAL"}, {Name: "b", DatabaseType: "VARBINARY"}, {Name: "t", DatabaseType: "DATETIME2"}, {Name: "offset", DatabaseType: "DATETIMEOFFSET"}, {Name: "z", DatabaseType: "BIT"}}
	timestamp := time.Date(2026, 9, 7, 12, 13, 14, 123456700, time.FixedZone("synthetic", -3*3600))
	values := []any{int64(9007199254740993), []byte("12345678901234567890.123456789"), []byte{0, 255}, timestamp, timestamp, nil}
	fields, err := sqlServerResultFields(columns)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sqlServerResultValues(columns, values)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := readexec.NewCollector(fields, 10, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = collector.Add(raw); err != nil {
		t.Fatal(err)
	}
	row := collector.Result().Rows[0]
	expected := []string{`"9007199254740993"`, `"12345678901234567890.123456789"`, `"00ff"`, `"2026-09-07T12:13:14.1234567"`, `"2026-09-07T12:13:14.1234567-03:00"`, `null`}
	for i, v := range row {
		if string(v) != expected[i] {
			t.Fatalf("value %d: %s", i, v)
		}
	}
	if _, err := sqlServerArguments([]readexec.Parameter{{Kind: "number", Value: "1.23"}}); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("decimal parameter silently coerced")
	}
	if _, err := sqlServerResultValues(columns, []any{big.NewRat(1, 2)}); err == nil {
		t.Fatal("wrong schema width")
	}
}
func TestSQLServerDSN(t *testing.T) {
	valid := "sqlserver://synthetic:fixture-password@localhost:1433?database=analytics&encrypt=disable&TrustServerCertificate=false"
	parsed, location, err := parseSQLServerDSN(valid)
	if err != nil || parsed.Database != "analytics" || location != "localhost:1433/analytics" {
		t.Fatalf("fixture DSN: %s %v", location, err)
	}
	for _, value := range []string{strings.Replace(valid, "localhost", "external.invalid", 1), valid + "&log=63", valid + "&database=foreign", strings.Replace(valid, "sqlserver://", "http://", 1), "sqlserver://synthetic@localhost:1433?database=analytics&encrypt=disable"} {
		if _, _, err := parseSQLServerDSN(value); err == nil {
			t.Fatal("invalid DSN accepted")
		}
	}
}
