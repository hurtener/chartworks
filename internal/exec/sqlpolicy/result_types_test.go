package sqlpolicy

import "testing"

func TestFunctionResultTypeDialectMatrix(t *testing.T) {
	for _, tc := range []struct{ d, n, input, want string }{
		{"postgres", "avg", "integer", "numeric"}, {"mysql", "avg", "integer", "numeric"}, {"sqlserver", "avg", "integer", "integer"}, {"bigquery", "avg", "integer", "float"}, {"databricks", "avg", "integer", "float"}, {"snowflake", "avg", "integer", "numeric"},
		{"postgres", "sum", "integer", "unknown"}, {"mysql", "sum", "integer", "numeric"}, {"bigquery", "sum", "integer", "integer"}, {"bigquery", "sum", "interval", "interval"}, {"databricks", "avg", "interval", "interval"},
		{"mysql", "round", "integer", "integer"}, {"bigquery", "round", "integer", "float"}, {"postgres", "round", "integer", "numeric"}, {"postgres", "round", "float", "float"}, {"postgres", "ceil", "integer", "numeric"}, {"mysql", "abs", "text", "unknown"},
	} {
		if got := FunctionResultType(tc.d, tc.n, []string{tc.input}); got != tc.want {
			t.Errorf("%s %s(%s)=%s want %s", tc.d, tc.n, tc.input, got, tc.want)
		}
	}
	if got := FunctionResultType("bigquery", "coalesce", []string{"integer", "numeric"}); got != "numeric" {
		t.Fatal("common numeric promotion lost", got)
	}
	if got := FunctionResultType("postgres", "lag", []string{"integer", "integer", "float"}); got != "float" {
		t.Fatal("window default promotion lost", got)
	}
	if got := FunctionResultType("bigquery", "coalesce", []string{"unknown", "numeric"}); got != "unknown" {
		t.Fatal("unknown type promoted", got)
	}
	s, _ := FunctionSignature("bigquery", "avg")
	s.ResultByInput["integer"] = "text"
	again, _ := FunctionSignature("bigquery", "avg")
	if again.ResultByInput["integer"] != "float" {
		t.Fatal("result matrix mutation leaked")
	}
}
