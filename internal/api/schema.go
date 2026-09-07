package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/invopop/jsonschema"
)

// SchemaFor derives a closed wire-shape schema from an actual DTO type. Domain
// validity, scopes and configured budgets remain enforced by the domain service.
// Responses admit nil slices because encoding/json emits them as null; requests
// retain the existing source handler's required, non-null shape.
func SchemaFor(name string, typ reflect.Type, response bool) (*gateway.Schema, error) {
	if !supportedType(typ, map[reflect.Type]bool{}, 0) {
		return nil, ErrRegistration
	}
	r := jsonschema.Reflector{Anonymous: true, DoNotReference: true}
	schema := r.ReflectFromType(typ)
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, ErrRegistration
	}
	if response {
		var shape map[string]any
		if json.Unmarshal(raw, &shape) != nil {
			return nil, ErrRegistration
		}
		allowNilSlices(shape, typ)
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

func supportedType(t reflect.Type, stack map[reflect.Type]bool, depth int) bool {
	if t == nil || depth > 16 || stack[t] {
		return false
	}
	if t == reflect.TypeFor[time.Time]() {
		return true
	}
	stack[t] = true
	defer delete(stack, t)
	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		return t.Elem().Kind() != reflect.Uint8 && supportedType(t.Elem(), stack, depth+1)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if !field.IsExported() || field.Anonymous || name == "" || name == "-" || !supportedType(field.Type, stack, depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

func allowNilSlices(schema map[string]any, t reflect.Type) {
	if t.Kind() == reflect.Slice {
		if items, ok := schema["items"].(map[string]any); ok {
			allowNilSlices(items, t.Elem())
		}
		// JSON Schema 2020-12 permits a type union; no OpenAPI 3.0 nullable flag.
		schema["type"] = []string{"array", "null"}
	} else if t.Kind() == reflect.Struct && t != reflect.TypeFor[time.Time]() {
		properties, _ := schema["properties"].(map[string]any)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if child, ok := properties[name].(map[string]any); ok {
				allowNilSlices(child, field.Type)
			}
		}
	}
}
