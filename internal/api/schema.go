package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/invopop/jsonschema"
)

// SchemaOption selects the concrete request decoder's wire-shape convention.
type SchemaOption uint8

// OptionalJSONFields matches a closed encoding/json decoder that accepts omitted
// and null fields before domain validation. It applies only to request schemas.
const OptionalJSONFields SchemaOption = 1

// NullableCollections preserves Go nil slice encoding while keeping request
// scalar fields required and non-null. In particular, explicit cell null bits
// cannot be bypassed by silently decoding JSON null into an empty string.
const NullableCollections SchemaOption = 2

// SchemaFor derives wire shapes from actual DTOs. Domain validity, scopes and
// configured budgets remain in the service. Responses admit nil pointer/slice/map
// values; default request schemas retain required, non-null fields.
func SchemaFor(name string, typ reflect.Type, response bool, options ...SchemaOption) (*gateway.Schema, error) {
	optional := len(options) == 1 && options[0] == OptionalJSONFields && !response
	nullableCollections := len(options) == 1 && options[0] == NullableCollections && !response
	if len(options) > 0 && !optional && !nullableCollections || !supportedType(typ, map[reflect.Type]bool{}, 0, response, optional) {
		return nil, ErrRegistration
	}
	r := jsonschema.Reflector{Anonymous: true, DoNotReference: true, RequiredFromJSONSchemaTags: optional}
	r.Mapper = func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeFor[json.RawMessage]() {
			// The read core emits exact scalar cells; structured values are JSON text.
			return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Type: "string"}, {Type: "number"}, {Type: "boolean"}, {Type: "null"}}}
		}
		return nil
	}
	schema := r.ReflectFromType(typ)
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, ErrRegistration
	}
	if response || optional || nullableCollections {
		var shape map[string]any
		if json.Unmarshal(raw, &shape) != nil {
			return nil, ErrRegistration
		}
		adjustWireSchema(shape, typ, optional, true)
		raw, err = json.Marshal(shape)
		if err != nil {
			return nil, ErrRegistration
		}
	}
	compiled, err := gateway.NewSchema(name, raw)
	if err != nil {
		return nil, ErrRegistration
	}
	return compiled, nil
}

func supportedType(t reflect.Type, stack map[reflect.Type]bool, depth int, response, optional bool) bool {
	if t == nil || depth > 16 || stack[t] {
		return false
	}
	if t == reflect.TypeFor[time.Time]() || response && t == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	stack[t] = true
	defer delete(stack, t)
	switch t.Kind() {
	case reflect.Pointer:
		return (response || optional) && supportedType(t.Elem(), stack, depth+1, response, optional)
	case reflect.Map:
		return response && t.Key().Kind() == reflect.String && responseMapValue(t.Elem())
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		return t.Elem().Kind() != reflect.Uint8 && supportedType(t.Elem(), stack, depth+1, response, optional)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if !field.IsExported() || name == "-" {
				if response {
					continue
				}
				return false
			}
			if field.Anonymous {
				if !supportedType(field.Type, stack, depth+1, response, optional) {
					return false
				}
				continue
			}
			if name == "" || !supportedType(field.Type, stack, depth+1, response, optional) {
				return false
			}
		}
		return true
	}
	return false
}

func responseMapValue(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func adjustWireSchema(schema map[string]any, t reflect.Type, optional, root bool) {
	if t == reflect.TypeFor[json.RawMessage]() {
		return
	}
	if t.Kind() == reflect.Pointer {
		adjustWireSchema(schema, t.Elem(), optional, false)
		allowNull(schema)
		return
	}
	switch t.Kind() {
	case reflect.Slice:
		if items, ok := schema["items"].(map[string]any); ok {
			adjustWireSchema(items, t.Elem(), optional, false)
		}
		allowNull(schema)
	case reflect.Map:
		allowNull(schema)
	case reflect.Struct:
		if t != reflect.TypeFor[time.Time]() {
			properties, _ := schema["properties"].(map[string]any)
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				name := strings.Split(field.Tag.Get("json"), ",")[0]
				if child, ok := properties[name].(map[string]any); ok {
					adjustWireSchema(child, field.Type, optional, false)
				}
			}
		}
	}
	if optional && !root {
		allowNull(schema)
	}
}

func allowNull(schema map[string]any) {
	if kind, ok := schema["type"].(string); ok {
		schema["type"] = []string{kind, "null"}
	}
}
