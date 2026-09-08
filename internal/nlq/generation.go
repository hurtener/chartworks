package nlq

import (
	"context"
	"sort"
)

type GenerationStrategy string

const (
	GenerationEditBase GenerationStrategy = "edit_base"
	GenerationHints    GenerationStrategy = "hints"
	GenerationExamples GenerationStrategy = "examples"
	GenerationDefault  GenerationStrategy = "default"
	MaxInstructions                       = 64
)

type Instruction struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type GenerationInput struct {
	Context  AssembledContext `json:"context"`
	EditBase []Instruction    `json:"edit_base"`
	Hints    []Instruction    `json:"hints"`
	Examples []Instruction    `json:"examples"`
	Default  []Instruction    `json:"default"`
}

type GenerationContext struct {
	Strategy             GenerationStrategy `json:"strategy"`
	FewShotDisabled      bool               `json:"few_shot_disabled"`
	Selected             []Instruction      `json:"selected"`
	MandatoryConstraints *ConstraintState   `json:"mandatory_constraints,omitempty"`
	PinnedMetrics        []PinnedMetric     `json:"pinned_metrics"`
	Context              AssembledContext   `json:"context"`
}

// ResolvePrecedence is the first phase-18 generation-context consumer. It
// chooses one detached source in the documented order and carries the already
// evaluated mandatory lanes forward unchanged. It never evaluates a rule,
// widens authority, calls a model, or executes a query.
func ResolvePrecedence(ctx context.Context, input GenerationInput) (GenerationContext, error) {
	if ctx == nil {
		return GenerationContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context"}
	}
	if err := ctx.Err(); err != nil {
		return GenerationContext{}, err
	}
	if input.Context.Prompt == "" || !input.Context.Tier.valid() || input.Context.Budget <= 0 || input.Context.Tokens < 0 || input.Context.Tokens > input.Context.Budget {
		return GenerationContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context"}
	}
	editBase, err := cloneInstructions(input.EditBase, "edit_base")
	if err != nil {
		return GenerationContext{}, err
	}
	hints, err := cloneInstructions(input.Hints, "hints")
	if err != nil {
		return GenerationContext{}, err
	}
	examples, err := cloneInstructions(input.Examples, "examples")
	if err != nil {
		return GenerationContext{}, err
	}
	defaults, err := cloneInstructions(input.Default, "default")
	if err != nil {
		return GenerationContext{}, err
	}
	result := GenerationContext{
		MandatoryConstraints: cloneConstraintState(input.Context.Constraints),
		PinnedMetrics:        cloneMetrics(input.Context.Metrics),
		Context:              cloneAssembled(input.Context),
	}
	switch {
	case len(editBase) > 0:
		result.Strategy, result.FewShotDisabled, result.Selected = GenerationEditBase, true, editBase
	case len(hints) > 0:
		result.Strategy, result.FewShotDisabled, result.Selected = GenerationHints, true, hints
	case len(examples) > 0:
		result.Strategy, result.Selected = GenerationExamples, examples
	default:
		result.Strategy, result.Selected = GenerationDefault, defaults
	}
	return result, nil
}

func cloneInstructions(items []Instruction, path string) ([]Instruction, error) {
	if len(items) > MaxInstructions {
		return nil, &ValidationError{Code: CodeLimit, Path: path}
	}
	out := append([]Instruction(nil), items...)
	seen := map[string]bool{}
	for i := range out {
		if !validID(out[i].Key) || !validText(out[i].Text, 16<<10) {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: path}
		}
		if seen[out[i].Key] {
			return nil, &ValidationError{Code: CodeDuplicateKey, Path: path}
		}
		seen[out[i].Key] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func cloneAssembled(input AssembledContext) AssembledContext {
	out := input
	out.Evidence = cloneEvidence(input.Evidence)
	out.Constraints = cloneConstraintState(input.Constraints)
	out.Metrics = cloneMetrics(input.Metrics)
	out.Advisory = cloneOptional(input.Advisory)
	out.Examples = cloneOptional(input.Examples)
	out.Omitted = append([]Omission(nil), input.Omitted...)
	out.Usage = append([]LaneUsage(nil), input.Usage...)
	out.Unpruned = cloneInput(input.Unpruned)
	return out
}

func cloneInput(input ContextInput) ContextInput {
	out := input
	out.Evidence = cloneEvidence(input.Evidence)
	out.Metrics = cloneMetrics(input.Metrics)
	out.Advisory = cloneOptional(input.Advisory)
	out.Examples = cloneOptional(input.Examples)
	if input.Constraints != nil {
		out.Constraints = &ConstraintState{Allowed: input.Constraints.Allowed, Required: cloneConstraints(input.Constraints.Required), Excluded: cloneConstraints(input.Constraints.Excluded)}
	}
	return out
}
