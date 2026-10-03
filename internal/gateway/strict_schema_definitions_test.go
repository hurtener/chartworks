package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestStrictScalarDefinitionsAreExactStableAndSmaller(t *testing.T) {
	properties := map[string]any{}
	required := []string{}
	value := map[string]any{}
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("field_%02d", i)
		properties[name] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "description": "Synthetic bounded identifier"}
		required = append(required, name)
		value[name] = "value"
	}
	root := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	before, _ := json.Marshal(root)
	factored := strictDefinitions(root)
	after, _ := json.Marshal(factored)
	unchanged, _ := json.Marshal(root)
	if !bytes.Equal(before, unchanged) {
		t.Fatal("factoring mutated the projection")
	}
	original := strictTestSchema(t, string(before))
	wire := strictTestSchema(t, string(after))
	raw, _ := json.Marshal(value)
	if original.Validate(raw, 4096) != nil || wire.Validate(raw, 4096) != nil {
		t.Fatal("valid value lost")
	}
	value["field_00"] = ""
	raw, _ = json.Marshal(value)
	if original.Validate(raw, 4096) == nil || wire.Validate(raw, 4096) == nil {
		t.Fatal("scalar constraint weakened")
	}
	value["field_00"] = nil
	raw, _ = json.Marshal(value)
	if wire.Validate(raw, 4096) == nil {
		t.Fatal("non-null value widened")
	}
	delete(value, "field_00")
	raw, _ = json.Marshal(value)
	if wire.Validate(raw, 4096) == nil {
		t.Fatal("required value lost")
	}
	indentedBefore, _ := json.MarshalIndent(root, "      ", "  ")
	indentedAfter, _ := json.MarshalIndent(factored, "      ", "  ")
	if len(indentedAfter) >= len(indentedBefore) || factored["$defs"] == nil {
		t.Fatal("factoring did not reduce actual framing")
	}
	for i := 0; i < 20; i++ {
		again, _ := json.Marshal(strictDefinitions(root))
		if !bytes.Equal(after, again) {
			t.Fatal("unstable definition identities")
		}
	}
	t.Logf("synthetic repeated-scalar schema framing reduced from %d to %d bytes", len(indentedBefore), len(indentedAfter))
}
func TestStrictDefinitionsLeaveUnprofitableSchemasUnchanged(t *testing.T) {
	root := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"items"}, "properties": map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
	before, _ := json.Marshal(root)
	after, _ := json.Marshal(strictDefinitions(root))
	if !bytes.Equal(before, after) {
		t.Fatal("unprofitable structured schema changed")
	}
}

func TestStrictFactoredAndUnfactoredNormalizationEquivalence(t *testing.T) {
	properties := map[string]any{}
	required := []string{}
	value := map[string]any{}
	for i := 0; i < 4; i++ {
		suffix := fmt.Sprint(i)
		properties["maybe"+suffix] = map[string]any{"enum": []any{"left", "right", nil}, "description": "Explicit null is a distinct domain value"}
		properties["text"+suffix] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[a-z]+$"}
		properties["number"+suffix] = map[string]any{"type": "integer", "enum": []any{json.Number("9007199254740993")}, "minimum": json.Number("9007199254740993"), "maximum": json.Number("9007199254740993")}
		required = append(required, "text"+suffix, "number"+suffix)
		value["maybe"+suffix] = map[string]any{"value": nil}
		value["text"+suffix] = "valid"
		value["number"+suffix] = json.Number("9007199254740993")
	}
	document, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required})
	domain := strictTestSchema(t, string(document))
	decoded, _ := DecodeJSON(document, 64<<10)
	node, projected, err := projectStrict(decoded.(map[string]any), 0, &strictLimits{})
	if err != nil {
		t.Fatal(err)
	}
	inlineRaw, _ := json.Marshal(projected)
	inline := &StrictSchema{domain: domain, root: node, wire: strictTestSchema(t, string(inlineRaw))}
	factored, err := NewStrictSchema(domain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(factored.Document(), []byte(`"$defs"`)) {
		t.Fatal("equivalence fixture not factored")
	}
	baseline, _ := json.Marshal(value)
	mutations := []func(map[string]any){
		func(v map[string]any) {},
		func(v map[string]any) { v["maybe0"] = nil },
		func(v map[string]any) { v["maybe0"] = map[string]any{"value": "left"} },
		func(v map[string]any) { v["maybe0"] = map[string]any{"value": "foreign"} },
		func(v map[string]any) { v["text0"] = "UPPER" },
		func(v map[string]any) { v["text0"] = "" },
		func(v map[string]any) { v["number0"] = json.Number("9007199254740992") },
		func(v map[string]any) { delete(v, "text0") },
	}
	for i, mutate := range mutations {
		decoded, _ := DecodeJSON(baseline, 64<<10)
		v := decoded.(map[string]any)
		mutate(v)
		raw, _ := json.Marshal(v)
		a, ae := inline.Normalize(raw, 64<<10)
		b, be := factored.Normalize(raw, 64<<10)
		if (ae == nil) != (be == nil) || !bytes.Equal(a, b) {
			t.Fatal("factoring changed acceptance/normalization", i, ae, be)
		}
		if (i < 3) != (be == nil) {
			t.Fatal("fixture unexpectedly accepted or rejected", i, be)
		}
	}
}

