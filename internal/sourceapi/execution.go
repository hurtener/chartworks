package sourceapi

import (
	"encoding/json"
	"net/http"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ExecutionRequest is a closed validated-SQL submission, not a serialized plan.
type ExecutionRequest struct {
	Context    string               `json:"context"`
	SQL        string               `json:"sql"`
	Parameters []readexec.Parameter `json:"parameters"`
	Execution  readexec.Options     `json:"execution"`
}

// ExecutionRegistry is the actual route/action/effect inventory for the read core.
func ExecutionRegistry() []Operation {
	r, err := ExecutionAPIRegistry()
	if err != nil {
		return nil
	}
	return r.Operations()
}

// ExecutionHandler adds thin execution/attempt operations to the existing surface.
// Every submitted SQL string is validated server-side before reaching Executor.
func ExecutionHandler(verifier *auth.Verifier, validator *readexec.Validator, executor *readexec.Executor, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if validator == nil || executor == nil {
		return next
	}
	registry, registrationErr := ExecutionAPIRegistry()
	if registrationErr != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { failure(w, registrationErr) })
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		op, id, _ := registry.Match(r.Method, r.URL.Path)
		if op.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !e.Has(op.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" || r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
			failure(w, store.ErrInvalid)
			return
		}
		var out any
		switch op.Path {
		case "/v1/read-operations/{id}":
			out, err = executor.ByOperation(r.Context(), e, id)
		case "/v1/sources/{id}/execute":
			var input ExecutionRequest
			if err = body(w, r, &input); err == nil {
				err = executor.ValidateOptions(input.Execution)
			}
			if err == nil {
				var p readexec.Plan
				p, err = validator.Validate(r.Context(), e, readexec.Request{Source: id, Context: input.Context, SQL: input.SQL, Parameters: input.Parameters})
				if err == nil {
					out, err = executor.Execute(r.Context(), e, p, input.Execution)
				}
			}
		case "/v1/read-executions/{id}":
			out, err = executor.Inspect(r.Context(), e, id)
		default:
			var empty struct{}
			if err = body(w, r, &empty); err == nil {
				out, err = executor.Control(r.Context(), e, id, op.Path == "/v1/read-executions/{id}/cancel")
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
