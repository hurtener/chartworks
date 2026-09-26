package exec

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestSQLRecoveryParameterScopesPositive(t *testing.T) {
	for _, pair := range [][2]string{
		{`WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id,amount FROM filtered ORDER BY id`, `WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id FROM filtered ORDER BY id DESC`},
		{`WITH x AS (SELECT id,amount FROM analytics.sales WHERE amount>$1), y AS (SELECT id FROM x) SELECT id FROM y`, `WITH x AS (SELECT id,amount FROM analytics.sales WHERE amount>$1), y AS (SELECT id FROM x) SELECT id FROM y LIMIT 2`},
		{`SELECT s.id,s.amount FROM (SELECT id,amount FROM analytics.sales WHERE amount>$1) AS s`, `SELECT s.id FROM (SELECT id,amount FROM analytics.sales WHERE amount>$1) AS s ORDER BY s.id`},
		{`SELECT s.id,s.amount FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id AND t.amount>$1`, `SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id AND t.amount>$1 ORDER BY s.id`},
		{`SELECT s.id FROM analytics.sales s LEFT JOIN analytics.sales t ON s.id=t.id WHERE s.amount>$1`, `SELECT s.id,s.amount FROM analytics.sales s LEFT JOIN analytics.sales t ON s.id=t.id WHERE s.amount>$1 LIMIT 2`},
		{`SELECT id,amount FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1)`, `SELECT id FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1) ORDER BY id DESC`},
		{`SELECT id,amount FROM analytics.sales WHERE EXISTS (SELECT id FROM analytics.sales WHERE amount>$1)`, `SELECT id FROM analytics.sales WHERE EXISTS (SELECT id FROM analytics.sales WHERE amount>$1) ORDER BY id`},
		{`SELECT id,(SELECT max(amount) FROM analytics.sales WHERE amount>$1) AS maximum FROM analytics.sales`, `SELECT id,(SELECT max(amount) FROM analytics.sales WHERE amount>$1) AS maximum FROM analytics.sales ORDER BY id`},
		{`SELECT id,sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) AS rolling FROM analytics.sales`, `SELECT id,sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) AS rolling FROM analytics.sales ORDER BY id DESC`},
		{`SELECT id,sum(amount) OVER w AS rolling FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) ORDER BY id`, `SELECT id,sum(amount) OVER w AS rolling FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) ORDER BY id LIMIT 2`},
		{`SELECT id FROM analytics.sales WHERE id>$1 UNION ALL SELECT id FROM analytics.sales WHERE id<$2`, `SELECT id FROM analytics.sales WHERE id>$1 UNION ALL SELECT id FROM analytics.sales WHERE id<$2 ORDER BY 1 DESC LIMIT 3`},
	} {
		t.Run(pair[0], func(t *testing.T) {
			count := 1
			if strings.Contains(pair[0], "$2") {
				count = 2
			}
			if err := CheckParameterContinuity(context.Background(), "postgres", pair[0], pair[1], count); err != nil {
				t.Fatal("immutable input-scope outer edit rejected", err)
			}
		})
	}
}

