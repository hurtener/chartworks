package nlqapi

import (
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/store"
)

// BYORegistry advertises only installed capabilities. Lookup and submit do not
// disappear when inference is disabled after an opaque context was created.
func BYORegistry(canCreate bool) (*api.Registry, error) {
	items := []struct {
		path, action, effect, id, summary, loader string
		request, response                         reflect.Type
	}{
		{"/v1/nlq/contexts/read", "query.context", "byo_context_read", "readQueryContext", "Read an exact reauthorized context and step receipts", "nlqbyo.Service.Lookup", reflect.TypeFor[nlqbyo.Reference](), reflect.TypeFor[nlqbyo.View]()},
		{"/v1/nlq/sql", "query.submit", "byo_validated_read_and_receipt", "submitSQL", "Submit one explicit SQL step; inspect the receipt status", "nlqbyo.Service.Submit", reflect.TypeFor[nlqbyo.SubmitRequest](), reflect.TypeFor[nlqbyo.SubmitResult]()},
	}
	if canCreate {
		items = append(items, struct {
			path, action, effect, id, summary, loader string
			request, response                         reflect.Type
		}{"/v1/nlq/contexts", "query.context", "byo_context_retrieval_and_commit", "getQueryContext", "Construct one bounded expiring semantic context; no SQL execution", "nlqbyo.Service.Create", reflect.TypeFor[nlqbyo.CreateRequest](), reflect.TypeFor[nlqbyo.CreateResult]()})
	}
	var definitions []api.Definition
	for _, item := range items {
		request, err := api.SchemaFor(item.id+"Request", item.request, false, api.OptionalJSONFields)
		if err != nil {
			return nil, err
		}
		response, err := api.SchemaFor(item.id+"Response", item.response, true)
		if err != nil {
			return nil, err
		}
		errors := append(executionErrors(), api.ErrorResponse{Status: 409, Code: "replan_required"}, api.ErrorResponse{Status: 429, Code: "bundle_budget_exhausted"})
		definitions = append(definitions, api.Definition{Operation: api.Operation{Method: http.MethodPost, Path: item.path, Action: item.action, Effect: item.effect}, ID: item.id, Summary: item.summary, ResourceLoader: item.loader, Audit: "bounded context and step evidence", MaxBodyBytes: MaxBodyBytes, Request: request, Response: response, Errors: errors})
	}
	return api.New(definitions)
}

// BYOHandler verifies fresh Pengui authority and delegates to the common service.
func BYOHandler(verifier *auth.Verifier, service *nlqbyo.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, err := BYORegistry(service.CanCreate())
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
		var out any
		switch selected.ID {
		case "getQueryContext":
			var in nlqbyo.CreateRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Create(r.Context(), e, in)
			}
		case "readQueryContext":
			var in nlqbyo.Reference
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Lookup(r.Context(), e, in)
			}
		case "submitSQL":
			var in nlqbyo.SubmitRequest
			err = decodeBody(w, r, selected.Request, &in)
			if err == nil {
				out, err = service.Submit(r.Context(), e, in)
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
