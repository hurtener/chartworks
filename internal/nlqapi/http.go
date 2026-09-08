// Package nlqapi exposes the bounded NLQ routing consumer through the shared
// operation registry. It performs bearer verification and delegates all domain
// checks to nlqroute.Service.
package nlqapi

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
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// MaxBodyBytes bounds one route request before JSON decoding or domain work.
const MaxBodyBytes = 2 << 20

// Registry returns the operation metadata and generated request/response
// schemas for the first actual routing consumer.
func Registry() (*api.Registry, error) {
	response, err := api.SchemaFor("routeNLQResponse", reflect.TypeFor[nlqroute.RouteResult](), true)
	if err != nil {
		return nil, err
	}
	request, err := api.SchemaFor("routeNLQRequest", reflect.TypeFor[nlqroute.RouteRequest](), false, api.OptionalJSONFields)
	if err != nil {
		return nil, err
	}
	return api.New([]api.Definition{{
		Operation:      api.Operation{Method: http.MethodPost, Path: "/v1/nlq/routes", Action: "topics.read", Effect: "gateway_retrieval_context"},
		ID:             "routeNLQ",
		Summary:        "Route an authorized question into bounded semantic context",
		ResourceLoader: "nlqroute.Service.Route",
		Audit:          "read_only_no_domain_audit",
		MaxBodyBytes:   MaxBodyBytes,
		Request:        request,
		Response:       response,
		Errors: []api.ErrorResponse{
			{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 401, Code: "unauthorized"},
			{Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"},
			{Status: 409, Code: "conflict"}, {Status: 409, Code: "context_changed"},
			{Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "insufficient_context"},
			{Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"},
		},
	}})
}

// Handler verifies the Pengui bearer and routes the registered operation.
func Handler(verifier *auth.Verifier, service *nlqroute.Service, next http.Handler) http.Handler {
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
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selected, _, known := registry.Match(r.Method, r.URL.Path)
		if selected.Path == "" {
			if known {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has(selected.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			failure(w, store.ErrInvalid)
			return
		}
		var in nlqroute.RouteRequest
		if err := decodeBody(w, r, selected.Request, &in); err != nil {
			failure(w, err)
			return
		}
		out, err := service.Route(r.Context(), e, in)
		if err != nil {
			failure(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nlq/routes" {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, schema interface{ Validate([]byte, int) error }, out any) error {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) != 0 {
		return store.ErrInvalid
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return readexec.ErrLimit
		}
		return store.ErrInvalid
	}
	if schema == nil || schema.Validate(b, MaxBodyBytes) != nil || json.Unmarshal(b, out) != nil {
		return store.ErrInvalid
	}
	return nil
}

func failure(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = http.StatusUnauthorized, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, access.ErrNotFound), errors.Is(err, store.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrInvalid), errors.Is(err, semantics.ErrInvalid), errors.Is(err, nlqroute.ErrInvalid):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, store.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, readexec.ErrBinding):
		status, code = http.StatusConflict, "context_changed"
	case errors.Is(err, readexec.ErrLimit):
		status, code = http.StatusRequestEntityTooLarge, "limit_exceeded"
	case errors.Is(err, nlq.ErrInsufficient):
		status, code = http.StatusUnprocessableEntity, "insufficient_context"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = http.StatusGatewayTimeout, "cancelled_or_timed_out"
	case errors.Is(err, gateway.ErrSpace):
		status, code = http.StatusConflict, "context_changed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
