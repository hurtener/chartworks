//go:build cgo && (linux || darwin)

package nlqexec

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// This actual MySQL control uses a private in-memory source registry only. Source
// discovery, physical nullability, native EXPLAIN, typed execution, final-output
// proof and the production result annotation all use their ordinary consumers.
func TestSQLRecoveryMySQLCompletenessLocal(t *testing.T) {
	dsn := os.Getenv("CHARTWORKS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("CHARTWORKS_TEST_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "cw_complete_" + exec.Hash([]string{t.Name(), time.Now().String()})[:12]
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, s := range []string{"DROP DATABASE IF EXISTS " + name, "DROP USER IF EXISTS '" + name + "'@'%'"} {
			if _, err := admin.ExecContext(ctx, s); err != nil {
				t.Error(err)
			}
		}
		_ = admin.Close()
	})
	for _, s := range []string{
		"CREATE DATABASE " + name,
		"CREATE TABLE " + name + ".sales(id BIGINT PRIMARY KEY,amount DECIMAL(20,2),category VARCHAR(20) NOT NULL,created_at DATETIME(6))",
		"CREATE TABLE " + name + ".anchors(id BIGINT PRIMARY KEY,category VARCHAR(20) NOT NULL)",
		"INSERT INTO " + name + ".anchors VALUES(1,'matched'),(99,'missing')",
		"INSERT INTO " + name + ".sales VALUES(1,10,'paid','2026-01-01'),(2,10,'paid','2026-01-15'),(3,NULL,'paid','2026-02-01'),(4,999,'cancelled','2026-03-01'),(5,0,'paid','2026-10-01')",
		"CREATE USER '" + name + "'@'%' IDENTIFIED BY 'SYNTHETIC_Completeness9!'",
		"GRANT SELECT ON " + name + ".* TO '" + name + "'@'%'",
	} {
		if _, err := admin.ExecContext(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	cfg.User, cfg.Passwd, cfg.DBName = name, "SYNTHETIC_Completeness9!", name
	settings := config.DefaultSources()
	settings.Enabled = true
	settings.Connections = []config.SourceConnection{{Dialect: "mysql", AllowInsecureLocal: true, Tenant: "tenant", ID: "warehouse", Version: "v1", ReadDSN: "env:MYSQL_COMPLETENESS_DSN", Relations: []config.SourceRelation{{Schema: name, Name: "sales", Columns: []string{"id", "amount", "category", "created_at"}}, {Schema: name, Name: "anchors", Columns: []string{"id", "category"}}}}}
	service, err := sources.New(&completenessSourceRepository{records: map[string]sources.Record{}}, settings, func(key string) (string, bool) { return cfg.FormatDSN(), key == "MYSQL_COMPLETENESS_DSN" })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	dataset := "ds:" + exec.Hash([]string{"source", name, "sales"})[:32]
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.write", "sources.read", "sources.query", "cw.tenant.write:tenant", "cw.source.read:source", "cw.source.write:source", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:" + dataset, "cw.dataset.query:ds:" + exec.Hash([]string{"source", name, "anchors"})[:32]}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	src, err := service.Create(t.Context(), e, sources.CreateRequest{ID: "source", Name: "Synthetic completeness source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.Binding(t.Context(), e, src.ID, src.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := exec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	a := knownAmountAdmission(t)
	a.binding = binding
	d := &a.publications[0].Definition
	d.Datasets[0].ID = dataset
	d.Datasets[0].Source.Source = src.ID
	d.Datasets[0].Source.Context = src.ContextID
	d.Datasets[0].Source.Dataset = dataset
	for i := range d.Datasets[0].Columns {
		c := &d.Datasets[0].Columns[i]
		c.SourceName = map[string]string{"amount": "amount", "id": "id", "region": "category"}[c.ID]
		for _, relation := range binding.Relations {
			if relation.ID == dataset {
				for _, actual := range relation.Columns {
					if actual.Name == c.SourceName {
						c.NativeType, c.Category, c.Nullable = actual.NativeType, actual.Category, actual.Nullable
					}
				}
			}
		}
	}
	for i := range d.Measures {
		m := &d.Measures[i]
		m.Field.Dataset = dataset
		m.Filters = []semantics.SemanticFilter{{ID: "paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset, ID: "region"}, Operator: "eq", Values: []string{"paid"}}}
	}
	analyticalReseal(&a)
	c, err := compileCurrentAnalytical(t.Context(), a)
	if err != nil {
		t.Fatal("actual MySQL completeness compiler", err)
	}
	validateRun := func(statement string, contract exec.AnalyticalContract, want [][]string, statuses []string) {
		t.Helper()
		plan, err := validator.Validate(t.Context(), e, exec.Request{Source: src.ID, Context: src.ContextID, SQL: statement})
		if err != nil {
			t.Fatal("native completeness", err)
		}
		proof, err := exec.CheckAnalyticalPlan(t.Context(), plan, contract)
		if err != nil {
			t.Fatal("completeness proof", err)
		}
		result, err := service.ExecuteRead(t.Context(), e, plan, exec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, exec.Hash([]string{statement, fmt.Sprint(want)})[:32], nil)
		if err != nil {
			t.Fatal("actual completeness result", err)
		}
		if len(result.Result.Rows) != len(want) {
			t.Fatal("wrong group domain", result.Result.Rows)
		}
		for i, row := range result.Result.Rows {
			if len(row) != len(want[i]) {
				t.Fatal("wrong outputs")
			}
			for j, raw := range row {
				value := string(raw)
				var text string
				if string(raw) != "null" && json.Unmarshal(raw, &text) == nil {
					value = text
				}
				if value != want[i][j] {
					t.Fatalf("actual value %q != %q", value, want[i][j])
				}
			}
		}
		evidence := resultAmountCompleteness(proof, &result.Result, true)
		if len(evidence) != 1 || len(evidence[0].Rows) != len(statuses) {
			t.Fatal("annotation missing", evidence)
		}
		for i, status := range statuses {
			if evidence[0].Rows[i].Status != status {
				t.Fatal("wrong proved completeness", evidence)
			}
		}
		t.Logf("actual MySQL rows=%v statuses=%v outputs=%v", want, statuses, proof.Outputs)
	}
	scalar := "SELECT count(id)-count(amount) AS known_total,sum(amount) AS unknown_count FROM " + name + ".sales WHERE category='paid'"
	validateRun(scalar, *c, [][]string{{"1", "20.00"}}, []string{"incomplete"})
	month := "CAST(DATE_FORMAT(created_at,'%Y-%m-01') AS DATE)"
	grouped := *c
	grouped.Grain = &exec.AnalyticalGrain{Policy: exec.AnalyticalCalendarPolicy, Dimensions: []string{"month"}, Buckets: []exec.AnalyticalBucket{{Column: "created_at", Calendar: "gregorian", Grain: "month"}}}
	grouped.GroupDomain = &exec.AnalyticalGroupDomain{Policy: exec.AnalyticalGroupDomainPolicy, Domain: exec.AnalyticalGroupDomainQualifying}
	statement := "SELECT " + month + " AS period,count(id)-count(amount) AS known_total,sum(amount) AS unknown_count FROM " + name + ".sales WHERE category='paid' GROUP BY 1 ORDER BY 1"
	validateRun(statement, grouped, [][]string{{"2026-01-01 00:00:00", "0", "20.00"}, {"2026-02-01 00:00:00", "1", "null"}, {"2026-10-01 00:00:00", "0", "0.00"}}, []string{"complete", "incomplete", "complete"})
	for _, bad := range []string{strings.Replace(statement, "count(id)-count(amount)", "count(amount)-count(id)", 1), strings.Replace(statement, "count(id)-count(amount)", "count(DISTINCT id)-count(amount)", 1), strings.Replace(statement, "count(id)-count(amount) AS known_total,sum(amount)", "count(CASE WHEN category='paid' THEN id END)-count(CASE WHEN category='paid' THEN amount END) AS known_total,sum(CASE WHEN category='paid' THEN amount END)", 1)} {
		if strings.Contains(bad, "sum(CASE") {
			bad = strings.Replace(bad, " WHERE category='paid'", "", 1)
		}
		plan, err := validator.Validate(t.Context(), e, exec.Request{Source: src.ID, Context: src.ContextID, SQL: bad})
		if err != nil {
			t.Fatal("negative native query", err)
		}
		if _, err := exec.CheckAnalyticalPlan(t.Context(), plan, grouped); err == nil {
			t.Fatal("changed MySQL meaning accepted", bad)
		}
	}
	// A right-side physical primary key is still NULL in an unmatched LEFT row.
	// This witness is read directly as an oracle, never exposed as certified SQL.
	var absentAmount sql.NullString
	var absentUnknown int64
	if err := admin.QueryRowContext(t.Context(), "SELECT SUM(s.amount),COUNT(s.id)-COUNT(s.amount) FROM "+name+".anchors a LEFT JOIN "+name+".sales s ON a.id=s.id WHERE a.id=99").Scan(&absentAmount, &absentUnknown); err != nil || absentAmount.Valid || absentUnknown != 0 {
		t.Fatal("missing-row null-extension witness", err)
	}
	anchor := "ds:" + exec.Hash([]string{"source", name, "anchors"})[:32]
	extended := *c
	extended.Dataset = anchor
	extended.Metrics = append([]exec.AnalyticalMetric(nil), c.Metrics...)
	for i := range extended.Metrics {
		extended.Metrics[i].Expression = rebaseAnalyticalExpression(extended.Metrics[i].Expression, dataset, anchor)
	}
	extended.Metrics = append(extended.Metrics, exec.AnalyticalMetric{ID: "anchor_sum", Expression: exec.AnalyticalExpression{Op: "sum", Column: "id"}})
	extended.Joins = []exec.AnalyticalJoin{{Left: anchor, Right: dataset, Type: "left", LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}
	extended.Grain = &exec.AnalyticalGrain{Policy: exec.AnalyticalGroupingPolicy, Columns: []string{"category"}, Dimensions: []string{"anchor-category"}}
	extended.GroupDomain = &exec.AnalyticalGroupDomain{Policy: exec.AnalyticalGroupDomainPolicy, Domain: exec.AnalyticalGroupDomainRaw}
	nullExtendedSQL := "SELECT a.category AS group_key,sum(a.id) AS left_total,sum(CASE WHEN s.category='paid' THEN s.amount END) AS right_total,count(CASE WHEN s.category='paid' THEN s.id END)-count(CASE WHEN s.category='paid' THEN s.amount END) AS unknown_amounts FROM " + name + ".anchors a LEFT JOIN " + name + ".sales s ON a.id=s.id GROUP BY a.category"
	nullPlan, err := validator.Validate(t.Context(), e, exec.Request{Source: src.ID, Context: src.ContextID, SQL: nullExtendedSQL})
	if err != nil {
		t.Fatal("native LEFT witness", err)
	}
	if _, err := exec.CheckAnalyticalPlan(t.Context(), nullPlan, extended); err == nil {
		t.Fatal("missing right fact certified complete from raw primary key")
	}
	t.Log("actual missing-right-row witness SUM=NULL/count-difference=0 rejected by completeness proof")
	a.binding.Relations = append([]exec.Relation(nil), a.binding.Relations...)
	for i := range a.binding.Relations {
		a.binding.Relations[i].Columns = append([]exec.Column(nil), a.binding.Relations[i].Columns...)
	}
	for _, relation := range a.binding.Relations {
		if relation.ID == dataset {
			for i := range relation.Columns {
				if relation.Columns[i].Name == "id" {
					relation.Columns[i].Nullable = true
				}
			}
		}
	}
	if _, err := compileCurrentAnalytical(t.Context(), a); err == nil {
		t.Fatal("physical nullable identity admitted")
	}
	if _, err := admin.ExecContext(t.Context(), "UPDATE "+name+".sales SET amount=NULL"); err != nil {
		t.Fatal(err)
	}
	validateRun(scalar, *c, [][]string{{"4", "null"}}, []string{"incomplete"})
	if _, err := admin.ExecContext(t.Context(), "DELETE FROM "+name+".sales"); err != nil {
		t.Fatal(err)
	}
	validateRun(scalar, *c, [][]string{{"0", "null"}}, []string{"complete"})
}

type completenessSourceRepository struct {
	mu      sync.RWMutex
	records map[string]sources.Record
}

func (r *completenessSourceRepository) PutSource(_ context.Context, s store.Scope, expected int64, next sources.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := s.Tenant() + "/" + next.Source.ID
	old, ok := r.records[key]
	if !ok && expected != 0 || ok && old.Source.Revision != expected {
		return store.ErrConflict
	}
	r.records[key] = next
	return nil
}
func (r *completenessSourceRepository) ReadSource(_ context.Context, s store.Scope, id string) (sources.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.records[s.Tenant()+"/"+id]
	if !ok {
		return sources.Record{}, store.ErrNotFound
	}
	return v, nil
}
func (r *completenessSourceRepository) ListSourcePage(context.Context, store.Scope, access.Selection, sources.SourceListRequest) ([]sources.Source, error) {
	return nil, store.ErrUnavailable
}
func (r *completenessSourceRepository) ReadDatasetCatalog(context.Context, identity.Envelope, sources.DatasetQuery) ([]sources.Dataset, error) {
	return nil, store.ErrUnavailable
}
func (r *completenessSourceRepository) WithSource(ctx context.Context, s store.Scope, id string, fn func(context.Context, sources.Record) error) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.records[s.Tenant()+"/"+id]
	if !ok {
		return store.ErrNotFound
	}
	return fn(ctx, v)
}
