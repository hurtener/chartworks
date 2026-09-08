package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
			got, err := assembler.ResolvePrecedence(context.Background(), GenerationInput{
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

func TestResolvePrecedenceCountsExactFinalPayload(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got, err := assembler.ResolvePrecedence(context.Background(), GenerationInput{
		Context: assembled,
		Hints:   []Instruction{{Key: "hint", Text: "keep this bounded instruction"}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.Contains(got.Prompt, "instruction[hint]:keep this bounded instruction") || got.Tokens <= assembled.Tokens || got.Tokens > got.Budget {
		t.Fatalf("final payload was not counted exactly: tokens=%d base=%d budget=%d prompt=%q", got.Tokens, assembled.Tokens, got.Budget, got.Prompt)
	}
	if got.Budget != TierLow.Budget() {
		t.Fatalf("unexpected generation budget %d", got.Budget)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal generation context: %v", err)
	}
	if strings.Contains(string(wire), `"audit"`) || strings.Contains(string(wire), "unpruned") {
		t.Fatalf("generation wire exposed assembly metadata: %s", wire)
	}
}

func TestResolvePrecedenceRejectsSelectedPayloadOverBudget(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	instructions := make([]Instruction, MaxInstructions)
	for i := range instructions {
		instructions[i] = Instruction{Key: "instruction_" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Text: strings.Repeat("bounded ", 1800)}
	}
	_, err = assembler.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, EditBase: instructions})
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("oversized final payload returned %v, want insufficiency", err)
	}
	var budgetErr *GenerationBudgetError
	if !errors.As(err, &budgetErr) || budgetErr.RequiredTokens <= budgetErr.Budget {
		t.Fatalf("missing final payload budget evidence: %v", err)
	}
	if budgetErr.Error() == "" {
		t.Fatal("generation budget error had empty text")
	}
}

func TestResolvePrecedenceRejectsUnsafeOrStoppedContext(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*AssembledContext)
	}{
		{name: "denied constraints", edit: func(c *AssembledContext) { c.Constraints = &ConstraintState{Allowed: false} }},
		{name: "wrong budget", edit: func(c *AssembledContext) { c.Budget = MediumBudget }},
		{name: "wrong token count", edit: func(c *AssembledContext) { c.Tokens++ }},
		{name: "prompt mismatch", edit: func(c *AssembledContext) { c.Prompt += "tampered" }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			bad := assembled
			tt.edit(&bad)
			if _, err := assembler.ResolvePrecedence(context.Background(), GenerationInput{Context: bad}); err == nil {
				t.Fatal("mutated context accepted")
			}
		})
	}
	for _, strategy := range []Strategy{StrategyClarify, StrategyNoRoute} {
		input := minimalInput()
		input.Strategy = strategy
		stopped, err := assembler.Assemble(context.Background(), input, TierLow)
		if err != nil {
			t.Fatalf("assemble %s: %v", strategy, err)
		}
		if _, err := assembler.ResolvePrecedence(context.Background(), GenerationInput{Context: stopped}); !errors.Is(err, ErrGenerationStopped) {
			t.Fatalf("strategy %s returned %v, want stop", strategy, err)
		}
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
	got, err := assembler.ResolvePrecedence(context.Background(), input)
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
	_, err = assembler.ResolvePrecedence(context.Background(), GenerationInput{
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

func TestResolvePrecedenceDefaultWrapperAndInstructionValidation(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	assembled, err := assembler.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got, err := ResolvePrecedence(context.Background(), GenerationInput{
		Context: assembled,
		Default: []Instruction{{Key: "default", Text: "use the default"}},
	})
	if err != nil || got.Strategy != GenerationDefault {
		t.Fatalf("default wrapper returned %#v, %v", got, err)
	}

	cases := []struct {
		name string
		edit func(*GenerationInput)
		code ValidationCode
	}{
		{name: "invalid key", edit: func(input *GenerationInput) { input.Hints = []Instruction{{Key: "bad key", Text: "hint"}} }, code: CodeInvalidValue},
		{name: "too many instructions", edit: func(input *GenerationInput) { input.Hints = make([]Instruction, MaxInstructions+1) }, code: CodeLimit},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := GenerationInput{Context: assembled}
			test.edit(&input)
			_, err := assembler.ResolvePrecedence(context.Background(), input)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != test.code {
				t.Fatalf("resolve returned %v, want validation code %q", err, test.code)
			}
		})
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
	_, err = assembler.ResolvePrecedence(ctx, GenerationInput{Context: assembled})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v did not preserve cancellation", err)
	}
}
