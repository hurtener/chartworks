package nlqroute

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestGroundedCalendarV2Phrases(t *testing.T) {
	for _, tc := range []struct {
		q      string
		locale nlq.Language
		group  bool
	}{
		{"Revenue for local calendar 2026", nlq.LanguageEnglish, false},
		{"Revenue by New York month in local calendar 2026", nlq.LanguageEnglish, true},
		{"Revenue by month in calendar year 2026", nlq.LanguageEnglish, true},
		{"Revenue in New York calendar year 2026", nlq.LanguageEnglish, false},
		{"Revenue for the cohort of New York calendar year 2026", nlq.LanguageEnglish, false},
		{"Revenue by New York booking month in 2026", nlq.LanguageEnglish, true},
		{"Ingresos durante el año calendario 2026 en Nueva York", nlq.LanguageSpanish, false},
		{"Ingresos para el año calendario local 2026", nlq.LanguageSpanish, false},
		{"Ingresos del año calendario local 2026", nlq.LanguageSpanish, false},
		{"Ingresos en el año local 2026", nlq.LanguageSpanish, false},
		{"Ingresos por mes de registro en Nueva York durante 2026", nlq.LanguageSpanish, true},
	} {
		t.Run(tc.q, func(t *testing.T) {
			p := cw07Publication("topic")
			p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
			p.Definition.Datasets[0].Columns[1].NativeType = "timestamptz"
			p.Definition.Datasets[0].Columns[1].Category = "timestamp"
			b := cw07Binding(1)
			b.Relations[0].Columns[1].NativeType = "timestamptz"
			b.Relations[0].Columns[1].Category = "timestamp"
			svc, engine := cw07Service(t, p, b)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.q, InterpretationPolicy: "grounded-calendar-v2", InterpretationAnchor: "2026-10-01"}
			out, err := svc.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 {
				t.Fatalf("missing grounded period: %v %+v", err, out.Interpretation)
			}
			span := out.Interpretation.Temporal[0]
			if span.Start != "2026-01-01T05:00:00Z" || span.End != "2027-01-01T05:00:00Z" || span.TimeZone != "America/New_York" {
				t.Fatalf("wrong bounds: %+v", span)
			}
			if tc.group && (out.Request.Grouping == nil || len(out.Request.Grouping.Keys) != 1 || out.Request.Grouping.Keys[0].Dimension != "event_date" || out.Request.Grouping.Keys[0].Grain != semantics.GrainMonth) {
				t.Fatal("missing reviewed grouping", out.Request.Grouping)
			}
			calls := engine.embeds
			if _, _, err = svc.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil || calls != engine.embeds {
				t.Fatal("replay changed", err)
			}
			tampered := out
			tampered.Request = cloneRouteRequest(out.Request)
			tampered.Request.InterpretationPolicy = ""
			if _, _, err = svc.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), tampered); err == nil {
				t.Fatal("policy stripping admitted")
			}
		})
	}
}

func TestGroundedCalendarV2RefusesUnsafeMeaning(t *testing.T) {
	for _, tc := range []struct {
		q      string
		locale nlq.Language
	}{
		{"Revenue in New York calendar year 2026 in UTC", nlq.LanguageEnglish},
		{"Revenue in 2026 by month in UTC", nlq.LanguageEnglish},
		{"Revenue in March 2026 in UTC", nlq.LanguageEnglish},
		{"Revenue last month in UTC", nlq.LanguageEnglish},
		{"Revenue by month in Tokyo in 2026", nlq.LanguageEnglish},
		{"Revenue in 2026 in UTC+09:00", nlq.LanguageEnglish},
		{"Revenue in 2026 in New York or UTC", nlq.LanguageEnglish},
		{"Revenue in 2026 in America/Los_Angeles", nlq.LanguageEnglish},
		{"Revenue in London calendar year 2026", nlq.LanguageEnglish},
		{"Revenue not for local calendar 2026", nlq.LanguageEnglish},
		{"Revenue excluding the period window for local calendar 2026", nlq.LanguageEnglish},
		{"Revenue not by New York booking month in 2026", nlq.LanguageEnglish},
		{"Revenue by New York month and by New York quarter in 2026", nlq.LanguageEnglish},
		{"Revenue in fiscal calendar year 2026", nlq.LanguageEnglish},
		{"Revenue in New York calendar year 2026 and in 2027", nlq.LanguageEnglish},
		{"Revenue in New York calendar year 2026 reference 2026", nlq.LanguageEnglish},
		{"Revenue en el año calendario 2026", nlq.LanguageEnglish},
		{"Ingresos in New York calendar year 2026", nlq.LanguageSpanish},
		{"Ingresos por mes en Londres durante 2026", nlq.LanguageSpanish},
		{"Ingresos por mes en UTC durante 2026", nlq.LanguageSpanish},
		{"Ingresos no para el año calendario 2026", nlq.LanguageSpanish},
		{"Ingresos durante el año pasado de UTC", nlq.LanguageSpanish},
		{"Ingresos mes pasado de UTC", nlq.LanguageSpanish},
	} {
		t.Run(tc.q, func(t *testing.T) {
			p := cw07Publication("topic")
			p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
			svc, engine := cw07Service(t, p, cw07Binding(1))
			out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.q, InterpretationPolicy: GroundedCalendarPolicy, InterpretationAnchor: "2026-10-01"})
			if err == nil && out.Clarification == nil {
				t.Fatal("unsafe meaning admitted", out.Interpretation, out.Request.Grouping)
			}
			if engine.embeds != 0 {
				t.Fatal("unsafe meaning reached provider")
			}
		})
	}
}

