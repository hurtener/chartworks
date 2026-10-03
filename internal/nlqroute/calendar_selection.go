package nlqroute

import (
	"context"
	"strings"

	"github.com/hurtener/chartworks/internal/semantics"
)

// selectImplicitCalendar closes a finite explicit grouping phrase against the
// current reviewed metric dependencies before context assembly and rule closure.
// It does not use vector similarity as field or time-basis authority.
func selectImplicitCalendar(ctx context.Context, in RouteRequest, admitted []admittedTopic) error {
	if in.Grouping != nil || len(admitted) != 1 || strings.ContainsAny(in.Question, "\"'“”‘’") {
		return nil
	}
	var redactions []semantics.ClarificationResolution
	for _, item := range admitted {
		for _, pattern := range item.rules.Definition.Patterns {
			for _, slot := range pattern.Slots {
				if slot.Sensitivity == semantics.LiteralSensitive {
					redactions = append(redactions, semantics.ClarificationResolution{Topic: item.id, Pattern: pattern.ID, Slot: slot.ID, Sensitivity: slot.Sensitivity})
				}
			}
		}
	}
	text := semantics.RedactClarificationText(in.Question, in.Answers, redactions)
	words := strings.Fields(normalizedPhrase(text))
	if len(words) < 3 {
		return nil
	}
	marker, unit := words[len(words)-2], words[len(words)-1]
	if marker != "by" && marker != "por" && marker != "per" {
		return nil
	}
	var grain semantics.TimeGrain
	switch unit {
	case "day", "día", "dia":
		grain = "day"
	case "month", "mes":
		grain = "month"
	case "quarter", "trimestre":
		grain = "quarter"
	case "year", "año", "ano":
		grain = "year"
	default:
		return nil
	}
	for _, word := range words[:len(words)-2] {
		if temporalNegator(word) || word == "filtered" || word == "filtrado" || word == "filtrados" {
			return nil
		}
	}
	item := &admitted[0]
	datasets := map[string]bool{}
	seen := map[semantics.Reference]bool{}
	var visit func(semantics.Reference) error
	visit = func(ref semantics.Reference) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[ref] {
			return nil
		}
		seen[ref] = true
		if len(seen) > maxSelectionReferences {
			return ErrMetricContext
		}
		_, inputs, ok := catalogReference(item.publication.Definition, ref)
		if !ok {
			return ErrMetricContext
		}
		if ref.Kind == semantics.KindColumn {
			datasets[ref.Dataset] = true
		}
		for _, input := range inputs {
			if err := visit(input); err != nil {
				return err
			}
		}
		return nil
	}
	for root, reason := range item.selection.roots {
		if reason == "required_rule" || (root.Kind != semantics.KindMeasure && root.Kind != semantics.KindKPI) {
			continue
		}
		// A complete metric label may itself end in a grouping-looking phrase.
		for _, m := range item.publication.Definition.Measures {
			if root.Kind == semantics.KindMeasure && m.ID == root.ID {
				for _, label := range append([]string{m.Name}, m.Aliases...) {
					if strings.HasSuffix(normalizedPhrase(label), marker+" "+unit) && containsPhrase(normalizedPhrase(text), normalizedPhrase(label)) {
						return nil
					}
				}
			}
		}
		for _, k := range item.publication.Definition.KPIs {
			if root.Kind == semantics.KindKPI && k.ID == root.ID {
				for _, label := range append([]string{k.Name}, k.Aliases...) {
					if strings.HasSuffix(normalizedPhrase(label), marker+" "+unit) && containsPhrase(normalizedPhrase(text), normalizedPhrase(label)) {
						return nil
					}
				}
			}
		}
		if err := visit(root); err != nil {
			return err
		}
	}
	if len(datasets) != 1 {
		return selectionFailure(in.Locale, "ambiguous_calendar_dimension")
	}
	// An explicitly selected direct dimension named Month keeps its meaning.
	// Existing reviewed date selection also disambiguates competing time bases.
	var selectedTemporal []semantics.Reference
	for _, d := range item.publication.Definition.Dimensions {
		ref := semantics.Reference{Kind: semantics.KindDimension, ID: d.ID}
		reason := item.selection.roots[ref]
		if reason == "" || reason == "required_rule" {
			continue
		}
		for _, label := range append([]string{d.Name}, d.Aliases...) {
			if normalizedPhrase(label) == unit {
				return nil
			}
		}
		if d.Role == semantics.DimensionTemporal && d.Temporal != nil && datasets[d.Field.Dataset] {
			for _, g := range d.Temporal.Grains {
				if g == grain {
					selectedTemporal = append(selectedTemporal, ref)
					break
				}
			}
		}
	}
	if len(selectedTemporal) == 1 {
		return nil
	}
	if len(selectedTemporal) > 1 {
		return selectionFailure(in.Locale, "ambiguous_calendar_dimension")
	}
	var candidates []semantics.Reference
	for _, d := range item.publication.Definition.Dimensions {
		if d.Role != semantics.DimensionTemporal || d.Temporal == nil || !datasets[d.Field.Dataset] {
			continue
		}
		ref := semantics.Reference{Kind: semantics.KindDimension, ID: d.ID}
		omitted := false
		for _, x := range in.OmittedRoots {
			omitted = omitted || x == ref
		}
		if omitted {
			continue
		}
		for _, g := range d.Temporal.Grains {
			if g == grain {
				candidates = append(candidates, ref)
				break
			}
		}
	}
	if len(candidates) != 1 {
		return selectionFailure(in.Locale, "ambiguous_calendar_dimension")
	}
	_, err := addSelectedRoot(item, candidates[0], "catalog_term", in.OmittedRoots)
	return err
}
