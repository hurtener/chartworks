package api

import (
	"reflect"
	"testing"
)

// An optional closed union member is not permission to make required scalars,
// the request root, or arbitrary map content nullable/untyped.
func TestNullableCollectionsAdmitsClosedOptionalPointersOnly(t *testing.T) {
	type member struct {
		Value string `json:"value"`
	}
	type request struct {
		Version int      `json:"version"`
		Choice  *member  `json:"choice"`
		Values  []string `json:"values"`
	}
	schema, err := SchemaFor("optionalPointer", reflect.TypeFor[request](), false, NullableCollections)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"version":1,"choice":null,"values":null}`,
		`{"version":1,"choice":{"value":"exact"},"values":[]}`,
	} {
		if err := schema.Validate([]byte(raw), 4096); err != nil {
			t.Fatalf("valid optional pointer %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		`null`, `{}`, `{"version":null,"choice":null,"values":null}`,
		`{"choice":null,"values":null}`,
		`{"version":1,"choice":{"value":null},"values":[]}`,
		`{"version":1,"choice":{"value":"exact","tenant":"injected"},"values":[]}`,
		`{"version":1,"version":2,"choice":null,"values":[]}`,
		`{"version":1,"choice":null,"values":[null]}`,
	} {
		if schema.Validate([]byte(raw), 4096) == nil {
			t.Fatalf("open/nullable scalar admitted: %s", raw)
		}
	}
	type unbounded struct {
		Options *map[string]string `json:"options"`
	}
	if _, err := SchemaFor("unboundedPointer", reflect.TypeFor[unbounded](), false, NullableCollections); err == nil {
		t.Fatal("pointer enabled arbitrary request maps")
	}
}

func TestNullableCollectionsInEmbeddedDTO(t *testing.T) {
	type Base struct {
		Version   int      `json:"version"`
		Arguments []string `json:"arguments"`
	}
	type request struct {
		Base
		Outputs []string `json:"outputs"`
	}
	schema, err := SchemaFor("embeddedRequest", reflect.TypeFor[request](), false, NullableCollections)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":1,"arguments":null,"outputs":null}`, `{"version":1,"arguments":[],"outputs":[]}`} {
		if err := schema.Validate([]byte(raw), 4096); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`{"version":null,"arguments":null,"outputs":null}`, `{"arguments":null,"outputs":null}`, `{"version":1,"arguments":[null],"outputs":[]}`} {
		if schema.Validate([]byte(raw), 4096) == nil {
			t.Fatal("embedded DTO opened scalar schema", raw)
		}
	}
}
