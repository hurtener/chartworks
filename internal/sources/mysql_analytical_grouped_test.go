//go:build cgo && (linux || darwin)

package sources

import (
	"context"
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSQLRecoveryMySQLGroupedPopulationsLocal(t *testing.T) {
	f := newMySQLGroupedFixture(t, true)
	ids := map[string]string{}
	for _, r := range f.binding.Relations {
		ids[r.Name] = r.ID
	}
	column := readexec.AnalyticalColumnName(f.dataset, ids["dimensions"], "group_key")
	contract := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedProgramsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-mysql-grouped"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "net", Expression: readexec.AnalyticalExpression{Op: "-", Args: []readexec.AnalyticalExpression{{Op: "sum", Column: "amount"}, {Op: "sum", Column: readexec.AnalyticalColumnName(f.dataset, ids["items"], "quantity")}}}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalGroupingPolicy, Columns: []string{column}, Dimensions: []string{"group"}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: &readexec.AnalyticalQueryPopulation{Policy: readexec.AnalyticalQueryPopulationPolicy}, GroupedPopulations: &readexec.AnalyticalGroupedPopulations{Policy: readexec.AnalyticalGroupedPopulationPolicy, Lanes: []readexec.AnalyticalGroupedLane{
		{Dataset: f.dataset, Joins: []readexec.AnalyticalJoin{{Left: f.dataset, Right: ids["dimensions"], Type: "left", LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}},
		{Dataset: ids["items"], Joins: []readexec.AnalyticalJoin{{Left: ids["items"], Right: ids["dimensions"], Type: "left", LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}},
	}}}
	sort.Slice(contract.GroupedPopulations.Lanes, func(i, j int) bool {
		return contract.GroupedPopulations.Lanes[i].Dataset < contract.GroupedPopulations.Lanes[j].Dataset
	})
	lanes := "WITH s AS (SELECT d.group_key k,sum(s.amount) v FROM " + f.schema + ".sales s LEFT JOIN " + f.schema + ".dimensions d ON s.id=d.id GROUP BY d.group_key),i AS (SELECT d.group_key k,sum(i.quantity) v FROM " + f.schema + ".items i LEFT JOIN " + f.schema + ".dimensions d ON i.id=d.id GROUP BY d.group_key) "
	statement := lanes + "SELECT k.k,s.v-i.v net FROM (SELECT k FROM s UNION SELECT k FROM i) k LEFT JOIN s ON k.k<=>s.k LEFT JOIN i ON k.k<=>i.k ORDER BY k.k"
	for index, tc := range []struct {
		sql  string
		pass bool
	}{{statement, true}, {strings.Replace(statement, "<=>", "=", 1), false}, {strings.Replace(statement, " UNION ", " UNION ALL ", 1), false}, {strings.Replace(statement, "sum(i.quantity)", "avg(i.quantity)", 1), false}} {
		plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: tc.sql})
		if err != nil {
			t.Fatal("native grouped query", err)
		}
		_, err = readexec.CheckAnalyticalPlan(t.Context(), plan, contract)
		if (err == nil) != tc.pass {
			t.Fatal("grouped proof", index, err)
		}
		if !tc.pass {
			continue
		}
		result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", 701+index), &cloudObserverCapture{})
		if err != nil || len(result.Result.Rows) != 3 {
			t.Fatalf("grouped result %v %#v", err, result.Result.Rows)
		}
		want := [][2]string{{"null", `"-4.00"`}, {`"10"`, `"13.00"`}, {`"20"`, "null"}}
		for i, row := range result.Result.Rows {
			if string(row[0]) != want[i][0] || string(row[1]) != want[i][1] {
				t.Fatalf("grouped row %#v != %#v", row, want[i])
			}
		}
	}
	calendar := contract
	calendar.Grain = &readexec.AnalyticalGrain{Policy: readexec.AnalyticalCalendarPolicy, Dimensions: []string{"calendar"}, Buckets: []readexec.AnalyticalBucket{{Column: readexec.AnalyticalColumnName(f.dataset, ids["dimensions"], "created_at"), Calendar: "gregorian", Grain: "month"}}}
	calendarSQL := strings.ReplaceAll(statement, "d.group_key", "CAST(DATE_FORMAT(d.created_at,'%Y-%m-01') AS DATE)")
	// GROUP BY ordinal preserves the same exact lane key and avoids repeating a
	// long expression within the native parser's unchanged bounded SQL profile.
	calendarSQL = strings.ReplaceAll(calendarSQL, "GROUP BY CAST(DATE_FORMAT(d.created_at,'%Y-%m-01') AS DATE)", "GROUP BY 1")
	for index, sql := range []string{calendarSQL, "SELECT q.k,q.net FROM (" + strings.TrimSuffix(calendarSQL, " ORDER BY k.k") + ") q ORDER BY q.k"} {
		plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: sql})
		if err != nil {
			t.Fatal("native grouped calendar", err)
		}
		if _, err = readexec.CheckAnalyticalPlan(t.Context(), plan, calendar); err != nil {
			t.Fatal("grouped calendar proof", err)
		}
		result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", 801+index), &cloudObserverCapture{})
		if err != nil || len(result.Result.Rows) != 3 {
			t.Fatalf("grouped calendar result %v %#v", err, result.Result.Rows)
		}
		want := [][2]string{{"null", `"-4.00"`}, {`"2026-01-01 00:00:00"`, `"13.00"`}, {`"2026-02-01 00:00:00"`, "null"}}
		for i, row := range result.Result.Rows {
			if string(row[0]) != want[i][0] || string(row[1]) != want[i][1] {
				t.Fatalf("calendar row %#v != %#v", row, want[i])
			}
		}
	}

}

type mysqlExplainCapture struct {
	*Service
	calls atomic.Int64
}

func (a *mysqlExplainCapture) Explain(ctx context.Context, e identity.Envelope, candidate readexec.Candidate) error {
	a.calls.Add(1)
	return a.Service.Explain(ctx, e, candidate)
}
func TestSQLRecoveryMySQLNullSafeParametersLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	adapter := &mysqlExplainCapture{Service: f.service}
	validator, err := readexec.NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	request := readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: "SELECT id FROM " + f.schema + ".sales WHERE id <=> ?"}
	for _, parameters := range [][]readexec.Parameter{nil, {{Kind: "integer", Value: "1"}, {Kind: "integer", Value: "2"}}} {
		request.Parameters = parameters
		if _, err = validator.Validate(t.Context(), f.envelope, request); !errors.Is(err, readexec.ErrBinding) || adapter.calls.Load() != 0 {
			t.Fatal("invalid null-safe bindings reached native Explain", err, adapter.calls.Load())
		}
	}
	request.Parameters = []readexec.Parameter{{Kind: "integer", Value: "1"}}
	plan, err := validator.Validate(t.Context(), f.envelope, request)
	if err != nil || adapter.calls.Load() != 1 {
		t.Fatal("exact null-safe binding", err)
	}
	result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, "00000000000000000000000000000901", &cloudObserverCapture{})
	if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"1"` {
		t.Fatalf("actual null-safe parameter result %v %#v", err, result.Result.Rows)
	}
}