func TestSQLRecoveryParameterScopesRejectRebinding(t *testing.T) {
	for _, pair := range [][2]string{
		{`WITH x AS (SELECT id,amount,$1::numeric AS bound FROM analytics.sales) SELECT id FROM x WHERE amount>bound`, `WITH x AS (SELECT id,amount,$1::numeric AS bound FROM analytics.sales) SELECT id FROM x WHERE amount<bound`},
		{`SELECT id FROM (SELECT id,amount,$1::numeric AS bound FROM analytics.sales) x WHERE amount>bound`, `SELECT id FROM (SELECT id,amount,$1::numeric AS bound FROM analytics.sales) x WHERE amount<bound`},
		{`WITH x AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id FROM x`, `WITH x AS (SELECT id,amount FROM analytics.sales WHERE id>$1) SELECT id FROM x`},
		{`WITH x AS (SELECT amount AS value FROM analytics.sales) SELECT value FROM x WHERE value>$1`, `WITH x AS (SELECT id AS value FROM analytics.sales) SELECT value FROM x WHERE value>$1`},
		{`SELECT s.id FROM (SELECT id,amount FROM analytics.sales) s WHERE s.amount>$1`, `SELECT s.id FROM (SELECT id,id AS amount FROM analytics.sales) s WHERE s.amount>$1`},
		{`SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON t.id=s.id WHERE t.amount>$1`, `SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON t.id=s.id AND t.active WHERE t.amount>$1`},
		{`SELECT s.id FROM analytics.sales s LEFT JOIN analytics.sales t ON t.id=s.id AND t.amount>$1`, `SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON t.id=s.id AND t.amount>$1`},
		{`SELECT id FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1)`, `SELECT id FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1 OR true)`},
		{`SELECT sum(amount) OVER w FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW)`, `SELECT avg(amount) OVER w FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW)`},
		{`SELECT sum(amount+$1) OVER w FROM analytics.sales WINDOW w AS (ORDER BY id)`, `SELECT sum(amount+$1) OVER w FROM analytics.sales WINDOW w AS (ORDER BY amount)`},
		{`SELECT sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) FROM analytics.sales`, `SELECT sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 FOLLOWING AND UNBOUNDED FOLLOWING) FROM analytics.sales`},
		{`SELECT id FROM analytics.sales WHERE amount>$1 UNION SELECT id FROM analytics.sales`, `SELECT id FROM analytics.sales WHERE amount>$1 UNION ALL SELECT id FROM analytics.sales`},
		{`WITH x AS (SELECT id FROM analytics.sales WHERE id=$1) SELECT id FROM x`, `WITH x AS (SELECT id FROM analytics.sales WHERE id=$1) SELECT $1 FROM x`},
		{`WITH x AS (SELECT id FROM analytics.sales WHERE amount>$1 AND id=9007199254740993) SELECT id FROM x`, `WITH x AS (SELECT id FROM analytics.sales WHERE amount>$1 AND id=9007199254740992) SELECT id FROM x`},
		{`WITH x AS (SELECT id FROM analytics.sales WHERE id>$1) SELECT id FROM x`, `WITH x AS MATERIALIZED (SELECT id FROM analytics.sales WHERE id>$1) SELECT id FROM x`},
	} {
		t.Run(pair[0], func(t *testing.T) {
			if err := CheckParameterContinuity(context.Background(), "postgres", pair[0], pair[1], 1); !errors.Is(err, ErrBinding) {
				t.Fatal("immutable scope/slot was changed", err)
			}
		})
	}
}

func TestSQLRecoveryParameterOutputNamespaceFences(t *testing.T) {
	for _, pair := range [][2]string{
		// An entire bound ordering clause includes its sibling alias/ordinal keys.
		{`SELECT amount AS key FROM analytics.sales ORDER BY key,$1`, `SELECT id AS key FROM analytics.sales ORDER BY key,$1`},
		{`SELECT amount,id FROM analytics.sales ORDER BY 1,$1`, `SELECT id,amount FROM analytics.sales ORDER BY 1,$1`},
		{`SELECT active,sum(amount) FROM analytics.sales GROUP BY 1,$1`, `SELECT name,sum(amount) FROM analytics.sales GROUP BY 1,$1`},
		{`SELECT id,sum(amount) OVER w AS result FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW)`, `SELECT id,sum(amount) OVER w AS result FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) ORDER BY avg(amount) OVER w`},
		{`SELECT DISTINCT ON (active,$1) id FROM analytics.sales ORDER BY active,id`, `SELECT DISTINCT ON (active,$1) id FROM analytics.sales ORDER BY active,id DESC`},
	} {
		if err := CheckParameterContinuity(context.Background(), "postgres", pair[0], pair[1], 1); !errors.Is(err, ErrBinding) {
			t.Fatal("indirect parameter consumer was rebound", err)
		}
	}
}

func TestSQLRecoveryParameterScopesNativeSafetyIndependent(t *testing.T) {
	// The custody checker does not certify safe new functions or grant access.
	// These edits preserve roles but MUST still fail the ordinary native gate.
	for _, sql := range []string{
		`WITH x AS (SELECT id FROM analytics.sales WHERE id>$1) SELECT pg_read_file('/etc/passwd') FROM x`,
		`SELECT pg_read_file('/etc/passwd') FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id WHERE s.id>$1`,
	} {
		target := "id"
		if strings.Contains(sql, " JOIN ") {
			target = "s.id"
		}
		base := strings.Replace(sql, "pg_read_file('/etc/passwd')", target, 1)
		if err := CheckParameterContinuity(context.Background(), "postgres", base, sql, 1); err != nil {
			t.Fatal("custody checker incorrectly owns new output authority", err)
		}
		if _, _, err := resolveFixture(sql); err == nil {
			t.Fatal("continuity widened native function authority")
		}
	}
	for _, sql := range []string{
		`WITH x AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id FROM x ORDER BY id`,
		`SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id AND t.amount>$1`,
		`SELECT id,sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) AS rolling FROM analytics.sales`,
	} {
		if _, _, err := resolveFixture(sql); err != nil {
			t.Fatal("existing native positive grammar does not accept supported role shape", err)
		}
	}
}

func TestSQLRecoveryParameterScopesNativeConcurrency(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	sql := `WITH x AS (SELECT id FROM analytics.sales WHERE id>$1) SELECT id FROM x`
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := CheckParameterContinuity(ctx, "postgres", sql, sql+" ORDER BY id", 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