func TestStrictClosedObjectDefinitionsPreserveSubtreeConstraints(t *testing.T) {
	endpoint := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "detail", "tags"}, "properties": map[string]any{
		"id":     map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
		"label":  map[string]any{"enum": []any{"left", "right"}},
		"detail": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"code"}, "properties": map[string]any{"code": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}},
		"tags":   map[string]any{"type": "array", "uniqueItems": true, "maxItems": 3, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}},
	}}
	document, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"a", "b", "c"}, "properties": map[string]any{"a": endpoint, "b": endpoint, "c": endpoint}})
	domain := strictTestSchema(t, string(document))
	decoded, _ := DecodeJSON(document, 64<<10)
	node, projected, err := projectStrict(decoded.(map[string]any), 0, &strictLimits{})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(projected)
	inline := &StrictSchema{domain: domain, root: node, wire: strictTestSchema(t, string(before))}
	factored, err := NewStrictSchema(domain)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ = DecodeJSON(factored.Document(), 64<<10)
	defs := decoded.(map[string]any)["$defs"].(map[string]any)
	if len(defs) != 1 {
		t.Fatal("parent identity lost or unused child definitions retained", len(defs))
	}
	for _, definition := range defs {
		encoded, _ := json.Marshal(definition)
		if bytes.Contains(encoded, []byte(`"$ref"`)) {
			t.Fatal("generated definition is not fully inlined")
		}
	}
	value := map[string]any{}
	for _, key := range []string{"a", "b", "c"} {
		value[key] = map[string]any{"id": "synthetic", "detail": map[string]any{"code": "code"}, "label": nil, "tags": []any{"one", "two"}}
	}
	baseline, _ := json.Marshal(value)
	mutations := []func(map[string]any){
		func(v map[string]any) {},
		func(v map[string]any) { v["a"].(map[string]any)["label"] = "left" },
		func(v map[string]any) { delete(v["a"].(map[string]any)["detail"].(map[string]any), "code") },
		func(v map[string]any) { v["a"].(map[string]any)["id"] = "" },
		func(v map[string]any) { v["a"].(map[string]any)["label"] = "foreign" },
		func(v map[string]any) { v["a"].(map[string]any)["foreign"] = true },
		func(v map[string]any) { v["a"].(map[string]any)["tags"] = []any{"same", "same"} },
		func(v map[string]any) { v["a"] = nil },
	}
	for i, mutate := range mutations {
		decoded, _ := DecodeJSON(baseline, 64<<10)
		value := decoded.(map[string]any)
		mutate(value)
		raw, _ := json.Marshal(value)
		a, ae := inline.Normalize(raw, 64<<10)
		b, be := factored.Normalize(raw, 64<<10)
		if (ae == nil) != (be == nil) || !bytes.Equal(a, b) || (i < 2) != (be == nil) {
			t.Fatal("structured factoring changed semantics", i, ae, be)
		}
	}
	unchanged, _ := json.Marshal(projected)
	if !bytes.Equal(before, unchanged) {
		t.Fatal("unfactored normalization schema mutated")
	}
}
