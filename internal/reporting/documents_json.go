package reporting

import (
	"bytes"
	"encoding/json"
	"io"
)

// documentJSON bounds nesting and rejects duplicate object members before a
// typed decode or hash. UseNumber avoids lossy float conversion during hashing.
func documentJSON(raw []byte) (any, error) {
	if len(raw) == 0 || len(raw) > 2<<20 {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	nodes := 0
	value, err := documentJSONValue(decoder, 0, &nodes)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	return value, nil
}

func documentJSONValue(decoder *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 32 || *nodes > 100000 {
		return nil, ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, ErrInvalid
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, ErrInvalid
			}
			name, ok := key.(string)
			if !ok {
				return nil, ErrInvalid
			}
			if _, duplicate := object[name]; duplicate {
				return nil, ErrInvalid
			}
			value, err := documentJSONValue(decoder, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrInvalid
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			if len(array) >= 1000 {
				return nil, ErrInvalid
			}
			value, err := documentJSONValue(decoder, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrInvalid
		}
		return array, nil
	default:
		return nil, ErrInvalid
	}
}
