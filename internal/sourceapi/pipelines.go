package sourceapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// PipelineRegistry is the executable action/effect inventory. Disabling new
// pipeline work preserves immutable definition reads and retained run controls.
func PipelineRegistry(enabled bool) []Operation {
	out := []Operation{
		{Method: "POST", Path: "/v1/pipeline-versions/read", Action: "engineering.pipeline.read", Effect: "private_definition_read"},
		{Method: "GET", Path: "/v1/pipeline-runs/{id}", Action: "jobs.read", Effect: "private_run_metadata_read"},
		{Method: "POST", Path: "/v1/pipeline-runs/{id}/cancel", Action: "jobs.cancel", Effect: "durable_cancellation_intent"},
	}
	if enabled {
		out = append(out,
			Operation{Method: "POST", Path: "/v1/pipeline-proposals", Action: "engineering.pipeline.write", Effect: "model_assisted_pipeline_draft"},
			Operation{Method: "POST", Path: "/v1/pipelines", Action: "engineering.pipeline.write", Effect: "versioned_pipeline_draft"},
			Operation{Method: "POST", Path: "/v1/pipelines/{id}/publish", Action: "engineering.pipeline.publish", Effect: "immutable_pipeline_publication"},
			Operation{Method: "POST", Path: "/v1/pipelines/{id}/runs", Action: "engineering.pipeline.run", Effect: "managed_warehouse_write"},
		)
	}
	return out
}

type pipelineDraftRequest struct {
	Definition       engineering.PipelineDefinition `json:"definition"`
	ExpectedRevision int64                          `json:"expected_revision"`
}
type pipelineVersionRequest struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type pipelineRunRequest struct {
	Version   int64  `json:"version"`
	Key       string `json:"key"`
	Resume    bool   `json:"resume"`
	AdmitOnly bool   `json:"admit_only"`
}

// PipelineHandler verifies the registered action before decoding a closed body;
// the service repeats complete resource and execution-context checks.
func PipelineHandler(verifier *auth.Verifier, service *engineering.PipelineService, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if service == nil {
		return next
	}
	registry := PipelineRegistry(service.Enabled())
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		var selected Operation
		id := ""
		for _, op := range registry {
			if candidate, ok := match(op.Path, r.URL.Path); ok && op.Method == r.Method {
				selected, id = op, candidate
				break
			}
		}
		if selected.Path == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !e.Has(selected.Action) {
			failure(w, access.ErrForbidden)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" || r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
			failure(w, store.ErrInvalid)
			return
		}
		var out any
		switch selected.Path {
		case "/v1/pipeline-proposals":
			var input engineering.PipelineProposalRequest
			if err = pipelineBody(w, r, &input); err == nil {
				out, err = service.Propose(r.Context(), e, input)
			}
		case "/v1/pipelines":
			var input pipelineDraftRequest
			if err = pipelineBody(w, r, &input); err == nil {
				out, err = service.Draft(r.Context(), e, input.Definition, input.ExpectedRevision)
			}
		case "/v1/pipelines/{id}/publish":
			var input struct {
				Version int64 `json:"version"`
			}
			if err = pipelineBody(w, r, &input); err == nil {
				out, err = service.Publish(r.Context(), e, id, input.Version)
			}
		case "/v1/pipeline-versions/read":
			var input pipelineVersionRequest
			if err = pipelineBody(w, r, &input); err == nil {
				out, err = service.Get(r.Context(), e, input.ID, input.Version)
			}
		case "/v1/pipelines/{id}/runs":
			var input pipelineRunRequest
			if err = pipelineBody(w, r, &input); err == nil {
				if input.AdmitOnly {
					out, err = service.AdmitRun(r.Context(), e, id, input.Version, input.Key)
				} else {
					out, err = service.Run(r.Context(), e, id, input.Version, input.Key, input.Resume)
				}
			}
		case "/v1/pipeline-runs/{id}":
			out, err = service.InspectRun(r.Context(), e, id)
		case "/v1/pipeline-runs/{id}/cancel":
			var input struct{}
			if err = pipelineBody(w, r, &input); err == nil {
				out, err = service.Cancel(r.Context(), e, id)
			}
		}
		if err != nil {
			pipelineFailure(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, op := range registry {
			if _, ok := match(op.Path, r.URL.Path); ok {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				protected.ServeHTTP(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Pipeline definitions contain optional closed fields. DisallowUnknownFields
// preserves a closed recursive schema without requiring every optional member.
func pipelineBody(w http.ResponseWriter, r *http.Request, out any) error {
	media, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(parameters) != 0 {
		return store.ErrInvalid
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return store.ErrInvalid
	}
	if _, err = gateway.DecodeJSON(data, 1<<20); err != nil {
		return store.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return store.ErrInvalid
	}
	return nil
}

func pipelineFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, engineering.ErrPipelineQuality):
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{"pipeline_quality_failed"})
	case errors.Is(err, engineering.ErrPipelineUncertain):
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{"reconciliation_required"})
	default:
		engineeringFailure(w, err)
	}
}
