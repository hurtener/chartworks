package nlq

import (
	"context"
	"errors"
	"testing"
)

func TestResolvePrecedenceUsesFirstAvailableSource(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := minimalInput()
	input.Constraints = &ConstraintState{
		Allowed:  true,
		Required: []MandatoryConstraint{{ID: "required_filter", Kind: "require_reference", Text: "keep the approved period"}},
		Excluded: []MandatoryConstraint{{ID: "private_field", Kind: "exclude_reference", Text: "exclude private fields"}},
	}
	input.Metrics = []PinnedMetric{{ID: "revenue", Text: "revenue"}}
	assembled, err := assembler.Assemble(context.Background(), input, TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	tests := []struct {
		name             string
		editBase         []Instruction
		hints            []Instruction
		examples         []Instruction
		defaults         []Instruction
		wantStrategy     GenerationStrategy
		wantFewShotOff   bool
		wantSelectedText string
	}{
		{
			name:             "edit base wins",
			editBase:         []Instruction{{Key: "edit", Text: "repair the approved edit"}},
			hints:            []Instruction{{Key: "hint", Text: "use the hint"}},
			examples:         []Instruction{{Key: "example", Text: "use the example"}},
			defaults:         []Instruction{{Key: "default", Text: "use the default"}},
			wantStrategy:     GenerationEditBase,
			wantFewShotOff:   true,
			wantSelectedText: "repair the approved edit",
		},
		{
			name:             "hints beat examples",
			hints:            []Instruction{{Key: "hint", Text: "use the hint"}},
			examples:         []Instruction{{Key: "example", Text: "use the example"}},
			defaults:         []Instruction{{Key: "default", Text: "use the default"}},
			wantStrategy:     GenerationHints,
			wantFewShotOff:   true,
			wantSelectedText: "use the hint",
		},
		{
			name:             "examples beat default",
			examples:         []Instruction{{Key: "example", Text: "use the example"}},
			defaults:         []Instruction{{Key: "default", Text: "use the default"}},
			wantStrategy:     GenerationExamples,
			wantSelectedText: "use the example",
		},
		{
			name:             "default fallback",
			defaults:         []Instruction{{Key: "default", Text: "use the default"}},
			wantStrategy:     GenerationDefault,
			wantSelectedText: "use the default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePrecedence(context.Background(), GenerationInput{
				Context:  assembled,
				EditBase: tt.editBase,
				Hints:    tt.hints,
				Examples: tt.examples,
				Default:  tt.defaults,
			})
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got.Strategy != tt.wantStrategy || got.FewShotDisabled != tt.wantFewShotOff {
				t.Fatalf("got strategy=%q few_shot_disabled=%v; want %q/%v", got.Strategy, got.FewShotDisabled, tt.wantStrategy, tt.wantFewShotOff)
			}
			if len(got.Selected) != 1 || got.Selected[0].Text != tt.wantSelectedText {
				t.Fatalf("selected %#v; want %q", got.Selected, tt.wantSelectedText)
			}
			if got.MandatoryConstraints == nil || len(got.MandatoryConstraints.Required) != 1 || len(got.MandatoryConstraints.Excluded) != 1 {
				t.Fatalf("mandatory constraints were dropped: %#v", got.MandatoryConstraints)
			}
			if len(got.PinnedMetrics) != 1 || got.PinnedMetrics[0].ID != "revenue" {
				t.Fatalf("pinned metrics were dropped: %#v", got.PinnedMetrics)
			}
		})
	}
}

func TestResolvePrecedenceDetachesSelectedInstructions(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	input := GenerationInput{
		Context: assembled,
		Hints:   []Instruction{{Key: "hint", Text: "preserve this hint"}},
	}
	got, err := ResolvePrecedence(context.Background(), input)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	input.Hints[0].Text = "mutated"
	input.Context.Question = "mutated"
	if got.Selected[0].Text != "preserve this hint" || got.Context.Question == "mutated" {
		t.Fatal("generation result aliases caller input")
	}
}

func TestResolvePrecedenceRejectsDuplicateInstructionKeys(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	_, err = ResolvePrecedence(context.Background(), GenerationInput{
		Context: assembled,
		Hints: []Instruction{
			{Key: "same", Text: "first"},
			{Key: "same", Text: "second"},
		},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != CodeDuplicateKey {
		t.Fatalf("error %v did not identify duplicate key", err)
	}
}

func TestResolvePrecedenceHonorsCancellation(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ResolvePrecedence(ctx, GenerationInput{Context: assembled})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v did not preserve cancellation", err)
	}
}
