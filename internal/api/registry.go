// Package api owns shared HTTP registration and public schema descriptions.
// Domain handlers and services retain request verification and resource enforcement.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// ErrRegistration indicates that an operation cannot be exposed safely.
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
	// Receipt marks an error response that includes the bounded gateway attempt
	// receipt alongside its stable error code.
	Receipt bool
}

// Parameter describes a bounded path, query or header value in the public
// contract. Request bodies remain represented by Schema.
type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	Type        string
	Format      string
	Min         int
	Max         int
	Pattern     string
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
	// Public marks a route that deliberately has no bearer requirement, such as
	// health and the document endpoint. Protected operations leave it false.
	Public bool
	// Query and Headers describe values the handler actually accepts. They do not
	// carry authority; identity and signed reach remain in the bearer envelope.
	Query   []Parameter
	Headers []Parameter
	// RequestContentType defaults to JSON; raw uploads use application/octet-stream.
	RequestContentType string
	// ResponseContentType defaults to JSON; metrics uses text/plain.
	ResponseContentType string
	MaxBodyBytes        int
	Request             *gateway.Schema
	Response            *gateway.Schema
	Errors              []ErrorResponse
}

// Registry is an immutable collection of actual HTTP operation definitions.
type Registry struct{ definitions []Definition }

// Compose joins concrete domain registries into one route/document inventory.
// Duplicate operation IDs and method/path pairs are rejected by New.
func Compose(registries ...*Registry) (*Registry, error) {
	definitions := make([]Definition, 0)
	for _, registry := range registries {
		if registry == nil {
			continue
		}
		definitions = append(definitions, registry.Definitions()...)
	}
	return New(definitions)
}

// New rejects incomplete/duplicate registration before a router can consume it.
// Schemas are immutable by API; returned metadata and documents are detached.
func New(definitions []Definition) (*Registry, error) {
	if len(definitions) < 1 || len(definitions) > 256 {
		return nil, ErrRegistration
	}
	ids, routes := map[string]bool{}, map[string]bool{}
	out := make([]Definition, len(definitions))
	for i, d := range definitions {
		if !identity.Identifier(d.ID) || !line(d.Summary) || !line(d.ResourceLoader) || !line(d.Audit) || (!d.Public && !line(d.Action)) || !line(d.Effect) || !validPath(d.Path, d.Public) || d.Response == nil || d.Response.Name() == "" || len(d.Errors) < 1 || len(d.Errors) > 32 {
			return nil, ErrRegistration
		}
		switch d.Method {
		case http.MethodGet, http.MethodHead:
			if d.Request != nil || d.MaxBodyBytes != 0 || d.RequestContentType != "" {
				return nil, ErrRegistration
			}
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			maxBody := 10 << 20
			if d.RequestContentType == "application/octet-stream" {
				maxBody = 100 << 20
			} else if d.RequestContentType != "" && d.RequestContentType != "application/json" {
				return nil, ErrRegistration
			}
			if d.Request == nil || d.Request.Name() == "" || d.MaxBodyBytes < 1 || d.MaxBodyBytes > maxBody {
				return nil, ErrRegistration
			}
		default:
			return nil, ErrRegistration
		}
		if !line(d.ResponseContentType) && d.ResponseContentType != "" {
			return nil, ErrRegistration
		}
		if !validParameters(d.Query, "query") || !validParameters(d.Headers, "header") {
			return nil, ErrRegistration
		}
		key := d.Method + " " + d.Path
		if ids[d.ID] || routes[key] {
			return nil, ErrRegistration
		}
		ids[d.ID], routes[key] = true, true
		seen := map[ErrorResponse]bool{}
		unauthorized, forbidden := false, false
		for _, e := range d.Errors {
			if e.Status < 400 || e.Status > 599 || !identity.Identifier(e.Code) || seen[e] {
				return nil, ErrRegistration
			}
			seen[e] = true
			unauthorized = unauthorized || e.Status == 401
			forbidden = forbidden || e.Status == 403
		}
		if !d.Public && (!unauthorized || !forbidden) {
			return nil, ErrRegistration
		}
		out[i] = cloneDefinition(d)
	}
	return &Registry{definitions: out}, nil
}

// Operations returns the legacy method/path/action/effect projection without
// exposing the registry's backing storage.
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

// Definitions returns detached metadata for every registered operation.
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

func validPath(path string, public bool) bool {
	if len(path) > 256 || path == "" || strings.HasSuffix(path, "/") {
		return false
	}
	if public {
		if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "/v1/") {
			return false
		}
	} else if path != "/metrics" && !strings.HasPrefix(path, "/v1/") {
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

func validParameters(parameters []Parameter, location string) bool {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if !identity.Identifier(parameter.Name) || parameter.In != location || seen[parameter.Name] || (parameter.Description != "" && !line(parameter.Description)) || (parameter.Type != "string" && parameter.Type != "integer" && parameter.Type != "number" && parameter.Type != "boolean") || parameter.Min < 0 || parameter.Max < 0 || parameter.Max > 0 && parameter.Min > parameter.Max || (parameter.Format != "" && !line(parameter.Format)) || (parameter.Pattern != "" && !line(parameter.Pattern)) {
			return false
		}
		seen[parameter.Name] = true
	}
	return true
}
func cloneDefinition(d Definition) Definition {
	d.Errors = append([]ErrorResponse(nil), d.Errors...)
	d.Query = append([]Parameter(nil), d.Query...)
	d.Headers = append([]Parameter(nil), d.Headers...)
	// The private schema backing is immutable, but the exported wrapper can be
	// overwritten by callers. Detach it at both input and output boundaries.
	if d.Request != nil {
		request := *d.Request
		d.Request = &request
	}
	if d.Response != nil {
		response := *d.Response
		d.Response = &response
	}
	return d
}

