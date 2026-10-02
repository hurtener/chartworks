package nlqroute

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestQuestionRequirementsCannotAcceptUnrelatedScalar(t *testing.T) {
	for _, tc := range []struct {
		q, reason string
		locale    nlq.Language
	}{
		{"Give definitive net revenue even though one posted refund amount is unknown.", "unknown_amount_policy_required", nlq.LanguageEnglish},
		{"Give the definitive paid gross total without caveats even though an order amount is unknown.", "unknown_amount_policy_required", nlq.LanguageEnglish},
		{"Some revenue amounts are missing; give a definitive total.", "unknown_amount_policy_required", nlq.LanguageEnglish},
		{"Quiero el total definitivo de ventas con importes desconocidos.", "unknown_amount_policy_required", nlq.LanguageSpanish},
		{"Show hourly paid revenue during the repeated 1 a.m. on the fall DST transition.", "ambiguous_local_time", nlq.LanguageEnglish},
		{"During daylight saving transition, show revenue for the repeated hour.", "ambiguous_local_time", nlq.LanguageEnglish},
		{"Muestra ingresos por la hora repetida durante el cambio de horario de verano.", "ambiguous_local_time", nlq.LanguageSpanish},
	} {
		t.Run(tc.q, func(t *testing.T) {
			s, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
			out, err := s.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.q})
			if err != nil || out.Clarification == nil || out.Clarification.Reason != tc.reason || out.Context != nil || engine.embeds != 0 {
				t.Fatal("unresolved intent reached generation", err, out.Clarification, engine.embeds)
			}
		})
	}
	for _, q := range []string{"Show known paid revenue and report unknown amounts separately", "Do not give a definitive total when amounts are unknown", "Show the definitive total; no unknown amounts remain", `Show "definitive unknown revenue"`, "Show yearly revenue during daylight saving time"} {
		if got := unresolvedQuestionRequirements(RouteRequest{Question: q, Locale: nlq.LanguageEnglish}); got != nil {
			t.Fatal("invented requirement", q, got)
		}
	}
}

func TestRequestedAmountDisclosureNeedsReviewedCompanion(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Measures = []semantics.Measure{{ID: "gross", Name: "Known gross", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "occurred"}, Aggregation: semantics.AggregationSum}}
	item := admittedTopic{id: "topic", publication: p, selection: &semanticSelectionState{roots: map[semantics.Reference]string{{Kind: semantics.KindMeasure, ID: "gross"}: "explicit_metric"}}}
	for _, q := range []string{"Show known gross and disclose unknown amounts", "Muestra el importe conocido e informa importes desconocidos", "Show known gross and count unknown amounts", "Show known gross with unknown amounts separately", "Muestra importes conocidos y cuenta los importes desconocidos", "Do not report gross; report unknown amounts"} {
		_, err := selectCompletenessOutputs(context.Background(), RouteRequest{Question: q}, &item)
		var c *Clarification
		if !errors.As(err, &c) || c.Reason != "conflicting_semantic_completeness" {
			t.Fatal("disclosure silently omitted", q, err)
		}
	}
	for _, q := range []string{`Show gross for "unknown amounts"`, "Do not report unknown amounts", "No informa importes desconocidos"} {
		if _, err := selectCompletenessOutputs(context.Background(), RouteRequest{Question: q}, &item); err != nil {
			t.Fatal("quoted or negated output became obligation", q, err)
		}
	}
}
