// Package onboardingapi exposes the guided setup coordinator through the shared
// authenticated HTTP registry. The handler contains no orchestration logic.
package onboardingapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/store"
)

const MaxBodyBytes = 64 << 10

var errBodyLimit = errors.New("onboarding api: request body limit exceeded")

type route struct {
	method, path, id, action, summary string
	request, response                 reflect.Type
}

func Registry() (*api.Registry, error) {
	routes := []route{
		{"POST", "/v1/onboarding/goal-search", "searchBusinessGoal", "onboarding.read", "Search signed-reachable reviewed topics and source/profile candidates", reflect.TypeFor[onboarding.GoalSearchRequest](), reflect.TypeFor[onboarding.GoalSearchResult]()},
		{"POST", "/v1/onboarding/goal-choice", "chooseBusinessGoal", "onboarding.write", "Recheck a reviewed topic or create a private profile-backed draft", reflect.TypeFor[onboarding.GoalChoiceRequest](), reflect.TypeFor[onboarding.GoalChoiceResult]()},
		{"POST", "/v1/onboarding", "startOnboarding", "onboarding.write", "Start an actor-private resumable onboarding run", reflect.TypeFor[onboarding.StartRequest](), reflect.TypeFor[onboarding.Run]()},
		{"GET", "/v1/onboarding/{id}", "getOnboarding", "onboarding.read", "Read actor-private onboarding progress", nil, reflect.TypeFor[onboarding.Run]()},
		{"POST", "/v1/onboarding/resume", "resumeOnboarding", "onboarding.write", "Advance exactly one bounded onboarding stage", reflect.TypeFor[onboarding.ResumeRequest](), reflect.TypeFor[onboarding.Run]()},
		{"POST", "/v1/onboarding/answers", "answerOnboarding", "onboarding.write", "Answer unresolved choices or submit an external review reference", reflect.TypeFor[onboarding.AnswerRequest](), reflect.TypeFor[onboarding.Run]()},
		{"POST", "/v1/onboarding/cancel", "cancelOnboarding", "onboarding.cancel", "Cancel future onboarding work", reflect.TypeFor[onboarding.CancelRequest](), reflect.TypeFor[onboarding.Run]()},
		{"POST", "/v1/onboarding/drift", "proposeOnboardingDrift", "onboarding.write", "Create an affected-only private drift amendment", reflect.TypeFor[onboarding.DriftRequest](), reflect.TypeFor[onboarding.Amendment]()},
	}
	defs := make([]api.Definition, 0, len(routes))
	errs := []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 409, Code: "attention_required"}, {Status: 409, Code: "cancelled"}, {Status: 413, Code: "limit_exceeded"}, {Status: 429, Code: "budget_exhausted"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}
	for _, r := range routes {
		response, err := api.SchemaFor(r.id+"Response", r.response, true)
		if err != nil {
			return nil, err
		}
		d := api.Definition{Operation: api.Operation{Method: r.method, Path: r.path, Action: r.action, Effect: "durable_bounded_orchestration"}, ID: r.id, Summary: r.summary, ResourceLoader: "onboarding.Service enforces private run and every delegated source/context/domain target", Audit: "onboarding progress only; no prompts, SQL, rows, credentials or tokens", Replay: "never", Response: response, Errors: errs}
		if r.id == "searchBusinessGoal" {
			d.Effect = "bounded_authorized_catalog_read"
			d.ResourceLoader = "signed source/dataset/context and complete topic dependencies before candidate selection"
			d.Audit = "read_only_no_domain_audit"
		}
		if r.id == "chooseBusinessGoal" {
			d.Effect = "current_topic_reuse_or_private_draft_write"
			d.ResourceLoader = "current publication contract or exact active private profile and source revision"
			d.Audit = "ordinary topic draft save audit; reuse is read only"
		}
		if r.method == "GET" {
			d.Replay = "read"
			d.Effect = "private_progress_read"
			d.Audit = "read_only_no_domain_audit"
		} else {
			d.Request, err = api.SchemaFor(r.id+"Request", r.request, false, api.OptionalJSONFields)
			if err != nil {
				return nil, err
			}
			d.MaxBodyBytes = MaxBodyBytes
		}
		defs = append(defs, d)
	}
	return api.New(defs)
}

func Handler(verifier *auth.Verifier, service *onboarding.Service, next http.Handler) http.Handler {
	if verifier == nil || service == nil || next == nil {
		return http.NotFoundHandler()
	}
	registry, err := Registry()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
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
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			failure(w, store.ErrInvalid)
			return
		}
		var out any
		switch selected.ID {
		case "searchBusinessGoal":
			var in onboarding.GoalSearchRequest
			if err = body(r, &in); err == nil {
				out, err = service.SearchGoal(r.Context(), e, in)
			}
		case "chooseBusinessGoal":
			var in onboarding.GoalChoiceRequest
			if err = body(r, &in); err == nil {
				out, err = service.ChooseGoal(r.Context(), e, in)
			}
		case "startOnboarding":
			var in onboarding.StartRequest
			if err = body(r, &in); err == nil {
				out, err = service.Start(r.Context(), e, in)
			}
		case "getOnboarding":
			if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
				err = store.ErrInvalid
			} else {
				out, err = service.Get(r.Context(), e, id)
			}
		case "resumeOnboarding":
			var in onboarding.ResumeRequest
			if err = body(r, &in); err == nil {
				out, err = service.Resume(r.Context(), e, in.ID, in)
			}
		case "answerOnboarding":
			var in onboarding.AnswerRequest
			if err = body(r, &in); err == nil {
				out, err = service.Answer(r.Context(), e, in.ID, in)
			}
		case "cancelOnboarding":
			var in onboarding.CancelRequest
			if err = body(r, &in); err == nil {
				out, err = service.Cancel(r.Context(), e, in.ID, in)
			}
		case "proposeOnboardingDrift":
			var in onboarding.DriftRequest
			if err = body(r, &in); err == nil {
				out, err = service.Drift(r.Context(), e, in.ID, in)
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

func body(r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil {
		return store.ErrInvalid
	}
	if len(data) > MaxBodyBytes {
		return errBodyLimit
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(out) != nil {
		return store.ErrInvalid
	}
	var extra any
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		return store.ErrInvalid
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
	case errors.Is(err, store.ErrNotFound), errors.Is(err, access.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, onboarding.ErrAttention):
		status, code = 409, "attention_required"
	case errors.Is(err, onboarding.ErrCancelled):
		status, code = 409, "cancelled"
	case errors.Is(err, store.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(err, onboarding.ErrBudget):
		status, code = 429, "budget_exhausted"
	case errors.Is(err, errBodyLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, onboarding.ErrInvalid), errors.Is(err, store.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 504, "cancelled_or_timed_out"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
