package sqlpolicy

import "testing"

func TestFunctionSignaturesCoverVocabulary(t *testing.T) {
	for _, d := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		p, err := ForDialect(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Signatures) != len(p.Functions) {
			t.Fatal("missing signatures", d)
		}
		for _, name := range p.Functions {
			s, ok := FunctionSignature(d, name)
			if !ok || s.Kind == "" || s.Result == "" {
				t.Fatal("incomplete signature", d, name)
			}
			args := make([]string, len(s.Arguments))
			for i := range args {
				args[i] = "unknown"
			}
			if !AllowsCall(d, name, args, false, s.Kind == "window", false, false, false, false) {
				t.Fatal("signature rejects own arity", d, name)
			}
			if !s.Variadic && AllowsCall(d, name, append(args, "unknown"), false, s.Kind == "window", false, false, false, false) {
				t.Fatal("excess arity accepted", d, name)
			}
		}
		for name := range p.Signatures {
			p.Signatures[name] = "corrupted"
			detached, _ := FunctionSignature(d, name)
			if len(detached.Arguments) > 0 {
				detached.Arguments[0] = "corrupted"
			}
		}
		again, _ := ForDialect(d)
		if again.Digest != p.Digest {
			t.Fatal("signature mutation leaked")
		}
	}
}
func TestFunctionSignatureTypesAndSyntax(t *testing.T) {
	for _, tc := range []struct {
		dialect, name                                       string
		args                                                []string
		star, over, distinct, filter, ordered, within, want bool
	}{
		{"postgres", "round", []string{"numeric", "integer"}, false, false, false, false, false, false, true},
		{"postgres", "round", []string{"float", "integer"}, false, false, false, false, false, false, false},
		{"postgres", "round", []string{"numeric", "numeric"}, false, false, false, false, false, false, false},
		{"sqlserver", "round", []string{"numeric"}, false, false, false, false, false, false, false},
		{"sqlserver", "round", []string{"numeric", "integer", "integer"}, false, false, false, false, false, false, true},
		{"postgres", "lower", []string{"integer"}, false, false, false, false, false, false, false},
		{"postgres", "abs", []string{"text"}, false, false, false, false, false, false, false},
		{"postgres", "sum", []string{"interval"}, false, false, false, false, false, false, true},
		{"bigquery", "sum", []string{"interval"}, false, false, false, false, false, false, false},
		{"postgres", "count", nil, true, false, false, false, false, false, true},
		{"postgres", "count", nil, true, false, true, false, false, false, false},
		{"postgres", "sum", nil, true, false, false, false, false, false, false},
		{"postgres", "rank", nil, false, false, false, false, false, false, false},
		{"postgres", "rank", nil, false, true, false, false, false, false, true},
		{"postgres", "lower", []string{"text"}, false, true, false, false, false, false, false},
		{"postgres", "sum", []string{"integer"}, false, true, true, false, false, false, false},
		{"postgres", "lag", []string{"integer", "integer", "text"}, false, true, false, false, false, false, false},
		{"postgres", "lag", []string{"integer", "integer", "unknown"}, false, true, false, false, false, false, true},
	} {
		if got := AllowsCall(tc.dialect, tc.name, tc.args, tc.star, tc.over, tc.distinct, tc.filter, tc.ordered, tc.within); got != tc.want {
			t.Errorf("%s %s %v: got %v", tc.dialect, tc.name, tc.args, got)
		}
	}
}
