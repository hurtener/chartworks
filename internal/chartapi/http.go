// Package chartapi provides thin, registered HTTP consumers for output specs.
// Authenticated inputs are caller-supplied data, not references granting access.
package chartapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// MaxBodyBytes includes both data and an exact saved definition; core limits are
// independently enforced before any specification or ranking is constructed.
const MaxBodyBytes = 10 << 20

// Registry is the only operation/schema inventory for these real consumers.
func Registry() (*api.Registry, error) {
	entries := []struct {
		path, action, id, summary string
		in, out                   reflect.Type
	}{
		{"/v1/charts/catalog", "charts.read", "chartCatalog", "Read the fourteen-kind output specification catalog", nil, reflect.TypeFor[chartservice.CatalogResult]()},
		{"/v1/charts/select", "charts.select", "selectChart", "Choose a deterministic suitable specification; optional bounded author ranking", reflect.TypeFor[chartservice.SelectRequest](), reflect.TypeFor[chartservice.SelectionResult]()},
		{"/v1/charts/specify", "charts.bind", "specifyChart", "Build one explicitly selected kind without fallback", reflect.TypeFor[chartservice.SpecifyRequest](), reflect.TypeFor[chartservice.BuildResult]()},
		{"/v1/charts/build", "charts.bind", "buildChart", "Apply an exact saved mapping without selection or source access", reflect.TypeFor[chartservice.BuildRequest](), reflect.TypeFor[chartservice.BuildResult]()},
		{"/v1/charts/rebind", "charts.bind", "rebindChart", "Propose an unambiguous semantic rebind for explicit review", reflect.TypeFor[chartservice.BuildRequest](), reflect.TypeFor[charts.Proposal]()},
	}
	definitions := make([]api.Definition, 0, len(entries))
	for _, entry := range entries {
		response, err := api.SchemaFor(entry.id+"Response", entry.out, true)
		if err != nil {
			return nil, err
		}
		d := api.Definition{Operation: api.Operation{Method: http.MethodGet, Path: entry.path, Action: entry.action, Effect: "caller_data_transform_no_persistence"}, ID: entry.id, Summary: entry.summary, ResourceLoader: "chartservice.Service: current signed tenant read reach", Audit: "read_only_no_domain_audit; optional rank usage receipt", Response: response, Errors: []api.ErrorResponse{
			{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "mapping_changed"}, {Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "unsuitable_binding"}, {Status: 429, Code: "busy"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}}
		if entry.in != nil {
			d.Method = http.MethodPost
			d.MaxBodyBytes = MaxBodyBytes
			d.Request, err = api.SchemaFor(entry.id+"Request", entry.in, false, api.NullableCollections)
			if err != nil {
				return nil, err
			}
		}
		if entry.id == "selectChart" {
			d.Effect = "caller_data_selection_optional_gateway_rank"
			for i := range d.Errors {
				if d.Errors[i].Status == 401 || d.Errors[i].Status == 504 {
					d.Errors[i].Receipt = true
				}
			}
		}
		definitions = append(definitions, d)
	}
	return api.New(definitions)
}

// Handler verifies the bearer before decoding and delegates all resource and
// shape checks to the common service. Missing dependencies expose no capability.
func Handler(verifier *auth.Verifier, service *chartservice.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, err := Registry()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	requests := make(chan struct{}, service.ConcurrencyLimit())
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selected, _, _ := registry.Match(r.Method, r.URL.Path)
		if selected.Path == "" {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if err = access.Require(e, selected.Action, access.Tenant(e, "read")); err != nil {
			failure(w, err)
			return
		}
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
		default:
			failure(w, chartservice.ErrBusy)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			failure(w, charts.ErrInvalid)
			return
		}
		var out any
		switch selected.ID {
		case "chartCatalog":
			// Reject even chunked GET bodies rather than letting content bypass limits.
			b, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
			if readErr != nil || len(b) > 0 {
				failure(w, charts.ErrInvalid)
				return
			}
			out, err = service.Catalog(r.Context(), e)
		case "selectChart":
			var in chartservice.SelectRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Select(r.Context(), e, in)
			}
		case "specifyChart":
			var in chartservice.SpecifyRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Specify(r.Context(), e, in)
			}
		case "buildChart":
			var in chartservice.BuildRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Build(r.Context(), e, in)
			}
		case "rebindChart":
			var in chartservice.BuildRequest
			err = decode(w, r, selected, &in)
			if err == nil {
				out, err = service.Rebind(r.Context(), e, in)
			}
		default:
			err = charts.ErrInvalid
		}
		if err != nil {
			failure(w, err)
			return
		}
		headers(w)
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, found := registry.Match(r.Method, r.URL.Path)
		if !found {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func decode(w http.ResponseWriter, r *http.Request, d api.Definition, out any) error {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) != 0 {
		return charts.ErrInvalid
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.MaxBodyBytes)))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return charts.ErrLimit
		}
		return charts.ErrInvalid
	}
	if d.Request == nil || d.Request.Validate(b, d.MaxBodyBytes) != nil || json.Unmarshal(b, out) != nil {
		return charts.ErrInvalid
	}
	return nil
}
func headers(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func failureStatus(w http.ResponseWriter, status int, code string) {
	headers(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
func failure(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, charts.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, charts.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, charts.ErrUnsuitable):
		status, code = 422, "unsuitable_binding"
	case errors.Is(err, charts.ErrMappingChanged):
		status, code = 409, "mapping_changed"
	case errors.Is(err, chartservice.ErrBusy):
		status, code = 429, "busy"
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	var interrupted *chartservice.Failure
	if errors.As(err, &interrupted) {
		headers(w)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(struct {
			Error   string          `json:"error"`
			Receipt gateway.Receipt `json:"receipt"`
		}{code, interrupted.Receipt})
		return
	}
	failureStatus(w, status, code)
}
