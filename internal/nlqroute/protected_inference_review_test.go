package nlqroute

import (
	"encoding/json"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestProtectedMixedExpressionsCannotLosePublicMeaning(t *testing.T) {
	for _, tc := range []struct {
		name, secret, question, start, end string
		value                              bool
	}{
		{"partial-governed-token", "nor", "Revenue for nor in North in March 2025", "2025-03-01", "2025-04-01", true},
		{"partial-year-token", "20", "Revenue for 20 in March 2025", "2025-03-01", "2025-04-01", false},
		{"private-year-component", "2025", "Revenue for 2025 in March 2025", "2025-03-01", "2025-04-01", false},
		{"private-range-component", "January", "Revenue from January through March 2025", "2025-01-01", "2025-04-01", false},
		{"private-negation-connector", "in", "Revenue not in March 2025", "", "", false},
		{"private-range-negator", "excluding", "Revenue excluding the period from January through March 2025", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, answer := protectedReplayFixture(t, tc.secret, false)
			e := testEnvelope(t, true)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"}
			pending, err := s.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal(err)
			}
			in.Question = tc.question
			in.Answers = []semantics.ClarificationAnswer{answer}
			in.AnswerContext = pending.AnswerContext
			out, err := s.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal("expected typed route clarification, not unclassified failure", err)
			}
			if out.Clarification != nil {
				return
			}
			if tc.start == "" {
				t.Fatal("unsupported temporal negation became executable")
			}
			if out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != tc.start || out.Interpretation.Temporal[0].End != tc.end {
				t.Fatal("mixed private/public period silently changed")
			}
			if tc.value && (len(out.Interpretation.Values) != 1 || out.Interpretation.Values[0].GovernedValue != "north") {
				t.Fatal("partial private token silently removed public North filter")
			}
		})
	}
}

func TestProtectedGroundedPolarityRefusesBeforeGenerate(t *testing.T) {
	for _, grouping := range []bool{false, true} {
		name := "concept"
		question := "not Revenue"
		if grouping {
			name = "categorical-grouping"
			question = "Revenue not by Product family"
		}
		t.Run(name, func(t *testing.T) {
			s, model := newGroundedFixture(t)
			model.response = func(q string, c []conceptCard) conceptchoice.Proposal { return choiceByID("Revenue", c, "revenue") }
			in := groundedRequest(question)
			var group *groupingIntentEngine
			if grouping {
				s, group = newGroupingIntentFixture(t)
				in = groupingIntentRequest(question)
				group.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal {
					return groupingProposal("Product family", c, "family", "")
				}
			}
			p := s.topics.(*testTopics).contract.Publication
			target := recoveryColumn("dataset", "amount")
			secret := "not"
			pattern := semantics.ClarificationPattern{ID: "private", Version: "v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-private-polarity"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"Revenue"}}, Why: "Reviewed private input."}, Slots: []semantics.ClarificationSlot{{ID: "value", Kind: semantics.SlotText, Sensitivity: semantics.LiteralSensitive, Prompt: "Private input?", Effect: &semantics.ClarificationEffect{Kind: "text", Target: target, Operator: "eq", Nulls: "exclude", MaxLength: 64, Values: []semantics.GovernedClarificationValue{{Canonical: "PRIVATE", Label: "Reviewed value", Aliases: []string{secret}}}}}}}
			// The dictionary alone classifies the known sensitive spelling before any model work.
			s.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{pattern})
			out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			calls := model.calls
			if grouping {
				calls = group.calls
			}
			if calls != 0 {
				t.Fatal("private negator reached grounded Generate")
			}
			if err == nil && out.Clarification == nil {
				t.Fatal("private negator became learned selection")
			}
		})
	}
}

func TestProtectedEarlyRequirementsUseSafeQuestion(t *testing.T) {
	for _, secret := range []string{"hourly", "net revenue"} {
		t.Run(secret, func(t *testing.T) {
			s, _, answer := protectedReplayFixture(t, secret, false)
			e := testEnvelope(t, true)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"}
			pending, err := s.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal(err)
			}
			in.AnswerContext = pending.AnswerContext
			in.Answers = []semantics.ClarificationAnswer{answer}
			in.Question = "Revenue for " + secret + " in March 2025"
			out, err := s.Route(t.Context(), e, in)
			if err != nil || out.Clarification != nil || out.Context == nil {
				t.Fatal("wholly private requirement altered public request", err, out.Clarification)
			}
			public, _ := json.Marshal(out.Request)
			if strings.Contains(strings.ToLower(string(public)), secret) {
				t.Fatal("private spelling retained")
			}
			in.Question = "Revenue for " + secret + " hourly in March 2025"
			if secret == "hourly" {
				in.Question = "Revenue for hourly net revenue in March 2025"
			}
			refused, err := s.Route(t.Context(), e, in)
			if err != nil || refused.Clarification == nil {
				t.Fatal("public unsupported requirement not refused", err)
			}
			if len(refused.Request.Answers) != 0 {
				t.Fatal("early refusal retained private answer payload")
			}
			if strings.Contains(strings.ToLower(refused.Request.Question), secret) {
				t.Fatal("early refusal retained private question spelling")
			}
		})
	}
}

