package semantics

import (
	"encoding/json"
	"testing"
)

func TestClarificationTermContextDoesNotComeFromJSON(t *testing.T) {
	subject, definition := cw01Definition(t, cw01NumberSlot())
	model := cw01Compile(t, subject, definition)
	matches := MatchClarificationTerms(definition, "Count orders")
	if len(matches) != 1 || matches[0].Specificity != 1 {
		t.Fatal("term match")
	}
	input := ClarificationInput{Locale: "en", Question: "Count [redacted answer]"}
	out := ResolveClarificationsWithTermContext(model, input, matches)
	if out.Outcome != ClarificationMissing || !out.MayRequireSourceBinding() {
		t.Fatal("protected seed lost reachable binding")
	}
	raw, _ := json.Marshal(input)
	var decoded ClarificationInput
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("decode")
	}
	if out := ResolveClarifications(model, decoded); out.Outcome != ClarificationNotApplicable {
		t.Fatal("JSON manufactured term context")
	}
	matches[0].Specificity = 8
	input = decoded
	if out := ResolveClarificationsWithTermContext(model, input, matches); out.Outcome != ClarificationInvalid {
		t.Fatal("invalid match coordinate accepted")
	}
}

func TestClarificationMarkerIsNotTermAuthority(t *testing.T) {
	_, definition := cw01Definition(t, cw01NumberSlot())
	definition.Patterns[0].Policy.When.AnyTerms = []string{"redacted", "answer", "count orders", "[redacted answer]"}
	if matches := MatchClarificationTerms(definition, "count [redacted answer] orders"); len(matches) != 0 {
		t.Fatal("marker generated term applicability")
	}
}
