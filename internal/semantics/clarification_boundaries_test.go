package semantics

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func cw01BoundaryBooleanSlot() ClarificationSlot {
	return ClarificationSlot{ID: "active", Prompt: "Which active state?", Kind: SlotBoolean, Required: true, Sensitivity: LiteralNonSensitive, Effect: &ClarificationEffect{Kind: "boolean", Target: Reference{Kind: KindColumn, Dataset: "orders", ID: "active"}, Operator: "eq", Nulls: "exclude"}}
}

func cw01BoundaryTextSlot() ClarificationSlot {
	return ClarificationSlot{ID: "region", Prompt: "Which reviewed region?", Kind: SlotText, Required: true, Sensitivity: LiteralSensitive, Effect: &ClarificationEffect{Kind: "entity", Target: Reference{Kind: KindColumn, Dataset: "orders", ID: "region"}, Operator: "eq", Nulls: "exclude", MaxLength: 64, Values: []GovernedClarificationValue{{Canonical: "N-01", Label: "North", Aliases: []string{"north", "norte"}}, {Canonical: "S-01", Label: "South", Aliases: []string{"south", "sur"}}}}}
}

func TestClarificationAuthoringBoundaryRejections(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ClarificationPattern)
	}{
		{"schema", func(p *ClarificationPattern) { p.Policy.SchemaVersion = 2 }},
		{"priority-low", func(p *ClarificationPattern) { p.Policy.Priority = -1001 }},
		{"priority-high", func(p *ClarificationPattern) { p.Policy.Priority = 1001 }},
		{"missing-explanation", func(p *ClarificationPattern) { p.Policy.Why = "" }},
		{"multiline-explanation", func(p *ClarificationPattern) { p.Policy.Why = "first\nsecond" }},
		{"localized-explanation", func(p *ClarificationPattern) { p.Policy.WhySpanish = "primero\nsegundo" }},
		{"missing-condition", func(p *ClarificationPattern) { p.Policy.When = ClarificationWhen{} }},
		{"too-many-terms", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = make([]string, 33) }},
		{"too-many-references", func(p *ClarificationPattern) { p.Policy.When.AnyReferences = make([]Reference, 33) }},
		{"empty-term", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = []string{""} }},
		{"punctuation-term", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = []string{"!!!"} }},
		{"long-term", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = []string{strings.Repeat("a", 129)} }},
		{"too-many-words", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = []string{"one two three four five six seven eight nine"} }},
		{"duplicate-normalized-term", func(p *ClarificationPattern) { p.Policy.When.AnyTerms = []string{"orders", "ORDERS"} }},
		{"malformed-condition-reference", func(p *ClarificationPattern) { p.Policy.When.AnyReferences = []Reference{{Kind: KindColumn, ID: "amount"}} }},
		{"duplicate-condition-reference", func(p *ClarificationPattern) { p.Policy.When.AnyReferences = []Reference{p.Targets[0], p.Targets[0]} }},
		{"foreign-condition-reference", func(p *ClarificationPattern) { p.Policy.When.AnyReferences = []Reference{{Kind: KindColumn, Dataset: "orders", ID: "unknown"}} }},
		{"unscoped-condition-reference", func(p *ClarificationPattern) { p.Policy.When.AnyReferences = []Reference{{Kind: KindColumn, Dataset: "orders", ID: "created"}} }},
		{"localized-prompt", func(p *ClarificationPattern) { p.Slots[0].PromptES = "primero\nsegundo" }},
		{"too-many-dependencies", func(p *ClarificationPattern) { p.Slots[0].DependsOn = make([]string, 17) }},
		{"unknown-dependency", func(p *ClarificationPattern) { p.Slots[0].DependsOn = []string{"unknown"} }},
		{"self-dependency", func(p *ClarificationPattern) { p.Slots[0].DependsOn = []string{p.Slots[0].ID} }},
		{"required-default", func(p *ClarificationPattern) { p.Slots[0].Default = &ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}} }},
		{"invalid-optional-default", func(p *ClarificationPattern) {
			p.Slots[0].Required = false
			p.Slots[0].Default = &ClarificationValue{Number: &ClarificationNumberInput{Value: "NaN", Unit: "USD"}}
		}},
		{"legacy-effect-without-review", func(p *ClarificationPattern) { p.Policy = nil }},
		{"legacy-default-without-review", func(p *ClarificationPattern) {
			p.Policy = nil
			p.Slots[0].Effect = nil
			p.Slots[0].Default = &ClarificationValue{Null: true}
		}},
		{"legacy-dependency-without-review", func(p *ClarificationPattern) {
			p.Policy = nil
			p.Slots[0].Effect = nil
			p.Slots[0].DependsOn = []string{"unknown"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subject, definition := cw01Definition(t, cw01NumberSlot())
			original := cw01Compile(t, subject, definition)
			before := original.Definition()
			changed := original.Definition()
			tc.mutate(&changed.Patterns[0])
			if _, err := CompilePublishedRules(subject, changed); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid authoring was not rejected by the real compiler", err)
			}
			if !reflect.DeepEqual(before, original.Definition()) {
				t.Fatal("failed authoring mutated the retained compiled policy")
			}
		})
	}
}

