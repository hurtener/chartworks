package nlqroute

import (
	"encoding/json"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSensitiveAnswerCannotBecomeGovernedInterpretation(t *testing.T) {
	p, b := cw07Publication("topic"), cw07Binding(1)
	p.Definition.Datasets[0].Columns = append(p.Definition.Datasets[0].Columns, semantics.Column{ID: "customer", SourceName: "customer_name", Name: "Customer", NativeType: "text", Category: "text", Sensitivity: semantics.LiteralSensitive})
	b.Relations[0].Columns = append(b.Relations[0].Columns, readexec.Column{Name: "customer_name", NativeType: "text", Category: "text", Safe: true})
	target := semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "customer"}
	pattern := semantics.ClarificationPattern{ID: "customer", Version: "v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-private-span-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"revenue"}}, Why: "Choose the reviewed customer."}, Slots: []semantics.ClarificationSlot{{ID: "customer", Kind: semantics.SlotText, Required: true, Sensitivity: semantics.LiteralSensitive, Prompt: "Which customer?", Effect: &semantics.ClarificationEffect{Kind: "entity", Target: target, Operator: "eq", Nulls: "exclude", MaxLength: 64, Values: []semantics.GovernedClarificationValue{{Canonical: "PRIVATE_CUSTOMER", Label: "Reviewed customer", Aliases: []string{"north"}}}}}}}
	service, engine := cw07Service(t, p, b)
	service.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{pattern})
	e := testEnvelope(t, true)
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north in March 2025", InterpretationAnchor: "2026-09-22"}
	pending, err := service.Route(t.Context(), e, in)
	if err != nil || pending.Clarification == nil {
		t.Fatal("source-bound private preflight", err)
	}
	text := "north"
	in.AnswerContext = pending.AnswerContext
	in.Answers = []semantics.ClarificationAnswer{{Topic: "topic", TopicVersion: "v1", RulesetVersion: "r1", Pattern: "customer", PatternVersion: "v1", Slot: "customer", Value: &semantics.ClarificationValue{Text: &text}}}
	out, err := service.Route(t.Context(), e, in)
	if err != nil || out.Clarification != nil || out.Context == nil {
		t.Fatal("answered private route", err, out.Clarification)
	}
	if out.Interpretation == nil || len(out.Interpretation.Values) != 0 || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != "2025-03-01" {
		t.Fatal("private answer manufactured public governed interpretation or lost independent time")
	}
	constraints, err := out.ResolvedBusinessConstraints()
	if err != nil || len(constraints) != 2 {
		t.Fatal("private and independent temporal constraints", err)
	}
	raw, _ := json.Marshal(out)
	var saved RouteResult
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	calls := engine.embeds
	replay, _, err := service.ReplayClarifications(t.Context(), e, saved)
	if err != nil || readexec.Hash(replay) != readexec.Hash(constraints) || engine.embeds != calls {
		t.Fatal("canonical private replay drift or model work", err)
	}
}

func TestProtectedInferenceSurfaceBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, question, secret, want string }{
		{"unicode-boundary", "Revenue SECRET\u00a0in March 2025", "SECRET", "revenue \ufffc in march 2025"},
		{"touching-suffix", "Revenue SECRETMarch in north", "SECRET", "revenue \ufffc in north"},
		{"touching-prefix", "Revenue MarchSECRET in north", "SECRET", "revenue \ufffc in north"},
		{"marker-touching-suffix", "Revenue [redacted answer]March in north", "SECRET", "revenue \ufffc in north"},
		{"marker-unicode-boundary", "Revenue [redacted answer]\u00a0March 2025", "SECRET", "revenue \ufffc march 2025"},
		{"punctuation-value", "Revenue by *** month", "***", "revenue by \ufffc month"},
		{"punctuation-independent-value", "Revenue SECRET,north", "SECRET", "revenue \ufffc north"},
		{"marker-punctuation-independent-value", "Revenue [redacted answer]/north", "SECRET", "revenue \ufffc north"},
		{"single-marker", "Revenue not [redacted answer] in north", "SECRET", "revenue not \ufffc in north"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := RouteRequest{Locale: nlq.LanguageEnglish, Question: tc.question, Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "customer", Value: &semantics.ClarificationValue{Text: &tc.secret}}}}
			items := []admittedTopic{{id: "topic"}}
			items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "customer", Slots: []semantics.ClarificationSlot{{ID: "customer", Sensitivity: semantics.LiteralSensitive}}}}
			got := questionInferenceSurface(in, items)
			if got.text != tc.want {
				t.Fatalf("surface = %q, want %q", got.text, tc.want)
			}
			if !got.stablePhrasePolarity("north") {
				t.Fatal("opaque replay marker changed public negation")
			}
		})
	}
}

