//go:build cgo && (linux || darwin)

package sources

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// TestSQLRecoveryMySQLAnalyticalLocal uses the real source adapter, catalog,
// native EXPLAIN and plan-only execution. Only repository persistence is in memory.
func TestSQLRecoveryMySQLAnalyticalLocal(t *testing.T) {
	fixture := newMySQLAnalyticalFixture(t)
	service, e, binding, dataset, name, validator := fixture.service, fixture.envelope, fixture.binding, fixture.dataset, fixture.schema, fixture.validator
	measure := func(op, column string) readexec.AnalyticalExpression {
		return readexec.AnalyticalExpression{Op: op, Column: column}
	}
	filtered := measure("sum", "amount")
	filtered.Filters = []readexec.AnalyticalFilter{{Column: "category", Kind: "eq", Values: []string{"A"}}}
	ratio := readexec.AnalyticalExpression{Op: "/", Args: []readexec.AnalyticalExpression{measure("sum", "amount"), measure("count", "id")}}
	tests := []struct {
		name, projection string
		expression       readexec.AnalyticalExpression
		want             string
	}{
		{"sum", "sum(amount)", measure("sum", "amount"), "50"},
		{"avg", "avg(amount)", measure("avg", "amount"), "16.666667"},
		{"avg_integral_cast", "avg(CAST(id AS DECIMAL(38,10)))", measure("avg", "id"), "2.5"},
		{"signed_constant", "sum(amount)/-2", readexec.AnalyticalExpression{Op: "/", Args: []readexec.AnalyticalExpression{measure("sum", "amount"), {Op: "number", Value: "-2"}}}, "-25"},
		{"count", "count(id)", measure("count", "id"), "4"},
		{"distinct_count", "count(DISTINCT amount)", measure("distinct_count", "amount"), "2"},
		{"ratio_case_guard", "CASE WHEN count(id)=0 THEN NULL ELSE sum(amount)/count(id) END", ratio, "12.5"},
		{"case_population", "sum(CASE WHEN category='A' THEN amount END)", filtered, "20"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			statement := "SELECT " + tc.projection + " AS value FROM " + name + ".sales"
			plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: binding.Source, Context: binding.Context, SQL: statement})
			if err != nil {
				t.Fatal("native validation", err)
			}
			contract := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(binding), Semantics: readexec.Hash("synthetic-reviewed-v1"), Dataset: dataset, Metrics: []readexec.AnalyticalMetric{{ID: "metric", Expression: tc.expression}}}
			receipt, err := readexec.CheckAnalyticalPlan(t.Context(), plan, contract)
			if err != nil || receipt == nil || receipt.Version != readexec.AnalyticalIntentVersion || receipt.Contract != readexec.Hash(contract) {
				t.Fatalf("analytical proof: %v", err)
			}
			result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", i+1), &cloudObserverCapture{})
			if err != nil {
				t.Fatal("native execution", err)
			}
			if result.RemoteState != "stopped" || len(result.Result.Rows) != 1 || len(result.Result.Rows[0]) != 1 {
				t.Fatal("missing completed scalar result")
			}
			raw := result.Result.Rows[0][0]
			var value string
			if json.Unmarshal(raw, &value) != nil {
				value = string(raw)
			}
			got, ok := new(big.Rat).SetString(value)
			want, _ := new(big.Rat).SetString(tc.want)
			if !ok || got.Cmp(want) != 0 {
				t.Fatalf("result %s, want %s", value, tc.want)
			}
			changed := contract
			changed.Binding = readexec.Hash("wrong-binding")
			if _, err = readexec.CheckAnalyticalPlan(t.Context(), plan, changed); !errors.Is(err, readexec.ErrBinding) {
				t.Fatal("binding mismatch admitted", err)
			}
			if tc.name == "case_population" {
				wrongPopulation := filtered
				wrongPopulation.Filters = []readexec.AnalyticalFilter{{Column: "category", Kind: "eq", Values: []string{"B"}}}
				changed = contract
				changed.Metrics = []readexec.AnalyticalMetric{{ID: "metric", Expression: wrongPopulation}}
				if proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, changed); err == nil || proof != nil {
					t.Fatal("different selected population admitted")
				}
			}
			changed = contract
			changed.Metrics = []readexec.AnalyticalMetric{{ID: "metric", Expression: measure("max", "amount")}}
			if proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, changed); err == nil || proof != nil {
				t.Fatal("different selected metric admitted")
			}
		})
	}
	t.Run("order_limit", func(t *testing.T) {
		contract := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(binding), Semantics: readexec.Hash("synthetic-reviewed-v1"), Dataset: dataset, Metrics: []readexec.AnalyticalMetric{{ID: "revenue", Expression: measure("sum", "amount")}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalGroupingPolicy, Columns: []string{"category"}, Dimensions: []string{"category"}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy, Order: []readexec.AnalyticalOrder{{Metric: "revenue", Descending: true, Nulls: "last"}}, Limit: 1}}
		for _, tc := range []struct {
			suffix string
			pass   bool
		}{{"ORDER BY sum(amount) DESC LIMIT 1", true}, {"ORDER BY revenue DESC LIMIT 1", true}, {"ORDER BY sum(amount) ASC LIMIT 1", false}, {"ORDER BY sum(amount) DESC LIMIT 2", false}, {"", false}} {
			plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: binding.Source, Context: binding.Context, SQL: "SELECT category,sum(amount) AS revenue FROM " + name + ".sales GROUP BY category " + tc.suffix})
			if err != nil {
				t.Fatal("native validation", err)
			}
			proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, contract)
			if (err == nil) != tc.pass || (proof != nil) != tc.pass {
				t.Fatalf("order/limit pass=%v: %v", tc.pass, err)
			}
			if tc.pass {
				result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, readexec.Hash(tc.suffix)[:32], &cloudObserverCapture{})
				if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"B"` {
					t.Fatalf("ordered limited native result: %v %#v", err, result.Result.Rows)
				}
			}
		}
	})
}

type mysqlAnalyticalFixture struct {
	service         *Service
	envelope        identity.Envelope
	binding         readexec.Binding
	dataset, schema string
	admin           *sql.DB
	validator       *readexec.Validator
}

func newMySQLAnalyticalFixture(t *testing.T, timezones ...string) mysqlAnalyticalFixture {
	t.Helper()
	adminDSN := os.Getenv("CHARTWORKS_TEST_MYSQL_DSN")
	if adminDSN == "" {
		t.Skip("CHARTWORKS_TEST_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "cw_analytical_" + hex.EncodeToString(nonce[:])
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, s := range []string{"DROP DATABASE IF EXISTS " + name, "DROP USER IF EXISTS '" + name + "'@'%'"} {
			if _, e := admin.ExecContext(ctx, s); e != nil {
				t.Errorf("synthetic cleanup: %v", e)
			}
		}
		if e := admin.Close(); e != nil {
			t.Error(e)
		}
	})
	for _, s := range []string{
		"CREATE DATABASE " + name,
		"CREATE TABLE " + name + ".sales(id BIGINT PRIMARY KEY,amount DECIMAL(20,2),category VARCHAR(20) NOT NULL,created_at DATETIME(6),created_timestamp TIMESTAMP(6) NULL)",
		"CREATE TABLE " + name + ".items(id BIGINT PRIMARY KEY,quantity DECIMAL(20,2))",
		"INSERT INTO " + name + ".items VALUES(1,3),(2,4)",
		"INSERT INTO " + name + ".sales(id,amount,category) VALUES(1,10,'A'),(2,10,'A'),(3,30,'B'),(4,NULL,'B')",
		"CREATE USER '" + name + "'@'%' IDENTIFIED BY 'SYNTHETIC_Analytical_Password9!'",
		"GRANT SELECT ON " + name + ".* TO '" + name + "'@'%'",
	} {
		if _, err = admin.ExecContext(t.Context(), s); err != nil {
			t.Fatal("synthetic setup", err)
		}
	}
	cfg.User, cfg.Passwd, cfg.DBName = name, "SYNTHETIC_Analytical_Password9!", name
	if len(timezones) > 0 {
		if cfg.Params == nil {
			cfg.Params = map[string]string{}
		}
		cfg.Params["time_zone"] = "'" + timezones[0] + "'"
	}
	settings := config.DefaultSources()
	settings.Enabled = true
	settings.Connections = []config.SourceConnection{{Dialect: "mysql", AllowInsecureLocal: true, Tenant: "tenant", ID: "warehouse", Version: "v1", ReadDSN: "env:MYSQL_ANALYTICAL_DSN", Relations: []config.SourceRelation{{Schema: name, Name: "sales", Columns: []string{"id", "amount", "category", "created_at", "created_timestamp"}}, {Schema: name, Name: "items", Columns: []string{"id", "quantity"}}}}}
	service, err := New(&cloudMemoryRepository{records: map[string]Record{}}, settings, func(k string) (string, bool) { return cfg.FormatDSN(), k == "MYSQL_ANALYTICAL_DSN" })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	dataset := "ds:" + readexec.Hash([]string{"source", name, "sales"})[:32]
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.write", "sources.read", "sources.query", "cw.tenant.write:tenant", "cw.source.read:source", "cw.source.write:source", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:" + dataset, "cw.dataset.query:ds:" + readexec.Hash([]string{"source", name, "items"})[:32]}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(t.Context(), e, CreateRequest{ID: "source", Name: "Synthetic analytical source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.Binding(t.Context(), e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}

	return mysqlAnalyticalFixture{service: service, envelope: e, binding: binding, dataset: dataset, schema: name, admin: admin, validator: validator}
}
