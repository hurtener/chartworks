package foundation

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/gateway"
)

// HealthResponse is the public liveness response.
type HealthResponse struct {
	Live bool `json:"live"`
}

// ReadyResponse is the public dependency-readiness response.
type ReadyResponse struct {
	Ready        bool              `json:"ready"`
	Dependencies map[string]string `json:"dependencies"`
}

// CapabilitiesResponse describes only capabilities wired into the running
// composition. It contains no identity, resource or credential material.
type CapabilitiesResponse struct {
	Phase          string   `json:"phase"`
	Implemented    []string `json:"implemented"`
	BusinessAPI    bool     `json:"business_api"`
	Authentication bool     `json:"authentication"`
}

func publicErrors() []api.ErrorResponse {
	return []api.ErrorResponse{
		{Status: 400, Code: "invalid_request"},
		{Status: 405, Code: "method_not_allowed"},
		{Status: 413, Code: "request_too_large"},
		{Status: 503, Code: "unavailable"},
	}
}

// PublicRegistry supplies health, capability and document operations to the
// same composed OpenAPI inventory as protected domain routes.
func PublicRegistry() (*api.Registry, error) {
	health, err := api.SchemaFor("healthResponse", reflect.TypeFor[HealthResponse](), true)
	if err != nil {
		return nil, err
	}
	ready, err := api.SchemaFor("readyResponse", reflect.TypeFor[ReadyResponse](), true)
	if err != nil {
		return nil, err
	}
	capabilities, err := api.SchemaFor("capabilitiesResponse", reflect.TypeFor[CapabilitiesResponse](), true)
	if err != nil {
		return nil, err
	}
	openapi, err := gateway.NewSchema("openapiResponse", []byte(`{"type":"object","additionalProperties":true}`))
	if err != nil {
		return nil, err
	}
	errors := publicErrors()
	definitions := []api.Definition{
		{Operation: api.Operation{Method: "GET", Path: "/healthz", Effect: "public_read"}, ID: "health", Summary: "Read liveness status", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: health, Errors: errors},
		{Operation: api.Operation{Method: "HEAD", Path: "/healthz", Effect: "public_read"}, ID: "healthHead", Summary: "Read liveness status headers", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: health, Errors: errors},
		{Operation: api.Operation{Method: "GET", Path: "/readyz", Effect: "public_read"}, ID: "ready", Summary: "Read dependency readiness status", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: ready, Errors: errors},
		{Operation: api.Operation{Method: "HEAD", Path: "/readyz", Effect: "public_read"}, ID: "readyHead", Summary: "Read dependency readiness headers", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: ready, Errors: errors},
		{Operation: api.Operation{Method: "GET", Path: "/capabilities", Effect: "public_read"}, ID: "capabilities", Summary: "Read implemented capability metadata", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: capabilities, Errors: errors},
		{Operation: api.Operation{Method: "HEAD", Path: "/capabilities", Effect: "public_read"}, ID: "capabilitiesHead", Summary: "Read capability metadata headers", ResourceLoader: "foundation.Server.Handler", Audit: "read_only_no_domain_audit", Public: true, Response: capabilities, Errors: errors},
		{Operation: api.Operation{Method: "GET", Path: "/openapi.json", Effect: "public_read"}, ID: "openapi", Summary: "Read the generated HTTP API document", ResourceLoader: "foundation.Server.OpenAPI", Audit: "read_only_no_domain_audit", Public: true, Response: openapi, Errors: errors},
		{Operation: api.Operation{Method: "HEAD", Path: "/openapi.json", Effect: "public_read"}, ID: "openapiHead", Summary: "Read generated HTTP API document headers", ResourceLoader: "foundation.Server.OpenAPI", Audit: "read_only_no_domain_audit", Public: true, Response: openapi, Errors: errors},
	}
	return api.New(definitions)
}
