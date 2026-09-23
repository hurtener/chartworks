package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSQLRecoveryFitSuppressesEveryContextExample(t *testing.T) {
	a, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatal(err)
	}
	in := minimalInput()
	in.Examples = []OptionalItem{{ID: "context-demo", Text: "context demonstration canary"}}
	assembled, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(assembled)
	for _, strategy := range []string{"edit", "hint"} {
		input := GenerationInput{Context: assembled, Examples: []Instruction{{Key: "learned", Text: "learned demonstration canary"}}}
		if strategy == "edit" {
			input.EditBase = []Instruction{{Key: "base", Text: "retained query"}}
		} else {
			input.Hints = []Instruction{{Key: "hint", Text: "reviewed hint"}}
		}
		got, err := a.ResolvePrecedence(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if !got.FewShotDisabled || strings.Contains(got.Prompt, "demonstration canary") || len(got.Context.Examples) != 0 || got.Fit.OmittedCount != 2 {
			t.Fatalf("example suppression failed: %#v", got.Fit)
		}
		if _, err := a.validateAssembledContext(got.Context); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := json.Marshal(assembled)
	if string(before) != string(after) {
		t.Fatal("mutated route context")
	}
}

func TestSQLRecoveryFitPreservesRankAndOneExampleLane(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := minimalInput()
	in.Examples = []OptionalItem{{ID: "context", Text: "third demonstration"}}
	assembled, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: []Instruction{{Key: "z-first", Text: "first demonstration"}, {Key: "a-second", Text: "second demonstration"}}})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, v := range got.Selected {
		keys = append(keys, v.Key)
	}
	if !reflect.DeepEqual(keys, []string{"z-first", "a-second", "context"}) || got.Strategy != GenerationExamples || len(got.Context.Examples) != 0 {
		t.Fatal("rank or lane changed", keys)
	}
	if strings.Index(got.Prompt, "first demonstration") > strings.Index(got.Prompt, "second demonstration") || strings.Count(got.Prompt, "third demonstration") != 1 {
		t.Fatal("rendered rank differs")
	}
}

func TestSQLRecoveryFitPrunesOptionalEvidenceForEdit(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := minimalInput()
	in.Metrics = []PinnedMetric{{ID: "metric", Text: "mandatory metric"}}
	in.Constraints = &ConstraintState{Allowed: true, Required: []MandatoryConstraint{{ID: "filter", Kind: "required", Text: "mandatory filter"}}}
	in.Evidence = []Evidence{{ID: "large", Text: strings.Repeat("optional ", 1100)}}
	assembled, err := a.Assemble(context.Background(), in, TierLow)
	if err != nil || len(assembled.Evidence) != 1 {
		t.Fatal(err)
	}
	got, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, EditBase: []Instruction{{Key: "base", Text: strings.Repeat("edit ", 600)}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Context.Evidence) != 0 || got.Tokens > got.Budget || !strings.Contains(got.Prompt, "mandatory metric") || !strings.Contains(got.Prompt, "mandatory filter") {
		t.Fatal("incorrect fit")
	}
	if got.Fit.OmittedCount != 1 || got.Fit.Omitted[0].ID != "large" {
		t.Fatal("missing omission evidence")
	}
}

func TestSQLRecoveryFitCapsAndDeduplicatesExamples(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	in := minimalInput()
	in.Examples = []OptionalItem{{ID: "same", Text: "same demonstration"}}
	assembled, err := a.Assemble(context.Background(), in, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	examples := []Instruction{{Key: "same", Text: "same demonstration"}}
	for _, key := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		examples = append(examples, Instruction{Key: key, Text: key})
	}
	got, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: examples})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Selected) != MaxExamples || got.Fit.OmittedCount != 2 || strings.Count(got.Prompt, "same demonstration") != 1 {
		t.Fatal("cap/dedup failed")
	}
	examples[0].Text = "conflicting"
	_, err = a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: examples})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Code != CodeDuplicateKey {
		t.Fatal("conflicting example accepted", err)
	}
}

func TestSQLRecoveryFitOversizedExamplesFallBackWithoutDroppingMeaning(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	assembled, err := a.Assemble(context.Background(), minimalInput(), TierLow)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: assembled, Examples: []Instruction{{Key: "large", Text: strings.Repeat("demo ", 1800)}}, Default: []Instruction{{Key: "default", Text: "default guidance"}}})
	if err != nil || got.Strategy != GenerationDefault || !strings.Contains(got.Prompt, "default guidance") || got.Fit.OmittedCount != 1 {
		t.Fatal("optional example blocked generation", err)
	}
}
