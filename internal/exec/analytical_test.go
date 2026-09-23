package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

func analyticalFixture(t *testing.T, sql string, metrics []AnalyticalMetric, params ...Parameter) (Plan, AnalyticalContract) {
	t.Helper()
	b := parserBinding()
	for i := range b.Relations[0].Columns {
		c := &b.Relations[0].Columns[i]
		switch c.Name {
		case "id":
			c.Category = "numeric"
		case "amount":
			c.Category = "numeric"
			c.Nullable = true
		case "name":
			c.Category = "text"
		case "active":
			c.Category = "boolean"
		}
	}
	e, err := identity.FromVerified(b.Tenant, "actor", "session", []string{"sources.query"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return Plan{candidate: Candidate{binding: b, owner: e, checked: true, statement: sql, parameters: params, dependencies: []string{"sales"}}, nativeChecked: true}, AnalyticalContract{Version: AnalyticalVersion, Binding: Hash(b), Semantics: Hash("synthetic-v1"), Dataset: "sales", Metrics: metrics}
}
func analyticalMeasure(op, column string, filters ...AnalyticalFilter) AnalyticalExpression {
	return AnalyticalExpression{Op: op, Column: column, Filters: filters}
}
func analyticalMetrics(e ...AnalyticalExpression) []AnalyticalMetric {
	out := make([]AnalyticalMetric, len(e))
	for i, x := range e {
		out[i] = AnalyticalMetric{ID: fmt.Sprintf("metric-%d", i), Expression: x}
	}
	return out
}

func TestSQLRecoveryAnalyticalAggregates(t *testing.T) {
	for _, tc := range []struct {
		op, sql string
		pass    bool
	}{
		{"sum", `SELECT sum(s.amount) AS revenue FROM analytics.sales AS s`, true},
		{"sum", `SELECT active, SUM(amount) AS revenue FROM analytics.sales GROUP BY active`, true},
		{"sum", `SELECT active AS category, sum(amount) AS revenue FROM analytics.sales GROUP BY 1`, true},
		{"sum", `SELECT active AS category, sum(amount) AS revenue FROM analytics.sales GROUP BY category`, true},
		{"sum", `SELECT avg(amount) AS revenue FROM analytics.sales`, false},
		{"sum", `SELECT amount AS revenue FROM analytics.sales`, false},
		{"sum", `SELECT sum(DISTINCT amount) AS revenue FROM analytics.sales`, false},
		{"sum", `SELECT sum(amount) * 0 AS revenue FROM analytics.sales`, false},
		{"avg", `SELECT avg(amount) FROM analytics.sales`, true},
		{"min", `SELECT min(amount) FROM analytics.sales`, true},
		{"max", `SELECT max(amount) FROM analytics.sales`, true},
		{"count", `SELECT count(amount) FROM analytics.sales`, true},
		{"count", `SELECT count(*) FROM analytics.sales`, false},
		{"distinct_count", `SELECT count(DISTINCT amount) FROM analytics.sales`, true},
		{"distinct_count", `SELECT count(amount) FROM analytics.sales`, false},
	} {
		t.Run(tc.op+tc.sql, func(t *testing.T) {
			p, c := analyticalFixture(t, tc.sql, analyticalMetrics(analyticalMeasure(tc.op, "amount")))
			before, _ := json.Marshal(c)
			receipt, err := CheckAnalyticalPlan(context.Background(), p, c)
			if (err == nil) != tc.pass {
				t.Fatalf("pass=%v: %v", tc.pass, err)
			}
			if tc.pass && (receipt == nil || receipt.Version != AnalyticalVersion || receipt.Contract != Hash(c)) {
				t.Fatal("missing proof")
			}
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("mutated contract")
			}
		})
	}
	for _, sql := range []string{`SELECT count(*) FROM analytics.sales`, `SELECT count(id) FROM analytics.sales`} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("count", "id")))
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal("non-null count equivalence", err)
		}
	}
}
func TestSQLRecoveryAnalyticalPopulations(t *testing.T) {
	a := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"A"}}
	b := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"B"}}
	metrics := analyticalMetrics(analyticalMeasure("sum", "amount", a), analyticalMeasure("sum", "amount", b))
	for _, tc := range []struct {
		sql  string
		pass bool
	}{
		{`SELECT sum(amount) FILTER (WHERE name='A'), sum(amount) FILTER (WHERE name='B') FROM analytics.sales`, true},
		{`SELECT sum(CASE WHEN name='A' THEN amount END), sum(CASE WHEN name='B' THEN amount ELSE NULL END) FROM analytics.sales WHERE active=true`, true},
		{`SELECT sum(amount), sum(amount) FROM analytics.sales WHERE name='A' AND name='B'`, false},
		{`SELECT sum(amount) FILTER (WHERE name='A'), sum(amount) FILTER (WHERE name='B') FROM analytics.sales WHERE name='A'`, false},
		{`SELECT sum(amount) FILTER (WHERE name='A' OR active), sum(amount) FILTER (WHERE name='B') FROM analytics.sales`, false},
		{`SELECT sum(CASE WHEN name='A' THEN amount ELSE 0 END), sum(amount) FILTER (WHERE name='B') FROM analytics.sales`, false},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			p, c := analyticalFixture(t, tc.sql, metrics)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); (err == nil) != tc.pass {
				t.Fatalf("pass=%v: %v", tc.pass, err)
			}
		})
	}
	for _, sql := range []string{`SELECT sum(amount) FROM analytics.sales WHERE name='A'`, `SELECT sum(amount) FILTER (WHERE 'A'=name) FROM analytics.sales`, `SELECT sum(amount) FILTER (WHERE name IN ('A')) FROM analytics.sales`} {
		p, c := analyticalFixture(t, sql, metrics[:1])
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal(err)
		}
	}
	p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales WHERE name='A' OR active`, metrics[:1])
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
		t.Fatal("OR established mandatory filter", err)
	}
}
func TestSQLRecoveryAnalyticalTypedFiltersAndPrivacy(t *testing.T) {
	for _, f := range []AnalyticalFilter{{Column: "name", Kind: "in", Values: []string{"B", "A"}}, {Column: "amount", Kind: "eq", Values: []string{"9007199254740993.125"}}, {Column: "active", Kind: "eq", Values: []string{"true"}}, {Column: "name", Kind: "not_null"}} {
		predicates := map[string]string{"name:in": "name IN ('A','B')", "amount:eq": "amount=$1", "active:eq": "active=true", "name:not_null": "name IS NOT NULL"}
		var params []Parameter
		if f.Column == "amount" {
			params = []Parameter{{Kind: "number", Value: "9007199254740993.125"}}
		}
		p, c := analyticalFixture(t, "SELECT sum(amount) FILTER (WHERE "+predicates[f.Column+":"+f.Kind]+") FROM analytics.sales", analyticalMetrics(analyticalMeasure("sum", "amount", f)), params...)
		out, err := CheckAnalyticalPlan(context.Background(), p, c)
		if err != nil {
			t.Fatal(err)
		}
		wire, _ := json.Marshal(out)
		if strings.Contains(string(wire)+fmt.Sprintf("%v %#v", c, c), "9007199254740993") {
			t.Fatal("private bound value leaked")
		}
	}
}
func TestSQLRecoveryAnalyticalRatioAndIntegerDivision(t *testing.T) {
	sum := analyticalMeasure("sum", "amount")
	count := analyticalMeasure("count", "id")
	ratio := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{sum, count}}
	for _, tc := range []struct {
		sql  string
		pass bool
	}{
		{`SELECT sum(amount)/NULLIF(count(id),0) FROM analytics.sales`, true},
		{`SELECT sum(amount)/NULLIF(count(*),0) FROM analytics.sales`, true},
		{`SELECT count(*)/NULLIF(sum(amount),0) FROM analytics.sales`, false},
		{`SELECT sum(amount)/count(id) FROM analytics.sales`, false},
		{`SELECT avg(amount) FROM analytics.sales`, false},
	} {
		p, c := analyticalFixture(t, tc.sql, analyticalMetrics(ratio))
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); (err == nil) != tc.pass {
			t.Fatalf("%s: %v", tc.sql, err)
		}
	}
	expr := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{count, analyticalMeasure("sum", "id")}}
	for _, tc := range []struct {
		sql  string
		pass bool
	}{{`SELECT count(id)/NULLIF(sum(id),0) FROM analytics.sales`, false}, {`SELECT count(id)::numeric/NULLIF(sum(id),0) FROM analytics.sales`, true}} {
		p, c := analyticalFixture(t, tc.sql, analyticalMetrics(expr))
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); (err == nil) != tc.pass {
			t.Fatal(tc.sql, err)
		}
	}
}
func TestSQLRecoveryAnalyticalRejectsUnprovedShapes(t *testing.T) {
	for _, sql := range []string{
		`WITH q AS (SELECT sum(amount) FROM analytics.sales) SELECT 0 FROM q`,
		`SELECT sum(s.amount) FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id`,
		`SELECT sum(amount) FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales)`,
		`SELECT sum(amount) OVER () FROM analytics.sales`,
		`SELECT DISTINCT sum(amount) FROM analytics.sales`,
		`SELECT sum(amount) FROM analytics.sales UNION ALL SELECT sum(amount) FROM analytics.sales`,
		`SELECT sum(amount) AS right_metric,avg(amount) AS wrong_metric FROM analytics.sales`,
		`SELECT sum(amount)::numeric(10,0) FROM analytics.sales`,
	} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
		out, err := CheckAnalyticalPlan(context.Background(), p, c)
		if err == nil || out != nil {
			t.Fatal("unproved shape accepted", sql)
		}
	}
}
func TestSQLRecoveryAnalyticalRequiresOpaquePlanAndBounds(t *testing.T) {
	p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount")))
	if _, err := CheckAnalyticalPlan(context.Background(), Plan{}, c); !errors.Is(err, ErrBinding) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckAnalyticalPlan(ctx, p, c); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	changed := c
	changed.Binding = Hash("other-source")
	if _, err := CheckAnalyticalPlan(context.Background(), p, changed); !errors.Is(err, ErrBinding) {
		t.Fatal(err)
	}
	bad := p
	bad.candidate.binding.Dialect = "mysql"
	changed = c
	changed.Binding = Hash(bad.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), bad, changed); !errors.Is(err, ErrAnalyticalUnsupported) {
		t.Fatal(err)
	}
	original := p.candidate.binding.Clone()
	_, _ = CheckAnalyticalPlan(context.Background(), p, c)
	if !reflect.DeepEqual(p.candidate.binding, original) {
		t.Fatal("binding mutated")
	}
}

func TestSQLRecoveryAnalyticalNumericAndNullPolicies(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"010", "10"}, {"-0.125", "-1/8"}, {"9007199254740993.125", "72057594037927945/8"}, {".5", "1/2"}} {
		value, ok := analyticalNumber(tc.input)
		if !ok || value != tc.want {
			t.Fatal("decimal normalization", tc.input, value)
		}
	}
	for _, value := range []string{"1e10000000000", "0x10", "1/2", "1.2.3", "+", "-", "NaN", "Inf"} {
		if _, ok := analyticalNumber(value); ok {
			t.Fatal("unbounded/nondecimal value admitted")
		}
	}
	for _, sql := range []string{`SELECT NULLIF(sum(amount),0) FROM analytics.sales`, `SELECT NULLIF(sum(amount),0)+0 FROM analytics.sales`} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("NULLIF changed a non-denominator metric")
		}
	}
	count := analyticalMeasure("count", "amount")
	ratio := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{count, count}}
	p, c := analyticalFixture(t, `SELECT count(amount)/NULLIF(count(amount),0) FROM analytics.sales`, analyticalMetrics(ratio))
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
		t.Fatal("decimal count input hid integer division", err)
	}
	p, c = analyticalFixture(t, `SELECT sum(amount) FILTER (WHERE id=010) FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "id", Kind: "eq", Values: []string{"8"}})))
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("leading zero changed decimal literal meaning")
	}
}

