package gateway

import (
	"encoding/json"
	"sort"
	"unicode/utf8"
)

// StrictSchema is an immutable, lossless transport projection. Domain schemas
// remain authoritative and are never rewritten. Only the documented strict
// subset is admitted; unsupported constraints fail before inference.
type StrictSchema struct {
	domain, wire *Schema
	localUnique  bool
	root         *strictNode
}

type strictNode struct {
	properties        map[string]*strictNode
	items             *strictNode
	branches          []*strictNode
	validator         *Schema
	optional, wrapped bool
	nullable          bool
}

type strictLimits struct {
	properties, enums int
	localUnique       bool
}

// NewStrictSchema admits a conservative OpenAI-compatible strict subset, also
// used on OpenRouter routes. No constraint is silently discarded. In particular,
// caller references and intersecting unions are not currently supported. Only
// exact repeated scalar/closed-object schemas may gain generated local references.
// uniqueItems is an explicitly local-only assertion enforced by the unchanged
// domain schema.
func NewStrictSchema(domain *Schema) (*StrictSchema, error) {
	if domain == nil || domain.compiled == nil {
		return nil, ErrInput
	}
	value, err := DecodeJSON(domain.document, 64<<10)
	if err != nil {
		return nil, ErrInput
	}
	document := value.(map[string]any)
	if document["type"] != "object" {
		return nil, ErrInput
	}
	if document["anyOf"] != nil || document["oneOf"] != nil {
		return nil, ErrInput
	}
	limits := &strictLimits{}
	node, projected, err := projectStrict(document, 0, limits)
	if err != nil {
		return nil, err
	}
	if !strictProjectedDepth(projected, 0) || limits.properties > 5000 || limits.enums > 1000 {
		return nil, ErrInput
	}
	projected = strictDefinitions(projected)
	raw, err := json.Marshal(projected)
	if err != nil {
		return nil, ErrInput
	}
	wire, err := NewSchema(domain.Name(), raw)
	if err != nil {
		return nil, err
	}
	return &StrictSchema{domain: domain, wire: wire, root: node, localUnique: limits.localUnique}, nil
}

// Document returns only the detached provider schema, not the domain schema.
func (s *StrictSchema) Document() json.RawMessage {
	if s == nil {
		return nil
	}
	return s.wire.Document()
}

// Normalize validates the complete provider shape, removes only introduced
// absence markers, then independently validates the original domain schema.
// Required fields and pre-existing domain nulls can never disappear.
func (s *StrictSchema) Normalize(raw []byte, maxBytes int) (json.RawMessage, error) {
	if s == nil || s.wire == nil || s.domain == nil || s.root == nil {
		return nil, ErrInput
	}
	value, err := DecodeJSON(raw, maxBytes)
	if err != nil || s.wire.compiled.Validate(value) != nil {
		return nil, ErrOutput
	}
	value, err = s.root.normalize(value)
	if err != nil {
		return nil, err
	}
	if s.domain.compiled.Validate(value) != nil {
		return nil, ErrOutput
	}
	out, err := json.Marshal(value)
	if err != nil || len(out) > maxBytes {
		return nil, ErrOutput
	}
	return out, nil
}

func (n *strictNode) normalize(value any) (any, error) {
	if n.wrapped {
		object, ok := value.(map[string]any)
		if !ok || len(object) != 1 {
			return nil, ErrOutput
		}
		value = object["value"]
	}
	if len(n.branches) != 0 {
		for _, branch := range n.branches {
			if branch.validator.compiled.Validate(value) == nil {
				return branch.normalize(value)
			}
		}
		return nil, ErrOutput
	}
	switch value := value.(type) {
	case map[string]any:
		for key, child := range n.properties {
			v, exists := value[key]
			if !exists {
				return nil, ErrOutput
			}
			if child.optional && v == nil {
				delete(value, key)
				continue
			}
			normalized, err := child.normalize(v)
			if err != nil {
				return nil, err
			}
			value[key] = normalized
		}
	case []any:
		if n.items != nil {
			for i, v := range value {
				normalized, err := n.items.normalize(v)
				if err != nil {
					return nil, err
				}
				value[i] = normalized
			}
		}
	}
	return value, nil
}

