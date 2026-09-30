package exec

import (
	"context"
	"errors"
	"testing"
)

func TestSQLRecoveryAnalyticalOrderLimitConformance(t *testing.T) {
	cases := []struct {
		name, tail string
		pass       bool
	}{
		{"alias", "ORDER BY revenue DESC NULLS LAST, category ASC LIMIT 3", true},
		{"ordinal", "ORDER BY 2 DESC NULLS LAST, 1 LIMIT 3", true},
		{"expression", "ORDER BY sum(amount) DESC NULLS LAST, name ASC NULLS LAST FETCH FIRST 3 ROWS ONLY", true},
		{"wrong direction", "ORDER BY revenue ASC NULLS LAST, category ASC LIMIT 3", false},
		{"wrong nulls", "ORDER BY revenue DESC, category ASC LIMIT 3", false},
		{"wrong priority", "ORDER BY category, revenue DESC NULLS LAST LIMIT 3", false},
		{"wrong metric", "ORDER BY avg(amount) DESC NULLS LAST, category LIMIT 3", false},
		{"missing order", "LIMIT 3", false},
		{"missing limit", "ORDER BY 2 DESC NULLS LAST, 1", false},
		{"changed limit", "ORDER BY 2 DESC NULLS LAST, 1 LIMIT 4", false},
		{"offset", "ORDER BY 2 DESC NULLS LAST, 1 LIMIT 3 OFFSET 1", false},
		{"zero offset", "ORDER BY 2 DESC NULLS LAST, 1 LIMIT 3 OFFSET 0", true},
		{"ties", "ORDER BY 2 DESC NULLS LAST, 1 FETCH FIRST 3 ROWS WITH TIES", false},
		{"limit all", "ORDER BY 2 DESC NULLS LAST, 1 LIMIT ALL", false},
		{"unexpected third sort", "ORDER BY 2 DESC NULLS LAST, 1, count(*) LIMIT 3", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, c := analyticalFixture(t, "SELECT name AS category,sum(amount) AS revenue FROM analytics.sales GROUP BY name "+tc.tail, analyticalMetrics(analyticalMeasure("sum", "amount")))
			c.Version = AnalyticalIntentVersion
			c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: "metric-0", Descending: true, Nulls: "last"}, {Column: "name", Nulls: "last"}}, Limit: 3}
			r, err := CheckAnalyticalPlan(context.Background(), p, c)
			if (err == nil) != tc.pass {
				t.Fatalf("pass=%v: %v", tc.pass, err)
			}
			if tc.pass && r.Intent != AnalyticalIntentPolicy {
				t.Fatal("missing policy")
			}
		})
	}
}

func TestSQLRecoveryAnalyticalIntentParametersAndEmptyPopulation(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		params []Parameter
		pass   bool
	}{
		{"ORDER BY 1 DESC LIMIT $1", []Parameter{{Kind: "integer", Value: "3"}}, true},
		{"ORDER BY 1 DESC LIMIT $1", []Parameter{{Kind: "integer", Value: "4"}}, false},
		{"ORDER BY 1 DESC LIMIT $1", []Parameter{{Kind: "text", Value: "3"}}, false},
		{"WHERE active=true ORDER BY 1 DESC LIMIT 3", nil, false},
		{"HAVING sum(amount)>0 ORDER BY 1 DESC LIMIT 3", nil, false},
	} {
		p, c := analyticalFixture(t, "SELECT sum(amount) FROM analytics.sales "+tc.sql, analyticalMetrics(analyticalMeasure("sum", "amount")), tc.params...)
		c.Version = AnalyticalIntentVersion
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: "metric-0", Descending: true, Nulls: "first"}}, Limit: 3}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		if (err == nil) != tc.pass {
			t.Fatalf("%s pass=%v: %v", tc.sql, tc.pass, err)
		}
	}
	p, c := analyticalFixture(t, "SELECT sum(amount) AS amount FROM analytics.sales ORDER BY amount", analyticalMetrics(analyticalMeasure("sum", "amount")))
	c.Version = AnalyticalIntentVersion
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: "metric-0", Nulls: "last"}}}
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("alias must shadow input", err)
	}
	c.Version = AnalyticalGroupingVersion
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
		t.Fatal("old policy accepted new intent", err)
	}
	c.Intent = nil
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("legacy replay changed", err)
	}
}

