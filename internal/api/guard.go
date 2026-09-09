package api

import (
	"encoding/json"
	"net/http"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

// Guard makes the registration's bearer/action requirement executable at the
// composition boundary. Domain services still resolve and enforce every resource;
// this does not replace their checks with descriptive ResourceLoader strings.
// Public routes retain their handlers; unregistered paths fail closed so a new
// handler cannot escape the registration contract. No bearer is
// accepted from cookies, body fields or a query string.
func Guard(verifier *auth.Verifier, registry *Registry, next http.Handler) http.Handler {
	if verifier == nil || registry == nil || next == nil {
		return http.NotFoundHandler()
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _, known := registry.Match(r.Method, r.URL.Path)
		if !known {
			guardError(w, 404, "not_found")
			return
		}
		if d.Path == "" {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		e, err := identity.FromContext(r.Context())
		if err != nil || !e.Valid() {
			guardError(w, 401, "unauthenticated")
			return
		}
		if !e.Has(d.Action) {
			guardError(w, 403, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, _, known := registry.Match(r.Method, r.URL.Path)
		if !known {
			guardError(w, 404, "not_found")
			return
		}
		if d.Public {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func guardError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
