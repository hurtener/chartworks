package workapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// Operation is the actual route's side-effect and authority contract. This registry is
// also consumed by the exhaustive endpoint-denial and SDK parity checks.
type Operation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Effect string `json:"effect"`
}

func Registry(models, queue bool) []Operation {
	out := []Operation{}
	if models {
		out = append(out, Operation{Method: "POST", Path: "/v1/gateway/probes", Action: "ops.model", Effect: "paid_model_call"})
	}
	if queue {
		out = append(out,
			Operation{Method: "POST", Path: "/v1/jobs", Action: "scheduling.write", Effect: "durable_admission"},
			Operation{Method: "GET", Path: "/v1/jobs", Action: "scheduling.read", Effect: "metadata_read"},
			Operation{Method: "GET", Path: "/v1/jobs/{id}", Action: "scheduling.read", Effect: "metadata_read"},
			Operation{Method: "POST", Path: "/v1/jobs/{id}/cancel", Action: "scheduling.cancel", Effect: "durable_cancellation"},
			Operation{Method: "POST", Path: "/v1/schedules", Action: "scheduling.write", Effect: "schedule_creation"},
			Operation{Method: "GET", Path: "/v1/schedules/{id}", Action: "scheduling.read", Effect: "metadata_read"},
			Operation{Method: "PUT", Path: "/v1/schedules/{id}/state", Action: "scheduling.write", Effect: "schedule_state"},
			Operation{Method: "POST", Path: "/v1/schedules/{id}/runs", Action: "scheduling.execute", Effect: "durable_admission"},
		)
	}
	return out
}

// Handler wraps every new operation in the existing Pengui verifier. Disabled capabilities
// have no routes. Existing unrelated routes are delegated unchanged to the supplied handler.
func Handler(v *auth.Verifier, engine gateway.Engine, queue *jobs.Service, next http.Handler) http.Handler {
	if v == nil || next == nil {
		return http.NotFoundHandler()
	}
	registered := Registry(engine != nil, queue != nil)
	protected := v.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := identity.FromContext(r.Context())
		if err != nil {
			fail(w, access.ErrUnauthenticated, nil)
			return
		}
		var selected Operation
		id := ""
		for _, op := range registered {
			if candidate, ok := match(op.Path, r.URL.Path); ok && r.Method == op.Method {
				selected = op
				id = candidate
				break
			}
		}
		if selected.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Check the operation before parsing bodies or loading resource metadata. Services then
		// require actual target/parent/binding reach, and repository SQL applies the tenant predicate.
		if !e.Has(selected.Action) {
			fail(w, access.ErrForbidden, nil)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			fail(w, jobs.ErrInvalid, nil)
			return
		}
		if r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
			fail(w, jobs.ErrInvalid, nil)
			return
		}
		var out any
		switch selected.Path {
		case "/v1/gateway/probes":
			var input ProbeRequest
			if err = body(w, r, &input); err == nil {
				var result ProbeResult
				result, err = Probe(r.Context(), e, engine, input)
				out = result
				if err != nil {
					fail(w, err, &result.Receipt)
					return
				}
			}
		case "/v1/jobs":
			if r.Method == http.MethodGet {
				out, err = queue.List(r.Context(), e, 100)
			} else {
				var input jobs.Submission
				key, keyErr := idempotency(r)
				if keyErr != nil {
					err = keyErr
				} else if err = body(w, r, &input); err == nil {
					out, err = queue.Submit(r.Context(), e, key, input)
				}
			}
		case "/v1/jobs/{id}":
			out, err = queue.Get(r.Context(), e, id)
		case "/v1/jobs/{id}/cancel":
			var empty struct{}
			if err = body(w, r, &empty); err == nil {
				out, err = queue.Cancel(r.Context(), e, id)
			}
		case "/v1/schedules":
			var input jobs.ScheduleRequest
			key, keyErr := idempotency(r)
			if keyErr != nil {
				err = keyErr
			} else if err = body(w, r, &input); err == nil {
				out, err = queue.CreateSchedule(r.Context(), e, key, input)
			}
		case "/v1/schedules/{id}":
			out, err = queue.GetSchedule(r.Context(), e, id)
		case "/v1/schedules/{id}/state":
			var input struct {
				Expected int64 `json:"expected_revision"`
				Enabled  bool  `json:"enabled"`
			}
			if err = body(w, r, &input); err == nil {
				out, err = queue.SetSchedule(r.Context(), e, id, input.Expected, input.Enabled)
			}
		case "/v1/schedules/{id}/runs":
			var empty struct{}
			key, keyErr := idempotency(r)
			if keyErr != nil {
				err = keyErr
			} else if err = body(w, r, &empty); err == nil {
				out, err = queue.Fire(r.Context(), e, id, key)
			}
		}
		if err != nil {
			fail(w, err, nil)
			return
		}
		// An accepted job is returned with its truthful pending/running/terminal state. 200 also
		// covers an idempotent replay; acceptance is never advertised as completed execution.
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		found := false
		for _, op := range registered {
			if _, ok := match(op.Path, r.URL.Path); ok {
				found = true
				break
			}
		}
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
func match(pattern, path string) (string, bool) {
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
		} else if part != got[i] {
			return "", false
		}
	}
	return id, true
}
func idempotency(r *http.Request) (string, error) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || !identity.Identifier(values[0]) {
		return "", jobs.ErrInvalid
	}
	return values[0], nil
}
func body(w http.ResponseWriter, r *http.Request, out any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return jobs.ErrInvalid
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		return jobs.ErrInvalid
	}
	if closed(data, reflect.TypeOf(out).Elem()) != nil {
		return jobs.ErrInvalid
	}
	if json.Unmarshal(data, out) != nil {
		return jobs.ErrInvalid
	}
	return nil
}

