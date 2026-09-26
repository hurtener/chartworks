package exec

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestSQLRecoveryParameterClauseContinuity(t *testing.T) {
	before := `SELECT id,amount FROM analytics.sales WHERE amount > $1 AND name <> $2 ORDER BY id`
	for _, after := range []string{
		`SELECT amount,id FROM analytics.sales WHERE amount > $1 AND name <> $2 ORDER BY amount DESC`,
		`SELECT sum(amount) FROM analytics.sales WHERE amount > $1 AND name <> $2`,
		`SELECT id, amount FROM analytics.sales WHERE amount > $1 AND name <> $2 ORDER BY id LIMIT 5`,
		"SELECT id FROM analytics.sales\nWHERE amount > $1 AND name <> $2 -- trailing comment\n",
	} {
		if err := CheckParameterContinuity(context.Background(), "postgres", before, after, 2); err != nil {
			t.Fatal("unbound projection/order edit rejected", err)
		}
	}
}
func TestSQLRecoveryParameterClauseRejectsReassignment(t *testing.T) {
	before := `SELECT id FROM analytics.sales WHERE amount > $1 AND name <> $2`
	for _, after := range []string{
		`SELECT id FROM analytics.sales WHERE amount > $2 AND name <> $1`,
		`SELECT id FROM analytics.sales WHERE amount < $1 AND name <> $2`,
		`SELECT id FROM analytics.sales WHERE amount > $1 OR name <> $2`,
		`SELECT id FROM analytics.sales WHERE amount > $1 AND name <> $2 AND active`,
		`SELECT id FROM analytics.sales WHERE id > $1 AND name <> $2`,
		`SELECT id FROM analytics.sales WHERE amount > 0 AND name <> $2`,
		`SELECT id FROM analytics.other WHERE amount > $1 AND name <> $2`,
		`SELECT id FROM analytics.sales s WHERE amount > $1 AND name <> $2`,
		`SELECT id FROM analytics.sales WHERE amount > $1 AND name <> $2 AND id>$3`,
	} {
		if err := CheckParameterContinuity(context.Background(), "postgres", before, after, 2); !errors.Is(err, ErrBinding) {
			t.Fatal("changed slot/namespace/clause accepted", after, err)
		}
	}
	// Large exact constants in a protected clause cannot collapse through a
	// float64 JSON intermediary when comparing the native parser's AST.
	big := "SELECT id FROM analytics.sales WHERE amount > $1 AND id=9007199254740993"
	if err := CheckParameterContinuity(context.Background(), "postgres", big, strings.ReplaceAll(big, "9007199254740993", "9007199254740992"), 1); !errors.Is(err, ErrBinding) {
		t.Fatal("large bound-clause constant lost precision", err)
	}

}
func TestSQLRecoveryParameterClauseProtectsEveryLocation(t *testing.T) {
	for _, pair := range [][2]string{
		{`SELECT $1 AS x FROM analytics.sales`, `SELECT $1::integer AS x FROM analytics.sales`},
		{`SELECT sum(amount) FROM analytics.sales HAVING sum(amount)>$1`, `SELECT sum(amount) FROM analytics.sales HAVING sum(amount)>$1 OR true`},
		{`SELECT id FROM analytics.sales LIMIT $1`, `SELECT id FROM analytics.sales OFFSET $1`},
		{`SELECT id FROM analytics.sales ORDER BY id FETCH FIRST $1 ROWS ONLY`, `SELECT id FROM analytics.sales ORDER BY id FETCH FIRST $1 ROWS WITH TIES`},
		{`SELECT id FROM analytics.sales WHERE amount>$1`, `SELECT $1 AS amount FROM analytics.sales`},
	} {
		if err := CheckParameterContinuity(context.Background(), "postgres", pair[0], pair[1], 1); err == nil {
			t.Fatal("parameter-bearing clause was repurposed")
		}
	}
	// It does not mistake parameter-looking literal text or comments for a slot.
	sql := `SELECT '$2' FROM analytics.sales WHERE id=$1 /* $42 */`
	if err := CheckParameterContinuity(context.Background(), "postgres", sql, sql, 1); err != nil {
		t.Fatal("literal marker treated as a binding", err)
	}
}
func TestSQLRecoveryParameterClauseRejectsUnprovedScopes(t *testing.T) {
	// Nonrecursive SELECT scopes moved to the explicit positive/negative scope
	// corpus. Unsafe command/recursive/function sources remain unsupported.
	for _, sql := range []string{
		`WITH RECURSIVE x AS (SELECT id FROM analytics.sales WHERE id=$1) SELECT id FROM x`,
		`WITH x AS (DELETE FROM analytics.sales WHERE id=$1 RETURNING id) SELECT id FROM x`,
		`SELECT value FROM generate_series(1,$1) AS value`,
		`SELECT id INTO copied FROM analytics.sales WHERE id=$1`,
		`SELECT id FROM analytics.sales WHERE id=$1 FOR UPDATE`,
		`DELETE FROM analytics.sales WHERE id=$1`,
		`SELECT id FROM analytics.sales WHERE id=$1; SELECT id FROM analytics.sales`,
	} {
		if err := CheckParameterContinuity(context.Background(), "postgres", sql, sql, 1); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("unproved command/scope admitted: %v", err)
		}
	}
}
func TestSQLRecoveryParameterClauseOtherDialectsConservative(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		sql := "SELECT id FROM sales WHERE id=?"
		if err := CheckParameterContinuity(context.Background(), dialect, sql, sql, 1); err != nil {
			t.Fatal("unchanged SQL rejected", err)
		}
		if err := CheckParameterContinuity(context.Background(), dialect, sql, sql+" ORDER BY id", 1); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unproved dialect edit accepted", err)
		}
	}
}
func TestSQLRecoveryParameterClauseBoundsAndCancellation(t *testing.T) {
	sql := `SELECT id FROM analytics.sales WHERE id=$1`
	for _, count := range []int{0, 65, 2} {
		if err := CheckParameterContinuity(context.Background(), "postgres", sql, sql, count); err == nil {
			t.Fatal("invalid parameter shape")
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 32769), "SELECT 1\x00", string([]byte{0xff})} {
		if err := CheckParameterContinuity(context.Background(), "postgres", sql, bad, 1); err == nil {
			t.Fatal("invalid SQL shape")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckParameterContinuity(ctx, "postgres", sql, sql, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
func TestSQLRecoveryParameterClauseConcurrentReuse(t *testing.T) {
	sql := `SELECT id FROM analytics.sales WHERE id=$1`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := CheckParameterContinuity(context.Background(), "postgres", sql, sql+" ORDER BY id", 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
