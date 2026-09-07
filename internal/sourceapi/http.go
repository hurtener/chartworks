// Package sourceapi provides thin, authenticated registry and validation HTTP surfaces.
package sourceapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// ValidationRequest never accepts a caller-issued plan, tenant, dialect or safety override.
type ValidationRequest struct {
	Context    string               `json:"context"`
	SQL        string               `json:"sql"`
	Parameters []readexec.Parameter `json:"parameters"`
}

// Handler verifies Pengui authority before body decoding and delegates all domain logic.
// Validation returns a receipt only; an HTTP response cannot deserialize into an executable Plan.
func Handler(verifier *auth.Verifier, service *sources.Service, validator *readexec.Validator, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, registrationErr := SourceRegistry(service.Enabled(), validator != nil)
	if registrationErr != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { failure(w, registrationErr) })
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		selected, id, _ := registry.Match(r.Method, r.URL.Path)
		if selected.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !e.Has(selected.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" || r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
			failure(w, store.ErrInvalid)
			return
		}
		var out any
		switch selected.Path {
		case "/v1/sources":
			if r.Method == http.MethodGet {
				out, err = service.List(r.Context(), e, 100)
			} else {
				var input sources.CreateRequest
				if err = body(w, r, &input); err == nil {
					out, err = service.Create(r.Context(), e, input)
				}
			}
		case "/v1/sources/{id}":
			out, err = service.Get(r.Context(), e, id)
		case "/v1/sources/{id}/test":
			var empty struct{}
			if err = body(w, r, &empty); err == nil {
				out, err = service.Test(r.Context(), e, id)
			}
		case "/v1/sources/{id}/schema":
			out, err = service.Discover(r.Context(), e, id)
		case "/v1/sources/{id}/rotate":
			var input rotateRequest
			if err = body(w, r, &input); err == nil {
				out, err = service.Rotate(r.Context(), e, id, input.Expected)
			}
		case "/v1/sources/{id}/validate":
			var input ValidationRequest
			if err = body(w, r, &input); err == nil {
				var plan readexec.Plan
				plan, err = validator.Validate(r.Context(), e, readexec.Request{Source: id, Context: input.Context, SQL: input.SQL, Parameters: input.Parameters})
				out = plan.Receipt()
			}
		}
		if err != nil {
			failure(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, found := registry.Match(r.Method, r.URL.Path)
		if !found {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		protected.ServeHTTP(w, r)
	})
}
func match(pattern, path string) (string, bool) { return api.MatchPath(pattern, path) }
func body(w http.ResponseWriter, r *http.Request, out any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sourceRequestMaxBytes))
	if err != nil {
		return store.ErrInvalid
	}
	if _, err = gateway.DecodeJSON(data, sourceRequestMaxBytes); err != nil {
		return store.ErrInvalid
	}
	if err = shape(data, reflect.TypeOf(out).Elem()); err != nil {
		return err
	}
	if json.Unmarshal(data, out) != nil {
		return store.ErrInvalid
	}
	return nil
}
func shape(data []byte, typ reflect.Type) error {
	if strings.TrimSpace(string(data)) == "null" {
		return store.ErrInvalid
	}
	if typ.Kind() == reflect.Slice {
		var elements []json.RawMessage
		if json.Unmarshal(data, &elements) != nil {
			return store.ErrInvalid
		}
		for _, item := range elements {
			if err := shape(item, typ.Elem()); err != nil {
				return err
			}
		}
		return nil
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	object, err := auth.Object(data, sourceRequestMaxBytes)
	if err != nil {
		return store.ErrInvalid
	}
	if len(object) != typ.NumField() {
		return store.ErrInvalid
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		value, ok := object[field.Tag.Get("json")]
		if !ok {
			return store.ErrInvalid
		}
		if err := shape(value, field.Type); err != nil {
			return err
		}
	}
	return nil
}
func failure(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, store.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, readexec.ErrReplay), errors.Is(err, store.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, readexec.ErrBinding):
		status, code = 409, "context_changed"
	case errors.Is(err, readexec.ErrUnsafe):
		status, code = 422, "sql_unsafe"
	case errors.Is(err, readexec.ErrType), errors.Is(err, readexec.ErrUnsupported):
		status, code = 422, "unsupported"
	case errors.Is(err, readexec.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
