package exec

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestReadExactNormalization(t *testing.T) {
	for _, s := range []string{"0", "-0.000", "+12.34", "9007199254740993.125", "1e400", "-1E-400", "1234567890123456789012345678901234567890"} {
		if !Decimal(s) {
			t.Errorf("finite decimal rejected: %s", s)
		}
	}
	for _, s := range []string{"", "+", "-", ".", "1.", ".1", "NaN", "Infinity", "-Infinity", "1e", "1e+", "1e12345", "1 2", strings.Repeat("1", 4097)} {
		if Decimal(s) {
			t.Errorf("invalid decimal accepted: %q", s)
		}
	}
	cases := []struct{ kind, native, value, want string }{
		{"integer", "int8", "9007199254740993", `"9007199254740993"`},
		{"decimal", "numeric", "9007199254740993.125", `"9007199254740993.125"`},
		{"number", "float8", "1.25", `1.25`},
		{"number", "float4", "1.25", `1.25`},
		{"boolean", "bool", "t", `true`}, {"boolean", "bool", "f", `false`},
		{"binary", "bytea", `\xcafe`, `"cafe"`},
		{"structured", "jsonb", `{"n":9007199254740993}`, `"{\"n\":9007199254740993}"`},
		{"temporal", "date", "2026-09-06", `"2026-09-06"`},
		{"text", "text", "<\n>", `"\u003c\n\u003e"`},
	}
	for _, c := range cases {
		got, err := Normalize(Field{Type: c.kind, NativeType: c.native}, []byte(c.value))
		if err != nil || string(got) != c.want {
			t.Errorf("%s normalization: %s %v", c.kind, got, err)
		}
	}
	for _, c := range []struct{ kind, value string }{{"integer", "9223372036854775808"}, {"decimal", "NaN"}, {"number", "NaN"}, {"number", "Infinity"}, {"boolean", "true"}, {"binary", "cafe"}, {"binary", `\xzz`}, {"structured", "{"}, {"temporal", "infinity"}, {"temporal", ""}, {"unknown", "1"}} {
		if _, err := Normalize(Field{Type: c.kind}, []byte(c.value)); err == nil {
			t.Errorf("unsafe normalization %v", c)
		}
	}
	if _, err := Normalize(Field{Type: "text"}, []byte{0xff}); err == nil {
		t.Fatal("invalid UTF8 replaced silently")
	}
	if got, err := Normalize(Field{Type: "boolean"}, nil); err != nil || string(got) != "null" {
		t.Fatal("NULL coerced")
	}
}
func TestReadCollectorBounds(t *testing.T) {
	schema := []Field{{Name: "value", Type: "text", Encoding: "string", NativeType: "text"}}
	c, err := NewCollector(schema, 1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if more, err := c.Add([][]byte{[]byte("<\n>é")}); err != nil || !more {
		t.Fatal(err)
	}
	if more, err := c.Add([][]byte{[]byte("second")}); err != nil || more || c.Result().Truncation != "rows" {
		t.Fatal("missing lookahead truncation")
	}
	r := c.Result()
	s, _ := json.Marshal(r.Schema)
	rows, _ := json.Marshal(r.Rows)
	if r.Bytes != len(s)+len(rows) {
		t.Fatal("serialized byte accounting")
	}
	c, err = NewCollector(schema, 10, 128)
	if err != nil {
		t.Fatal(err)
	}
	if more, err := c.Add([][]byte{[]byte(strings.Repeat("\n", 100))}); err != nil || more || c.Result().Truncation != "bytes" {
		t.Fatal("escaped-byte budget")
	}
	if _, err = c.Add(nil); err == nil {
		t.Fatal("wrong result width")
	}
	for _, s := range [][]Field{nil, {{Name: "", Type: "text", Encoding: "string", NativeType: "text"}}, {{Name: "x", Type: "unknown", Encoding: "string", NativeType: "custom"}}, {{Name: "x", Type: "text", Encoding: "number", NativeType: "text"}}, {{Name: "x", Type: "boolean", Encoding: "string", NativeType: "bool"}}} {
		if _, err := NewCollector(s, 1, 1024); err == nil {
			t.Fatal("invalid schema")
		}
	}
	if _, err = NewCollector(schema, 0, 1024); err == nil {
		t.Fatal("zero rows")
	}
	if _, err = NewCollector(schema, 1, 127); err == nil {
		t.Fatal("byte lower bound")
	}
	long := append([]Field(nil), schema...)
	long[0].Name = strings.Repeat("x", 200)
	if _, err = NewCollector(long, 1, 128); err == nil {
		t.Fatal("schema exceeded result budget")
	}
}
func TestReadLimitsAndQueryIdentity(t *testing.T) {
	l := Limits{Rows: 1, Bytes: 128, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1}
	if !l.Valid() {
		t.Fatal("valid bounds")
	}
	for _, change := range []func(*Limits){func(l *Limits) { l.Rows = 100001 }, func(l *Limits) { l.Bytes = 1 }, func(l *Limits) { l.Timeout = 0 }, func(l *Limits) { l.CancelGrace = 4 * time.Second }, func(l *Limits) { l.PlannerCost = math.NaN() }, func(l *Limits) { l.PlannerCost = math.Inf(1) }} {
		b := l
		change(&b)
		if b.Valid() {
			t.Fatal("invalid limits accepted")
		}
	}
	q := RemoteQuery{PID: 1, Started: time.Now(), Tag: "cw-read:" + strings.Repeat("a", 32)}
	if !q.Valid() {
		t.Fatal("valid query identity")
	}
	for _, tag := range []string{"", strings.Repeat("a", 40), "cw-read:" + strings.Repeat("G", 32), "cw-read:" + strings.Repeat("A", 32)} {
		q.Tag = tag
		if q.Valid() {
			t.Fatal("noncanonical query identity")
		}
	}
}