func TestClarificationEffectBoundaryRejections(t *testing.T) {
	cases := []struct {
		name   string
		make   func() ClarificationSlot
		mutate func(*ClarificationSlot)
	}{
		{"missing-effect", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect = nil }},
		{"non-field-target", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Target = Reference{Kind: KindDataset, ID: "orders"} }},
		{"invalid-target", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Target = Reference{} }},
		{"unknown-null-policy", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Nulls = "default" }},
		{"oversized-grains", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Grains = make([]string, 6) }},
		{"oversized-values", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values = make([]GovernedClarificationValue, 65) }},
		{"wrong-time-kind", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Kind = "number" }},
		{"wrong-time-operator", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Operator = "eq" }},
		{"wrong-time-bounds", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Bounds = "[]" }},
		{"wrong-calendar", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Calendar = "unknown" }},
		{"unknown-zone", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.TimeZone = "Unknown/Zone" }},
		{"oversized-zone", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.TimeZone = strings.Repeat("x", 129) }},
		{"no-grain", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Grains = nil }},
		{"duplicate-grain", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Grains = []string{"month", "month"} }},
		{"unknown-grain", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Grains = []string{"minute"} }},
		{"unknown-temporal-type", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.TemporalType = "local" }},
		{"time-with-unit", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.Unit = "USD" }},
		{"time-with-text-limit", cw01TimeSlot, func(s *ClarificationSlot) { s.Effect.MaxLength = 10 }},
		{"number-with-wrong-kind", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Kind = "entity" }},
		{"number-without-unit", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Unit = "" }},
		{"number-invalid-precision", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Precision = 0 }},
		{"number-oversized-precision", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Precision = 77 }},
		{"number-negative-scale", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Scale = -1 }},
		{"number-scale-exceeds-precision", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Scale = 39 }},
		{"range-without-bounds", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Operator = "range" }},
		{"scalar-with-range-bounds", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Bounds = "[]" }},
		{"number-with-calendar", cw01NumberSlot, func(s *ClarificationSlot) { s.Effect.Calendar = "gregorian" }},
		{"boolean-wrong-kind", cw01BoundaryBooleanSlot, func(s *ClarificationSlot) { s.Effect.Kind = "number" }},
		{"boolean-wrong-operator", cw01BoundaryBooleanSlot, func(s *ClarificationSlot) { s.Effect.Operator = "ne" }},
		{"boolean-with-dictionary", cw01BoundaryBooleanSlot, func(s *ClarificationSlot) { s.Effect.Values = []GovernedClarificationValue{{Canonical: "true", Label: "True"}} }},
		{"text-wrong-kind", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Kind = "sql" }},
		{"text-wrong-operator", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Operator = "gt" }},
		{"text-empty-dictionary", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values = nil }},
		{"text-no-length-limit", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.MaxLength = 0 }},
		{"text-oversized-length-limit", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.MaxLength = 513 }},
		{"text-with-numeric-unit", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Unit = "USD" }},
		{"text-empty-canonical", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].Canonical = "" }},
		{"text-duplicate-canonical", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[1].Canonical = "N-01" }},
		{"text-empty-label", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].Label = "" }},
		{"text-bad-localized-label", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].LabelES = "Norte\nSur" }},
		{"text-too-many-aliases", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].Aliases = make([]string, 17) }},
		{"text-empty-alias", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].Aliases = []string{""} }},
		{"text-control-alias", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[0].Aliases = []string{"north\x00"} }},
		{"text-ambiguous-alias", cw01BoundaryTextSlot, func(s *ClarificationSlot) { s.Effect.Values[1].Aliases = []string{"NORTH"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subject, definition := cw01Definition(t, tc.make())
			model := cw01Compile(t, subject, definition)
			before := model.Definition()
			changed := model.Definition()
			tc.mutate(&changed.Patterns[0].Slots[0])
			if _, err := CompilePublishedRules(subject, changed); !errors.Is(err, ErrInvalid) {
				t.Fatal("mixed or invalid effect was accepted", err)
			}
			if !reflect.DeepEqual(before, model.Definition()) {
				t.Fatal("failed effect compilation mutated shared state")
			}
		})
	}
}