func protectedReplayFixture(t *testing.T, secret string, useDefault bool) (*Service, *testEngine, semantics.ClarificationAnswer) {
	t.Helper()
	p, b := cw07Publication("topic"), cw07Binding(1)
	p.Definition.Dimensions[1].Temporal.Grains = []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter}
	p.Definition.Datasets[0].Columns = append(p.Definition.Datasets[0].Columns, semantics.Column{ID: "customer", SourceName: "customer_name", Name: "Customer", NativeType: "text", Category: "text", Sensitivity: semantics.LiteralSensitive})
	b.Relations[0].Columns = append(b.Relations[0].Columns, readexec.Column{Name: "customer_name", NativeType: "text", Category: "text", Safe: true})
	target := semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "customer"}
	slot := semantics.ClarificationSlot{ID: "customer", Kind: semantics.SlotText, Required: true, Sensitivity: semantics.LiteralSensitive, Prompt: "Which customer?", Effect: &semantics.ClarificationEffect{Kind: "entity", Target: target, Operator: "eq", Nulls: "exclude", MaxLength: 128, Values: []semantics.GovernedClarificationValue{{Canonical: "PRIVATE_CUSTOMER", Label: "Reviewed customer", Aliases: []string{secret}}}}}
	value := &semantics.ClarificationValue{Text: &secret}
	if useDefault {
		slot.Required = false
		slot.Default = value
	}
	pattern := semantics.ClarificationPattern{ID: "customer", Version: "v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-private-span-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"revenue", "ingresos"}}, Why: "Choose the reviewed customer."}, Slots: []semantics.ClarificationSlot{slot}}
	service, engine := cw07Service(t, p, b)
	service.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{pattern})
	answer := semantics.ClarificationAnswer{Topic: "topic", TopicVersion: "v1", RulesetVersion: "r1", Pattern: "customer", PatternVersion: "v1", Slot: "customer", Value: value}
	return service, engine, answer
}

func TestProtectedAnswerPolarityAndCanonicalReplay(t *testing.T) {
	for _, tc := range []struct {
		name, question, secret, locale, operator, start string
		refusal, defaults, continuation                 bool
	}{
		{name: "private-north", question: "Revenue north in March 2025", secret: "north", locale: "en", start: "2025-03-01"},
		{name: "private-canonical", question: "Revenue PRIVATE_CUSTOMER in north in March 2025", secret: "customer alias", locale: "en", operator: "eq", start: "2025-03-01"},
		{name: "punctuation-public-value", question: "Revenue SECRET,north", secret: "SECRET", locale: "en", operator: "eq"},
		{name: "inert-default", question: "Revenue for customer alias in north in March 2025", secret: "customer alias", locale: "en", operator: "eq", start: "2025-03-01", defaults: true},
		{name: "private-period-independent-relative-month", question: "Revenue for not in March 2025 during last month", secret: "not in March 2025", locale: "en", start: "2026-08-01"},
		{name: "private-period-independent-relative-quarter", question: "Revenue for not in March 2025 during last quarter", secret: "not in March 2025", locale: "en", start: "2026-04-01", continuation: true},
		{name: "private-period-independent-public-period", question: "Revenue for not in March 2025 in April 2024", secret: "not in March 2025", locale: "en", start: "2024-04-01"},
		{name: "wholly-private-negation", question: "Revenue for not in March 2025", secret: "not in March 2025", locale: "en"},
		{name: "private-not-value", question: "Revenue not in north", secret: "not", locale: "en", refusal: true},
		{name: "private-sin-value", question: "Ingresos sin norte", secret: "sin", locale: "es", refusal: true},
		{name: "private-not-period", question: "Revenue not in March 2025", secret: "not", locale: "en", refusal: true},
		{name: "private-sin-period", question: "Ingresos sin marzo 2025", secret: "sin", locale: "es", refusal: true},
		{name: "private-continuation", question: "Revenue not last quarter", secret: "not", locale: "en", refusal: true, continuation: true},
		{name: "private-spanish-continuation", question: "Ingresos sin este trimestre", secret: "sin", locale: "es", refusal: true, continuation: true},
		{name: "public-not-value", question: "Revenue not SECRET in north", secret: "SECRET", locale: "en", operator: "ne"},
		{name: "public-sin-value", question: "Ingresos sin SECRET en norte", secret: "SECRET", locale: "es", operator: "ne"},
		{name: "public-not-period", question: "Revenue SECRET not in March 2025", secret: "SECRET", locale: "en", refusal: true},
		{name: "unicode-independent-time", question: "Revenue SECRET\u00a0in March 2025", secret: "SECRET", locale: "en", start: "2025-03-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, answer := protectedReplayFixture(t, tc.secret, tc.defaults)
			e := testEnvelope(t, true)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.Language(tc.locale), Question: "Revenue", InterpretationAnchor: "2026-09-22"}
			pending, err := service.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal("preflight", err)
			}
			in.Question = tc.question
			if !tc.defaults {
				in.Answers = []semantics.ClarificationAnswer{answer}
				in.AnswerContext = pending.AnswerContext
			}
			if tc.continuation {
				in.InterpretationPolicy = InterpretationContinuationPolicy
			}
			out, err := service.Route(t.Context(), e, in)
			if tc.refusal {
				if err == nil && out.Clarification == nil {
					t.Fatal("material private/public polarity change became executable")
				}
				if out.Clarification != nil && len(out.Request.Answers) > 0 {
					t.Fatal("refusal retained private answers")
				}
				return
			}
			if err != nil || out.Clarification != nil || out.Context == nil {
				t.Fatal("answered route", err, out.Clarification)
			}
			if tc.operator == "" {
				if len(out.Interpretation.Values) != 0 {
					t.Fatal("private value became public filtering")
				}
			} else if len(out.Interpretation.Values) != 1 || out.Interpretation.Values[0].Operator != tc.operator {
				t.Fatal("public filtering changed")
			}
			if tc.start == "" {
				if len(out.Interpretation.Temporal) != 0 {
					t.Fatal("private period became temporal inference")
				}
			} else if len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != tc.start {
				t.Fatal("independent public period lost")
			}
			constraints, err := out.ResolvedBusinessConstraints()
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(out)
			var saved RouteResult
			if json.Unmarshal(raw, &saved) != nil {
				t.Fatal("saved route")
			}
			calls := engine.embeds
			replay, _, err := service.ReplayClarifications(t.Context(), e, saved)
			if err != nil || readexec.Hash(replay) != readexec.Hash(constraints) || engine.embeds != calls {
				t.Fatal("zero-model canonical replay", err)
			}
		})
	}
}

func TestProtectedCatalogOccurrenceAndMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, question, secret string
		selected, refusal      bool
	}{
		{"repeated-no-private", "not Revenue but Revenue", "", true, false},
		{"repeated-unrelated-private", "not Revenue but Revenue for SECRET", "SECRET", true, false},
		{"canonical-private-label", "PRIVATE_CUSTOMER", "customer alias", false, false},
		{"public-marker", "[redacted answer]", "", false, false},
		{"private-negator", "not Revenue", "not", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := recoveryPublication()
			p.Definition.Measures[0].Aliases = append(p.Definition.Measures[0].Aliases, "PRIVATE_CUSTOMER", "[redacted answer]", "redacted", "answer")
			items := []admittedTopic{{id: "topic", publication: p}}
			in := RouteRequest{Locale: nlq.LanguageEnglish, Question: tc.question}
			if tc.secret != "" {
				items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "customer", Slots: []semantics.ClarificationSlot{{ID: "customer", Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical: "PRIVATE_CUSTOMER", Aliases: []string{tc.secret}}}}}}}}
				in.Answers = []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "customer", Value: &semantics.ClarificationValue{Text: &tc.secret}}}
			}
			err := initialSemanticSelection(t.Context(), in, items, nil)
			if tc.refusal {
				if err == nil {
					t.Fatal("private negation selected public catalog term")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			selected := items[0].selection.roots[semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}] != ""
			if selected != tc.selected {
				t.Fatal("wrong catalog selection")
			}
		})
	}
}

func TestProtectedImplicitGroupingPolarity(t *testing.T) {
	for _, tc := range []struct {
		name, question, secret string
		selected, refusal      bool
	}{
		{"public-negation", "Revenue not by month", "SECRET", false, false},
		{"private-negation", "Revenue not by month", "not", false, true},
		{"private-grouping", "Revenue for by month", "by month", false, false},
		{"private-quote", "Revenue for O'Private by month", "O'Private", true, false},
		{"ordinary-private", "Revenue for SECRET by month", "SECRET", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := recoveryPublication()
			p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "event", Name: "Order date", Field: recoveryColumn("dataset", "event"), Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{"month"}}})
			items := []admittedTopic{{id: "topic", publication: p}}
			items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "customer", Slots: []semantics.ClarificationSlot{{ID: "customer", Sensitivity: semantics.LiteralSensitive}}}}
			in := RouteRequest{Locale: nlq.LanguageEnglish, Question: tc.question, Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "customer", Value: &semantics.ClarificationValue{Text: &tc.secret}}}}
			if err := initialSemanticSelection(t.Context(), in, items, nil); err != nil {
				t.Fatal(err)
			}
			err := selectImplicitCalendar(t.Context(), in, items)
			if tc.refusal {
				if err == nil {
					t.Fatal("private negation manufactured grouping")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			selected := items[0].selection.roots[semantics.Reference{Kind: semantics.KindDimension, ID: "event"}] != ""
			if selected != tc.selected {
				t.Fatal("wrong calendar grouping selection")
			}
		})
	}
}
