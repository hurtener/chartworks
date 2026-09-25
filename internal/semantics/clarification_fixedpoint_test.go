package semantics

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func fixedPointDefinition(t *testing.T) (RuleSubject, RuleSetDefinition) {
	t.Helper()
	created := Reference{Kind: KindColumn, Dataset: "orders", ID: "created"}
	region := Reference{Kind: KindColumn, Dataset: "orders", ID: "region"}
	choice := ClarificationSlot{ID: "basis", Prompt: "Which basis?", Required: true, Kind: SlotChoice, Sensitivity: LiteralNonSensitive,
		Choices: []ClarificationChoice{{ID: "dated", Label: "Dated", Target: &created}, {ID: "regional", Label: "Regional", Target: &region}}}
	subject, definition := cw01Definition(t, choice)
	child := definition.Patterns[0]
	child.ID = "dependent_period"
	child.Slots = []ClarificationSlot{cw01TimeSlot()}
	child.Targets = []Reference{created}
	child.Policy = &ClarificationPolicy{SchemaVersion: ClarificationSchemaVersion, When: ClarificationWhen{AnyReferences: []Reference{created}}, Why: "A dated basis requires an exact period."}
	// Put the dependent policy first: publication/storage order cannot control
	// whether an answer can activate it.
	definition.Patterns = []ClarificationPattern{child, definition.Patterns[0]}
	return subject, definition
}

