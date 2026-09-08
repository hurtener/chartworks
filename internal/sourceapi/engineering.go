package sourceapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// EngineeringRegistry is the executable operation inventory. Retained reads and
// cancellation remain available when new upload/profile work is disabled.
func EngineeringRegistry(uploads, profiles bool) []Operation {
	r, err := EngineeringAPIRegistry(uploads, profiles, config.DefaultUploads().MaxBytes)
	if err != nil {
		return nil
	}
	return r.Operations()
}

// UploadAction preserves an explicit operation key and resume decision. A failed
// or cancelled response never silently becomes a new physical attempt.
type UploadAction struct {
	Key    string `json:"key"`
	Resume bool   `json:"resume"`
}

// ProfileRequest cannot supply raw SQL, source credentials or execution evidence.
type ProfileRequest struct {
	Spec   engineering.ProfileSpec `json:"spec"`
	Key    string                  `json:"key"`
	Resume bool                    `json:"resume"`
}

// ProfileHistoryRequest identifies the exact currently authorized data partition.
type ProfileHistoryRequest struct {
	Source  string `json:"source"`
	Context string `json:"context"`
	Dataset string `json:"dataset"`
	Limit   int    `json:"limit"`
}

// EngineeringHandler reuses the existing strict JSON decoder, verifier and route
// matcher. All tenant, session, context and effect checks remain in the core.
func EngineeringHandler(verifier *auth.Verifier, service *engineering.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry, registrationErr := EngineeringAPIRegistry(service.UploadsEnabled(), service.ProfilingEnabled(), service.UploadByteLimit())
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
		case "/v1/uploads":
			var input engineering.UploadSpec
			if err = body(w, r, &input); err == nil {
				out, err = service.ReserveUpload(r.Context(), e, input)
			}
		case "/v1/uploads/{id}":
			out, err = service.InspectUpload(r.Context(), e, id)
		case "/v1/uploads/{id}/content":
			media, parameters, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || media != "application/octet-stream" || len(parameters) != 0 {
				err = store.ErrInvalid
				break
			}
			// StageUpload proves current reach and retained private ownership before
			// reading from this bounded body. Bytes are never sent through JSON.
			out, err = service.StageUpload(r.Context(), e, id, http.MaxBytesReader(w, r.Body, service.UploadByteLimit()))
		case "/v1/uploads/{id}/load", "/v1/uploads/{id}/erase":
			var input UploadAction
			if err = body(w, r, &input); err == nil {
				if selected.Action == "sources.erase" {
					out, err = service.EraseUpload(r.Context(), e, id, input.Key, input.Resume)
				} else {
					out, err = service.LoadUpload(r.Context(), e, id, input.Key, input.Resume)
				}
			}
		case "/v1/upload-sweeps":
			var input uploadSweepRequest
			if err = body(w, r, &input); err == nil {
				out, err = service.SweepUploads(r.Context(), e, input.Limit)
			}
		case "/v1/profiles":
			var input ProfileRequest
			if err = body(w, r, &input); err == nil {
				out, err = service.Build(r.Context(), e, input.Spec, input.Key, input.Resume)
			}
		case "/v1/profiles/{id}":
			out, err = service.InspectProfile(r.Context(), e, id)
		case "/v1/profiles/{id}/evidence":
			out, err = service.Evidence(r.Context(), e, id)
		case "/v1/profile-history":
			var input ProfileHistoryRequest
			if err = body(w, r, &input); err == nil {
				out, err = service.History(r.Context(), e, input.Source, input.Context, input.Dataset, input.Limit)
			}
		case "/v1/profiles/{id}/dependencies":
			var input engineering.Dependency
			if err = body(w, r, &input); err == nil {
				err = service.RegisterDependency(r.Context(), e, id, input)
				if err == nil {
					out = dependencyRegistered{true}
				}
			}
		case "/v1/profile-dependency-health":
			var input engineering.Dependency
			if err = body(w, r, &input); err == nil {
				out, err = service.Health(r.Context(), e, input)
			}
		case "/v1/engineering-operations/{id}":
			out, err = service.RequestOperation(r.Context(), e, id)
		case "/v1/engineering-operations/{id}/cancel":
			var input struct{}
			if err = body(w, r, &input); err == nil {
				out, err = service.CancelOperation(r.Context(), e, id)
			}
		}
		if err != nil {
			engineeringFailure(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, known := registry.Match(r.Method, r.URL.Path); known {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			protected.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func engineeringFailure(w http.ResponseWriter, err error) {
	status, code := 0, ""
	switch {
	case errors.Is(err, engineering.ErrInvalid), errors.Is(err, jobs.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(err, engineering.ErrFormat):
		status, code = 422, "unsafe_or_unsupported_file"
	case errors.Is(err, engineering.ErrChecksum):
		status, code = 422, "checksum_mismatch"
	case errors.Is(err, engineering.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(err, engineering.ErrOwnership):
		status, code = 409, "workspace_ownership_unproven"
	case errors.Is(err, engineering.ErrState):
		status, code = 409, "reconciliation_required"
	case errors.Is(err, jobs.ErrAuthority):
		status, code = 403, "authority_blocked"
	case errors.Is(err, store.ErrExpired):
		status, code = 410, "expired"
	default:
		failure(w, err)
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
