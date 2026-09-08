package nlqapi

import (
	"encoding/json"
	"net/http"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
)

// ExecutionHandler exposes the existing NLQ service through the shared
// authenticated HTTP boundary. It does not construct authority or execute SQL
// itself; the service retains all route, gateway, validation and session checks.
func ExecutionHandler(verifier *auth.Verifier, service *nlqexec.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, err := ExecutionRegistry()
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
		// Examples are intentionally readable by either planning or feedback
		// callers, matching nlqexec.Service.Examples' least-privilege contract.
		if !e.Has(selected.Action) && !(selected.ID == "examplesNLQ" && e.Has("feedback.write")) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			failure(w, store.ErrInvalid)
			return
		}

		var out any
		switch selected.ID {
		case "preflightNLQ":
			var in nlqexec.PreflightRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Preflight(r.Context(), e, in)
			}
		case "planNLQ":
			var in nlqexec.PlanRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Plan(r.Context(), e, in)
			}
		case "runNLQ":
			var in nlqexec.RunRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Run(r.Context(), e, in)
			}
		case "refineNLQ":
			var in nlqexec.RefineRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Refine(r.Context(), e, in)
			}
		case "feedbackNLQ":
			var in nlqexec.FeedbackRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				err = service.Feedback(r.Context(), e, in)
				out = FeedbackResult{Accepted: err == nil}
			}
		case "exampleStateNLQ":
			var in nlqexec.ExampleStateRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.ExampleState(r.Context(), e, in)
			}
		case "examplesNLQ":
			var in ExampleListRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Examples(r.Context(), e, in.Topic, in.Limit)
			}
		default:
			err = store.ErrInvalid
		}
		if err != nil {
			failure(w, err)
			return
		}
		writeJSON(w, out)
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

func writeJSON(w http.ResponseWriter, out any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(out)
}