func fixedPointAnswers(d RuleSetDefinition, locale string) []ClarificationAnswer {
	period := "January 2026"
	if locale == "es" {
		period = "enero 2026"
	}
	return []ClarificationAnswer{
		cw01Answer(d, "dependent_period", "period", ClarificationValue{Time: &ClarificationTimeInput{Period: period, Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires", Grain: "month"}}),
		cw01Answer(d, "order_period", "basis", ClarificationValue{OptionID: "dated"}),
	}
}

func TestSQLRecoveryAnswerDependentClarification(t *testing.T) {
	for _, locale := range []string{"en", "es"} {
		t.Run(locale, func(t *testing.T) {
			subject, d := fixedPointDefinition(t)
			model := cw01Compile(t, subject, d)
			question := "Count orders"
			if locale == "es" {
				question = "Contar pedidos"
			}
			in := ClarificationInput{Locale: locale, Question: question, Selection: &ClarificationReferenceSelection{References: []Reference{}}}
			pending := ResolveClarifications(model, in)
			if pending.Outcome != ClarificationMissing || len(pending.Resolutions) != 0 {
				t.Fatal("initial", pending)
			}
			for _, s := range pending.Slots {
				if s.Pattern == "dependent_period" && s.Outcome != ClarificationNotApplicable {
					t.Fatal("unselected branch activated", s)
				}
			}
			in.Answers = fixedPointAnswers(d, locale)[1:]
			dependent := ResolveClarifications(model, in)
			if dependent.Outcome != ClarificationMissing || len(dependent.Resolutions) != 1 {
				t.Fatal("answer did not activate required dependent question", dependent)
			}
			found := false
			for _, s := range dependent.Slots {
				found = found || s.Pattern == "dependent_period" && s.Outcome == ClarificationMissing && s.Prompt != ""
			}
			if !found {
				t.Fatal("dependent prompt absent", dependent)
			}
			in.Answers = fixedPointAnswers(d, locale)
			before, _ := json.Marshal(in)
			got := ResolveClarifications(model, in)
			if got.Outcome != ClarificationSatisfied || len(got.Resolutions) != 2 {
				t.Fatal("simultaneous dependent answers rejected", got)
			}
			for _, r := range got.Resolutions {
				if r.Time != nil && (r.Time.StartUTC != "2026-01-01T03:00:00Z" || r.Time.EndUTC != "2026-02-01T03:00:00Z") {
					t.Fatal("exact period lost", r.Time)
				}
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("caller mutated")
			}
			canonical := CanonicalClarificationAnswers(got.Resolutions)
			in.Answers = canonical
			if replay := ResolveClarifications(model, in); !reflect.DeepEqual(got, replay) {
				t.Fatal("canonical replay changed", got, replay)
			}
			in.Answers[0], in.Answers[1] = in.Answers[1], in.Answers[0]
			if reordered := ResolveClarifications(model, in); !reflect.DeepEqual(got, reordered) {
				t.Fatal("answer order changed outcome")
			}
		})
	}
}

func TestSQLRecoveryInactiveAnswersCannotSeedApplicability(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	model := cw01Compile(t, subject, d)
	for _, tc := range []struct {
		name, question string
		answers        []ClarificationAnswer
	}{
		{"unrelated", "Count customers", fixedPointAnswers(d, "en")},
		{"parent_missing", "Count orders", fixedPointAnswers(d, "en")[:1]},
		{"different_branch", "Count orders", []ClarificationAnswer{fixedPointAnswers(d, "en")[0], cw01Answer(d, "order_period", "basis", ClarificationValue{OptionID: "regional"})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: tc.question, Answers: tc.answers})
			if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 || len(got.References) != 0 {
				t.Fatal("inactive supplied answer leaked/activated", got)
			}
			found := false
			for _, e := range got.Errors {
				found = found || e.Code == "not_applicable"
			}
			if !found {
				t.Fatal("wrong inactive error", got.Errors)
			}
		})
	}
	d.Patterns[0].Policy.Disabled = true
	got := ResolveClarifications(cw01Compile(t, subject, d), ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en")})
	if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 {
		t.Fatal("disabled policy became active", got)
	}
}

func TestSQLRecoveryDerivedFactsAreNotAutomaticChoices(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	child := d.Patterns[0]
	child.ID = "independent_choice"
	child.Slots = []ClarificationSlot{d.Patterns[1].Slots[0]}
	child.Targets = d.Patterns[1].Targets
	d.Patterns = append(d.Patterns, child)
	got := ResolveClarifications(cw01Compile(t, subject, d), ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en"), Selection: &ClarificationReferenceSelection{References: []Reference{}}})
	if got.Outcome != ClarificationMissing || len(got.Resolutions) != 2 {
		t.Fatal("derived fact silently answered independent choice", got)
	}
	found := false
	for _, s := range got.Slots {
		found = found || s.Pattern == "independent_choice" && s.Outcome == ClarificationMissing
	}
	if !found {
		t.Fatal("missing independent choice")
	}
}

func TestSQLRecoveryClarificationCyclesNeedASeed(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	created := d.Patterns[0].Targets[0]
	region := Reference{Kind: KindColumn, Dataset: "orders", ID: "region"}
	a := d.Patterns[1]
	a.ID = "cycle_a"
	a.Policy = &ClarificationPolicy{SchemaVersion: 1, When: ClarificationWhen{AnyReferences: []Reference{region}}, Why: "Synthetic reference cycle."}
	b := a
	b.ID = "cycle_b"
	b.Policy = &ClarificationPolicy{SchemaVersion: 1, When: ClarificationWhen{AnyReferences: []Reference{created}}, Why: "Synthetic reference cycle."}
	d.Patterns = []ClarificationPattern{a, b}
	answers := []ClarificationAnswer{cw01Answer(d, "cycle_a", "basis", ClarificationValue{OptionID: "dated"}), cw01Answer(d, "cycle_b", "basis", ClarificationValue{OptionID: "regional"})}
	model := cw01Compile(t, subject, d)
	noSeed := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count items", Answers: answers})
	if noSeed.Outcome != ClarificationInvalid || len(noSeed.Resolutions) != 0 {
		t.Fatal("unseeded cycle bootstrapped", noSeed)
	}
	seeded := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count items", References: []Reference{region}, Answers: answers})
	if seeded.Outcome != ClarificationSatisfied || len(seeded.Resolutions) != 2 || len(seeded.References) != 2 {
		t.Fatal("seeded cycle did not converge", seeded)
	}
}

func TestSQLRecoveryDependentInvalidAndConflictAreAtomic(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	in := ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en")}
	in.Answers[0].Value.Time.Period = "2026-02-30"
	got := ResolveClarifications(cw01Compile(t, subject, d), in)
	if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 || len(got.References) != 0 {
		t.Fatal("invalid child retained partial parent", got)
	}
	other := d.Patterns[0]
	other.ID = "conflicting_period"
	other.Slots = []ClarificationSlot{cw01TimeSlot()}
	other.Slots[0].Effect.TimeZone = "UTC"
	d.Patterns = append(d.Patterns, other)
	got = ResolveClarifications(cw01Compile(t, subject, d), ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en")[1:]})
	if got.Outcome != ClarificationConflicting || len(got.Resolutions) != 0 || len(got.References) != 0 {
		t.Fatal("activated contradictory policies were not atomic", got)
	}
}

func TestSQLRecoveryAnswerDependentClarificationConcurrentCopies(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	model := cw01Compile(t, subject, d)
	in := ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en")}
	expected := ResolveClarifications(model, in)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := ResolveClarifications(model, in)
			if !reflect.DeepEqual(expected, got) {
				t.Error("nondeterministic reused model")
			}
			got.Resolutions[0].Value = "caller-mutation"
			got.Slots[0].Prompt = "caller-mutation"
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(expected, ResolveClarifications(model, in)) {
		t.Fatal("returned state aliased model/input")
	}
}

func TestSQLRecoveryClarificationClosureIsBounded(t *testing.T) {
	subject, d := fixedPointDefinition(t)
	in := ClarificationInput{Locale: "en", Question: "Count orders", Answers: fixedPointAnswers(d, "en")[1:]}
	for i := 0; i < 128; i++ {
		in.References = append(in.References, Reference{Kind: KindColumn, Dataset: "orders", ID: "region"})
	}
	got := ResolveClarifications(cw01Compile(t, subject, d), in)
	if got.Outcome != ClarificationInvalid || len(got.Resolutions) != 0 || len(got.References) != 0 || len(got.Errors) != 1 || got.Errors[0].Code != "invalid_union" {
		t.Fatal("reference bound returned a partial answer closure", got)
	}
}
