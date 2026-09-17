package nlqapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestClarificationRepairProjection(t *testing.T) {
	err := &nlqroute.Clarification{Outcome: semantics.ClarificationInvalid, Reason: "invalid_date", Errors: []semantics.ClarificationFieldError{{Field: "time.start", Code: "invalid_date", Message: "Ingresá una fecha válida."}}, Questions: []semantics.ClarificationSlotOutcome{{Effect: &semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical: "private-scalar"}}}}}}
	recorder := httptest.NewRecorder()
	failure(recorder, err)
	if recorder.Code != 400 || strings.Contains(recorder.Body.String(), "private-scalar") {
		t.Fatal("unsafe or misclassified repair response")
	}
	var out struct {
		Error         string
		Clarification *semantics.ClarificationProblem
	}
	if json.Unmarshal(recorder.Body.Bytes(), &out) != nil || out.Error != "invalid_request" || out.Clarification == nil || out.Clarification.Fields[0].Message != "Ingresá una fecha válida." {
		t.Fatal("localized field contract lost")
	}
	if len(err.Questions[0].Effect.Values) != 1 {
		t.Fatal("projection mutated caller")
	}
}
