package reportingapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// DocumentsHandler applies the existing authentication, closed schema, body and
// query bounds before calling common document/composition domain services.
func DocumentsHandler(verifier *auth.Verifier, documents *reporting.Documents, runs *reporting.Compositions, next http.Handler) http.Handler {
	if verifier == nil || documents == nil || runs == nil || next == nil {
		return http.NotFoundHandler()
	}
	registry, err := DocumentsRegistry()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	calls := map[string]runtimeEndpoint{}
	for _, entry := range documentEntries(documents, runs) {
		calls[entry.definition.ID] = entry
	}
	slots := make(chan struct{}, 16)
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		d, id, _ := registry.Match(r.Method, r.URL.Path)
		if d.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has(d.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			failure(w, reporting.ErrBusy)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || !validQuery(q, d.Query) || r.URL.RawPath != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" {
			failure(w, reporting.ErrInvalid)
			return
		}
		var raw json.RawMessage
		if r.Method == "GET" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
			if err != nil || len(body) != 0 {
				failure(w, reporting.ErrInvalid)
				return
			}
		} else if err := decode(w, r, d, &raw); err != nil {
			failure(w, err)
			return
		}
		out, err := calls[d.ID].call(r.Context(), e, id, q, raw)
		if err != nil {
			failure(w, err)
			return
		}
		if err := r.Context().Err(); err != nil {
			failure(w, err)
			return
		}
		if !e.Valid() {
			failure(w, access.ErrUnauthenticated)
			return
		}
		encoded, err := json.Marshal(out)
		if err != nil || len(encoded) > 32<<20 {
			failure(w, reporting.ErrBudget)
			return
		}
		_, _ = w.Write(append(encoded, '\n'))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, found := registry.Match(r.Method, r.URL.Path); !found {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