// OpenAPI generates a self-contained OpenAPI 3.1.1 document from this exact
// registry. It adds no routes and grants no bearer or resource authority.
func (r *Registry) OpenAPI(title, version string) ([]byte, error) {
	return r.OpenAPIAt(title, version, "/")
}

// OpenAPIAt generates a self-contained OpenAPI 3.1.1 document and pins its
// server URL to the configured HTTP base path.
func (r *Registry) OpenAPIAt(title, version, basePath string) ([]byte, error) {
	if r == nil || !line(title) || !line(version) {
		return nil, ErrRegistration
	}
	if !validBasePath(basePath) {
		return nil, ErrRegistration
	}
	paths := map[string]map[string]any{}
	for _, d := range r.definitions {
		if paths[d.Path] == nil {
			paths[d.Path] = map[string]any{}
		}
		responses := map[string]any{}
		response := map[string]any{"description": "Successful response"}
		if d.Method != http.MethodHead {
			response["content"] = contentType(d.ResponseContentType, d.Response.Document())
		}
		responses[statusString(defaultStatus(d))] = response
		byStatus := map[int][]string{}
		receiptByStatus := map[int]bool{}
		for _, e := range d.Errors {
			byStatus[e.Status] = append(byStatus[e.Status], e.Code)
			receiptByStatus[e.Status] = receiptByStatus[e.Status] || e.Receipt
		}
		var receiptDocument any
		for status := range receiptByStatus {
			if !receiptByStatus[status] {
				continue
			}
			receipt, err := SchemaFor("gatewayErrorReceipt", reflect.TypeFor[gateway.Receipt](), true)
			if err != nil || json.Unmarshal(receipt.Document(), &receiptDocument) != nil {
				return nil, ErrRegistration
			}
			break
		}
		for status, codes := range byStatus {
			sort.Strings(codes)
			properties := map[string]any{"error": map[string]any{"type": "string", "enum": codes}}
			if receiptByStatus[status] {
				properties["receipt"] = receiptDocument
			}
			schema := map[string]any{"type": "object", "properties": properties, "required": []string{"error"}, "additionalProperties": false}
			responses[statusString(status)] = map[string]any{"description": http.StatusText(status), "content": content(schema)}
		}
		// The existing router's authenticated wrong-method response has no body.
		responses["405"] = map[string]any{"description": "Method not allowed; empty response body"}
		op := map[string]any{"operationId": d.ID, "summary": d.Summary, "responses": responses, "x-chartworks-auth": "bearer", "x-chartworks-effect": d.Effect, "x-chartworks-resource-loader": d.ResourceLoader, "x-chartworks-audit": d.Audit}
		if d.Public {
			op["x-chartworks-auth"] = "none"
		} else {
			op["security"] = []any{map[string]any{"penguiBearer": []string{}}}
			op["x-chartworks-action"] = d.Action
		}
		if strings.Contains(d.Path, "{id}") {
			op["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9_.:-]+$"}}}
		}
		for _, parameter := range append(append([]Parameter(nil), d.Query...), d.Headers...) {
			op["parameters"] = appendParameter(op["parameters"], parameter)
		}
		if d.Request != nil {
			media := d.RequestContentType
			if media == "" {
				media = "application/json"
			}
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{media: map[string]any{"schema": d.Request.Document()}}}
			op["x-chartworks-max-body-bytes"] = d.MaxBodyBytes
		}
		paths[d.Path][strings.ToLower(d.Method)] = op
	}
	document := map[string]any{"openapi": "3.1.1", "info": map[string]any{"title": title, "version": version}, "servers": []any{map[string]any{"url": basePath}}, "paths": paths, "components": map[string]any{"securitySchemes": map[string]any{"penguiBearer": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT", "description": "Existing Pengui bearer; signed operation and resource reach remain required."}}}}
	return json.MarshalIndent(document, "", "  ")
}

func content(schema any) map[string]any {
	return contentType("", schema)
}

func contentType(media string, schema any) map[string]any {
	if media == "" {
		media = "application/json"
	}
	return map[string]any{media: map[string]any{"schema": schema}}
}

func appendParameter(existing any, parameter Parameter) any {
	parameters, _ := existing.([]any)
	item := map[string]any{"name": parameter.Name, "in": parameter.In, "required": parameter.Required, "schema": map[string]any{"type": parameter.Type}}
	if parameter.Description != "" {
		item["description"] = parameter.Description
	}
	schema := item["schema"].(map[string]any)
	if parameter.Format != "" {
		schema["format"] = parameter.Format
	}
	if parameter.Type == "string" {
		if parameter.Min > 0 {
			schema["minLength"] = parameter.Min
		}
		if parameter.Max > 0 {
			schema["maxLength"] = parameter.Max
		}
	} else {
		if parameter.Min > 0 {
			schema["minimum"] = parameter.Min
		}
		if parameter.Max > 0 {
			schema["maximum"] = parameter.Max
		}
	}
	if parameter.Pattern != "" {
		schema["pattern"] = parameter.Pattern
	}
	return append(parameters, item)
}

func defaultStatus(d Definition) int {
	return 200
}

func validBasePath(path string) bool {
	if path == "/" {
		return true
	}
	if len(path) > 64 || !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, "?#\\\x00\r\n\t") {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" || !identity.Identifier(part) {
			return false
		}
	}
	return true
}
func statusString(status int) string {
	return strconv.Itoa(status)
}
