package nlqexec

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
)

// ExamplePromptUsage distinguishes candidate ranking from demonstrations that
// survived final fitting. Absence means legacy/unknown, never zero actual use.
// It describes the initial sqlgen packet; repair has its own generation packet.
type ExamplePromptUsage struct {
	Version string             `json:"version"`
	Used    []ExampleSelection `json:"used,omitempty"`
	Omitted []ExampleSelection `json:"omitted,omitempty"`
}

func (s *Service) selectGenerationExamples(ctx context.Context, e identity.Envelope, a admission, question QuestionRequest, call gateway.Call, budget *gateway.Budget) ([]nlq.Instruction, ExampleSelectionEvidence, gateway.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, ExampleSelectionEvidence{}, gateway.Receipt{}, err
	}
	if len(question.EditBase) > 0 || len(question.Hints) > 0 {
		// This runs only after normal question/source/authority admission.
		// There is no reason to query mutable learning state or spend a model
		// rerank when precedence makes those demonstrations irrelevant.
		return nil, ExampleSelectionEvidence{SchemaVersion: 1, PolicyVersion: "suppressed-by-precedence-v1"}, gateway.Receipt{}, nil
	}
	return s.selectLearnedInstructions(ctx, e, a, question.Question, question.Rerank, call, budget)
}

func actualExampleUsage(selection ExampleSelectionEvidence, generation nlq.GenerationContext) *ExamplePromptUsage {
	usage := &ExamplePromptUsage{Version: "example-prompt-usage-v1"}
	positions := map[string]int{}
	if generation.Strategy == nlq.GenerationExamples && !generation.FewShotDisabled {
		for i, item := range generation.Selected {
			positions[item.Key] = i + 1
		}
	}
	reasons := map[string]string{}
	if generation.Fit != nil {
		for _, omitted := range generation.Fit.Omitted {
			if omitted.Lane == nlq.LaneExamples {
				reasons[omitted.ID] = omitted.Reason
			}
		}
	}
	for _, item := range selection.Selected {
		// Detach even optional score pointers from the selection receipt.
		if item.RankScore != nil {
			score := *item.RankScore
			item.RankScore = &score
		}
		key := "learned-" + item.ExampleID
		if position, ok := positions[key]; ok {
			item.Position, item.Decision, item.Reason = position, "used", "rendered"
			usage.Used = append(usage.Used, item)
		} else {
			item.Position, item.Decision, item.Reason = 0, "omitted", "not_rendered"
			if generation.FewShotDisabled {
				item.Reason = "precedence"
			} else if reason := reasons[key]; reason != "" {
				item.Reason = reason
			}
			usage.Omitted = append(usage.Omitted, item)
		}
	}
	sort.Slice(usage.Used, func(i, j int) bool { return usage.Used[i].Position < usage.Used[j].Position })
	return usage
}
