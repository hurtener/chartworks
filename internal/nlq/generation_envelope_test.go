package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSQLRecoveryEnvelopeRefitProtectsMeaningAndRank(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := minimalInput()
	in.Metrics = []PinnedMetric{{ID: "metric", Text: "mandatory metric"}}
	in.Constraints = &ConstraintState{Allowed: true, Required: []MandatoryConstraint{{ID: "filter", Kind: "required", Text: "mandatory filter"}}}
	in.Evidence = []Evidence{{ID: "optional", Text: "optional_evidence_canary"}}
	assembled, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil {
		t.Fatal(err)
	}
	g, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: []Instruction{{Key: "z-first", Text: "ranked first"}, {Key: "a-second", Text: "ranked second"}}, Default: []Instruction{{Key: "default", Text: "fallback guidance"}}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(g)
	got, err := a.RefitGeneration(context.Background(), g, func(prompt string) (bool, error) {
		return !strings.Contains(prompt, "optional_evidence_canary") && !strings.Contains(prompt, "ranked second"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Strategy != GenerationExamples || len(got.Selected) != 1 || got.Selected[0].Key != "z-first" || !strings.Contains(got.Prompt, "mandatory metric") || !strings.Contains(got.Prompt, "mandatory filter") || !reflect.DeepEqual(got.Context.Relations, g.Context.Relations) || got.Fit.OmittedCount != 2 {
		t.Fatal("fit changed meaning/rank/scope")
	}
	if got.seal != sealGenerationContext(got) {
		t.Fatal("not resealed")
	}
	after, _ := json.Marshal(g)
	if string(before) != string(after) {
		t.Fatal("mutated initial packet")
	}
	got.Fit.Omitted[0].Reason = "changed"
	if len(g.Fit.Omitted) != 0 {
		t.Fatal("shared omission state")
	}
}

func TestSQLRecoveryEnvelopeRefitKeepsDefaultFallback(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	assembled, err := a.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatal(err)
	}
	g, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: []Instruction{{Key: "demo", Text: "demonstration"}}, Default: []Instruction{{Key: "default", Text: "essential default guidance"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.RefitGeneration(context.Background(), g, func(prompt string) (bool, error) { return !strings.Contains(prompt, "demonstration"), nil })
	if err != nil || got.Strategy != GenerationDefault || !strings.Contains(got.Prompt, "essential default guidance") {
		t.Fatal("lost fallback", err)
	}
}

func TestSQLRecoveryEnvelopeRefitCannotDropRequiredInstructions(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	assembled, err := a.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatal(err)
	}
	g, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, EditBase: []Instruction{{Key: "base", Text: "required previous SQL"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RefitGeneration(context.Background(), g, func(string) (bool, error) { return false, nil })
	if !errors.Is(err, ErrEnvelopeBudget) || !errors.Is(err, ErrInsufficient) {
		t.Fatal("mandatory packet did not fail closed", err)
	}
}

func TestSQLRecoveryEnvelopeRefitRejectsUnsealedOrCancelled(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	assembled, err := a.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatal(err)
	}
	g, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	fits := func(string) (bool, error) { calls++; return true, nil }
	raw, _ := json.Marshal(g)
	var decoded GenerationContext
	_ = json.Unmarshal(raw, &decoded)
	if _, err = a.RefitGeneration(context.Background(), decoded, fits); err == nil {
		t.Fatal("retained/client JSON established seal")
	}
	bad := g
	bad.Prompt = "tampered"
	if _, err = a.RefitGeneration(context.Background(), bad, fits); err == nil {
		t.Fatal("tampered packet accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = a.RefitGeneration(ctx, g, fits); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("invalid input reached fitting")
	}
	failure := errors.New("closed diagnostic")
	if _, err = a.RefitGeneration(context.Background(), g, func(string) (bool, error) { return false, failure }); !errors.Is(err, failure) {
		t.Fatal("swallowed measurement error", err)
	}
}
