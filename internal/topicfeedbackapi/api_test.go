package topicfeedbackapi

import (
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicfeedback"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTopicFeedbackRegistryClosedOperations(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/topic-feedback/evidence", "/v1/topic-feedback/proposals", "/v1/topic-feedback/read", "/v1/topic-feedback/apply"} {
		d, _, _ := r.Match("POST", path)
		if d.ID == "" || d.Action != "topics.write" || d.ResourceLoader == "" {
			t.Fatal(path)
		}
	}
	d, _, _ := r.Match("POST", "/v1/topic-feedback/proposals")
	if err = d.Request.Validate([]byte(`{"id":"x","topic":"sales","feedback_id":"f","expected_draft_revision":1,"author_intent":"Correct mean","sql":"private"}`), maxBody); err == nil {
		t.Fatal("unknown raw SQL field accepted")
	}
	if _, err = MCPBindings(nil); err != nil {
		t.Fatal(err)
	}
	var _ *topicfeedback.Service
}

func TestTopicFeedbackTransportBoundaries(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{{access.ErrUnauthenticated, 401, "unauthenticated"}, {access.ErrForbidden, 403, "forbidden"}, {store.ErrNotFound, 404, "not_found"}, {store.ErrConflict, 409, "conflict"}, {exec.ErrBinding, 409, "conflict"}, {exec.ErrLimit, 413, "limit_exceeded"}, {gateway.ErrOutput, 422, "invalid_proposal"}, {store.ErrInvalid, 400, "invalid_request"}, {store.ErrUnavailable, 503, "unavailable"}} {
		status, code := classify(test.err)
		if status != test.status || code != test.code {
			t.Fatal(test.err, status, code)
		}
		w := httptest.NewRecorder()
		failure(w, test.err)
		if w.Code != status || !strings.Contains(w.Body.String(), code) {
			t.Fatal(w.Body.String())
		}
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	if h := Handler(nil, nil, next); h == nil {
		t.Fatal("nil fallback")
	}
	if h := Handler(&auth.Verifier{}, nil, next); h == nil {
		t.Fatal("nil service fallback")
	}
	h := Handler(&auth.Verifier{}, &topicfeedback.Service{}, next)
	for _, tc := range []struct {
		path string
		want int
	}{{"/v1/topic-feedback/read", 405}, {"/unrelated", 204}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.want {
			t.Fatal(tc, w.Code)
		}
	}
	bindings, err := MCPBindings(&topicfeedback.Service{})
	if err != nil || len(bindings) != 4 {
		t.Fatal("registered real bindings", err, len(bindings))
	}
}
