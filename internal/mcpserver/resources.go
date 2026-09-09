package mcpserver

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// Canonical resource URIs deliberately have no credentials, queries, fragments,
// percent aliases or externally fetched URLs. Variable segments are opaque IDs.
func resourceParts(uri string) ([]string, bool) {
	if len(uri) > 1024 || strings.ContainsAny(uri, "%?#@\\\x00\r\n\t") {
		return nil, false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "chartworks" || !toolName(u.Host) || u.User != nil || u.Opaque != "" || u.RawPath != "" && !strings.Contains(u.Path, "{") || !strings.HasPrefix(u.Path, "/") {
		return nil, false
	}
	parts := strings.Split(u.Host+u.Path, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, false
		}
	}
	return parts, true
}
func validResourcePattern(pattern string, input *gateway.Schema) bool {
	parts, ok := resourceParts(pattern)
	if !ok || len(parts) > 8 {
		return false
	}
	value, err := gateway.DecodeJSON(input.Document(), 65536)
	if err != nil {
		return false
	}
	schema := value.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	fields := map[string]bool{}
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := part[1 : len(part)-1]
			v, ok := properties[name].(map[string]any)
			if !toolName(name) || fields[name] || !ok || v["type"] != "string" {
				return false
			}
			fields[name] = true
		} else if !identity.Identifier(part) {
			return false
		}
	}
	// The resource can only be an exact projection of its tool's complete input.
	return len(fields) == len(properties)
}
func resourceArguments(pattern, uri string) (json.RawMessage, bool) {
	want, ok := resourceParts(pattern)
	if !ok {
		return nil, false
	}
	got, ok := resourceParts(uri)
	if !ok || len(want) != len(got) {
		return nil, false
	}
	in := map[string]string{}
	for i, p := range want {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			if !identity.Identifier(got[i]) {
				return nil, false
			}
			in[p[1:len(p)-1]] = got[i]
		} else if got[i] != p {
			return nil, false
		}
	}
	b, err := json.Marshal(in)
	return b, err == nil
}
func (r *Registry) resource(uri string) (Binding, json.RawMessage, bool) {
	for _, b := range r.bindings {
		if b.resource != "" {
			if raw, ok := resourceArguments(b.resource, uri); ok {
				return b, raw, true
			}
		}
	}
	return Binding{}, nil, false
}

// resourcePatternsOverlap rejects ambiguous dispatch even when templates use
// different variable names or trade a literal segment for a variable. A URI
// must identify exactly one action/service, independent of registration order.
func resourcePatternsOverlap(a, b string) bool {
	left, ok := resourceParts(a)
	if !ok {
		return false
	}
	right, ok := resourceParts(b)
	if !ok || len(left) != len(right) {
		return false
	}
	for i, part := range left {
		variable := strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")
		otherVariable := strings.HasPrefix(right[i], "{") && strings.HasSuffix(right[i], "}")
		if !variable && !otherVariable && part != right[i] {
			return false
		}
	}
	return true
}