func TestClarificationChoiceReviewAndDependencyGuards(t *testing.T) {
	a, b := Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Reference{Kind: KindColumn, Dataset: "orders", ID: "created"}
	slot := ClarificationSlot{ID: "field", Prompt: "Which field?", Kind: SlotChoice, Required: true, Sensitivity: LiteralNonSensitive, Choices: []ClarificationChoice{{ID: "amount", Label: "Amount", Target: &a}, {ID: "created", Label: "Created", Target: &b}}}
	for _, mutate := range []func(*ClarificationSlot){
		func(s *ClarificationSlot) { s.Effect = cw01NumberSlot().Effect },
		func(s *ClarificationSlot) { s.Choices[0].Target = nil },
		func(s *ClarificationSlot) { s.Choices[0].Target = &Reference{} },
		func(s *ClarificationSlot) { s.Choices[0].LabelES = "primero\nsegundo" },
	} {
		subject, definition := cw01Definition(t, slot)
		copy := cw01Compile(t, subject, definition).Definition()
		mutate(&copy.Patterns[0].Slots[0])
		if _, err := CompilePublishedRules(subject, copy); !errors.Is(err, ErrInvalid) {
			t.Fatal("unreviewed or malformed reference choice accepted", err)
		}
	}
	for _, slots := range [][]ClarificationSlot{
		{{ID: "a"}, {ID: "a"}},
		{{ID: "bad id"}},
		{{ID: "a", DependsOn: []string{"missing"}}},
		{{ID: "a", DependsOn: []string{"b", "b"}}, {ID: "b"}},
		{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}},
	} {
		out, err := orderedClarificationSlots(slots)
		if !errors.Is(err, ErrInvalid) || len(out) != 0 {
			t.Fatal("invalid dependency graph returned partial presentation", err)
		}
	}
	subject, definition := cw01Definition(t, cw01NumberSlot())
	definition.Patterns[0].Policy.Disabled = true
	definition.Patterns[0].Policy.When = ClarificationWhen{}
	model := cw01Compile(t, subject, definition)
	out := ResolveClarifications(model, ClarificationInput{Locale: "en", Question: "Count orders"})
	if out.Outcome != ClarificationNotApplicable || len(out.Resolutions) != 0 || ClarificationMigrationDisposition(definition.Patterns[0]) != "reviewed_disabled" {
		t.Fatal("reviewed disabled migration became an accidental blocker")
	}
}

func TestClarificationScalarFailureIsAtomic(t *testing.T) {
	word := "unreviewed-secret"
	cases := []struct {
		name  string
		slot  ClarificationSlot
		value ClarificationValue
		lang  string
		code  string
	}{
		{"locale", cw01NumberSlot(), ClarificationValue{Null: true}, "fr", "invalid_union"},
		{"date-union", cw01TimeSlot(), ClarificationValue{Text: &word}, "en", "invalid_union"},
		{"number-union", cw01NumberSlot(), ClarificationValue{Text: &word}, "en", "invalid_union"},
		{"boolean-union", cw01BoundaryBooleanSlot(), ClarificationValue{Text: &word}, "es", "invalid_boolean"},
		{"text-union", cw01BoundaryTextSlot(), ClarificationValue{Boolean: &word}, "en", "unresolved_value"},
		{"null-excluded", cw01NumberSlot(), ClarificationValue{Null: true}, "es", "null_not_allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ResolveClarificationValue(tc.slot, tc.value, tc.lang)
			if err == nil || err.Code != tc.code || !reflect.DeepEqual(out, ClarificationResolution{}) || strings.Contains(err.Message, word) || err.Field == "" {
				t.Fatal("invalid scalar returned partial evidence or an unhelpful/leaky field error")
			}
		})
	}
	for _, bounds := range []string{"[]", "[)", "(]", "()"} {
		slot := cw01NumberSlot()
		slot.Effect.Operator, slot.Effect.Bounds = "range", bounds
		for _, input := range []ClarificationNumberInput{{Value: "2", Upper: "1", Unit: "USD"}, {Value: "1", Upper: "NaN", Unit: "USD"}, {Value: "1", Upper: "1", Unit: "USD"}} {
			out, err := ResolveClarificationValue(slot, ClarificationValue{Number: &input}, "es")
			if input.Value == "1" && input.Upper == "1" && bounds == "[]" {
				if err != nil || out.Value != "1" || out.Upper != "1" {
					t.Fatal("closed singleton interval was lost", err)
				}
			} else if err == nil || !reflect.DeepEqual(out, ClarificationResolution{}) || err.Field != "number.upper" {
				t.Fatal("invalid interval returned a partial lower bound")
			}
		}
	}
	slot := cw01NumberSlot()
	slot.Effect.Nulls = "only"
	null, err := ResolveClarificationValue(slot, ClarificationValue{Null: true}, "en")
	if err != nil || !null.Null || null.Value != "" || null.Effect.Nulls != "only" {
		t.Fatal("explicit reviewed null semantics were lost", err)
	}
	out, err := ResolveClarificationValue(slot, ClarificationValue{Number: &ClarificationNumberInput{Value: "1", Unit: "USD"}}, "en")
	if err == nil || err.Code != "null_not_allowed" || !reflect.DeepEqual(out, ClarificationResolution{}) {
		t.Fatal("nonnull scalar satisfied null-only policy")
	}
}
