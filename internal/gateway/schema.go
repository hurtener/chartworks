package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// Schema is independently compiled, immutable JSON Schema with network resolution disabled.
type Schema struct {
	name     string
	document json.RawMessage
	compiled *jsonschema.Schema
}

func NewSchema(name string, document []byte) (*Schema, error) {
	if len(name) == 0 || len(name) > 64 {
		return nil, ErrInput
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return nil, ErrInput
		}
	}
	value, err := DecodeJSON(document, 64<<10)
	if err != nil {
		return nil, ErrInput
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, ErrInput
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.LoadURL = func(string) (io.ReadCloser, error) { return nil, ErrInput }
	const location = "https://chartworks.invalid/schema.json"
	if compiler.AddResource(location, bytes.NewReader(document)) != nil {
		return nil, ErrInput
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return nil, ErrInput
	}
	return &Schema{name: name, document: append([]byte(nil), document...), compiled: compiled}, nil
}
func (s *Schema) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}
func (s *Schema) Document() json.RawMessage {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.document...)
}
func (s *Schema) Validate(document []byte, maxBytes int) error {
	if s == nil || s.compiled == nil {
		return ErrInput
	}
	value, err := DecodeJSON(document, maxBytes)
	if err != nil {
		return ErrOutput
	}
	if s.compiled.Validate(value) != nil {
		return ErrOutput
	}
	return nil
}

// DecodeJSON preserves numbers and nullable values, rejects duplicates/trailing values and bounds nesting.
func DecodeJSON(document []byte, maxBytes int) (any, error) {
	if len(document) == 0 || len(document) > maxBytes || !utf8.Valid(document) {
		return nil, ErrOutput
	}
	d := json.NewDecoder(bytes.NewReader(document))
	d.UseNumber()
	if walkJSON(d, 0) != nil {
		return nil, ErrOutput
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrOutput
	}
	d = json.NewDecoder(bytes.NewReader(document))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, ErrOutput
	}
	return v, nil
}
func walkJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrOutput
	}
	v, err := d.Token()
	if err != nil {
		return ErrOutput
	}
	if delim, ok := v.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return ErrOutput
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return ErrOutput
				}
				seen[key] = true
				if len(seen) > 4096 {
					return ErrOutput
				}
				if walkJSON(d, depth+1) != nil {
					return ErrOutput
				}
			}
		case '[':
			for i := 0; d.More(); i++ {
				if i >= 65536 {
					return ErrOutput
				}
				if walkJSON(d, depth+1) != nil {
					return ErrOutput
				}
			}
		default:
			return ErrOutput
		}
		_, err = d.Token()
		if err != nil {
			return ErrOutput
		}
	}
	return nil
}
