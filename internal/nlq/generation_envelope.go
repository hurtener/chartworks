package nlq

import (
	"context"
	"errors"
)

// ErrEnvelopeBudget distinguishes final provider admission from the route tier.
var ErrEnvelopeBudget = errors.Join(ErrInsufficient, errors.New("nlq: mandatory generation packet exceeds provider envelope"))

// RefitGeneration prunes only optional content against a trusted adapter's final
// request bound. The context owner is the only pruner. Required meaning, complete
// reviewed Relations and essential edit/hint/default instructions never yield.
// Input must be an in-process sealed generation, not a client or retained JSON DTO.
func (a *ContextAssembler) RefitGeneration(ctx context.Context, original GenerationContext, fits func(string) (bool, error)) (GenerationContext, error) {
	if ctx == nil || fits == nil || original.seal == [32]byte{} || original.seal != sealGenerationContext(original) {
		return GenerationContext{}, &ValidationError{Code: CodeInvalidValue, Path: "generation"}
	}
	if err := ctx.Err(); err != nil {
		return GenerationContext{}, err
	}
	if _, err := a.validateAssembledContext(original.Context); err != nil {
		return GenerationContext{}, err
	}
	result := original
	result.Context = cloneAssembled(original.Context)
	result.Selected = append([]Instruction(nil), original.Selected...)
	result.fallback = append([]Instruction(nil), original.fallback...)
	result.PinnedMetrics = cloneMetrics(original.PinnedMetrics)
	result.MandatoryConstraints = cloneConstraintState(original.MandatoryConstraints)
	result.Fit = &GenerationFit{Version: "generation-fit-v2"}
	if original.Fit != nil {
		*result.Fit = *original.Fit
		result.Fit.Omitted = append([]Omission(nil), original.Fit.Omitted...)
	}
	input := outputInput(result.Context)
	check := func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		prompt := renderGenerationPrompt(renderAssembledPrompt(input), result.Selected)
		tokens, err := a.counter.Count(prompt)
		if err != nil {
			return false, err
		}
		if tokens > result.Budget {
			return false, nil
		}
		return fits(prompt)
	}
	ok, err := check()
	if err != nil {
		return GenerationContext{}, err
	}
	// Reverse admission order: evidence/advisory were fitted after examples.
	optional := optionalCandidates(input)
	for i := len(optional) - 1; !ok && i >= 0; i-- {
		item := optional[i]
		switch item.lane {
		case LaneEvidence:
			for j, x := range input.Evidence {
				if x.ID == item.id {
					input.Evidence = append(input.Evidence[:j], input.Evidence[j+1:]...)
					break
				}
			}
		case LaneAdvisory:
			for j, x := range input.Advisory {
				if x.ID == item.id {
					input.Advisory = append(input.Advisory[:j], input.Advisory[j+1:]...)
					break
				}
			}
		default:
			return GenerationContext{}, &ValidationError{Code: CodeInvalidValue, Path: "generation.examples"}
		}
		result.Fit.omit(item.lane, item.id, "provider_budget")
		ok, err = check()
		if err != nil {
			return GenerationContext{}, err
		}
	}
	for !ok && result.Strategy == GenerationExamples && len(result.Selected) > 0 {
		last := len(result.Selected) - 1
		result.Fit.omit(LaneExamples, result.Selected[last].Key, "provider_budget")
		result.Selected = result.Selected[:last]
		if last == 0 {
			result.Strategy = GenerationDefault
			result.Selected = append([]Instruction(nil), result.fallback...)
		}
		ok, err = check()
		if err != nil {
			return GenerationContext{}, err
		}
	}
	if !ok {
		return GenerationContext{}, ErrEnvelopeBudget
	}
	result.Context, err = a.Assemble(ctx, input, original.Context.Tier)
	if err != nil {
		return GenerationContext{}, err
	}
	result.Prompt = renderGenerationPrompt(result.Context.Prompt, result.Selected)
	result.Tokens, err = a.counter.Count(result.Prompt)
	if err != nil {
		return GenerationContext{}, err
	}
	ok, err = fits(result.Prompt)
	if err != nil {
		return GenerationContext{}, err
	}
	if !ok || result.Tokens > result.Budget {
		return GenerationContext{}, ErrEnvelopeBudget
	}
	result.seal = sealGenerationContext(result)
	return result, nil
}
