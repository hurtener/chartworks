package semantics

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func cw01Compile(t *testing.T, subject RuleSubject, definition RuleSetDefinition) RuleModel {
	t.Helper()
	model, err := CompilePublishedRules(subject, definition)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func cw01Answer(definition RuleSetDefinition, pattern, slot string, value ClarificationValue) ClarificationAnswer {
	version := ""
	for _, p := range definition.Patterns {
		if p.ID == pattern {
			version = p.Version
		}
	}
	return ClarificationAnswer{Topic: definition.Topic, TopicVersion: definition.TopicVersion, RulesetVersion: definition.Version, Pattern: pattern, PatternVersion: version, Slot: slot, Value: &value}
}

func TestConditionalClarification(t *testing.T) {
	t.Run("MatchingAndUnrelated", func(t *testing.T) {
		subject, definition := cw01Definition(t, cw01TimeSlot())
		model := cw01Compile(t, subject, definition)
		for _, question := range []string{"Count active customers today", "Show preorders by customer"} {
			got := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: question})
			if got.Outcome != ClarificationNotApplicable || len(got.Resolutions) != 0 || len(got.Errors) != 0 || got.Slots[0].Outcome != ClarificationNotApplicable {
				t.Fatalf("unrelated question interrupted: %+v", got)
			}
		}
		got := ResolveClarifications(model, ClarificationInput{Locale: "es", Question: "Contar pedidos"})
		if got.Outcome != ClarificationMissing || got.Slots[0].Prompt != "¿Qué período?" || got.Slots[0].Why != definition.Patterns[0].Policy.WhySpanish || !got.Slots[0].Required {
			t.Fatalf("required localized question missing: %+v", got)
		}
	})
	t.Run("CanonicalRoundtripAndPins", func(t *testing.T) {
		subject, definition := cw01Definition(t, cw01TimeSlot())
		model := cw01Compile(t, subject, definition)
		answer := cw01Answer(definition, "order_period", "period", ClarificationValue{Time: &ClarificationTimeInput{Period: "enero 2026", Calendar: "gregorian", TimeZone: cw01TimeSlot().Effect.TimeZone, Grain: "month"}})
		input := ClarificationInput{Locale: "es", Question: "Contar pedidos", Answers: []ClarificationAnswer{answer}}
		first := ResolveClarifications(model, input)
		if first.Outcome != ClarificationSatisfied || len(first.Resolutions) != 1 {
			t.Fatalf("not resolved: %+v", first)
		}
		r := first.Resolutions[0]
		if r.ID != ClarificationResolutionDigest(r) || r.PackDigest != definition.PackDigest || r.TopicVersion != definition.TopicVersion || r.RulesetDigest != model.Digest() || r.QuestionDigest == "" || r.Time.StartUTC != "2026-01-01T03:00:00Z" {
			t.Fatalf("resolution pins or effect missing: %+v", r)
		}
		canonical := CanonicalClarificationAnswers(first.Resolutions)
		raw, err := json.Marshal(canonical)
		if err != nil {
			t.Fatal(err)
		}
		var decoded []ClarificationAnswer
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		input.Answers = decoded
		second := ResolveClarifications(model, input)
		if !reflect.DeepEqual(first.Resolutions, second.Resolutions) || strings.Contains(string(raw), "enero") {
			t.Fatal("canonical replay changed the constraint or retained raw localized input")
		}
	})
	t.Run("InvalidIsAtomic", func(t *testing.T) {
		subject, definition := cw01Definition(t, cw01TimeSlot(), cw01NumberSlot())
		model := cw01Compile(t, subject, definition)
		period := cw01Answer(definition, "order_period", "period", ClarificationValue{Time: &ClarificationTimeInput{Start: "2026-01-01", End: "2026-02-01", Calendar: "gregorian", TimeZone: cw01TimeSlot().Effect.TimeZone, Grain: "month"}})
		amount := cw01Answer(definition, "order_period", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "NaN", Unit: "USD"}})
		got := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{period, amount}})
		if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 || len(got.References) != 0 || len(got.Errors) == 0 {
			t.Fatalf("partial invalid constraint accepted: %+v", got)
		}
		for _, mutate := range []func(*ClarificationAnswer){func(a *ClarificationAnswer) { a.TopicVersion = "old" }, func(a *ClarificationAnswer) { a.RulesetVersion = "old" }, func(a *ClarificationAnswer) { a.PatternVersion = "old" }, func(a *ClarificationAnswer) { a.Topic = "foreign" }, func(a *ClarificationAnswer) { a.Slot = "foreign" }, func(a *ClarificationAnswer) { a.Remove = true }} {
			answer := period
			mutate(&answer)
			got := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{answer}})
			if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 {
				t.Fatal("stale/foreign/removal input accepted")
			}
		}
	})
	t.Run("StableSpecificityAndConflicts", func(t *testing.T) {
		subject, definition := cw01Definition(t, cw01NumberSlot())
		second := cloneRules(definition).Patterns[0]
		second.ID = "upper_limit"
		second.Policy.When.AnyTerms = []string{"count orders"}
		second.Policy.Priority = -100
		second.Slots[0].Effect.Operator = "lt"
		definition.Patterns = append(definition.Patterns, second)
		model := cw01Compile(t, subject, definition)
		lower := cw01Answer(definition, "order_period", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "20", Unit: "USD"}})
		upper := cw01Answer(definition, "upper_limit", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}})
		input := ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{lower, upper}}
		got := ResolveClarifications(model, input)
		if got.Outcome != ClarificationConflicting || len(got.Resolutions) != 0 || got.Slots[0].Pattern != "upper_limit" || got.Slots[0].Outcome != ClarificationConflicting || got.Slots[1].Outcome != ClarificationConflicting {
			t.Fatalf("conflict or ordering lost: %+v", got)
		}
		definition.Patterns[0], definition.Patterns[1] = definition.Patterns[1], definition.Patterns[0]
		reordered := ResolveClarifications(cw01Compile(t, subject, definition), input)
		if !reflect.DeepEqual(got, reordered) {
			t.Fatal("storage order changed evaluation")
		}
		input.Answers[1].Value.Number.Value = "30"
		compatible := ResolveClarifications(model, input)
		if compatible.Outcome != ClarificationSatisfied || len(compatible.Resolutions) != 2 {
			t.Fatalf("compatible intersection rejected: %+v", compatible)
		}
	})
	t.Run("DefaultAndDependencyPresentation", func(t *testing.T) {
		period, number := cw01TimeSlot(), cw01NumberSlot()
		number.Required = false
		number.Default = &ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}}
		subject, definition := cw01Definition(t, period, number)
		got := ResolveClarifications(cw01Compile(t, subject, definition), ClarificationInput{Locale: "en", Question: "Count orders"})
		if got.Outcome != ClarificationMissing || !got.Slots[1].Defaulted || got.Slots[1].Reason != "reviewed_default" || len(got.Resolutions) != 1 || len(CanonicalClarificationAnswers(got.Resolutions)) != 0 {
			t.Fatalf("default attribution lost or promoted to answer: %+v", got)
		}
		number.Required, number.Default, number.DependsOn = true, nil, []string{"period"}
		subject, definition = cw01Definition(t, number, period)
		got = ResolveClarifications(cw01Compile(t, subject, definition), ClarificationInput{Locale: "en", Question: "Count orders"})
		if got.Slots[0].Slot != "period" || got.Slots[1].Reason != "dependency_missing" || got.Slots[1].Prompt != "" || got.Slots[1].Outcome != ClarificationMissing {
			t.Fatalf("dependent blockers asked out of order or silently skipped: %+v", got)
		}
	})
	t.Run("SafeLegacyChoices", func(t *testing.T) {
		a, b := Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Reference{Kind: KindColumn, Dataset: "orders", ID: "created"}
		slot := ClarificationSlot{ID: "field", Prompt: "Which field?", Required: true, Kind: SlotChoice, Sensitivity: LiteralNonSensitive, Choices: []ClarificationChoice{{ID: "amount_option", Label: "Amount", Target: &a}, {ID: "created_option", Label: "Created", Target: &b}}}
		subject, definition := cw01Definition(t, slot)
		definition.Patterns[0].Policy = nil
		model := cw01Compile(t, subject, definition)
		got := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count customers"})
		if got.Outcome != ClarificationNotApplicable || !strings.Contains(got.Dispositions[0], "legacy_reference_only_review_required") {
			t.Fatal("legacy pattern became a new blocker without review")
		}
		got = ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count orders", LegacyChoices: []LegacyClarificationChoice{{Pattern: "order_period", Slot: "field", Value: "amount_option"}}})
		if got.Outcome != ClarificationSatisfied || len(got.References) != 1 || got.References[0] != a || got.Resolutions[0].Reference == nil {
			t.Fatalf("safe reference choice lost: %+v", got)
		}
		subject, definition = cw01Definition(t, ClarificationSlot{ID: "raw", Prompt: "What date?", Kind: SlotDate, Required: true, Sensitivity: LiteralNonSensitive})
		definition.Patterns[0].Targets = []Reference{a}
		definition.Patterns[0].Policy = nil
		got = ResolveClarifications(cw01Compile(t, subject, definition), ClarificationInput{Locale: "en", Question: "Count orders", LegacyChoices: []LegacyClarificationChoice{{Slot: "raw", Value: "2026-01-01"}}})
		if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 {
			t.Fatal("legacy date dismissed clarification without a typed effect")
		}
	})
	t.Run("ConcurrentReuse", func(t *testing.T) {
		subject, definition := cw01Definition(t, cw01NumberSlot())
		model := cw01Compile(t, subject, definition)
		input := ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{cw01Answer(definition, "order_period", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "10.5", Unit: "USD"}})}}
		want := ResolveClarifications(model, input)
		var group sync.WaitGroup
		for range 16 {
			group.Go(func() {
				got := ResolveClarifications(model, input)
				if !reflect.DeepEqual(got, want) {
					t.Error("concurrent compiled policy changed")
				}
				got.Resolutions[0].Effect.Unit = "mutated"
				got.Slots[0].Effect.Unit = "mutated"
			})
		}
		group.Wait()
		if !reflect.DeepEqual(want, ResolveClarifications(model, input)) {
			t.Fatal("returned state aliases compiled policy")
		}
	})
}
