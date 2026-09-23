package nlq

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrGenerationStopped identifies strategies that must not generate output.
	ErrGenerationStopped = errors.New("nlq: generation is not permitted for this strategy")
)

// GenerationStrategy identifies the precedence source selected for generation.
type GenerationStrategy string

const (
	// GenerationEditBase selects an explicit edit of an existing base.
	GenerationEditBase GenerationStrategy = "edit_base"
	// GenerationHints selects hint instructions when no edit base is present.
	GenerationHints GenerationStrategy = "hints"
	// GenerationExamples selects examples when no edit base or hints are present.
	GenerationExamples GenerationStrategy = "examples"
	// GenerationDefault selects the default instructions as the final fallback.
	GenerationDefault GenerationStrategy = "default"
	// MaxInstructions bounds each precedence source.
	MaxInstructions = 64
)

// Instruction is a bounded keyed generation instruction.
type Instruction struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// GenerationInput contains the assembled context and precedence candidates.
type GenerationInput struct {
	Context  AssembledContext `json:"context"`
	EditBase []Instruction    `json:"edit_base"`
	Hints    []Instruction    `json:"hints"`
	Examples []Instruction    `json:"examples"`
	Default  []Instruction    `json:"default"`
}

// GenerationContext is the sealed final payload for a generation adapter.
type GenerationContext struct {
	Strategy             GenerationStrategy `json:"strategy"`
	FewShotDisabled      bool               `json:"few_shot_disabled"`
	Selected             []Instruction      `json:"selected"`
	MandatoryConstraints *ConstraintState   `json:"mandatory_constraints,omitempty"`
	PinnedMetrics        []PinnedMetric     `json:"pinned_metrics"`
	Prompt               string             `json:"prompt"`
	Tokens               int                `json:"tokens"`
	Budget               int                `json:"budget"`
	Context              AssembledContext   `json:"context"`
	Fit                  *GenerationFit     `json:"fit,omitempty"`
	seal                 [32]byte
}

// ResolvePrecedence is the first phase-18 generation-context consumer. It
// chooses one detached source in the documented order, fits optional context
// around it and counts the exact final payload. It never evaluates a
// rule, widens authority, calls a model, or executes a query.
func (a *ContextAssembler) ResolvePrecedence(ctx context.Context, input GenerationInput) (GenerationContext, error) {
	if ctx == nil {
		return GenerationContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context"}
	}
	if err := ctx.Err(); err != nil {
		return GenerationContext{}, err
	}
	assembled, err := a.validateAssembledContext(input.Context)
	if err != nil {
		return GenerationContext{}, err
	}
	if assembled.Strategy == StrategyClarify || assembled.Strategy == StrategyNoRoute {
		return GenerationContext{}, ErrGenerationStopped
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
	return a.fitGeneration(ctx, assembled, editBase, hints, examples, defaults)
}

// ResolvePrecedence uses the pinned default tokenizer for callers that do not
// retain the assembler. Consumers that assembled with an injected counter must
// call the method on that same assembler so both stages share one currency.
func ResolvePrecedence(ctx context.Context, input GenerationInput) (GenerationContext, error) {
	a, err := NewDefaultContextAssembler()
	if err != nil {
		return GenerationContext{}, err
	}
	return a.ResolvePrecedence(ctx, input)
}

// GenerationBudgetError reports that the final payload exceeds its tier.
type GenerationBudgetError struct {
	Tier           Tier
	Budget         int
	RequiredTokens int
}

func (e *GenerationBudgetError) Error() string {
	return fmt.Sprintf("nlq: final generation context exceeds %d-token budget", e.Budget)
}
func (e *GenerationBudgetError) Unwrap() error { return ErrInsufficient }

func renderGenerationPrompt(base string, selected []Instruction) string {
	var b strings.Builder
	b.WriteString(base)
	for _, item := range selected {
		b.WriteString("instruction[")
		b.WriteString(item.Key)
		b.WriteString("]:")
		b.WriteString(item.Text)
		b.WriteByte('\n')
	}
	return b.String()
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
	return out, nil
}

func cloneAssembled(input AssembledContext) AssembledContext {
	out := input
	out.Topics = cloneTopicRevisions(input.Topics)
	out.Relations = cloneSourceRelations(input.Relations)
	out.Evidence = cloneEvidence(input.Evidence)
	out.Constraints = cloneConstraintState(input.Constraints)
	out.Metrics = cloneMetrics(input.Metrics)
	out.Advisory = cloneOptional(input.Advisory)
	out.Examples = cloneOptional(input.Examples)
	out.Audit = cloneAudit(input.Audit)
	return out
}

func sealGenerationContext(input GenerationContext) [32]byte {
	input.seal = [32]byte{}
	return sealBytes(input)
}
