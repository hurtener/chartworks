package semantics

import (
	"encoding/json"
	"testing"
)

func TestSQLRecoveryPotentialBindingIsNotAResolution(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	model := cw01Compile(t, subject, d)
	in := ClarificationInput{Locale: "en", Question: "Count orders"}
	got := ResolveClarifications(model, in)
	if !got.MayRequireSourceBinding() || len(got.Resolutions) != 0 || len(got.References) != 0 {
		t.Fatal("future binding was a hypothetical selection", got)
	}
	raw, _ := json.Marshal(got)
	var restored ClarificationEvaluation
	if err := json.Unmarshal(raw, &restored); err != nil || restored.MayRequireSourceBinding() {
		t.Fatal("untrusted retained JSON acquired binding hint", err)
	}
	in.Question = "List customers"
	if ResolveClarifications(model, in).MayRequireSourceBinding() {
		t.Fatal("unrelated graph requires binding")
	}
	// Optional reviewed default reference selections can activate a required
	// typed child without being relabeled as user answers.
	d.Patterns[1].Slots[0].Required = false
	d.Patterns[1].Slots[0].Default = &ClarificationValue{OptionID: "dated"}
	in.Question = "Count orders"
	got = ResolveClarifications(cw01Compile(t, subject, d), in)
	if got.Outcome != ClarificationMissing || len(got.Resolutions) != 1 || got.Resolutions[0].Provenance != "reviewed_default" || len(CanonicalClarificationAnswers(got.Resolutions)) != 0 {
		t.Fatal("default closure lost provenance", got)
	}
}
