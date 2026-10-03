package gateway

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// strictDefinitions factors exact repeated scalar or closed-object schemas after all
// admission/disjointness checks. Generated local refs are acyclic and cannot
// change data shape, normalization, constraints or provider nesting depth. The
// complete factored wire schema is independently compiled by NewStrictSchema.
func strictDefinitions(root map[string]any) map[string]any {
	counts := map[string]int{}
	originals := map[string]map[string]any{}
	walkStrictSchemas(root, func(schema map[string]any) map[string]any {
		types := strictTypes(schema)
		if len(types) == 0 || types["array"] {
			return schema
		}
		if types["object"] {
			if schema["additionalProperties"] != false {
				return schema
			}
			for typ := range types {
				if typ != "object" && typ != "null" {
					return schema
				}
			}
		}
		raw, _ := json.Marshal(schema)
		if len(raw) < 32 {
			return schema
		}
		key := string(raw)
		counts[key]++
		originals[key] = schema
		return schema
	})
	keys := []string{}
	for key, count := range counts {
		if count >= 2 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return root
	}
	references := map[string]string{}
	definitions := map[string]any{}
	used := map[string]bool{}
	for i, key := range keys {
		name := "s" + strconv.Itoa(i)
		references[key] = "#/$defs/" + name
		definitions[name] = originals[key]
	}
	out := walkStrictSchemas(root, func(schema map[string]any) map[string]any {
		raw, _ := json.Marshal(schema)
		if ref, ok := references[string(raw)]; ok {
			used[strings.TrimPrefix(ref, "#/$defs/")] = true
			return map[string]any{"$ref": ref}
		}
		return schema
	})
	for name := range definitions {
		if !used[name] {
			delete(definitions, name)
		}
	}
	out["$defs"] = definitions
	// Compare at the actual response_format.json_schema.schema indentation level.
	// A factoring candidate must shrink the effective request, never merely add refs.
	before, _ := json.MarshalIndent(root, "      ", "  ")
	after, _ := json.MarshalIndent(out, "      ", "  ")
	if len(after) >= len(before) {
		return root
	}
	return out
}

func walkStrictSchemas(schema map[string]any, visit func(map[string]any) map[string]any) map[string]any {
	// Only properties, items and anyOf contain schema values in this admitted
	// subset. Never interpret literal enum values as schemas. Rebuild privately so
	// an unprofitable factoring attempt cannot mutate the admitted projection.
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		out[key] = value
	}
	// Visit before rewriting children: candidate identities use the complete
	// original subtree. Definitions themselves stay inlined, with no ref chains.
	out = visit(out)
	if out["$ref"] != nil {
		return out
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		copy := make(map[string]any, len(properties))
		for key, value := range properties {
			copy[key] = walkStrictSchemas(value.(map[string]any), visit)
		}
		out["properties"] = copy
	}
	if items, ok := schema["items"].(map[string]any); ok {
		out["items"] = walkStrictSchemas(items, visit)
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		copy := make([]any, len(branches))
		for i, value := range branches {
			copy[i] = walkStrictSchemas(value.(map[string]any), visit)
		}
		out["anyOf"] = copy
	}
	return out
}
