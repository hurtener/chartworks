package nlqroute

import (
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestRequestedMetricMeaningCannotBecomeGross(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Measures = []semantics.Measure{{ID: "misleading_net_total", Name: "Known gross", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "region"}}}
	a := []admittedTopic{{id: "topic", publication: p}}
	for _, tc := range []struct {
		q string
		l nlq.Language
	}{{"What was net revenue in 2026?", nlq.LanguageEnglish}, {"Show net sales for the current period", nlq.LanguageEnglish}, {"¿Cuáles fueron las ventas netas de 2026?", nlq.LanguageSpanish}} {
		got := requestedNetMeaning(RouteRequest{Question: tc.q, Locale: tc.l, MetricIDs: []string{"misleading_net_total"}}, a)
		if got == nil || got.Reason != "reviewed_metric_meaning_required" {
			t.Fatal("gross substituted", tc.q, got)
		}
	}
	a[0].publication.Definition.KPIs = []semantics.KPI{{ID: "cohort", Name: "Order-cohort net value"}, {ID: "activity", Name: "Refund-activity net value"}}
	in := RouteRequest{Question: "What was net revenue in 2026?", Locale: nlq.LanguageEnglish, MetricIDs: []string{"misleading_net_total"}}
	if got := requestedNetMeaning(in, a); got == nil || got.Reason != "ambiguous_metric_meaning" {
		t.Fatal("ambiguous meanings", got)
	}
	in.MetricIDs = []string{"cohort"}
	if got := requestedNetMeaning(in, a); got != nil {
		t.Fatal("explicit reviewed meaning rejected", got)
	}
	for _, q := range []string{`Show "net revenue" as a label`, "Show internet sales", "Show known paid gross"} {
		in.Question = q
		in.MetricIDs = []string{"misleading_net_total"}
		if got := requestedNetMeaning(in, a); got != nil {
			t.Fatal("invented net intent", q, got)
		}
	}
}

func TestMetricRequirementKeepsTypedIdentity(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Measures = []semantics.Measure{{ID: "same", Name: "Gross revenue", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}}, {ID: "refunds", Name: "Refund value", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "refunds", ID: "amount"}}}
	p.Definition.KPIs = []semantics.KPI{{ID: "same", Name: "Net revenue", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "refunds"}}}}
	a := []admittedTopic{{id: "topic", publication: p}}
	in := RouteRequest{Question: "Net revenue", Locale: nlq.LanguageEnglish, References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "same"}}}
	if got := requestedNetMeaning(in, a); got == nil {
		t.Fatal("unselected same-ID KPI supplied meaning")
	}
	in.References = []semantics.Reference{{Kind: semantics.KindKPI, ID: "same"}}
	facts := requestedMetricFacts(in, a)
	if len(facts) != 1 || !facts["topic\x00refunds"] {
		t.Fatal("same-ID measure replaced KPI fact", facts)
	}
}
