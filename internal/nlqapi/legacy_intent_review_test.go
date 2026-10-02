package nlqapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
)

func TestSQLRecoveryLegacyIntentReviewPublicSchema(t *testing.T) {
	r, err := ExecutionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	refine, _, ok := r.Match(http.MethodPost, "/v1/nlq/refinements")
	if !ok {
		t.Fatal("missing refinement")
	}
	body := []byte(`{"query_id":"legacy","intent_review":{"query_id":"pending","answer_context":"pin","answers":[]}}`)
	if err := refine.Request.Validate(body, MaxBodyBytes); err != nil {
		t.Fatal(err)
	}
	if err := refine.Request.Validate([]byte(`{"query_id":"legacy","intent_review":{"query_id":"ready","selection_digest":"current-selection"}}`), MaxBodyBytes); err != nil {
		t.Fatal("ready review schema", err)
	}
	var decoded nlqexec.RefineRequest
	if err := json.Unmarshal(body, &decoded); err != nil || decoded.IntentReview == nil || decoded.IntentReview.QueryID != "pending" {
		t.Fatal("review wire", err)
	}
	for _, bad := range []string{`{"intent_review":{"query_id":"pending","approve_sql":true}}`, `{"intent_review":{"query_id":12}}`, `{"intent_review":true}`} {
		if err := refine.Request.Validate([]byte(bad), MaxBodyBytes); err == nil {
			t.Fatal("open review schema")
		}
	}
	plan, _, _ := r.Match(http.MethodPost, "/v1/nlq/plans")
	if err := plan.Request.Validate(body, MaxBodyBytes); err == nil {
		t.Fatal("review bypassed parent-only surface")
	}
}
