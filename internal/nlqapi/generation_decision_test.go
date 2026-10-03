package nlqapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
)

func TestSQLRecoveryDecisionHTTPDiscriminators(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{nlqexec.ErrGenerationClarification, "generation_clarification_required"}, {errors.Join(nlqexec.ErrValidationBudget, nlqexec.ErrGenerationContext), "generation_context_insufficient"}} {
		status, code := classify(tc.err)
		if status != http.StatusUnprocessableEntity || code != tc.code {
			t.Fatal("decision masked by validation budget", status, code)
		}
		w := httptest.NewRecorder()
		failure(w, tc.err)
		var body map[string]any
		if w.Code != 422 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"] != tc.code || body["generation"] != nil || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("sentinel invented model questions or cached response")
		}
	}
	reg, err := ExecutionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := reg.OpenAPI("test", "v1")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("bad public registry")
	}
	projection := doc.Paths["/v1/nlq/plans"]["post"].Responses["422"].Content["application/json"].Schema.Properties["generation"]
	if len(projection) == 0 {
		t.Fatal("public error schema excludes generation decision")
	}
}
