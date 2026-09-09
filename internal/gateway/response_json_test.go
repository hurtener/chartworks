package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseCollectionLimitDoesNotWidenRequests(t *testing.T) {
	schema, err := NewSchema("boundedRows", []byte(`{"type":"array","items":{"type":"integer"}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{65536, 65537, MaxResponseItems, MaxResponseItems + 1} {
		data := []byte("[" + strings.Repeat("1,", count-1) + "1]")
		if (schema.Validate(data, len(data)) == nil) != (count <= 65536) {
			t.Fatalf("request bound changed at %d", count)
		}
		if (schema.ValidateResponse(data, len(data)) == nil) != (count <= MaxResponseItems) {
			t.Fatalf("result bound incorrect at %d", count)
		}
	}
	for _, data := range []string{`{"id":1,"id":2}`, `[1] [2]`, `[1e999]`, strings.Repeat("[", 34) + "1" + strings.Repeat("]", 34)} {
		if _, err := DecodeResponseJSON([]byte(data), 1<<20); err == nil {
			t.Fatal("response parsing weakened")
		}
	}
	if _, err := DecodeResponseJSON([]byte(`[1,2]`), 4); err == nil {
		t.Fatal("byte bound widened")
	}
	if (*Schema)(nil).ValidateResponse([]byte(`[]`), 2) == nil {
		t.Fatal("missing compiled schema accepted")
	}
	if (&Schema{}).ValidateResponse([]byte(`[]`), 2) == nil {
		t.Fatal("zero compiled schema accepted")
	}
	if schema.ValidateResponse([]byte(`["wrong"]`), 100) == nil {
		t.Fatal("output schema ignored")
	}
	value, err := DecodeResponseJSON([]byte(`[9007199254740993]`), 100)
	if err != nil || value.([]any)[0].(json.Number).String() != "9007199254740993" {
		t.Fatal("exact number coerced", err)
	}
}
