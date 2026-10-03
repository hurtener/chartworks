package exec

import (
	"context"
	"errors"
	"testing"
)

func TestSQLRecoveryLearningDoesNotPublishCopiedBindingLiterals(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		values []Parameter
	}{
		{`SELECT amount FROM analytics.sales WHERE amount>$1 AND amount=.5`, []Parameter{{Kind: "number", Value: "0.5"}}},
		{`SELECT amount FROM analytics.sales WHERE amount>$1 AND amount=1e2`, []Parameter{{Kind: "number", Value: "100"}}},
		{`SELECT amount FROM analytics.sales WHERE amount>$1`, []Parameter{{Kind: "number", Value: "1e2"}}},
		{`SELECT id AS "customer-731" FROM analytics.sales WHERE name=$1`, []Parameter{{Kind: "text", Value: "customer-731"}}},
		{`SELECT id AS customer731 FROM analytics.sales WHERE name=$1`, []Parameter{{Kind: "text", Value: "customer731"}}},
		{`SELECT id FROM analytics.sales WHERE name=$1 AND other='customer-731'`, []Parameter{{Kind: "text", Value: "customer-731"}}},
		{`SELECT id FROM analytics.sales WHERE id=$1 /* customer-731 */`, []Parameter{{Kind: "integer", Value: "1"}}},
		{`SELECT id FROM analytics.sales WHERE id=$1 -- any unreviewed comment`, []Parameter{{Kind: "integer", Value: "1"}}},
		{`SELECT id FROM analytics.sales WHERE amount>$1 AND amount<5.250`, []Parameter{{Kind: "number", Value: "5.25"}}},
		{`SELECT id FROM analytics.sales WHERE amount>$1 AND amount=-5.25`, []Parameter{{Kind: "number", Value: "5.250"}}},
		{`SELECT id FROM analytics.sales WHERE active=$1 AND active=true`, []Parameter{{Kind: "boolean", Value: "true"}}},
		{`SELECT id FROM analytics.sales WHERE name=$1 AND name='prefix CUSTOMER  NAME suffix'`, []Parameter{{Kind: "text", Value: "customer name"}}},
	} {
		if err := CheckLearningParameterContent(context.Background(), tc.sql, tc.values); !errors.Is(err, ErrUnsupported) {
			t.Fatal("known binding or unsupported annotation eligible", err)
		}
	}
}
func TestSQLRecoveryLearningMarkersAreNotBindings(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		values []Parameter
	}{
		{`SELECT id FROM analytics.sales WHERE id=$1`, []Parameter{{Kind: "integer", Value: "1"}}},
		{`SELECT amount FROM analytics.sales WHERE amount>$1 AND active=true`, []Parameter{{Kind: "number", Value: "5.25"}}},
		{`SELECT id FROM analytics.sales WHERE name=$1 AND active=true`, []Parameter{{Kind: "text", Value: "customer-731"}}},
		{`SELECT id FROM analytics.sales WHERE name=@p1`, []Parameter{{Kind: "text", Value: "customer-731"}}},
		{`SELECT id FROM analytics.sales WHERE name=?`, []Parameter{{Kind: "text", Value: "customer-731"}}},
	} {
		if err := CheckLearningParameterContent(context.Background(), tc.sql, tc.values); err != nil {
			t.Fatal("safe templating subset rejected", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckLearningParameterContent(ctx, `SELECT id WHERE id=$1`, []Parameter{{Kind: "integer", Value: "1"}}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
}