func TestSQLRecoveryAnalyticalUnaryExpressions(t *testing.T) {
	metric := analyticalMeasure("sum", "amount")
	negated := AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{{Op: "number", Value: "0"}, metric}}
	for _, tc := range []struct {
		sql  string
		expr AnalyticalExpression
		pass bool
	}{
		{"-sum(amount)", negated, true},
		{"0-sum(amount)", negated, true},
		{"+sum(amount)", metric, true},
		{"-sum(amount)", metric, false},
		{"sum(amount)", negated, false},
	} {
		p, c := analyticalFixture(t, "SELECT "+tc.sql+" FROM analytics.sales", analyticalMetrics(tc.expr))
		c.Version = AnalyticalIntentVersion
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		if (err == nil) != tc.pass {
			t.Fatalf("%s: %v", tc.sql, err)
		}
	}
	p, c := analyticalFixture(t, "SELECT -sum(amount) FROM analytics.sales", analyticalMetrics(negated))
	c.Version = AnalyticalGroupingVersion
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalUnsupported) {
		t.Fatal("retained version widened", err)
	}
}

func TestSQLRecoveryAnalyticalLimitWithoutRanking(t *testing.T) {
	for _, tail := range []string{"LIMIT 3", "ORDER BY 1 LIMIT 3"} {
		p, c := analyticalFixture(t, "SELECT name,sum(amount) FROM analytics.sales GROUP BY name "+tail, analyticalMetrics(analyticalMeasure("sum", "amount")))
		c.Version = AnalyticalIntentVersion
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Limit: 3}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal("limit-only must not invent order", err)
		}
	}
}

func TestSQLRecoveryAnalyticalCaseRatioEquivalence(t *testing.T) {
	ratio := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{analyticalMeasure("sum", "amount"), analyticalMeasure("count", "id")}}
	for _, tc := range []struct {
		expression string
		pass       bool
	}{
		{"CASE WHEN count(*)=0 THEN NULL ELSE sum(amount)/count(*) END", true},
		{"CASE WHEN 0=count(*) THEN NULL ELSE sum(amount)/count(*) END", true},
		{"CASE WHEN count(*)<>0 THEN sum(amount)/count(*) END", true},
		{"CASE WHEN count(*)!=0 THEN sum(amount)/count(*) ELSE NULL END", true},
		{"CASE WHEN count(*)=0 THEN 0 ELSE sum(amount)/count(*) END", false},
		{"CASE WHEN count(*)=1 THEN NULL ELSE sum(amount)/count(*) END", false},
		{"CASE WHEN count(amount)=0 THEN NULL ELSE sum(amount)/count(*) END", false},
		{"CASE WHEN count(*)=0 THEN NULL ELSE sum(amount)/count(amount) END", false},
		{"CASE WHEN count(*)<>0 THEN sum(amount)/count(*) ELSE 1 END", false},
		{"CASE WHEN count(*)>0 THEN sum(amount)/count(*) END", false},
	} {
		p, c := analyticalFixture(t, "SELECT "+tc.expression+" FROM analytics.sales", analyticalMetrics(ratio))
		c.Version = AnalyticalIntentVersion
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		if (err == nil) != tc.pass {
			t.Fatalf("%s pass=%v: %v", tc.expression, tc.pass, err)
		}
	}
}

func TestSQLRecoveryAnalyticalCalendarOrdering(t *testing.T) {
	for _, sort := range []string{"bucket", "1", "date_trunc('month',created_at,'America/New_York')"} {
		p, c := analyticalFixture(t, "SELECT date_trunc('month',created_at,'America/New_York') AS bucket,sum(amount) FROM analytics.sales GROUP BY 1 ORDER BY "+sort+" DESC", analyticalMetrics(analyticalMeasure("sum", "amount")))
		for i := range p.candidate.binding.Relations[0].Columns {
			if p.candidate.binding.Relations[0].Columns[i].Name == "created_at" {
				p.candidate.binding.Relations[0].Columns[i] = Column{Name: "created_at", NativeType: "timestamptz", Category: "temporal", Safe: true}
			}
		}
		c.Binding = Hash(p.candidate.binding)
		c.Version = AnalyticalIntentVersion
		bucket := AnalyticalBucket{Column: "created_at", Grain: "month", Calendar: "gregorian", Timezone: "America/New_York"}
		c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Dimensions: []string{"event"}, Buckets: []AnalyticalBucket{bucket}}
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Bucket: &bucket, Descending: true, Nulls: "first"}}}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal(sort, err)
		}
		wrong := bucket
		wrong.Grain = "day"
		c.Intent.Order[0].Bucket = &wrong
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
			t.Fatal("wrong reviewed bucket", err)
		}
	}
}
