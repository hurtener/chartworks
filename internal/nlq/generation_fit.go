package nlq

import "context"

// GenerationFit records the actual final fit rather than candidate selection.
// It contains IDs and closed reasons, never omitted text or parameter values.
// Absent Fit denotes a retained legacy generation; it is not reinterpreted.
type GenerationFit struct {
	Version      string     `json:"version"`
	Omitted      []Omission `json:"omitted,omitempty"`
	OmittedCount int        `json:"omitted_count"`
}

func (f *GenerationFit) omit(lane Lane, id, reason string) {
	f.OmittedCount++
	if len(f.Omitted) < MaxOmissions {
		f.Omitted = append(f.Omitted, Omission{Lane: lane, ID: id, Reason: reason})
	}
}

// fitGeneration keeps all required semantics, chooses precedence before fitting,
// and admits demonstrations in declared relevance order in one canonical lane.
// Optional evidence is atomic. An edit does not fail just because old optional
// context filled the tier, and suppressed examples cannot survive in the base.
func (a *ContextAssembler) fitGeneration(ctx context.Context, assembled AssembledContext, editBase, hints, examples, defaults []Instruction) (GenerationContext, error) {
	result := GenerationContext{
		MandatoryConstraints: cloneConstraintState(assembled.Constraints),
		PinnedMetrics:        cloneMetrics(assembled.Metrics), Budget: assembled.Budget,
		Fit: &GenerationFit{Version: "generation-fit-v2"},
	}
	// Context examples have already been validated and ranked by the context
	// owner. Merge them after explicitly ranked instruction examples. IDs are
	// unique across this one lane; conflicting definitions fail rather than win
	// by an accidental serialization order.
	seen := make(map[string]string, len(examples)+len(assembled.Examples))
	for _, item := range examples {
		seen[item.Key] = item.Text
	}
	for _, item := range assembled.Examples {
		if text, exists := seen[item.ID]; exists {
			if text != item.Text {
				return GenerationContext{}, &ValidationError{Code: CodeDuplicateKey, Path: "examples"}
			}
			continue
		}
		seen[item.ID] = item.Text
		examples = append(examples, Instruction{Key: item.ID, Text: item.Text})
	}
	input := outputInput(assembled)
	input.Evidence, input.Advisory, input.Examples = nil, nil, nil
	switch {
	case len(editBase) > 0:
		result.Strategy, result.FewShotDisabled, result.Selected = GenerationEditBase, true, editBase
	case len(hints) > 0:
		result.Strategy, result.FewShotDisabled, result.Selected = GenerationHints, true, hints
	case len(examples) > 0:
		result.Strategy = GenerationExamples
	default:
		result.Strategy, result.Selected = GenerationDefault, defaults
	}
	count := func(selected []Instruction) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return a.counter.Count(renderGenerationPrompt(renderAssembledPrompt(input), selected))
	}
	required, err := count(result.Selected)
	if err != nil {
		return GenerationContext{}, err
	}
	if required > result.Budget {
		return GenerationContext{}, &GenerationBudgetError{Tier: assembled.Tier, Budget: result.Budget, RequiredTokens: required}
	}
	for _, item := range examples {
		if result.FewShotDisabled {
			result.Fit.omit(LaneExamples, item.Key, "precedence")
			continue
		}
		if len(result.Selected) >= MaxExamples {
			result.Fit.omit(LaneExamples, item.Key, "max_examples")
			continue
		}
		candidate := append(append([]Instruction(nil), result.Selected...), item)
		n, err := count(candidate)
		if err != nil {
			return GenerationContext{}, err
		}
		if n > result.Budget {
			result.Fit.omit(LaneExamples, item.Key, "budget")
			continue
		}
		result.Selected = candidate
	}
	if result.Strategy == GenerationExamples && len(result.Selected) == 0 {
		result.Strategy, result.Selected = GenerationDefault, defaults
		required, err = count(result.Selected)
		if err != nil {
			return GenerationContext{}, err
		}
		if required > result.Budget {
			return GenerationContext{}, &GenerationBudgetError{Tier: assembled.Tier, Budget: result.Budget, RequiredTokens: required}
		}
	}
	for _, item := range optionalCandidates(outputInput(assembled)) {
		if item.lane == LaneExamples {
			continue // Already merged into the canonical instruction lane.
		}
		beforeEvidence, beforeAdvisory := len(input.Evidence), len(input.Advisory)
		if item.lane == LaneEvidence {
			input.Evidence = append(input.Evidence, item.evidence)
		} else {
			input.Advisory = append(input.Advisory, item.advisory)
		}
		n, err := count(result.Selected)
		if err != nil {
			return GenerationContext{}, err
		}
		if n > result.Budget {
			input.Evidence, input.Advisory = input.Evidence[:beforeEvidence], input.Advisory[:beforeAdvisory]
			result.Fit.omit(item.lane, item.id, "budget")
		}
	}
	// Re-seal the actual retained context through the same owner and tokenizer.
	// The route's context and audit remain unchanged; this is a detached packet.
	result.Context, err = a.Assemble(ctx, input, assembled.Tier)
	if err != nil {
		return GenerationContext{}, err
	}
	result.Prompt = renderGenerationPrompt(result.Context.Prompt, result.Selected)
	result.Tokens, err = a.counter.Count(result.Prompt)
	if err != nil {
		return GenerationContext{}, err
	}
	if result.Tokens > result.Budget {
		return GenerationContext{}, &GenerationBudgetError{Tier: assembled.Tier, Budget: result.Budget, RequiredTokens: result.Tokens}
	}
	result.seal = sealGenerationContext(result)
	return result, nil
}
