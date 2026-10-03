// Package topicfeedbackapi exposes one semantic proposal service through thin
// authenticated HTTP and MCP surfaces. It has no independent editing logic.
package topicfeedbackapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicfeedback"
)

const maxBody = 16 << 10

func Registry() (*api.Registry, error) {
	defs := []api.Definition{}
	for _, r := range []struct {
		id, path, summary, effect string
		in, out                   reflect.Type
	}{
		{"listTopicFeedbackEvidence", "/v1/topic-feedback/evidence", "List content-free owned correction evidence", "retained_metadata_read", reflect.TypeFor[topicfeedback.EvidenceRequest](), reflect.TypeFor[[]nlqexec.SemanticFeedbackReference]()},
		{"proposeTopicFeedback", "/v1/topic-feedback/proposals", "Propose private semantic corrections from explicit author intent", "model_and_proposal_commit", reflect.TypeFor[topicfeedback.ProposeRequest](), reflect.TypeFor[topicfeedback.Proposal]()},
		{"readTopicFeedback", "/v1/topic-feedback/read", "Read an owned semantic correction proposal", "retained_metadata_read", reflect.TypeFor[topicfeedback.ReadRequest](), reflect.TypeFor[topicfeedback.Proposal]()},
		{"applyTopicFeedback", "/v1/topic-feedback/apply", "Apply exact reviewed proposal edits to a private draft", "atomic_proposal_and_draft_commit", reflect.TypeFor[topicfeedback.ApplyRequest](), reflect.TypeFor[drafts.Version]()},
	} {
		request, err := api.SchemaFor(r.id+"Request", r.in, false, api.OptionalJSONFields)
		if err != nil {
			return nil, err
		}
		response, err := api.SchemaFor(r.id+"Response", r.out, true)
		if err != nil {
			return nil, err
		}
		defs = append(defs, api.Definition{Operation: api.Operation{Method: "POST", Path: r.path, Action: "topics.write", Effect: r.effect}, ID: r.id, Summary: r.summary, ResourceLoader: "topicfeedback.Service", Audit: "private proposal evidence and topic.drafted on apply", MaxBodyBytes: maxBody, Request: request, Response: response, Errors: []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 413, Code: "limit_exceeded"}, {Status: 422, Code: "invalid_proposal"}, {Status: 503, Code: "unavailable"}}})
	}
	return api.New(defs)
}
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		return 401, "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		return 403, "forbidden"
	case errors.Is(err, access.ErrNotFound), errors.Is(err, store.ErrNotFound):
		return 404, "not_found"
	case errors.Is(err, store.ErrConflict), errors.Is(err, exec.ErrBinding), errors.Is(err, nlqexec.ErrForeignSession), errors.Is(err, nlqexec.ErrFeedbackRetention):
		return 409, "conflict"
	case errors.Is(err, exec.ErrLimit), errors.Is(err, gateway.ErrBudget):
		return 413, "limit_exceeded"
	case errors.Is(err, gateway.ErrOutput), errors.Is(err, semantics.ErrInvalid):
		return 422, "invalid_proposal"
	case errors.Is(err, store.ErrInvalid), errors.Is(err, nlqexec.ErrInvalid), errors.Is(err, nlqexec.ErrNoPlan):
		return 400, "invalid_request"
	default:
		return 503, "unavailable"
	}
}
func failure(w http.ResponseWriter, err error) {
	status, code := classify(err)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
func Handler(verifier *auth.Verifier, s *topicfeedback.Service, next http.Handler) http.Handler {
	if verifier == nil || next == nil {
		return http.NotFoundHandler()
	}
	if s == nil {
		return next
	}
	registry, err := Registry()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	protected := verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op, _, _ := registry.Match(r.Method, r.URL.Path)
		e, err := identity.FromContext(r.Context())
		if err != nil {
			failure(w, access.ErrUnauthenticated)
			return
		}
		if !e.Has("topics.write") || !e.Has("feedback.write") {
			failure(w, access.ErrForbidden)
			return
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(params) != 0 || r.URL.RawQuery != "" || r.URL.RawPath != "" || r.Header.Get("Content-Encoding") != "" {
			failure(w, store.ErrInvalid)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			failure(w, exec.ErrLimit)
			return
		}
		if op.Request == nil || op.Request.Validate(raw, maxBody) != nil {
			failure(w, store.ErrInvalid)
			return
		}
		var out any
		switch op.ID {
		case "listTopicFeedbackEvidence":
			var in topicfeedback.EvidenceRequest
			err = json.Unmarshal(raw, &in)
			if err == nil {
				out, err = s.Evidence(r.Context(), e, in)
			}
		case "proposeTopicFeedback":
			var in topicfeedback.ProposeRequest
			err = json.Unmarshal(raw, &in)
			if err == nil {
				out, err = s.Propose(r.Context(), e, in)
			}
		case "readTopicFeedback":
			var in topicfeedback.ReadRequest
			err = json.Unmarshal(raw, &in)
			if err == nil {
				out, err = s.Read(r.Context(), e, in)
			}
		case "applyTopicFeedback":
			var in topicfeedback.ApplyRequest
			err = json.Unmarshal(raw, &in)
			if err == nil {
				out, err = s.Apply(r.Context(), e, in)
			}
		default:
			err = store.ErrInvalid
		}
		if err != nil {
			failure(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(out)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op, _, known := registry.Match(r.Method, r.URL.Path)
		if op.Path == "" {
			if known {
				w.WriteHeader(405)
			} else {
				next.ServeHTTP(w, r)
			}
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func MCPBindings(s *topicfeedback.Service) ([]mcpserver.Binding, error) {
	if s == nil {
		return nil, nil
	}
	registry, err := Registry()
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault { _, code := classify(err); return mcpserver.Fault{Code: code} }
	a, err := mcpserver.Bind(registry, "proposeTopicFeedback", "propose_topic_feedback", "query", "Generate a private semantic correction proposal using explicit current author intent and exact owned feedback evidence. Historical SQL, notes and parameter values are excluded. This never applies or publishes changes.", s.Propose, mapper)
	if err != nil {
		return nil, err
	}
	b, err := mcpserver.Bind(registry, "readTopicFeedback", "read_topic_feedback", "query", "Read an owned semantic correction proposal, including exact edits and origin. Requires current signed dependency reach.", s.Read, mapper)
	if err != nil {
		return nil, err
	}
	c, err := mcpserver.Bind(registry, "applyTopicFeedback", "apply_topic_feedback", "query", "Explicitly accept an exact proposal digest into a private topic draft. This does not publish: use normal human review and publication afterward.", s.Apply, mapper)
	if err != nil {
		return nil, err
	}
	d, err := mcpserver.Bind(registry, "listTopicFeedbackEvidence", "list_topic_feedback_evidence", "query", "List exact owned correction or negative-feedback IDs for an existing query. Does not disclose historical questions, notes, SQL or values.", s.Evidence, mapper)
	if err != nil {
		return nil, err
	}
	return []mcpserver.Binding{a, b, c, d}, nil
}
