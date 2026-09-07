package exec

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSQLBindingAndParameterBoundaries(t *testing.T) {
	b := parserBinding()
	for _, mutate := range []func(*Binding){
		func(b *Binding) { b.Tenant = "" }, func(b *Binding) { b.Source = "bad/" }, func(b *Binding) { b.Context = "" }, func(b *Binding) { b.Catalog = "server.database" }, func(b *Binding) { b.Contract = "" }, func(b *Binding) { b.Revision = 0 }, func(b *Binding) { b.Fingerprint = "short" }, func(b *Binding) { b.Fingerprint = strings.Repeat("z", 64) }, func(b *Binding) { b.Relations = nil }, func(b *Binding) { b.Relations = append(b.Relations, b.Relations[0]) }, func(b *Binding) { b.Relations[0].ID = "bad/" }, func(b *Binding) { b.Relations[0].Schema = "1bad" }, func(b *Binding) { b.Relations[0].Name = "" }, func(b *Binding) { b.Relations[0].Columns = nil }, func(b *Binding) { b.Relations[0].Columns = append(b.Relations[0].Columns, b.Relations[0].Columns[0]) }, func(b *Binding) { b.Relations[0].Columns[0].Name = strings.Repeat("x", 64) }, func(b *Binding) { b.Relations[0].Columns[0].NativeType = "" },
	} {
		changed := b.Clone()
		mutate(&changed)
		if changed.Valid() {
			t.Fatal("malformed resolved binding accepted")
		}
		if !b.Valid() {
			t.Fatal("cloned binding mutated original")
		}
	}
	for _, p := range []Parameter{{"null", ""}, {"text", "a'"}, {"boolean", "true"}, {"boolean", "false"}, {"integer", "-9223372036854775808"}, {"number", "9007199254740993.125"}} {
		if !p.Valid() {
			t.Fatal("valid typed value rejected", p.Kind)
		}
	}
	for _, p := range []Parameter{{"null", "null"}, {"text", "a\x00b"}, {"text", strings.Repeat("x", 4097)}, {"boolean", "TRUE"}, {"integer", "9223372036854775808"}, {"number", "NaN"}, {"number", "1e9999"}, {"number", "+1"}, {"shell", "value"}} {
		if p.Valid() {
			t.Fatal("ambiguous parameter accepted", p.Kind)
		}
	}
	// Internal test payloads establish that every formatting/serialization path
	// redacts sensitive query text, including detailed Go formatting.
	c := Candidate{statement: "PRIVATE_QUERY_CANARY", parameters: []Parameter{{"text", "PRIVATE_VALUE_CANARY"}}}
	p := Plan{candidate: c}
	for _, value := range []any{p, c} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded)+fmt.Sprintf("%v %#v", value, value), "PRIVATE_") {
			t.Fatal("opaque plan or candidate leaked query input", err)
		}
	}
	if _, _, err := p.SQL(p.candidate.owner, b); err == nil {
		t.Fatal("zero native proof admitted")
	}
}