func TestProtectedReviewedCalendarGroupingUsesSafeSurface(t *testing.T) {
	for _, tc := range []struct {
		name, secret, question string
		grouped, refusal       bool
	}{
		{"private-month-with-public-period", "by month", "Revenue for by month in March 2025", false, false},
		{"private-local-month-with-public-period", "by local calendar month", "Revenue for by local calendar month in March 2025", false, false},
		{"public-month-with-public-period", "SECRET", "Revenue for SECRET by month in March 2025", true, false},
		{"public-local-month-with-public-period", "SECRET", "Revenue for SECRET by local calendar month in March 2025", true, false},
		{"private-distant-grouping-negator", "not", "Revenue not grouped by month in March 2025", false, true},
		{"private-distant-monthly-negator", "not", "Revenue not grouped monthly in March 2025", false, true},
		{"public-distant-grouping-negator", "SECRET", "Revenue for SECRET not grouped by month in March 2025", false, true},
		{"public-grouped-by-month", "SECRET", "Revenue for SECRET grouped by month in March 2025", true, false},
		{"private-local-grouping-negator", "not", "Revenue not by local calendar month in March 2025", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, engine, answer := protectedReplayFixture(t, tc.secret, false)
			e := testEnvelope(t, true)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue", InterpretationAnchor: "2026-09-22"}
			pending, err := s.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal(err)
			}
			in.Question = tc.question
			in.Answers = []semantics.ClarificationAnswer{answer}
			in.AnswerContext = pending.AnswerContext
			out, err := s.Route(t.Context(), e, in)
			if err != nil {
				t.Fatal(err)
			}
			if tc.refusal {
				if out.Clarification == nil {
					t.Fatal("private negation became local calendar grouping")
				}
				return
			}
			if out.Clarification != nil || out.Context == nil {
				t.Fatal("public period failed", out.Clarification)
			}
			if (out.Request.Grouping != nil) != tc.grouped {
				t.Fatal("private phrase manufactured reviewed calendar grouping")
			}
			if tc.grouped && (len(out.Request.Grouping.Keys) != 1 || out.Request.Grouping.Keys[0].Grain != semantics.GrainMonth) {
				t.Fatal("public month grouping changed")
			}
			if out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != "2025-03-01" {
				t.Fatal("independent public period changed")
			}
			expected, err := out.ResolvedBusinessConstraints()
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(out)
			var saved RouteResult
			if json.Unmarshal(raw, &saved) != nil {
				t.Fatal("saved route")
			}
			before := engine.embeds
			replay, _, err := s.ReplayClarifications(t.Context(), e, saved)
			if err != nil || readexec.Hash(replay) != readexec.Hash(expected) || engine.embeds != before {
				t.Fatal("zero-model grouping replay changed", err)
			}
		})
	}
}

func TestProtectedCatalogKeepsIndependentRepeatedOccurrence(t *testing.T) {
	p := recoveryPublication()
	items := []admittedTopic{{id: "topic", publication: p}}
	secret := "not Revenue"
	items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "private", Slots: []semantics.ClarificationSlot{{ID: "value", Sensitivity: semantics.LiteralSensitive}}}}
	in := RouteRequest{Locale: nlq.LanguageEnglish, Question: "not Revenue but Revenue", Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "private", Slot: "value", Value: &semantics.ClarificationValue{Text: &secret}}}}
	if err := protectedCatalogMeaning(in, items); err != nil {
		t.Fatal("wholly private first occurrence negated independent public second occurrence", err)
	}
	if err := initialSemanticSelection(t.Context(), in, items, nil); err != nil {
		t.Fatal(err)
	}
	if items[0].selection.roots[semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}] == "" {
		t.Fatal("independent public Revenue not selected")
	}
}
