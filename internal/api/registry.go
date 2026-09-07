// Package api owns shared HTTP registration and public schema descriptions.
// Domain handlers and services retain request verification and resource enforcement.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

var ErrRegistration = errors.New("api: invalid operation registration")

// Operation is the common method/path/action/effect manifest projection.
type Operation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Effect string `json:"effect"`
}

// ErrorResponse describes an existing wire error, not a new mapping policy.
type ErrorResponse struct {
	Status int
	Code   string
}

// Definition records a concrete operation's public wire schema and domain owner.
// ResourceLoader names the existing service path that resolves/enforces resources;
// it is descriptive metadata, not a replacement authorization callback.
type Definition struct {
	Operation
	ID             string
	Summary        string
	ResourceLoader string
	Audit          string
	MaxBodyBytes   int
	Request        *gateway.Schema
	Response       *gateway.Schema
	Errors         []ErrorResponse
}

type Registry struct{ definitions []Definition }

// New rejects incomplete/duplicate registration before a router can consume it.
// Schemas are immutable by API; returned metadata and documents are detached.
func New(definitions []Definition) (*Registry, error) {
	if len(definitions) < 1 || len(definitions) > 256 {
		return nil, ErrRegistration
	}
	ids, routes := map[string]bool{}, map[string]bool{}
	out := make([]Definition, len(definitions))
	for i, d := range definitions {
		if !identity.Identifier(d.ID) || !line(d.Summary) || !line(d.ResourceLoader) || !line(d.Audit) || !line(d.Action) || !line(d.Effect) || !validPath(d.Path) || d.Response == nil || d.Response.Name() == "" || len(d.Errors) < 1 || len(d.Errors) > 32 {
			return nil, ErrRegistration
		}
		switch d.Method {
		case http.MethodGet:
			if d.Request != nil || d.MaxBodyBytes != 0 {
				return nil, ErrRegistration
			}
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if d.Request == nil || d.Request.Name() == "" || d.MaxBodyBytes < 1 || d.MaxBodyBytes > 10<<20 {
				return nil, ErrRegistration
			}
		default:
			return nil, ErrRegistration
		}
		key := d.Method + " " + d.Path
		if ids[d.ID] || routes[key] {
			return nil, ErrRegistration
		}
		ids[d.ID], routes[key] = true, true
		seen := map[ErrorResponse]bool{}
		for _, e := range d.Errors {
			if e.Status < 400 || e.Status > 599 || !identity.Identifier(e.Code) || seen[e] {
				return nil, ErrRegistration
			}
			seen[e] = true
		}
		out[i] = cloneDefinition(d)
	}
	return &Registry{definitions: out}, nil
}

func (r *Registry) Operations() []Operation {
	if r == nil {
		return nil
	}
	out := make([]Operation, len(r.definitions))
	for i, d := range r.definitions {
		out[i] = d.Operation
	}
	return out
}

func (r *Registry) Definitions() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, len(r.definitions))
	for i, d := range r.definitions {
		out[i] = cloneDefinition(d)
	}
	return out
}

// Match distinguishes a known path with an unsupported method from fallthrough,
// preserving the caller's authenticated 405 handling.
func (r *Registry) Match(method, path string) (Definition, string, bool) {
	if r == nil {
		return Definition{}, "", false
	}
	known := false
	for _, d := range r.definitions {
		if id, ok := MatchPath(d.Path, path); ok {
			known = true
			if d.Method == method {
				return cloneDefinition(d), id, true
			}
		}
	}
	return Definition{}, "", known
}

// MatchPath implements the existing exact single-{id} source route shape.
func MatchPath(pattern, path string) (string, bool) {
	want, got := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(want) != len(got) {
		return "", false
	}
	id := ""
	for i, part := range want {
		if part == "{id}" {
			if !identity.Identifier(got[i]) {
				return "", false
			}
			id = got[i]
		} else if got[i] != part {
			return "", false
		}
	}
	return id, true
}

func validPath(path string) bool {
	if len(path) > 256 || !strings.HasPrefix(path, "/v1/") || strings.HasSuffix(path, "/") {
		return false
	}
	ids := 0
	for _, part := range strings.Split(path, "/")[1:] {
		if part == "{id}" {
			ids++
			continue
		}
		if !identity.Identifier(part) {
			return false
		}
	}
	return ids <= 1
}

func line(s string) bool {
	return len(s) > 0 && len(s) <= 256 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n\t")
}
func cloneDefinition(d Definition) Definition {
	d.Errors = append([]ErrorResponse(nil), d.Errors...)
	return d
}

// OpenAPI generates a self-contained OpenAPI 3.1.1 document from this exact
// registry. It adds no routes and grants no bearer or resource authority.
func (r *Registry) OpenAPI(title, version string) ([]byte, error) {
	if r == nil || !line(title) || !line(version) {
		return nil, ErrRegistration
	}
	paths := map[string]map[string]any{}
	for _, d := range r.definitions {
		if paths[d.Path] == nil {
			paths[d.Path] = map[string]any{}
		}
		responses := map[string]any{"200": map[string]any{"description": "Successful response", "content": content(d.Response.Document())}}
		byStatus := map[int][]string{}
		for _, e := range d.Errors {
			byStatus[e.Status] = append(byStatus[e.Status], e.Code)
		}
		for status, codes := range byStatus {
			sort.Strings(codes)
			schema := map[string]any{"type": "object", "properties": map[string]any{"error": map[string]any{"type": "string", "enum": codes}}, "required": []string{"error"}, "additionalProperties": false}
			responses[statusString(status)] = map[string]any{"description": http.StatusText(status), "content": content(schema)}
		}
		// The existing router's authenticated wrong-method response has no body.
		responses["405"] = map[string]any{"description": "Method not allowed; empty response body"}
		op := map[string]any{"operationId": d.ID, "summary": d.Summary, "security": []any{map[string]any{"penguiBearer": []string{}}}, "responses": responses, "x-chartworks-action": d.Action, "x-chartworks-effect": d.Effect, "x-chartworks-resource-loader": d.ResourceLoader, "x-chartworks-audit": d.Audit}
		if strings.Contains(d.Path, "{id}") {
			op["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9_.:-]+$"}}}
		}
		if d.Request != nil {
			op["requestBody"] = map[string]any{"required": true, "content": content(d.Request.Document())}
			op["x-chartworks-max-body-bytes"] = d.MaxBodyBytes
		}
		paths[d.Path][strings.ToLower(d.Method)] = op
	}
	return json.MarshalIndent(map[string]any{"openapi": "3.1.1", "info": map[string]any{"title": title, "version": version}, "paths": paths, "components": map[string]any{"securitySchemes": map[string]any{"penguiBearer": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT", "description": "Existing Pengui bearer; signed operation and resource reach remain required."}}}}, "", "  ")
}

func content(schema any) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": schema}}
}
func statusString(status int) string {
	return string([]byte{byte('0' + status/100), byte('0' + status/10%10), byte('0' + status%10)})
}
