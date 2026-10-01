package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func strictTestSchema(t *testing.T, document string) *Schema {
	t.Helper()
	s, err := NewSchema("strict_test", []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestStrictSchemaOptionalNullAndRequiredSemantics(t *testing.T) {
	original := `{"type":"object","additionalProperties":false,"required":["must","nullable"],"properties":{"must":{"type":"string","minLength":1},"nullable":{"type":["string","null"]},"optional":{"type":"string","minLength":1},"maybe":{"anyOf":[{"type":"null"},{"type":"object","additionalProperties":false,"properties":{"inner":{"type":"boolean"}}}]}}}`
	schema := strictTestSchema(t, original)
	projection, err := NewStrictSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if string(schema.Document()) != original {
		t.Fatal("domain mutated")
	}
	cases := []struct{ wire, want string }{
		{`{"must":"yes","nullable":null,"optional":null,"maybe":null}`, `{"must":"yes","nullable":null}`},
		{`{"must":"yes","nullable":null,"optional":"yes","maybe":{"value":null}}`, `{"maybe":null,"must":"yes","nullable":null,"optional":"yes"}`},
		{`{"must":"yes","nullable":"x","optional":null,"maybe":{"value":{"inner":null}}}`, `{"maybe":{},"must":"yes","nullable":"x"}`},
		{`{"must":"yes","nullable":"x","optional":null,"maybe":{"value":{"inner":false}}}`, `{"maybe":{"inner":false},"must":"yes","nullable":"x"}`},
	}
	for _, tc := range cases {
		got, err := projection.Normalize([]byte(tc.wire), 4096)
		if err != nil || string(got) != tc.want {
			t.Fatalf("normalize %s: %s %v", tc.wire, got, err)
		}
	}
	for _, bad := range []string{
		`{"nullable":null,"optional":null,"maybe":null}`, `{"must":null,"nullable":null,"optional":null,"maybe":null}`,
		`{"must":"yes","optional":null,"maybe":null}`, `{"must":"yes","nullable":null,"maybe":null}`,
		`{"must":"yes","nullable":null,"optional":"","maybe":null}`, `{"must":"yes","nullable":null,"optional":null,"maybe":{"value":{"inner":null,"foreign":true}}}`,
		`{"must":"yes","nullable":null,"optional":null,"maybe":{"inner":true}}`,
	} {
		if _, err := projection.Normalize([]byte(bad), 4096); !errors.Is(err, ErrOutput) {
			t.Fatal("accepted malformed transport", bad, err)
		}
	}
	detached := projection.Document()
	detached[0] = '!'
	if projection.Document()[0] != '{' {
		t.Fatal("projection shared bytes")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				out, err := projection.Normalize([]byte(cases[0].wire), 4096)
				if err != nil || string(out) != cases[0].want {
					t.Error("concurrent mutation", err)
				}
			}
		}()
	}
	wg.Wait()
}
func TestStrictSchemaDisjointUnionsAndLocalUniqueness(t *testing.T) {
	schema := strictTestSchema(t, `{"type":"object","additionalProperties":false,"required":["items"],"properties":{"items":{"type":"array","uniqueItems":true,"minItems":1,"maxItems":2,"items":{"oneOf":[{"type":"object","additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"left"},"note":{"type":"string"}}},{"type":"object","additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"right"}}}]}}}}`)
	p, err := NewStrictSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(p.Document(), []byte(`"oneOf"`)) || bytes.Contains(p.Document(), []byte(`"uniqueItems"`)) || !p.localUnique {
		t.Fatal("invalid projection", string(p.Document()))
	}
	good := `{"items":[{"kind":"left","note":null},{"kind":"right"}]}`
	if _, err = p.Normalize([]byte(good), 4096); err != nil {
		t.Fatal(err)
	}
	// Transport permits duplicates; only the unchanged domain schema rejects them.
	duplicate := []byte(`{"items":[{"kind":"left","note":null},{"kind":"left","note":null}]}`)
	if p.wire.Validate(duplicate, 4096) != nil {
		t.Fatal("fixture does not exercise local validation")
	}
	if _, err = p.Normalize(duplicate, 4096); !errors.Is(err, ErrOutput) {
		t.Fatal("original uniqueItems bypassed", err)
	}
	for _, bad := range []string{`{"items":[]}`, `{"items":[{"kind":"left"}]}`, `{"items":[{"kind":"foreign"}]}`, `{"items":[{"kind":"right","note":null}]}`} {
		if _, err = p.Normalize([]byte(bad), 4096); err == nil {
			t.Fatal("invalid domain result", bad)
		}
	}
}
func TestStrictSchemaUnsupportedAndAmbiguousFailClosed(t *testing.T) {
	for name, property := range map[string]string{
		"overlap_oneOf":            `{"oneOf":[{"type":"integer"},{"type":"number"}]}`,
		"overlap_anyOf":            `{"anyOf":[{"type":"string"},{"type":"string","minLength":2}]}`,
		"nullable_object_overlap":  `{"oneOf":[{"type":["object","null"],"additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"a"}}},{"type":["object","null"],"additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"b"}}}]}`,
		"mixed_object_overlap":     `{"oneOf":[{"type":["object","string"],"additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"a"}}},{"type":["object","string"],"additionalProperties":false,"required":["kind"],"properties":{"kind":{"const":"b"}}}]}`,
		"numeric_enum_equivalence": `{"oneOf":[{"enum":[1]},{"enum":[1.0]}]}`,
		"union_siblings":           `{"type":"string","anyOf":[{"enum":["a"]},{"enum":["b"]}]}`,
		"open_object":              `{"type":"object","properties":{}}`,
		"allOf":                    `{"allOf":[{"type":"string"}]}`,
		"contains":                 `{"type":"array","items":{"type":"string"},"contains":{"const":"a"}}`,
		"unknown_annotation":       `{"type":"string","unrecognized":true}`,
		"unsupported_format":       `{"type":"string","format":"uri"}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := strictTestSchema(t, `{"type":"object","additionalProperties":false,"required":["x"],"properties":{"x":`+property+`}}`)
			if _, err := NewStrictSchema(s); !errors.Is(err, ErrInput) {
				t.Fatal("unsupported schema admitted", err)
			}
		})
	}
	if _, err := NewStrictSchema(strictTestSchema(t, `{"type":"array","items":{"type":"string"}}`)); err == nil {
		t.Fatal("root array admitted")
	}
	if _, err := NewStrictSchema(nil); err == nil {
		t.Fatal("nil schema admitted")
	}
}
func TestStrictEnvelopeProjectionBytesAndIdentity(t *testing.T) {
	s := strictTestSchema(t, `{"type":"object","additionalProperties":false,"required":["items"],"properties":{"items":{"type":"array","uniqueItems":true,"items":{"type":"string"}},"optional":{"type":"string"}}}`)
	e, err := NewPromptEnvelope("route", "model", "system", "", s, 16384, 0, 1024, 64)
	if err != nil {
		t.Fatal(err)
	}
	without, _, _ := e.Measure("prompt")
	e = e.RequireStructuredParameters()
	u, fit, err := e.Measure("prompt")
	if err != nil || !fit || u.SchemaProjection != "strict-optional-v1" || len(u.DomainSchemaDigest) != 64 || len(u.WireSchemaDigest) != 64 || u.DomainSchemaDigest == u.WireSchemaDigest || u.LocalAssertions != "uniqueItems" {
		t.Fatal("projection receipt", u, err)
	}
	if u.RequestBytes != without.RequestBytes+len(`,
  "provider": {
    "require_parameters": true
  }`) || u.Digest == without.Digest {
		t.Fatal("routing not measured", u, without)
	}
	permissive := strictTestSchema(t, strings.Replace(string(s.Document()), `"uniqueItems":true,`, "", 1))
	other, _ := NewPromptEnvelope("route", "model", "system", "", permissive, 16384, 0, 1024, 64)
	other = other.RequireStructuredParameters()
	v, _, _ := other.Measure("prompt")
	if u.WireSchemaDigest != v.WireSchemaDigest || u.DomainSchemaDigest == v.DomainSchemaDigest || u.Digest == v.Digest {
		t.Fatal("domain-only constraints missing from identity")
	}
	e.maxBytes = u.RequestBytes - 1
	if _, fit, err := e.Measure("prompt"); err != nil || fit {
		t.Fatal("projected byte overflow admitted")
	}
	var extra map[string]any
	raw, _ := json.Marshal(e.ExtraParameters())
	_ = json.Unmarshal(raw, &extra)
	if extra["provider"].(map[string]any)["require_parameters"] != true {
		t.Fatal("capability routing absent")
	}
}

func TestStrictSchemaProjectedProviderLimits(t *testing.T) {
	leaf := map[string]any{"type": []string{"string", "null"}}
	for i := 0; i < 10; i++ {
		parent := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"x": leaf}}
		if i != 0 {
			parent["required"] = []string{"x"}
		}
		leaf = parent
	}
	raw, _ := json.Marshal(leaf)
	s := strictTestSchema(t, string(raw))
	if _, err := NewStrictSchema(s); !errors.Is(err, ErrInput) {
		t.Fatal("introduced wrapper exceeded provider depth", err)
	}
	values := make([]string, 251)
	for i := range values {
		values[i] = strings.Repeat("x", 61) + strings.Repeat("y", i)
	}
	raw, _ = json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"x"}, "properties": map[string]any{"x": map[string]any{"enum": values}}})
	s = strictTestSchema(t, string(raw))
	if _, err := NewStrictSchema(s); !errors.Is(err, ErrInput) {
		t.Fatal("large individual string enum admitted", err)
	}
}

func TestStrictSchemaOptionalEnumsAndNumericUnionsRemainLossless(t *testing.T) {
	s := strictTestSchema(t, `{"type":"object","additionalProperties":false,"properties":{"choice":{"enum":["left","right"]},"number":{"type":["integer","number"]},"union":{"anyOf":[{"type":"boolean"},{"type":"string","minLength":1}]}}}`)
	p, err := NewStrictSchema(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ wire, want string }{
		{`{"choice":null,"number":null,"union":null}`, `{}`},
		{`{"choice":"left","number":1.5,"union":false}`, `{"choice":"left","number":1.5,"union":false}`},
		{`{"choice":"right","number":1,"union":"yes"}`, `{"choice":"right","number":1,"union":"yes"}`},
	} {
		out, err := p.Normalize([]byte(tc.wire), 4096)
		if err != nil || string(out) != tc.want {
			t.Fatal("lossy optional projection", string(out), err)
		}
	}
	if _, err = p.Normalize([]byte(`{"choice":"foreign","number":null,"union":null}`), 4096); err == nil {
		t.Fatal("enum lost its original constraint")
	}
}

func TestStrictSchemaRejectsCallerReferences(t *testing.T) {
	for name, document := range map[string]string{
		"local":             `{"type":"object","additionalProperties":false,"required":["x"],"properties":{"x":{"$ref":"#/$defs/value"}},"$defs":{"value":{"type":"string"}}}`,
		"recursive":         `{"type":"object","additionalProperties":false,"required":["x"],"properties":{"x":{"$ref":"#"}}}`,
		"unused_definition": `{"type":"object","additionalProperties":false,"properties":{},"$defs":{"value":{"type":"string"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := strictTestSchema(t, document)
			if _, err := NewStrictSchema(s); !errors.Is(err, ErrInput) {
				t.Fatal("caller reference admitted", err)
			}
		})
	}
	if _, err := NewSchema("remote_reference", []byte(`{"type":"object","additionalProperties":false,"required":["x"],"properties":{"x":{"$ref":"https://untrusted.invalid/schema"}}}`)); !errors.Is(err, ErrInput) {
		t.Fatal("remote schema resolution enabled", err)
	}
}
