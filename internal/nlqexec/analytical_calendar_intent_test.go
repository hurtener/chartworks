package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryImplicitReviewedCalendarIntent(t *testing.T) {
	for _, question := range []string{"Revenue by month", "Ingresos por mes", "Revenue by quarter", "Ingresos por trimestre"} {
		a := calendarAdmission(question, "timestamptz")
		before := exec.Hash(a)
		c, err := compileAnalytical(context.Background(), a)
		if err != nil || c == nil || c.Grain == nil || len(c.Grain.Buckets) != 1 {
			t.Fatal(question, err)
		}
		if c.Grain.Buckets[0].Column != "event_native" || c.Grain.Buckets[0].Timezone != "America/New_York" || exec.Hash(a) != before {
			t.Fatal("implicit calendar lost reviewed identity")
		}
		if !strings.Contains(analyticalGrainGuidance(c), "date_trunc") {
			t.Fatal("missing concrete calendar generation guidance")
		}
		old, err := compileAnalyticalVersion(context.Background(), a, 7)
		if err != nil || old.Grain != nil {
			t.Fatal("retained version gained new intent", err)
		}
	}
}

func TestSQLRecoveryImplicitCalendarRequiresUniqueTimeBasis(t *testing.T) {
	for _, mode := range []string{"ambiguous", "unselected", "different_dataset", "unsupported_grain"} {
		t.Run(mode, func(t *testing.T) {
			a := calendarAdmission("Revenue by month", "date")
			d := &a.publications[0].Definition.Dimensions[2]
			switch mode {
			case "ambiguous":
				other := *d
				other.ID = "other_event"
				other.Name = "Shipment date"
				other.Aliases = nil
				a.publications[0].Definition.Dimensions = append(a.publications[0].Definition.Dimensions, other)
				a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: other.ID}, Reason: "catalog_term"})
			case "unselected":
				a.route.Selection.Topics[0].Roots = a.route.Selection.Topics[0].Roots[:3]
			case "different_dataset":
				d.Field.Dataset = "unselected_orders"
			case "unsupported_grain":
				d.Temporal.Grains = []semantics.TimeGrain{"day"}
			}
			analyticalReseal(&a)
			_, err := compileAnalytical(context.Background(), a)
			if mode == "different_dataset" && errors.Is(err, exec.ErrBinding) {
				return
			}
			var decision *generationDecisionError
			if !errors.As(err, &decision) || len(decision.problem.Questions) != 1 {
				t.Fatal("missing bounded clarification", err)
			}
		})
	}
}

func TestSQLRecoveryCalendarBusinessNameIsNotGrouping(t *testing.T) {
	for _, question := range []string{"Monthly recurring revenue", "Ingresos recurrentes mensuales", "Revenue filtered by month", "Revenue not grouped by month", "Revenue sin agrupar por mes", "Revenue for “by month”", "Revenue by month"} {
		a := calendarAdmission(question, "timestamptz")
		if question == "Revenue by month" {
			a.publications[0].Definition.Measures[0].Name = question
			analyticalReseal(&a)
		}
		c, err := compileAnalytical(context.Background(), a)
		if err != nil || c == nil || c.Grain != nil {
			t.Fatal("business name or excluded intent became grouping", question, err)
		}
	}
}

func TestSQLRecoveryCalendarYearRequiresOwnedProof(t *testing.T) {
	a := calendarAdmission("Revenue by month in 2026", "date")
	words := []string{"revenue", "by", "month", "in", "2026"}
	if len(calendarWordsWithoutOwnedYear(a, words)) != len(words) {
		t.Fatal("unowned year was discarded")
	}
	a.route.Interpretation = &nlqroute.Interpretation{Temporal: []nlqroute.TemporalInterpretation{{Dataset: "sales", Column: "event_native", LocalStart: "2026-01-01", LocalEnd: "2027-01-01", Start: "2026-01-01", End: "2027-01-01"}}}
	if len(calendarWordsWithoutOwnedYear(a, words)) != len(words) {
		t.Fatal("unsealed interpretation became owned population")
	}
}
