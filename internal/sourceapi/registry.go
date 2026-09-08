package sourceapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/sources"
)

// Operation aliases the shared manifest projection for existing downstream
// registries and consumers. Each source family supplies shared definitions.
type Operation = api.Operation

const sourceRequestMaxBytes = 65536

type rotateRequest struct {
	Expected int64 `json:"expected_revision"`
}

type registration struct {
	op                         Operation
	id, summary, loader, audit string
	request, response          reflect.Type
}

// SourceRegistry is the sole inventory for the seven existing source routes.
// The router, legacy manifest projection, and OpenAPI use these same definitions.
// Resource loader/audit labels identify existing domain/store behavior; they do
// not move authorization or transaction boundaries into this metadata package.
func SourceRegistry(warehouse, validation bool) (*api.Registry, error) {
	definitions := []registration{
		{Operation{"GET", "/v1/sources", "sources.read", "metadata_read"}, "listSources", "List authorized retained source registrations", "sources.Service.List", "read_only_no_domain_audit", nil, reflect.TypeFor[[]sources.Source]()},
		{Operation{"GET", "/v1/sources/{id}", "sources.read", "metadata_read"}, "getSource", "Read a retained source registration", "sources.Service.Get", "read_only_no_domain_audit", nil, reflect.TypeFor[sources.Source]()},
	}
	if warehouse {
		definitions = append(definitions,
			registration{Operation{"POST", "/v1/sources", "sources.write", "source_registration"}, "createSource", "Register an approved source alias", "sources.Service.Create", "source.created", reflect.TypeFor[sources.CreateRequest](), reflect.TypeFor[sources.Source]()},
			registration{Operation{"POST", "/v1/sources/{id}/test", "sources.read", "warehouse_catalog_read"}, "testSource", "Probe the current read-only source context", "sources.Service.Test", "read_only_no_domain_audit", reflect.TypeFor[struct{}](), reflect.TypeFor[sources.Status]()},
			registration{Operation{"GET", "/v1/sources/{id}/schema", "sources.read", "warehouse_catalog_read"}, "discoverSource", "Discover the configured source relations", "sources.Service.Discover", "read_only_no_domain_audit", nil, reflect.TypeFor[sources.Discovery]()},
			registration{Operation{"POST", "/v1/sources/{id}/rotate", "sources.rotate", "context_rotation"}, "rotateSource", "Rotate a source at its expected revision", "sources.Service.Rotate", "source.rotated", reflect.TypeFor[rotateRequest](), reflect.TypeFor[sources.Source]()},
		)
		if validation {
			definitions = append(definitions, registration{Operation{"POST", "/v1/sources/{id}/validate", "sources.query", "bounded_native_planning"}, "validateRead", "Validate a read without executing result work", "exec.Validator.Validate", "read_only_no_domain_audit", reflect.TypeFor[ValidationRequest](), reflect.TypeFor[readexec.Receipt]()})
		}
	}
	return compileRegistrations(definitions, sourceErrors(), sourceRequestMaxBytes, false, 0)
}

// Registry retains the checked legacy method/path/action/effect projection.
func Registry(warehouse, validation bool) []Operation {
	r, err := SourceRegistry(warehouse, validation)
	if err != nil {
		return nil
	}
	return r.Operations()
}

func sourceErrors() []api.ErrorResponse {
	return []api.ErrorResponse{{400, "invalid_request"}, {401, "unauthenticated"}, {401, "unauthorized"}, {403, "forbidden"}, {404, "not_found"}, {409, "conflict"}, {409, "context_changed"}, {413, "limit_exceeded"}, {422, "sql_unsafe"}, {422, "unsupported"}, {503, "unavailable"}, {504, "cancelled_or_timed_out"}}
}

// uploadContent is a registration marker for the existing raw binary body.
type uploadContent struct{}

func compileRegistrations(definitions []registration, errors []api.ErrorResponse, maxBody int, optional bool, uploadBytes int64) (*api.Registry, error) {
	var options []api.SchemaOption
	if optional {
		options = []api.SchemaOption{api.OptionalJSONFields}
	}
	out := make([]api.Definition, 0, len(definitions))
	for _, item := range definitions {
		response, err := api.SchemaFor(item.id+"Response", item.response, true)
		if err != nil {
			return nil, err
		}
		d := api.Definition{Operation: item.op, ID: item.id, Summary: item.summary, ResourceLoader: item.loader, Audit: item.audit, Response: response, Errors: errors}
		if item.request == reflect.TypeFor[uploadContent]() {
			d.Request, err = gateway.NewSchema(item.id+"Request", []byte(`{"type":"string","format":"binary"}`))
			if err != nil {
				return nil, err
			}
			d.RequestContentType = "application/octet-stream"
			d.MaxBodyBytes = int(uploadBytes)
		} else if item.request != nil {
			d.Request, err = api.SchemaFor(item.id+"Request", item.request, false, options...)
			if err != nil {
				return nil, err
			}
			d.MaxBodyBytes = maxBody
		}
		out = append(out, d)
	}
	return api.New(out)
}
