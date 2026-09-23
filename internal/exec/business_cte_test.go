package exec

import (
	"context"
	"errors"
	"strings"
	"testing"

	pgquery "github.com/wasilibs/go-pgquery"
)

func TestBusinessCTEBindsBeforeAggregate(t *testing.T) {
	b := parserBinding()
	c := businessFixtureConstraint()
	sql := `WITH paid AS (SELECT s.id, s.amount FROM analytics.sales AS s WHERE s.active = true), item_totals AS (SELECT i.sale_id, SUM(i.quantity) AS quantity FROM analytics.items AS i GROUP BY i.sale_id) SELECT SUM(p.amount) - COALESCE(SUM(t.quantity),0) FROM paid p LEFT JOIN item_totals t ON t.sale_id = p.id`
	out, err := BindBusinessConstraints(context.Background(), b, sql, nil, []BusinessConstraint{c})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.SQL, `WHERE (s.active = true) AND (("s"."amount" >= CAST($1 AS DECIMAL(38,6))))`) || len(out.Parameters) != 1 || out.Parameters[0].Value != c.Value {
		t.Fatal("row constraint missing from base relation before aggregation")
	}
	if _, err := pgquery.ParseToJSON(out.SQL); err != nil {
		t.Fatal("bound SQL did not parse", err)
	}
}

func TestBusinessCTEAllowsSingleAcyclicDependency(t *testing.T) {
	c := businessFixtureConstraint()
	sql := `WITH paid AS (SELECT s.id, s.amount FROM analytics.sales s), totals AS (SELECT SUM(p.amount) AS amount FROM paid p) SELECT amount FROM totals`
	out, err := BindBusinessConstraints(context.Background(), parserBinding(), sql, nil, []BusinessConstraint{c})
	if err != nil || !strings.Contains(out.SQL, `FROM analytics.sales s WHERE ("s"."amount" >=`) {
		t.Fatal("bounded CTE dependency lost base-row predicate", err)
	}
	if _, err := pgquery.ParseToJSON(out.SQL); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessCTEAllowsFilteredInputReusedAtSeparateGrains(t *testing.T) {
	c := businessFixtureConstraint()
	sql := `WITH paid AS (SELECT s.id, s.amount FROM analytics.sales s), item_totals AS (SELECT i.sale_id, SUM(i.quantity) AS quantity FROM analytics.items i JOIN paid p ON p.id=i.sale_id GROUP BY i.sale_id) SELECT SUM(p.amount) - COALESCE(SUM(t.quantity),0) FROM paid p LEFT JOIN item_totals t ON t.sale_id=p.id`
	out, err := BindBusinessConstraints(context.Background(), parserBinding(), sql, nil, []BusinessConstraint{c})
	if err != nil || !strings.Contains(out.SQL, `FROM analytics.sales s WHERE ("s"."amount" >=`) {
		t.Fatal("shared filtered CTE did not bind before both consumers", err)
	}
}

func TestBusinessCTERejectsAmbiguousOrUnreachableTargets(t *testing.T) {
	c := businessFixtureConstraint()
	for _, sql := range []string{
		`WITH p AS (SELECT id FROM analytics.sales), q AS (SELECT id FROM analytics.sales) SELECT * FROM p JOIN q ON p.id=q.id`,
		`WITH p AS (SELECT id FROM analytics.sales), q AS (SELECT sale_id FROM analytics.items) SELECT * FROM q`,
		`WITH p AS (SELECT id FROM analytics.sales) SELECT * FROM (SELECT * FROM p) x`,
		`WITH p AS (SELECT id FROM analytics.sales UNION SELECT id FROM analytics.sales) SELECT * FROM p`,
		`WITH RECURSIVE p AS (SELECT id FROM analytics.sales) SELECT * FROM p`,
		`WITH p AS (SELECT id FROM analytics.sales) SELECT * FROM p JOIN p x ON p.id=x.id`,
		`WITH p AS (SELECT id FROM analytics.sales) SELECT * FROM p WHERE EXISTS (SELECT 1)`,
	} {
		out, err := BindBusinessConstraints(context.Background(), parserBinding(), sql, nil, []BusinessConstraint{c})
		if !errors.Is(err, ErrUnsupported) || out.SQL != "" || len(out.Parameters) != 0 {
			t.Errorf("admitted ambiguous CTE or returned partial result: %q: %v", sql, err)
		}
	}
	b := parserBinding()
	b.Dialect = "mysql"
	if _, err := BindBusinessConstraints(context.Background(), b, `WITH p AS (SELECT id FROM analytics.sales) SELECT * FROM p`, nil, []BusinessConstraint{c}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unqualified dialect received CTE expansion", err)
	}
	shadow := parserBinding()
	shadow.Relations[0].ID = "__cte_paid"
	shadowConstraint := businessFixtureConstraint()
	shadowConstraint.Dataset = "__cte_paid"
	if out, err := BindBusinessConstraints(context.Background(), shadow, `WITH paid AS (SELECT sale_id FROM analytics.items), totals AS (SELECT sale_id FROM paid) SELECT sale_id FROM totals`, nil, []BusinessConstraint{shadowConstraint}); err == nil || out.SQL != "" {
		t.Fatal("virtual CTE name impersonated reviewed base relation")
	}
}
