package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
)

func TestSQLRecoveryEditAndHintsSkipExampleRepositoryAndRerank(t *testing.T) {
	e := testEnvelope(t)
	a, _, call, budget := testGeneration(t, e)
	// Nil dependencies deliberately panic if an irrelevant selection touches
	// the repository or the model. The production caller admits authority first.
	service := &Service{}
	for _, question := range []QuestionRequest{
		{Rerank: true, EditBase: []nlq.Instruction{{Key: "base", Text: "previous query"}}},
		{Rerank: true, Hints: []nlq.Instruction{{Key: "hint", Text: "reviewed hint"}}},
	} {
		examples, selection, receipt, err := service.selectGenerationExamples(context.Background(), e, a, question, call, budget)
		if err != nil || len(examples) != 0 || len(receipt.Calls) != 0 || len(selection.Selected) != 0 || selection.PolicyVersion != "suppressed-by-precedence-v1" {
			t.Fatal("irrelevant learning performed work", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := service.selectGenerationExamples(ctx, e, a, QuestionRequest{Hints: []nlq.Instruction{{Key: "hint", Text: "text"}}}, call, budget)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("suppression ignored cancellation")
	}
}

func TestSQLRecoveryExampleUsageReflectsActualRenderedOrder(t *testing.T) {
	score := 0.8
	selection := ExampleSelectionEvidence{Selected: []ExampleSelection{
		{ExampleID: "first", Version: 1, Position: 1, Decision: "selected", RankScore: &score},
		{ExampleID: "second", Version: 1, Position: 2, Decision: "selected"},
		{ExampleID: "third", Version: 1, Position: 3, Decision: "selected"},
	}}
	before, _ := json.Marshal(selection)
	generation := nlq.GenerationContext{Strategy: nlq.GenerationExamples, Selected: []nlq.Instruction{
		{Key: "external", Text: "external"}, {Key: "learned-second", Text: "second"}, {Key: "learned-first", Text: "first"},
	}}
	usage := actualExampleUsage(selection, generation)
	if len(usage.Used) != 2 || len(usage.Omitted) != 1 || usage.Used[0].Position != 2 || usage.Used[1].Position != 3 || usage.Omitted[0].ExampleID != "third" || usage.Omitted[0].Position != 0 {
		t.Fatal("usage confused candidate rank with rendered rank")
	}
	*usage.Used[1].RankScore = 0
	after, _ := json.Marshal(selection)
	if string(before) != string(after) {
		t.Fatal("usage mutated original selection")
	}
	generation.Strategy, generation.FewShotDisabled = nlq.GenerationEditBase, true
	usage = actualExampleUsage(selection, generation)
	if len(usage.Used) != 0 || len(usage.Omitted) != 3 || usage.Omitted[0].Reason != "precedence" {
		t.Fatal("edit attributed unused examples")
	}
}

func TestSQLRecoveryRepairDoesNotNestSuppressedDemonstrations(t *testing.T) {
	e := testEnvelope(t)
	a, _, _, _ := testGeneration(t, e)
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	original, err := assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: a.assembled, Examples: []nlq.Instruction{{Key: "demo", Text: "demonstration-canary-only"}}})
	if err != nil {
		t.Fatal(err)
	}
	repair, err := validationRepairContext(context.Background(), original, generatedCandidate{SQL: "SELECT id FROM analytics.sales"}, "validation_unsafe")
	if err != nil {
		t.Fatal(err)
	}
	if !repair.FewShotDisabled || strings.Contains(repair.Prompt, "demonstration-canary-only") || !strings.Contains(repair.Prompt, "SELECT id FROM analytics.sales") {
		t.Fatal("repair resurrected examples or lost failed SQL")
	}
}