func TestGroundedCalendarV2ReviewedZoneAndDimension(t *testing.T) {
	for _, zone := range []string{"UTC", "Europe/London", "US/Eastern"} {
		p := cw07Publication("topic")
		p.Definition.Dimensions[1].Temporal.Timezone = zone
		svc, engine := cw07Service(t, p, cw07Binding(1))
		out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in New York calendar year 2026", InterpretationPolicy: GroundedCalendarPolicy})
		if err == nil && out.Clarification == nil || engine.embeds != 0 {
			t.Fatal("zone overrode reviewed policy", zone, err)
		}
	}
	p := cw07Publication("topic")
	p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	other := p.Definition.Dimensions[1]
	other.ID = "second_time"
	other.Name = "New York"
	other.Aliases = nil
	p.Definition.Dimensions = append(p.Definition.Dimensions, other)
	svc, engine := cw07Service(t, p, cw07Binding(1))
	out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in New York calendar year 2026", InterpretationPolicy: GroundedCalendarPolicy})
	if err == nil && out.Clarification == nil || engine.embeds != 0 {
		t.Fatal("zone silently selected ambiguous dimension", err)
	}
}

func TestGroundedCalendarV2PrivacyAndQuotes(t *testing.T) {
	for _, tc := range []struct{ q, secret string }{
		{"Revenue in New York calendar year 2026", "New York"},
		{"Revenue in 2026 in UTC", "in UTC"},
		{"Revenue in March 2026 in UTC", "UTC"},
		{"Revenue excluding the period window in local calendar 2026", "excluding"},
		{"Revenue excluding the period window by New York month in 2026", "excluding"},
		{"Revenue by New York booking month in 2026", "booking"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			in := RouteRequest{Locale: nlq.LanguageEnglish, Question: tc.q, Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "private", Slot: "private", Value: &semantics.ClarificationValue{Text: &tc.secret}}}}
			admitted := []admittedTopic{{id: "topic"}}
			admitted[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "private", Slots: []semantics.ClarificationSlot{{ID: "private", Sensitivity: semantics.LiteralSensitive}}}}
			if err := protectedGroundedCalendarMeaning(in, admitted); err == nil {
				t.Fatal("mixed protected calendar accepted")
			}
		})
	}
	p := cw07Publication("topic")
	svc, _ := cw07Service(t, p, cw07Binding(1))
	for _, q := range []string{"Revenue for 'North' for local calendar 2026", "Revenue for 'North' \"for local calendar 2026\""} {
		out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: q, InterpretationPolicy: GroundedCalendarPolicy})
		if err != nil || len(out.Interpretation.Values) != 1 || out.Interpretation.Values[0].GovernedValue != "north" {
			t.Fatal("quoted public value lost", err)
		}
		want := 1
		if strings.Contains(q, "\"") {
			want = 0
		}
		if len(out.Interpretation.Temporal) != want {
			t.Fatal("quoted calendar became intent")
		}
	}
}

func TestGroundedCalendarV2LegacyAndManualGrouping(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	for _, policy := range []string{"", InterpretationContinuationPolicy, GroundedCalendarPolicy} {
		svc, _ := cw07Service(t, p, cw07Binding(1))
		out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by New York booking month in 2026", InterpretationPolicy: policy})
		if err != nil {
			t.Fatal(err)
		}
		if (out.Request.Grouping != nil) != (policy == GroundedCalendarPolicy) {
			t.Fatal("legacy regrouped", policy, out.Request.Grouping)
		}
	}
	svc, _ := cw07Service(t, p, cw07Binding(1))
	manual := &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "event_date", Grain: semantics.GrainMonth}}}
	out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by New York quarter in 2026", InterpretationPolicy: GroundedCalendarPolicy, Grouping: manual})
	if err != nil || out.Request.Grouping.Keys[0].Grain != semantics.GrainMonth {
		t.Fatal("manual grouping lost", err)
	}
	p.Definition.Measures = []semantics.Measure{{ID: "events", Name: "Revenue by month", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "dataset", ID: "occurred"}, Aggregation: semantics.AggregationCount}}
	svc, _ = cw07Service(t, p, cw07Binding(1))
	out, err = svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by month in 2026", InterpretationPolicy: GroundedCalendarPolicy})
	if err != nil || out.Request.Grouping != nil {
		t.Fatal("measure label became grouping", err)
	}
}

