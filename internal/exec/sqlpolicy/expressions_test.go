package sqlpolicy

import "testing"

func TestPostgresSpecialExpressionSignatures(t *testing.T) {
	for _, name := range []string{"coalesce", "greatest", "least", "nullif"} {
		if AllowsFunction("postgres", []string{name}) {
			t.Fatal("special expression granted ordinary call permission", name)
		}
		if _, ok := ExpressionSignature("mysql", name); ok {
			t.Fatal("wrong dialect expression admitted")
		}
		if !AllowsExpression("postgres", name, []string{"integer", "numeric"}) {
			t.Fatal("common numeric type rejected", name)
		}
		if AllowsExpression("postgres", name, nil) {
			t.Fatal("missing arguments admitted", name)
		}
		if AllowsExpression("postgres", name, []string{"integer", "boolean"}) {
			t.Fatal("incompatible primitive types admitted", name)
		}
		s, _ := ExpressionSignature("postgres", name)
		s.Arguments[0] = "changed"
		again, _ := ExpressionSignature("postgres", name)
		if again.Arguments[0] != "any" {
			t.Fatal("mutation leaked")
		}
	}
	for _, name := range []string{"coalesce", "greatest", "least"} {
		if !AllowsExpression("postgres", name, []string{"unknown", "integer", "numeric"}) {
			t.Fatal("unknown literal common type rejected", name)
		}
		if AllowsExpression("postgres", name, []string{"unknown", "integer", "text"}) {
			t.Fatal("NULL concealed incompatible known types", name)
		}
	}
	if AllowsExpression("postgres", "nullif", []string{"integer"}) || AllowsExpression("postgres", "nullif", []string{"integer", "integer", "integer"}) {
		t.Fatal("NULLIF arity mismatch")
	}
	if AllowsExpression("postgres", "unknown", []string{"integer"}) {
		t.Fatal("unknown expression admitted")
	}
}