// closed enforces exact nested field names, duplicate-free JSON and required non-omitempty
// fields. time.Time and other scalar custom decoders keep their ordinary JSON semantics.
func closed(data []byte, typ reflect.Type) error {
	if _, err := gateway.DecodeJSON(data, 8192); err != nil {
		return jobs.ErrInvalid
	}
	if typ.Kind() != reflect.Struct || typ.PkgPath() == "time" {
		return nil
	}
	object, err := auth.Object(data, 8192)
	if err != nil {
		return jobs.ErrInvalid
	}
	expected := map[string]reflect.StructField{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		parts := strings.Split(f.Tag.Get("json"), ",")
		if parts[0] == "-" {
			continue
		}
		name := parts[0]
		if name == "" {
			name = f.Name
		}
		expected[name] = f
		if len(parts) == 1 && object[name] == nil {
			return jobs.ErrInvalid
		}
	}
	for name, value := range object {
		f, ok := expected[name]
		if !ok {
			return jobs.ErrInvalid
		}
		if closed(value, f.Type) != nil {
			return jobs.ErrInvalid
		}
	}
	return nil
}
func fail(w http.ResponseWriter, err error, receipt *gateway.Receipt) {
	status, code := http.StatusServiceUnavailable, "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status, code = 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, store.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, store.ErrExpired):
		status, code = 410, "expired"
	case errors.Is(err, jobs.ErrInvalid) || errors.Is(err, gateway.ErrInput) || errors.Is(err, store.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, gateway.ErrDisabled):
		status, code = 409, "role_disabled"
	case errors.Is(err, jobs.ErrBusy) || errors.Is(err, gateway.ErrBusy) || errors.Is(err, gateway.ErrBudget):
		status, code = 429, "budget_exceeded"
		w.Header().Set("Retry-After", strconv.Itoa(1))
	case errors.Is(err, gateway.ErrOutput) || errors.Is(err, gateway.ErrSpace):
		status, code = 502, "invalid_provider_output"
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		status, code = 504, "cancelled_or_timed_out"
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    string           `json:"error"`
		Receipt *gateway.Receipt `json:"receipt,omitempty"`
	}{code, receipt})
}
