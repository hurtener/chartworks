package charts

import (
	"context"
	"errors"
	"sort"
)

// Select preserves the public rules-only entry point for callers without intent.
func Select(ctx context.Context, d Data, limits Limits) (Selection, error) {
	return SelectWithIntent(ctx, d, "", limits)
}

// SelectWithIntent applies bounded question, cardinality and semantic signals
// before suitability, floor and alternative limits seal the candidate set. It
// performs no inference, retrieval, SQL or persistence.
func SelectWithIntent(ctx context.Context, d Data, intent string, limits Limits) (Selection, error) {
	if !text(intent, 1024) {
		return Selection{}, ErrInvalid
	}
	if err := ValidateData(ctx, d, limits); err != nil {
		return Selection{}, err
	}
	p, err := profile(ctx, d, intent)
	if err != nil {
		return Selection{}, err
	}
	candidates := []Candidate{}
	for _, e := range Catalog() {
		if err = ctx.Err(); err != nil {
			return Selection{}, err
		}
		b, order, score, reason, eligible := candidateBinding(e.Kind, d, p)
		evaluation := CandidateEvaluation{Kind: e.Kind, Variant: variantID(e.Kind, b), Outcome: reason, Signals: []string{}}
		if eligible {
			score, evaluation.Signals = candidateScore(e.Kind, d, b, p, score)
			evaluation.Score = score
			// Bind validates every measure and preserves exact pins. There is no
			// retry with a silently reduced first-measure binding on failure.
			mapping, bindErr := bindValidated(ctx, d, e.Kind, b, order, DefaultOptions(), limits)
			switch {
			case errors.Is(bindErr, ErrUnsuitable):
				evaluation.Outcome = "unsuitable_binding"
			case errors.Is(bindErr, ErrLimit):
				evaluation.Outcome = "shape_limit"
			case bindErr != nil:
				return Selection{}, bindErr
			default:
				p.evidence.Suitable++
				evaluation.Outcome = "below_floor"
				if score >= limits.SelectionFloor {
					p.evidence.AboveFloor++
					evaluation.Outcome = "suitable"
					candidate := Candidate{Mapping: mapping, Score: score, Reason: reason, Variant: evaluation.Variant, Signals: append([]string{}, evaluation.Signals...)}
					for _, c := range d.Columns {
						if columnIndex(mapping.Columns, c.ID) < 0 {
							candidate.UnusedColumns = append(candidate.UnusedColumns, c.ID)
						}
					}
					candidates = append(candidates, candidate)
				}
			}
		}
		p.evidence.Evaluations = append(p.evidence.Evaluations, evaluation)
	}
	if len(candidates) == 0 {
		ids := make([]string, len(d.Columns))
		for i := range d.Columns {
			ids[i] = d.Columns[i].ID
		}
		mapping, bindErr := bindValidated(ctx, d, Table, Bindings{Columns: ids}, nil, DefaultOptions(), limits)
		if bindErr != nil {
			return Selection{}, bindErr
		}
		return Selection{Selected: Candidate{Mapping: mapping, Score: 0, Reason: "table_fallback", Variant: "scalar"}, Alternatives: []Candidate{}, Fallback: true, Reason: "no_suitable_chart_above_floor", Evidence: p.evidence}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	count := len(candidates) - 1
	if count > limits.MaxAlternatives {
		count = limits.MaxAlternatives
	}
	return Selection{Selected: candidates[0], Alternatives: append([]Candidate{}, candidates[1:1+count]...), Reason: "rules_first", Evidence: p.evidence}, ctx.Err()
}
