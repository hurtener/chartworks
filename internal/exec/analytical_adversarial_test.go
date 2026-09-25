package exec

import (
	"context"
	"errors"
	"testing"
)

func TestSQLRecoveryAdversarialOnlyCannotChangePopulation(t *testing.T) {
	for _, from := range []string{"ONLY analytics.sales", "ONLY (analytics.sales)", "ONLY analytics.sales AS s"} {
		p, c := analyticalFixture(t, "SELECT sum(amount) FROM "+from, analyticalMetrics(analyticalMeasure("sum", "amount")))
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || !errors.Is(err, ErrAnalyticalMismatch) {
			t.Fatal("physical-parent-only scan acquired a complete relation proof", err)
		}
	}
	for _, from := range []string{"analytics.sales", "analytics.sales *", "analytics.sales * AS s"} {
		p, c := analyticalFixture(t, "SELECT sum(amount) FROM "+from, analyticalMetrics(analyticalMeasure("sum", "amount")))
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof == nil || err != nil {
			t.Fatal("normal descendant-inclusive relation rejected", err)
		}
	}
}

func TestSQLRecoveryAdversarialUnknownLiteralCannotProveDecimalDivision(t *testing.T) {
	expr := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), {Op: "number", Value: "2"}}}
	for _, sql := range []string{
		`SELECT count(id)/'2' FROM analytics.sales`,
		`SELECT (count(id)/'2')::numeric FROM analytics.sales`,
		`SELECT count(id)/2 FROM analytics.sales`,
		`SELECT (count(id)/2)::numeric FROM analytics.sales`,
	} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(expr))
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || err == nil {
			t.Fatal("truncated integer result acquired a decimal-ratio proof", sql)
		}
	}
	for _, sql := range []string{
		`SELECT count(id)/2.0 FROM analytics.sales`,
		`SELECT count(id)::numeric/2 FROM analytics.sales`,
		`SELECT count(id)/2::numeric FROM analytics.sales`,
	} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(expr))
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof == nil || err != nil {
			t.Fatal("explicit numeric division rejected", sql, err)
		}
	}
	// Quoted constants are still valid typed comparisons in metric filters;
	// hardening arithmetic must not change their separate column-directed proof.
	p, c := analyticalFixture(t, `SELECT sum(amount) FILTER (WHERE id='2') FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "id", Kind: "eq", Values: []string{"2"}})))
	if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof == nil || err != nil {
		t.Fatal("typed filter literal regressed", err)
	}
}

func TestSQLRecoveryAdversarialBigintParserStorageIsNotNumericType(t *testing.T) {
	for _, divisor := range []string{"2147483648", "9007199254740993", "9223372036854775807"} {
		expr := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), {Op: "number", Value: divisor}}}
		for _, statement := range []string{"SELECT count(id)/" + divisor + " FROM analytics.sales", "SELECT (count(id)/" + divisor + ")::numeric FROM analytics.sales"} {
			p, c := analyticalFixture(t, statement, analyticalMetrics(expr))
			if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || !errors.Is(err, ErrAnalyticalMismatch) {
				t.Fatal("bigint fval acquired a decimal-division proof", divisor, err)
			}
		}
		for _, statement := range []string{"SELECT count(id)/" + divisor + ".0 FROM analytics.sales", "SELECT count(id)::numeric/" + divisor + " FROM analytics.sales"} {
			p, c := analyticalFixture(t, statement, analyticalMetrics(expr))
			if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof == nil || err != nil {
				t.Fatal("explicit decimal widening rejected", divisor, err)
			}
		}
	}
	// A value beyond the signed bigint range really is a numeric literal.
	expr := AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), {Op: "number", Value: "9223372036854775808"}}}
	p, c := analyticalFixture(t, `SELECT count(id)/9223372036854775808 FROM analytics.sales`, analyticalMetrics(expr))
	if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof == nil || err != nil {
		t.Fatal("true numeric literal rejected", err)
	}
}