func TestSQLRecoveryNativeNullifStaysPositive(t *testing.T) {
	for _, sql := range []string{`SELECT sum(amount)/NULLIF(count(id),0) FROM analytics.sales`, `SELECT NULLIF(amount,0) FROM analytics.sales`} {
		if _, _, err := resolveFixture(sql); err != nil {
			t.Fatal("native NULLIF rejected", err)
		}
	}
	for _, sql := range []string{`SELECT NULLIF(pg_read_file('/etc/passwd'),'')`, `SELECT NULLIF(public.unknown_function(amount),0) FROM analytics.sales`, `SELECT NULLIF((SELECT secret FROM analytics.sales),0)`} {
		if _, _, err := resolveFixture(sql); err == nil {
			t.Fatal("NULLIF bypassed native safety")
		}
	}
}

func TestSQLRecoveryAnalyticalRejectsHiddenNullPopulationChanges(t *testing.T) {
	for _, op := range []string{"sum", "avg", "min", "max", "count"} {
		for _, arg := range []string{"NULLIF(amount,0)", "NULLIF(amount,0)::numeric"} {
			p, c := analyticalFixture(t, "SELECT "+op+"("+arg+") FROM analytics.sales", analyticalMetrics(analyticalMeasure(op, "amount")))
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("NULLIF changed aggregate input population", op, arg)
			}
		}
	}
	p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales GROUP BY NULLIF(id,0)`, analyticalMetrics(analyticalMeasure("sum", "amount")))
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("transformed grouping presented as a direct column")
	}
}

func TestSQLRecoveryAnalyticalNativeNumericClassification(t *testing.T) {
	for _, native := range []string{"integer", "bigint", "int4", "numeric", "numeric(20,3)", "numeric(12,-2)", "decimal(20, 3)", "pg_catalog.numeric"} {
		t.Run(native, func(t *testing.T) {
			p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount")))
			p.candidate.binding.Relations[0].Columns[1].NativeType = native
			c.Binding = Hash(p.candidate.binding)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
				t.Fatal("verified broad numeric family rejected", err)
			}
		})
	}
	for _, native := range []string{"", "text", "float8", "double precision", "money", "private.money", "numeric[]", "numeric(20,3)[]", "numeric(0)", "numeric(3);other", "numeric(99999999999999999999)", "numeric(20,3,1)"} {
		p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount")))
		p.candidate.binding.Relations[0].Columns[1].NativeType = native
		c.Binding = Hash(p.candidate.binding)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalUnsupported) {
			t.Fatal("unsupported native type acquired exact arithmetic from category", native, err)
		}
	}
}
