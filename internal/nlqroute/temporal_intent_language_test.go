package nlqroute

import (
	"context"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
	"time"
)

func TestReviewedLocalCalendarYearLanguage(t *testing.T) {
	for _, tc := range []struct {
		q      string
		locale nlq.Language
	}{
		{"What was the known paid booked amount in local calendar 2026? Disclose that some order amounts are unknown.", nlq.LanguageEnglish},
		{"How many paid orders were booked in local 2026, including orders whose amount is unknown?", nlq.LanguageEnglish},
		{"Show gross during local calendar year 2026.", nlq.LanguageEnglish},
		{"¿Cuál fue el importe bruto conocido de pedidos pagados en el año calendario local 2026? Aclara que hay importes desconocidos.", nlq.LanguageSpanish},
		{"Desglosa los importes por mes local de 2026; conserva como NULL un mes sin importes conocidos.", nlq.LanguageSpanish},
	} {
		span, ok, err := temporalSpan(normalizedPhrase(tc.q), tc.locale, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		if err != nil || !ok || span.grain != "year" || span.start != "2026-01-01" || span.end != "2027-01-01" {
			t.Fatalf("%q: %+v %v %v", tc.q, span, ok, err)
		}
	}
	for _, tc := range []struct {
		q      string
		locale nlq.Language
	}{
		{"Revenue in fiscal 2026", nlq.LanguageEnglish},
		{"Revenue in 2026 or 2027", nlq.LanguageEnglish},
		{"Revenue en el año calendario local 2026", nlq.LanguageEnglish},
		{"Ingresos in local calendar 2026", nlq.LanguageSpanish},
		{"Revenue reference local 2026", nlq.LanguageEnglish},
		{"Referencia local de 2026", nlq.LanguageSpanish},
	} {
		if _, _, err := temporalSpan(normalizedPhrase(tc.q), tc.locale, time.Now()); err == nil {
			t.Fatalf("unsupported phrase accepted: %q", tc.q)
		}
	}
}

func TestReviewedMetricScopesLocalCalendarDimension(t *testing.T) {
	for _, tc := range []struct {
		q string
		l nlq.Language
	}{{"Show paid event counts by local calendar month in 2026; preserve an all-unknown month as NULL.", nlq.LanguageEnglish}, {"Desglosa los eventos pagados por mes local de 2026; conserva como NULL un mes sin importes conocidos.", nlq.LanguageSpanish}} {
		p := cw07Publication("topic")
		b := cw07Binding(1)
		p.Definition.Measures = []semantics.Measure{{ID: "events", Name: "Event count", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "occurred"}, Aggregation: semantics.Aggregation("count")}}
		d := p.Definition.Datasets[0]
		d.ID = "refunds"
		d.Source.Dataset = "refunds"
		p.Definition.Datasets = append(p.Definition.Datasets, d)
		dim := p.Definition.Dimensions[1]
		dim.ID = "refund_date"
		dim.Name = "Refund date"
		dim.Aliases = nil
		dim.Field.Dataset = "refunds"
		p.Definition.Dimensions = append(p.Definition.Dimensions, dim)
		rel := b.Relations[0]
		rel.ID = "refunds"
		rel.Name = "refunds"
		b.Relations = append(b.Relations, rel)
		s, _ := cw07Service(t, p, b)
		out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.l, Question: tc.q, MetricIDs: []string{"events"}, InterpretationAnchor: "2026-10-01"})
		if err != nil || out.Context == nil || out.Request.Grouping == nil || len(out.Request.Grouping.Keys) != 1 || out.Request.Grouping.Keys[0].Dimension != "event_date" {
			t.Fatal("wrong calendar scope", tc.q, err, out.Clarification)
		}
		if _, _, err = s.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil {
			t.Fatal("grouping replay", err)
		}
	}
}

func TestMappedYearClausesCannotChangePeriod(t *testing.T) {
	for _, q := range []string{"Subtract refunds posted during local 2026 from gross booked during local 2026", "Gross in calendar 2026 and refunds in calendar 2026"} {
		p, ok := commonMappedYear(normalizedPhrase(q), nlq.LanguageEnglish)
		if !ok || p.start != "2026-01-01" || p.end != "2027-01-01" {
			t.Fatal(q, p, ok)
		}
	}
	for _, q := range []string{"Gross in 2026 and refunds in 2027", "Gross from January 2026 and refunds in 2026", "Gross in 2026 and refunds last month in 2026", "Gross in 2026 or 2026"} {
		if _, ok := commonMappedYear(normalizedPhrase(q), nlq.LanguageEnglish); ok {
			t.Fatal("unsupported mapped period", q)
		}
	}
}