func projectStrict(in map[string]any, depth int, limits *strictLimits) (*strictNode, map[string]any, error) {
	if depth > 10 {
		return nil, nil, ErrInput
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		switch key {
		case "uniqueItems":
			if _, ok := value.(bool); !ok {
				return nil, nil, ErrInput
			}
			limits.localUnique = limits.localUnique || value == true
		case "type", "properties", "required", "additionalProperties", "items", "anyOf", "oneOf", "enum", "const", "title", "description", "minLength", "maxLength", "pattern", "format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minItems", "maxItems":
			out[key] = value
		default:
			return nil, nil, ErrInput
		}
	}
	n := &strictNode{}
	union, hasAny := in["anyOf"]
	if other, hasOne := in["oneOf"]; hasOne {
		if hasAny {
			return nil, nil, ErrInput
		}
		union = other
	}
	if union != nil {
		// Constraint siblings intersect branches. Do not guess how their validation
		// or absence normalization composes.
		for key := range in {
			if key != "anyOf" && key != "oneOf" && key != "title" && key != "description" {
				return nil, nil, ErrInput
			}
		}
		values, ok := union.([]any)
		if !ok || len(values) < 2 || len(values) > 64 {
			return nil, nil, ErrInput
		}
		branches := make([]map[string]any, len(values))
		projected := make([]any, len(values))
		for i, value := range values {
			branch, ok := value.(map[string]any)
			if !ok {
				return nil, nil, ErrInput
			}
			child, wire, err := projectStrict(branch, depth, limits)
			if err != nil {
				return nil, nil, err
			}
			raw, _ := json.Marshal(wire)
			child.validator, err = NewSchema("strict_branch", raw)
			if err != nil {
				return nil, nil, err
			}
			n.branches = append(n.branches, child)
			n.nullable = n.nullable || child.nullable
			branches[i], projected[i] = wire, wire
		}
		for i := range branches {
			for j := 0; j < i; j++ {
				if !strictDisjoint(branches[i], branches[j]) {
					return nil, nil, ErrInput
				}
			}
		}
		delete(out, "oneOf")
		out["anyOf"] = projected
		return n, out, nil
	}
	// const and untyped enums have exact equivalent typed enum representations.
	if value, ok := in["const"]; ok {
		if existing, exists := in["enum"]; exists {
			found := false
			for _, item := range existing.([]any) {
				a, _ := json.Marshal(value)
				b, _ := json.Marshal(item)
				if string(a) == string(b) {
					found = true
				}
			}
			if !found {
				return nil, nil, ErrInput
			}
		}
		delete(out, "const")
		out["enum"] = []any{value}
	}
	if enum, ok := out["enum"].([]any); ok {
		limits.enums += len(enum)
		if limits.enums > 1000 {
			return nil, nil, ErrInput
		}
		types := map[string]bool{}
		characters := 0
		for _, value := range enum {
			if text, ok := value.(string); ok {
				characters += utf8.RuneCountInString(text)
			}
			typ := strictValueType(value)
			if typ == "" {
				return nil, nil, ErrInput
			}
			types[typ] = true
		}
		if len(enum) > 250 && characters > 15000 {
			return nil, nil, ErrInput
		}
		if out["type"] == nil {
			out["type"] = strictTypeValue(types)
		}
	}
	types := strictTypes(out)
	if len(types) == 0 {
		return nil, nil, ErrInput
	}
	n.nullable = types["null"]
	// Type-specific constraints must belong to their type; unknown annotations
	// and unsupported formats never become unenforced provider hints.
	for key := range out {
		var typ string
		switch key {
		case "properties", "required", "additionalProperties":
			typ = "object"
		case "items", "minItems", "maxItems":
			typ = "array"
		case "minLength", "maxLength", "pattern", "format":
			typ = "string"
		case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
			typ = "number"
		}
		if typ != "" && !types[typ] {
			return nil, nil, ErrInput
		}
	}
	if format, ok := out["format"]; ok {
		switch format {
		case "date-time", "time", "date", "duration", "email", "hostname", "ipv4", "ipv6", "uuid":
		default:
			return nil, nil, ErrInput
		}
	}
	if types["object"] {
		if out["additionalProperties"] != false {
			return nil, nil, ErrInput
		}
		properties, ok := in["properties"].(map[string]any)
		if !ok {
			return nil, nil, ErrInput
		}
		required := map[string]bool{}
		if names, ok := in["required"].([]any); ok {
			for _, name := range names {
				key, ok := name.(string)
				if !ok || properties[key] == nil {
					return nil, nil, ErrInput
				}
				required[key] = true
			}
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		limits.properties += len(keys)
		if limits.properties > 5000 {
			return nil, nil, ErrInput
		}
		n.properties = map[string]*strictNode{}
		wireProperties := map[string]any{}
		for _, key := range keys {
			childSchema, ok := properties[key].(map[string]any)
			if !ok {
				return nil, nil, ErrInput
			}
			child, wire, err := projectStrict(childSchema, depth+1, limits)
			if err != nil {
				return nil, nil, err
			}
			if !required[key] {
				child.optional = true
				if child.nullable {
					child.wrapped = true
					wire = map[string]any{"anyOf": []any{map[string]any{"type": "object", "additionalProperties": false, "required": []string{"value"}, "properties": map[string]any{"value": wire}}, map[string]any{"type": "null"}}, "description": "Optional nullable domain field. Null omits; an object with value preserves explicit null or a present value."}
					limits.properties++
				} else {
					// Nullable type unions avoid an extra anyOf indentation layer for large
					// schemas. Enum null is required too: type alone cannot widen an enum.
					if types := strictTypes(wire); len(types) != 0 {
						types["null"] = true
						if types["integer"] && !strictDeclaredType(wire["type"], "number") {
							delete(types, "number")
						}
						wire["type"] = strictTypeValue(types)
						if enum, ok := wire["enum"].([]any); ok {
							wire["enum"] = append(append([]any(nil), enum...), nil)
							limits.enums++
						}
					} else if branches, ok := wire["anyOf"].([]any); ok {
						wire["anyOf"] = append(append([]any(nil), branches...), map[string]any{"type": "null"})
					} else {
						return nil, nil, ErrInput
					}
					description, _ := wire["description"].(string)
					wire["description"] = "Null omits. " + description
				}
			}
			n.properties[key], wireProperties[key] = child, wire
		}
		out["properties"], out["required"] = wireProperties, keys
	}
	if types["array"] {
		item, ok := in["items"].(map[string]any)
		if !ok {
			return nil, nil, ErrInput
		}
		var err error
		n.items, out["items"], err = projectStrict(item, depth+1, limits)
		if err != nil {
			return nil, nil, err
		}
	}
	return n, out, nil
}

func strictValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	default:
		return ""
	}
}
func strictTypeValue(types map[string]bool) any {
	keys := make([]string, 0, len(types))
	for key := range types {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		return keys[0]
	}
	return keys
}
func strictTypes(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	add := func(value any) bool {
		key, ok := value.(string)
		if !ok {
			return false
		}
		switch key {
		case "null", "string", "boolean", "object", "array", "number", "integer":
			out[key] = true
			if key == "integer" {
				out["number"] = true
			}
			return true
		}
		return false
	}
	switch value := schema["type"].(type) {
	case string:
		if !add(value) {
			return nil
		}
	case []any:
		for _, typ := range value {
			if !add(typ) {
				return nil
			}
		}
	case []string:
		for _, typ := range value {
			if !add(typ) {
				return nil
			}
		}
	default:
		return nil
	}
	return out
}
func strictDisjoint(a, b map[string]any) bool {
	at, bt := strictTypes(a), strictTypes(b)
	if len(at) != 0 && len(bt) != 0 {
		overlap := false
		for typ := range at {
			overlap = overlap || bt[typ]
		}
		if !overlap {
			return true
		}
	}
	if ae, aok := a["enum"].([]any); aok {
		if be, bok := b["enum"].([]any); bok {
			overlap := false
			for _, av := range ae {
				if strictValueType(av) == "number" {
					return false
				}
				for _, bv := range be {
					if strictValueType(bv) == "number" {
						return false
					}
					ar, _ := json.Marshal(av)
					br, _ := json.Marshal(bv)
					overlap = overlap || string(ar) == string(br)
				}
			}
			if !overlap {
				return true
			}
		}
	}
	if !at["object"] || !bt["object"] {
		return false
	}
	// An object discriminator cannot distinguish a shared null, string, or any
	// other non-object branch in a multi-type schema.
	for typ := range at {
		if typ != "object" && bt[typ] {
			return false
		}
	}
	ap, _ := a["properties"].(map[string]any)
	bp, _ := b["properties"].(map[string]any)
	// Projection makes every property required, but only an unwrapped finite enum
	// can prove distinctness. Optional discriminator values are nullable unions.
	for key, av := range ap {
		am, aok := av.(map[string]any)
		bm, bok := bp[key].(map[string]any)
		if aok && bok && am["enum"] != nil && bm["enum"] != nil && strictDisjoint(am, bm) {
			return true
		}
	}
	return false
}

// The provider's nesting bound also applies to introduced nullable wrappers.
func strictProjectedDepth(schema map[string]any, depth int) bool {
	if depth > 10 {
		return false
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		for _, value := range properties {
			if !strictProjectedDepth(value.(map[string]any), depth+1) {
				return false
			}
		}
	}
	if item, ok := schema["items"].(map[string]any); ok {
		if !strictProjectedDepth(item, depth+1) {
			return false
		}
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		for _, value := range branches {
			if !strictProjectedDepth(value.(map[string]any), depth) {
				return false
			}
		}
	}
	return true
}

func strictDeclaredType(value any, target string) bool {
	switch value := value.(type) {
	case string:
		return value == target
	case []any:
		for _, v := range value {
			if v == target {
				return true
			}
		}
	case []string:
		for _, v := range value {
			if v == target {
				return true
			}
		}
	}
	return false
}
