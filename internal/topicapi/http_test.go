package topicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func TestRegistryManifestAndConcreteSchemas(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Definitions()) != 24 {
		t.Fatal("operation inventory")
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-topic-draft-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []api.Operation
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	actual := r.Operations()
	sort.Slice(manifest, func(i, j int) bool {
		if manifest[i].Path != manifest[j].Path {
			return manifest[i].Path < manifest[j].Path
		}
		return manifest[i].Method < manifest[j].Method
	})
	sort.Slice(actual, func(i, j int) bool {
		if actual[i].Path != actual[j].Path {
			return actual[i].Path < actual[j].Path
		}
		return actual[i].Method < actual[j].Method
	})
	if !reflect.DeepEqual(manifest, actual) {
		t.Fatal("manifest differs from actual registration")
	}
	doc, err := r.OpenAPI("Topic drafts", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range r.Definitions() {
		if !bytes.Contains(doc, []byte(d.ID)) {
			t.Fatal("OpenAPI omitted operation", d.ID)
		}
		if d.Request != nil && d.Request.Validate([]byte(`{"unexpected":true}`), MaxBodyBytes) == nil {
			t.Fatal("open request schema", d.ID)
		}
	}
}

func TestRuleDraftSchemaAcceptsClosedOptionalConstraintShape(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	d, _, known := r.Match("POST", "/v1/topics/commerce/rule-drafts")
	if !known || d.ID != "saveRuleDraft" {
		t.Fatal("rule draft route")
	}
	request := struct {
		Expected   int64                       `json:"expected_revision"`
		Definition semantics.RuleSetDefinition `json:"definition"`
		Change     string                      `json:"change"`
	}{Definition: semantics.RuleSetDefinition{
		SchemaVersion: semantics.SchemaVersion,
		ID:            "commerce-rules",
		Version:       "rules-v1",
		Topic:         "commerce",
		TopicVersion:  "topic-v1",
		PackDigest:    strings.Repeat("a", 64),
		Rules: []semantics.RuleDefinition{{
			ID: "require-revenue", Version: "v1", Category: semantics.RuleComputation,
			Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic},
			Priority: 100, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceFeedback, Evidence: "feedback-1"},
			Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}},
		}},
		Patterns: []semantics.ClarificationPattern{},
	}, Change: "Propose required revenue"}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Request.Validate(raw, MaxBodyBytes); err != nil {
		t.Fatal("valid rule draft rejected", err, string(raw))
	}
}
func TestBodyRejectsMalformedAndOversizedRequests(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := r.Match("POST", "/v1/topics/commerce/draft-history")
	for _, tc := range []struct {
		name, media, raw string
		want             error
	}{{"valid", "application/json", `{"before":0,"limit":32}`, nil}, {"unknown", "application/json", `{"before":0,"limit":32,"grant":"all"}`, store.ErrInvalid}, {"duplicate", "application/json", `{"limit":1,"limit":2}`, store.ErrInvalid}, {"trailing", "application/json", `{} {}`, store.ErrInvalid}, {"type", "application/json", `{"limit":"32"}`, store.ErrInvalid}, {"media", "text/plain", `{}`, store.ErrInvalid}, {"media parameter", "application/json; charset=utf-8", `{}`, store.ErrInvalid}, {"oversized", "application/json", strings.Repeat(" ", MaxBodyBytes+1), readexec.ErrLimit}} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", strings.NewReader(tc.raw))
			request.Header.Set("Content-Type", tc.media)
			var out HistoryRequest
			err := body(httptest.NewRecorder(), request, d.Request, &out)
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
}
func TestFailureClassifications(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{access.ErrUnauthenticated, 401, "unauthenticated"}, {access.ErrForbidden, 403, "forbidden"}, {access.ErrNotFound, 404, "not_found"}, {store.ErrNotFound, 404, "not_found"}, {store.ErrInvalid, 400, "invalid_request"}, {semantics.ErrInvalid, 400, "invalid_request"}, {store.ErrConflict, 409, "conflict"}, {readexec.ErrBinding, 409, "context_changed"}, {readexec.ErrLimit, 413, "limit_exceeded"}, {readexec.ErrUnsupported, 422, "unsupported"}, {context.Canceled, 504, "cancelled_or_timed_out"}, {context.DeadlineExceeded, 504, "cancelled_or_timed_out"}, {errors.New("PRIVATE_DETAIL"), 503, "unavailable"}} {
		w := httptest.NewRecorder()
		failure(w, tc.err)
		if w.Code != tc.status || w.Body.String() != `{"error":"`+tc.code+`"}`+"\n" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if Handler(nil, nil, nil, nil, http.NotFoundHandler()) == nil {
		t.Fatal("nil dependencies")
	}
}