func TestGroundedCalendarV2ZoneTokensAreNotValues(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	p.Definition.Dimensions[0].Values = []semantics.GovernedValue{{ID: "ny", Value: "New York", Aliases: []string{"New York"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "review", Policy: "review"}}}
	for _, tc := range []struct {
		q     string
		count int
		op    string
	}{
		{"Revenue in New York calendar year 2026", 0, ""},
		{"Revenue in 2026 in New York", 0, ""},
		{"Revenue by New York booking month in 2026", 0, ""},
		{"Revenue for New York in New York calendar year 2026", 1, "eq"},
		{"Revenue not New York in New York calendar year 2026", 1, "ne"},
		{"Revenue for 'New York' in New York calendar year 2026", 1, "eq"},
		{"Revenue 'in New York calendar year 2026' in New York calendar year 2026", 1, "eq"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			svc, _ := cw07Service(t, p, cw07Binding(1))
			out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: tc.q, InterpretationPolicy: GroundedCalendarPolicy})
			if err != nil || out.Interpretation == nil || len(out.Interpretation.Values) != tc.count {
				t.Fatal("zone/value occurrence crossed meanings", err, out.Interpretation)
			}
			if tc.count > 0 && out.Interpretation.Values[0].Operator != tc.op {
				t.Fatal("explicit city polarity changed")
			}
		})
	}
}

func TestGroundedCalendarV2QuotedDimensionAndPublicPrivacy(t *testing.T) {
	p := cw07Publication("topic")
	other := p.Definition.Dimensions[1]
	other.ID = "second"
	other.Name = "Booking date"
	other.Aliases = nil
	p.Definition.Dimensions = append(p.Definition.Dimensions, other)
	svc, engine := cw07Service(t, p, cw07Binding(1))
	out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue for local calendar 2026 'Booking date'", InterpretationPolicy: GroundedCalendarPolicy})
	if err == nil && out.Clarification == nil || engine.embeds != 0 {
		t.Fatal("quoted temporal label resolved ambiguity")
	}
	for _, secret := range []string{"SECRET", "in UTC"} {
		in := RouteRequest{Locale: nlq.LanguageEnglish, Question: "Revenue in 2026 in UTC for SECRET", Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "private", Slot: "private", Value: &semantics.ClarificationValue{Text: &secret}}}}
		items := []admittedTopic{{id: "topic"}}
		items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "private", Slots: []semantics.ClarificationSlot{{ID: "private", Sensitivity: semantics.LiteralSensitive}}}}
		err := protectedGroundedCalendarMeaning(in, items)
		if (err != nil) != (secret == "in UTC") {
			t.Fatal("private independent word changed public zone", secret, err)
		}
	}
}

func TestGroundedCalendarV2MixedGrainReplay(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
	other := p.Definition.Dimensions[1]
	other.ID = "quarter_time"
	other.Name = "Quarter time"
	other.Aliases = nil
	other.Temporal = &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{semantics.GrainQuarter}}
	p.Definition.Dimensions = append(p.Definition.Dimensions, other)
	svc, engine := cw07Service(t, p, cw07Binding(1))
	out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by New York month in 2026", InterpretationPolicy: GroundedCalendarPolicy})
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 {
		t.Fatal("fresh candidate selection", err, out.Clarification)
	}
	calls := engine.embeds
	if _, _, err = svc.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err != nil || engine.embeds != calls {
		t.Fatal("mixed-grain replay reinterpreted calendar", err)
	}
}

