//go:build cgo && (linux || darwin)

package sources

import (
	"fmt"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"sort"
	"testing"
	"time"
)

func TestSQLRecoveryMySQLCalendarLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	if _, err := f.admin.ExecContext(t.Context(), "UPDATE "+f.schema+".sales SET created_at=CASE id WHEN 1 THEN '2025-12-31 23:59:59.999999' WHEN 2 THEN '2026-01-01 00:00:00' WHEN 3 THEN '2026-04-01 00:00:00' ELSE NULL END"); err != nil {
		t.Fatal(err)
	}
	for index, tc := range []struct {
		grain, expr string
		want        []string
	}{
		{"month", `CAST(DATE_FORMAT(created_at,'%Y-%m-01') AS DATE)`, []string{`null`, `"2025-12-01 00:00:00"`, `"2026-01-01 00:00:00"`, `"2026-04-01 00:00:00"`}},
		{"quarter", `MAKEDATE(EXTRACT(YEAR FROM created_at),1) + INTERVAL EXTRACT(QUARTER FROM created_at)-1 QUARTER`, []string{`null`, `"2025-10-01 00:00:00"`, `"2026-01-01 00:00:00"`, `"2026-04-01 00:00:00"`}},
	} {
		t.Run(tc.grain, func(t *testing.T) {
			statement := "SELECT " + tc.expr + " AS period,sum(amount) AS value FROM " + f.schema + ".sales GROUP BY " + tc.expr + " ORDER BY period"
			plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
			if err != nil {
				t.Fatal("native calendar", err)
			}
			c := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-calendar"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalCalendarPolicy, Dimensions: []string{"period"}, Buckets: []readexec.AnalyticalBucket{{Column: "created_at", Calendar: "gregorian", Grain: tc.grain}}}}
			if _, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c); err != nil {
				t.Fatal("calendar proof", err)
			}
			result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", index+101), &cloudObserverCapture{})
			if err != nil || len(result.Result.Rows) != len(tc.want) {
				t.Fatal("calendar native result", err)
			}
			for i, row := range result.Result.Rows {
				if string(row[0]) != tc.want[i] {
					t.Fatalf("bucket %s != %s", row[0], tc.want[i])
				}
			}
			c.Grain.Buckets[0].Grain = "day"
			if _, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c); err == nil {
				t.Fatal("wrong calendar partition admitted")
			}
		})
	}
}

func TestSQLRecoveryMySQLIndependentPopulationsLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	items := ""
	for _, r := range f.binding.Relations {
		if r.Name == "items" {
			items = r.ID
		}
	}
	if items == "" {
		t.Fatal("missing second actual relation")
	}
	populations := []string{f.dataset, items}
	sort.Strings(populations)
	contract := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-independent"), Dataset: f.dataset, Populations: populations, Metrics: []readexec.AnalyticalMetric{{ID: "net", Expression: readexec.AnalyticalExpression{Op: "-", Args: []readexec.AnalyticalExpression{{Op: "sum", Column: "amount"}, {Op: "sum", Column: readexec.AnalyticalColumnName(f.dataset, items, "quantity")}}}}}}
	for index, statement := range []string{
		"WITH s AS (SELECT sum(amount) AS total FROM " + f.schema + ".sales), i AS (SELECT sum(quantity) AS total FROM " + f.schema + ".items) SELECT s.total-i.total AS net FROM s CROSS JOIN i",
		"SELECT s.total-i.total AS net FROM (SELECT sum(amount) AS total FROM " + f.schema + ".sales) s CROSS JOIN (SELECT sum(quantity) AS total FROM " + f.schema + ".items) i",
	} {
		plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
		if err != nil {
			t.Fatal("native independent populations", err)
		}
		if _, err := readexec.CheckAnalyticalPlan(t.Context(), plan, contract); err != nil {
			t.Fatal("independent proof", err)
		}
		result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", index+201), &cloudObserverCapture{})
		if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"43.00"` {
			t.Fatalf("independent result: %v %#v", err, result.Result.Rows)
		}
	}
}

func TestSQLRecoveryMySQLCalendarSessionDriftLocal(t *testing.T) {
	for index, zone := range []string{"+00:00", "+05:30", "-07:00"} {
		t.Run(zone, func(t *testing.T) {
			f := newMySQLAnalyticalFixture(t, zone)
			if _, err := f.admin.ExecContext(t.Context(), "UPDATE "+f.schema+".sales SET created_at='2026-01-01 00:15:00',created_timestamp='2026-01-01 00:15:00'"); err != nil {
				t.Fatal(err)
			}
			for _, column := range []string{"created_at", "created_timestamp"} {
				statement := "SELECT CAST(DATE_FORMAT(" + column + ",'%Y-%m-01') AS DATE) AS period,sum(amount) AS value FROM " + f.schema + ".sales GROUP BY 1"
				plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
				if err != nil {
					t.Fatal(err)
				}
				c := readexec.AnalyticalContract{Version: readexec.AnalyticalIntentVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-calendar"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalCalendarPolicy, Dimensions: []string{"period"}, Buckets: []readexec.AnalyticalBucket{{Column: column, Calendar: "gregorian", Grain: "month"}}}}
				proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c)
				if column == "created_timestamp" {
					if err == nil || proof != nil {
						t.Fatal("session-sensitive timestamp certified")
					}
					continue
				}
				if err != nil {
					t.Fatal("civil month proof", err)
				}
				result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", index+301), &cloudObserverCapture{})
				if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"2026-01-01 00:00:00"` {
					t.Fatalf("civil date changed with session: %v %#v", err, result.Result.Rows)
				}
			}
		})
	}
}

