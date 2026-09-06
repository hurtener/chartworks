package exec

import (
	"encoding/json"
	"testing"
)

func FuzzReadExactValues(f *testing.F) {
	for _, s := range []string{"9007199254740993.125", "-9223372036854775808", "null", "NaN", "1e9999", "<\n>"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			return
		}
		for _, kind := range []string{"decimal", "integer", "number", "boolean", "text", "structured"} {
			raw, err := Normalize(Field{Type: kind, NativeType: "float8"}, []byte(s))
			if err != nil {
				continue
			}
			if !json.Valid(raw) {
				t.Fatal("normalizer emitted invalid JSON")
			}
			if kind == "decimal" || kind == "integer" {
				var restored string
				if json.Unmarshal(raw, &restored) != nil || restored != s {
					t.Fatal("exact value changed")
				}
			}
		}
	})
}
