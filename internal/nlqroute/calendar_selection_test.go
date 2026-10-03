package nlqroute

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryImplicitCalendarSelection(t *testing.T) {
	for _, tc := range []struct {
		question            string
		ambiguous, selected bool
	}{
		{"Revenue by month", false, true}, {"Revenue por mes", false, true},
		{"Revenue by month", true, false}, {"Monthly recurring revenue", false, false},
		{"Revenue filtered by month", false, false}, {"Revenue not grouped by month", false, false},
		{"Revenue for “by month”", false, false},
	} {
		t.Run(tc.question, func(t *testing.T) {
			p := recoveryPublication()
			d := semantics.Dimension{ID: "event", Name: "Order date", Field: recoveryColumn("dataset", "event"), Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{"month"}}}
			p.Definition.Dimensions = append(p.Definition.Dimensions, d)
			if tc.ambiguous {
				d.ID = "shipment"
				d.Name = "Shipment date"
				p.Definition.Dimensions = append(p.Definition.Dimensions, d)
			}
			items := []admittedTopic{{id: "topic", publication: p}}
			in := RouteRequest{Locale: nlq.LanguageEnglish, Question: tc.question}
			if err := initialSemanticSelection(context.Background(), in, items, nil); err != nil {
				t.Fatal(err)
			}
			err := selectImplicitCalendar(context.Background(), in, items)
			if tc.ambiguous {
				if err == nil {
					t.Fatal("arbitrary time basis selected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ref := semantics.Reference{Kind: semantics.KindDimension, ID: "event"}
			if (items[0].selection.roots[ref] != "") != tc.selected {
				t.Fatal("incorrect grouping selection")
			}
		})
	}
}

func TestSQLRecoveryImplicitCalendarIgnoresPrivateAnswer(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "event", Name: "Order date", Field: recoveryColumn("dataset", "event"), Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{"month"}}})
	items := []admittedTopic{{id: "topic", publication: p}}
	items[0].rules.Definition.Patterns = []semantics.ClarificationPattern{{ID: "customer", Slots: []semantics.ClarificationSlot{{ID: "name", Sensitivity: semantics.LiteralSensitive}}}}
	value := "by month"
	in := RouteRequest{Locale: nlq.LanguageEnglish, Question: "Revenue for by month", Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "name", Value: &semantics.ClarificationValue{Text: &value}}}}
	if err := initialSemanticSelection(context.Background(), in, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := selectImplicitCalendar(context.Background(), in, items); err != nil {
		t.Fatal(err)
	}
	if items[0].selection.roots[semantics.Reference{Kind: semantics.KindDimension, ID: "event"}] != "" {
		t.Fatal("private answer became grouping authority")
	}
}