func TestSQLRecoveryMySQLReviewedNullPolicyLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	aggregate := readexec.AnalyticalExpression{Op: "sum", Column: "amount", Filters: []readexec.AnalyticalFilter{{Column: "category", Kind: "eq", Values: []string{"missing"}}}}
	c := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedPopulationsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-reviewed-null"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "refund", Expression: readexec.AnalyticalExpression{Op: "coalesce", Args: []readexec.AnalyticalExpression{aggregate, {Op: "number", Value: "0"}}}}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: &readexec.AnalyticalQueryPopulation{Policy: readexec.AnalyticalQueryPopulationPolicy}}
	statement := "SELECT coalesce(sum(CASE WHEN category='missing' THEN amount END),0) AS value FROM " + f.schema + ".sales"
	plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c); err != nil {
		t.Fatal("reviewed null fallback", err)
	}
	result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, "00000000000000000000000000000401", &cloudObserverCapture{})
	if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"0.00"` {
		t.Fatalf("reviewed empty population result: %v %#v", err, result.Result.Rows)
	}
	c.Metrics[0].Expression = aggregate
	if _, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c); err == nil {
		t.Fatal("unreviewed zero fill admitted")
	}
}

// Explicit UTC retrieval must keep the same partitions under differing session
// offsets, including instants adjacent to US DST transitions and UTC midnight.
func TestSQLRecoveryMySQLUTCInstantCalendarLocal(t *testing.T) {
	for zi, zone := range []string{"+00:00", "+05:30", "-07:00"} {
		t.Run(zone, func(t *testing.T) {
			f := newMySQLAnalyticalFixture(t, zone)
			conn, err := f.admin.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err = conn.ExecContext(t.Context(), "SET SESSION time_zone='+00:00'"); err != nil {
				t.Fatal(err)
			}
			if _, err = conn.ExecContext(t.Context(), "UPDATE "+f.schema+".sales SET created_timestamp=CASE id WHEN 1 THEN '2026-01-01 00:00:00.000001' WHEN 2 THEN '2026-03-08 06:59:59.999999' WHEN 3 THEN '2026-11-01 06:00:00.000001' ELSE NULL END"); err != nil {
				t.Fatal(err)
			}
			utc := "CAST(created_timestamp AT TIME ZONE '+00:00' AS DATETIME(6))"
			for gi, tc := range []struct {
				grain, expr string
				want        []string
			}{
				{"month", "CAST(DATE_FORMAT(" + utc + ",'%Y-%m-01') AS DATE)", []string{`null`, `"2026-01-01 00:00:00"`, `"2026-03-01 00:00:00"`, `"2026-11-01 00:00:00"`}},
				{"quarter", "MAKEDATE(EXTRACT(YEAR FROM " + utc + "),1) + INTERVAL (EXTRACT(QUARTER FROM " + utc + ")-1) QUARTER", []string{`null`, `"2026-01-01 00:00:00"`, `"2026-10-01 00:00:00"`}},
			} {
				statement := "SELECT " + tc.expr + " AS period,sum(amount) AS value FROM " + f.schema + ".sales GROUP BY 1 ORDER BY period"
				plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
				if err != nil {
					t.Fatal("native UTC bucket", err)
				}
				c := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedProgramsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-utc-bucket"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}, Grain: &readexec.AnalyticalGrain{Policy: readexec.AnalyticalCalendarPolicy, Dimensions: []string{"period"}, Buckets: []readexec.AnalyticalBucket{{Column: "created_timestamp", Calendar: "gregorian", Grain: tc.grain, Timezone: "UTC"}}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: &readexec.AnalyticalQueryPopulation{Policy: readexec.AnalyticalQueryPopulationPolicy}}
				if _, err = readexec.CheckAnalyticalPlan(t.Context(), plan, c); err != nil {
					t.Fatal("UTC bucket proof", err)
				}
				result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", 501+zi*10+gi), &cloudObserverCapture{})
				if err != nil || len(result.Result.Rows) != len(tc.want) {
					t.Fatalf("UTC result %v %#v", err, result.Result.Rows)
				}
				for i, row := range result.Result.Rows {
					if string(row[0]) != tc.want[i] {
						t.Fatalf("UTC partition %s != %s", row[0], tc.want[i])
					}
				}
				wantValues := []string{`null`, `"10.00"`, `"10.00"`, `"30.00"`}
				if tc.grain == "quarter" {
					wantValues = []string{`null`, `"20.00"`, `"30.00"`}
				}
				for i, row := range result.Result.Rows {
					if string(row[1]) != wantValues[i] {
						t.Fatalf("UTC aggregate %s != %s", row[1], wantValues[i])
					}
				}

				c.Grain.Buckets[0].Timezone = "America/New_York"
				if _, err = readexec.CheckAnalyticalPlan(t.Context(), plan, c); err == nil {
					t.Fatal("UTC result borrowed local-zone proof")
				}
			}
		})
	}
}