func TestGroundedCalendarV2UTCValueAndSpanishPrompt(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Dimensions[0].Values = []semantics.GovernedValue{{ID: "utc", Value: "UTC", Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "e", Policy: "p"}}}
	svc, _ := cw07Service(t, p, cw07Binding(1))
	out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in 2026 in UTC", InterpretationPolicy: GroundedCalendarPolicy})
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Values) != 0 {
		t.Fatal("UTC zone became value", err)
	}
	out, err = svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageSpanish, Question: "Ingresos en el año calendario 2026 de Nueva York", InterpretationPolicy: GroundedCalendarPolicy})
	if err != nil || out.Clarification == nil || !strings.HasPrefix(out.Clarification.Prompt, "Aclara") {
		t.Fatal("Spanish mismatch prompt lost", err, out.Clarification)
	}
}
func TestGroundedCalendarV2RequiresCheckedZoneTarget(t *testing.T) {
	for _, tc := range []struct {
		name, zone            string
		retained, categorical bool
		allow                 bool
	}{
		{"empty-scalar", "UTC", false, false, false},
		{"retained-mismatch", "UTC", true, false, false},
		{"categorical-only", "UTC", false, true, false},
		{"retained-match", "New York", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := cw07Publication("topic")
			p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
			svc, engine := cw07Service(t, p, cw07Binding(1))
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue by " + tc.zone + " month", InterpretationPolicy: GroundedCalendarPolicy, Grouping: &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{}}}
			if tc.categorical {
				in.Grouping.Keys = []GroupingKey{{Topic: "topic", Dimension: "sales_region"}}
			}
			if tc.retained {
				in.InterpretationSelections = []InterpretationSelection{{Topic: "topic", Dimension: "event_date", Period: &InterpretationPeriod{Start: "2026-01-01", End: "2027-01-01", Grain: "year"}}}
			}
			out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			if !tc.allow {
				if err == nil && out.Clarification == nil || engine.embeds != 0 {
					t.Fatal("unverified zone assertion admitted", err, out.Interpretation)
				}
				return
			}
			if err != nil || out.Interpretation == nil || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].TimeZone != "America/New_York" {
				t.Fatal("matching retained calendar lost", err, out.Clarification)
			}
			if out.Interpretation.Parser != "deterministic-grounded-calendar-v2" {
				t.Fatal("retained selections downgraded parser", out.Interpretation.Parser)
			}
			calls := engine.embeds
			if _, _, err = svc.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err != nil || engine.embeds != calls {
				t.Fatal("retained-zone replay failed", err)
			}
			for _, policy := range []string{"", InterpretationContinuationPolicy} {
				changed := out
				changed.Request = cloneRouteRequest(out.Request)
				changed.Request.InterpretationPolicy = policy
				if _, _, err = svc.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), changed); err == nil {
					t.Fatal("retained parser policy downgraded on replay")
				}
			}
		})
	}
}

func TestGroundedCalendarV2UnbalancedQuotesClarify(t *testing.T) {
	for _, q := range []string{"Revenue \"for local calendar 2026", "Revenue ‘for local calendar 2026"} {
		svc, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
		out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: q, InterpretationPolicy: GroundedCalendarPolicy})
		if err == nil && out.Clarification == nil || engine.embeds != 0 {
			t.Fatal("unbalanced quote hid calendar meaning")
		}
	}
}

func TestGroundedCalendarV2CombinedGroupingPolicy(t *testing.T) {
	for _, tc := range []struct {
		q        string
		produced bool
	}{
		{"Revenue by New York booking month in 2026", true},
		{"Revenue in New York calendar year 2026", false},
	} {
		t.Run(tc.q, func(t *testing.T) {
			p := cw07Publication("topic")
			p.Definition.Dimensions[1].Temporal.Timezone = "America/New_York"
			p.Definition.Dimensions[1].Temporal.Grains = []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter}
			svc, base := cw07Service(t, p, cw07Binding(1))
			model := &groupingIntentEngine{Engine: base, respond: func(q string, cards []groupingIntentCard) conceptchoice.Proposal {
				return groupingProposal(q, cards, "event_date", semantics.GrainQuarter)
			}}
			svc.engine = model
			out, err := svc.Route(t.Context(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: tc.q, InterpretationPolicy: GroundedCalendarPolicy, GroupingIntentPolicy: GroundedGroupingIntentPolicy})
			if model.calls != 0 {
				t.Fatal("model may overwrite checked calendar grouping", out.Request.Grouping, err)
			}
			if !tc.produced {
				if err == nil && out.Clarification == nil {
					t.Fatal("unproved combined policy did not clarify")
				}
				return
			}
			if err != nil || out.Request.Grouping == nil || len(out.Request.Grouping.Keys) != 1 || out.Request.Grouping.Keys[0].Grain != semantics.GrainMonth || out.Request.GroupingIntentPolicy != "" || out.GroupingIntent != nil {
				t.Fatal("deterministic grouping not preserved", err, out.Request.Grouping)
			}
			if _, _, err = svc.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err != nil || model.calls != 0 {
				t.Fatal("combined-policy replay changed", err)
			}
		})
	}
}
