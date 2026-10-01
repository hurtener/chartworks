package nlqexec

import (
	"fmt"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/semantics"
	"strconv"
)

// implicitCalendarDimension resolves a bare calendar unit only inside the
// selected, reviewed metric dataset. It never searches unselected catalog fields
// or chooses between two reviewed time bases using model preference.
func implicitCalendarDimension(a admission, contract exec.AnalyticalContract, terms map[string][]grainDimension, grain string) ([]grainDimension, error) {
	seen := map[string]bool{}
	var choices []grainDimension
	for _, values := range terms {
		for _, d := range values {
			if seen[d.id] || d.field.Dataset != contract.Dataset || d.role != semantics.DimensionTemporal || d.temporal == nil {
				continue
			}
			seen[d.id] = true
			for _, allowed := range d.temporal.Grains {
				if string(allowed) == grain {
					choices = append(choices, d)
					break
				}
			}
		}
	}
	if len(choices) == 1 {
		return choices, nil
	}
	question := "Select the reviewed date dimension and calendar grouping for this metric before generating SQL."
	if a.route.Context != nil && a.route.Context.Locale == nlq.LanguageSpanish {
		question = "Seleccioná la dimensión de fecha revisada y la agrupación de calendario para esta métrica antes de generar SQL."
	}
	return nil, &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Insufficient, Questions: []string{question}}}
}

// calendarWordsWithoutOwnedYear composes grouping with a trailing year only
// when the exact year is already sealed as a current service-owned population.
// The predicate remains in the query-population proof and is never dropped.
func calendarWordsWithoutOwnedYear(a admission, words []string) []string {
	if len(words) < 4 || a.route.Interpretation == nil {
		return words
	}
	connector, year := words[len(words)-2], words[len(words)-1]
	if connector != "in" && connector != "en" && connector != "during" && connector != "durante" {
		return words
	}
	n, err := strconv.Atoi(year)
	if err != nil || len(year) != 4 || n < 1 || n >= 9999 {
		return words
	}
	constraints := a.calendarConstraints
	if len(constraints) == 0 {
		var err error
		constraints, err = a.route.ResolvedBusinessConstraints()
		if err != nil {
			return words
		}
	}
	for _, temporal := range a.route.Interpretation.Temporal {
		if temporal.LocalStart != year+"-01-01" || temporal.LocalEnd != fmt.Sprintf("%04d-01-01", n+1) {
			continue
		}
		for _, c := range constraints {
			if c.Dataset == temporal.Dataset && c.Column == temporal.Column && c.Value == temporal.Start && c.Upper == temporal.End && c.Bounds == "[)" && c.Calendar == temporal.Calendar && c.TimeZone == temporal.TimeZone {
				return words[:len(words)-2]
			}
		}
	}
	return words
}
