package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestTiktokenCounterIsDeterministicForEnglishAndSpanish(t *testing.T) {
	counter, err := NewTiktokenCounter()
	if err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}
	if TokenizerEncoding != "cl100k_base" {
		t.Fatalf("unexpected tokenizer encoding %q", TokenizerEncoding)
	}
	for _, text := range []string{"revenue by month", "ingresos por mes"} {
		first, err := counter.Count(text)
		if err != nil {
			t.Fatalf("count %q: %v", text, err)
		}
		second, err := counter.Count(text)
		if err != nil {
			t.Fatalf("count %q twice: %v", text, err)
		}
		if first <= 0 || first != second {
			t.Fatalf("count %q was not stable and positive: %d, %d", text, first, second)
		}
	}
}

func TestTierForConfidence(t *testing.T) {
	tests := []struct {
		confidence float64
		want       Tier
	}{
		{confidence: 0, want: TierLow},
		{confidence: 0.69, want: TierLow},
		{confidence: 0.70, want: TierMedium},
		{confidence: 0.84, want: TierMedium},
		{confidence: 0.85, want: TierHigh},
		{confidence: 1, want: TierHigh},
	}
	for _, tt := range tests {
		got, err := TierForConfidence(tt.confidence)
		if err != nil || got != tt.want {
			t.Errorf("confidence %.2f: got %q, %v; want %q", tt.confidence, got, err, tt.want)
		}
	}
	for _, confidence := range []float64{-0.01, 1.01} {
		if _, err := TierForConfidence(confidence); err == nil {
			t.Errorf("confidence %.2f was accepted", confidence)
		}
	}
}

func TestAssemblerPreservesMandatoryLanesAndDetachesInput(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := ContextInput{
		Locale:       LanguageEnglish,
		Strategy:     StrategyMultiTopic,
		Topic:        "sales",
		TopicVersion: "v3",
		Question:     "Compare revenue by month for both approved topics",
		Constraints: &ConstraintState{
			Allowed:  true,
			Required: []MandatoryConstraint{{ID: "date_range", Kind: "require_reference", Text: "use the approved reporting period"}},
			Excluded: []MandatoryConstraint{{ID: "raw_customer", Kind: "exclude_reference", Text: "do not include raw customer records"}},
		},
		Metrics:  []PinnedMetric{{ID: "revenue", Text: "revenue is the pinned metric"}},
		Evidence: []Evidence{{ID: "join", Text: "same source relationship is confirmed", Priority: 10, Source: "catalog"}},
		Advisory: []OptionalItem{{ID: "hint", Text: "prefer the monthly grain", Priority: 5}},
		Examples: []OptionalItem{{ID: "example", Text: "show the approved comparison", Priority: 1}},
	}
	assembled, err := assembler.Assemble(context.Background(), input, TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if assembled.Tokens <= 0 || assembled.Tokens > LowBudget {
		t.Fatalf("assembled token count %d is outside low budget", assembled.Tokens)
	}
	if assembled.Constraints == nil || len(assembled.Constraints.Required) != 1 || len(assembled.Constraints.Excluded) != 1 {
		t.Fatalf("mandatory constraints were not preserved: %#v", assembled.Constraints)
	}
	if len(assembled.Metrics) != 1 || assembled.Metrics[0].ID != "revenue" {
		t.Fatalf("pinned metrics were not preserved: %#v", assembled.Metrics)
	}
	if !strings.Contains(assembled.Prompt, "do not include raw customer records") {
		t.Fatal("excluded constraint was omitted from the prompt")
	}
	if assembled.Evidence[0].Source != "catalog" {
		t.Fatalf("evidence provenance was not retained: %#v", assembled.Evidence[0])
	}

	input.Question = "mutated after assembly"
	input.Constraints.Required[0].Text = "mutated required constraint"
	input.Evidence[0].Text = "mutated evidence"
	if assembled.Question == input.Question {
		t.Fatal("assembled question aliases caller input")
	}
	if assembled.Constraints.Required[0].Text == input.Constraints.Required[0].Text {
		t.Fatal("assembled constraints alias caller input")
	}
	if assembled.Evidence[0].Text == input.Evidence[0].Text {
		t.Fatal("assembled evidence aliases caller input")
	}
}

func TestAssemblerAcceptsDetachedSpanishInput(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := ContextInput{
		Locale:   LanguageSpanish,
		Strategy: StrategySingleTopic,
		Question: "muestra los ingresos aprobados por mes",
		Metrics:  []PinnedMetric{{ID: "ingresos", Text: "ingresos netos"}},
	}
	assembled, err := assembler.Assemble(context.Background(), input, TierMedium)
	if err != nil {
		t.Fatalf("assemble Spanish input: %v", err)
	}
	if assembled.Locale != LanguageSpanish || assembled.Question != input.Question || assembled.Tokens <= 0 {
		t.Fatalf("Spanish input was not retained: %#v", assembled)
	}
}

func TestAssemblerReturnsTypedInsufficiencyForMandatoryContext(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := minimalInput()
	input.Constraints = &ConstraintState{
		Allowed: true,
		Required: []MandatoryConstraint{{
			ID:   "required_filter",
			Kind: "require_reference",
			Text: strings.Repeat("1234567890 ", 1300),
		}},
	}
	_, err = assembler.Assemble(context.Background(), input, TierLow)
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("error %v does not identify insufficient context", err)
	}
	var budgetErr *BudgetError
	if !errors.As(err, &budgetErr) || budgetErr.RequiredTokens <= LowBudget || budgetErr.Tier != TierLow {
		t.Fatalf("error %v does not include the mandatory budget evidence", err)
	}
}

func TestAssemblerCapsOmissionDetailsAndExamples(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := minimalInput()
	for i := 0; i < 20; i++ {
		input.Advisory = append(input.Advisory, OptionalItem{ID: "advisory_" + string(rune('a'+i)), Text: "advisory guidance", Priority: i})
		input.Examples = append(input.Examples, OptionalItem{ID: "example_" + string(rune('a'+i)), Text: "example guidance", Priority: i})
	}
	assembled, err := assembler.Assemble(context.Background(), input, TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(assembled.Examples) > MaxExamples {
		t.Fatalf("assembled %d examples, max is %d", len(assembled.Examples), MaxExamples)
	}
	if assembled.Audit.OmittedCount < len(input.Examples)-MaxExamples {
		t.Fatalf("omitted count %d did not include capped examples", assembled.Audit.OmittedCount)
	}
	if len(assembled.Audit.Omitted) > MaxOmissions {
		t.Fatalf("recorded %d omissions, max is %d", len(assembled.Audit.Omitted), MaxOmissions)
	}
}

func TestAssemblerRejectsUnsatisfiedConstraints(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	input := minimalInput()
	input.Constraints = &ConstraintState{Allowed: false}
	_, err = assembler.Assemble(context.Background(), input, TierLow)
	if !errors.Is(err, ErrConstraintConflict) {
		t.Fatalf("error %v does not identify the unsatisfied constraints", err)
	}
}

func minimalInput() ContextInput {
	return ContextInput{
		Locale:   LanguageEnglish,
		Strategy: StrategySingleTopic,
		Question: "show the approved metric",
	}
}

func TestAssembledContextWireOmitsPrunedPayload(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	omittedText := "omitted-sensitive-guidance"
	input := minimalInput()
	input.Advisory = []OptionalItem{{ID: "oversized", Text: omittedText + " " + strings.Repeat("guidance ", 1800), Priority: 1}}
	assembled, err := assembler.Assemble(context.Background(), input, TierLow)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if assembled.Audit.OmittedCount != 1 || len(assembled.Advisory) != 0 {
		t.Fatalf("oversized advisory was not omitted: %#v", assembled.Audit)
	}
	wire, err := json.Marshal(assembled)
	if err != nil {
		t.Fatalf("marshal assembled context: %v", err)
	}
	if strings.Contains(string(wire), "unpruned") || strings.Contains(string(wire), omittedText) {
		t.Fatalf("wire context exposed omitted payload: %s", wire)
	}
}

func TestTiktokenCounterKnownVectors(t *testing.T) {
	counter, err := NewTiktokenCounter()
	if err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}
	for _, tt := range []struct {
		input string
		want  int
	}{
		{"revenue by month", 4},
		{"ingresos por mes", 5},
		{"Hello, world!", 4},
		{"Use the approved period.", 5},
	} {
		got, err := counter.Count(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("Count(%q) = %d, %v; want %d", tt.input, got, err, tt.want)
		}
	}
}

func TestAssemblerConfidenceAndCounterErrors(t *testing.T) {
	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	if _, err := assembler.AssembleForConfidence(context.Background(), minimalInput(), 0.85); err != nil {
		t.Fatalf("assemble high confidence: %v", err)
	}
	if _, err := assembler.AssembleForConfidence(context.Background(), minimalInput(), 1.1); err == nil {
		t.Fatal("invalid confidence accepted")
	}

	base, err := NewTiktokenCounter()
	if err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}
	failing, err := NewContextAssembler(&failAfterCounter{base: base, failAt: 3})
	if err != nil {
		t.Fatalf("new failing assembler: %v", err)
	}
	if _, err := failing.Assemble(context.Background(), minimalInput(), TierLow); err == nil {
		t.Fatal("lane accounting swallowed tokenizer error")
	}
}

func TestAssemblerRejectsInvalidInputsAndCounterValues(t *testing.T) {
	if _, err := NewContextAssembler(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil counter returned %v", err)
	}
	var nilCounter *TiktokenCounter
	if _, err := nilCounter.Count("text"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil tokenizer returned %v", err)
	}
	counter := &TiktokenCounter{}
	if _, err := counter.Count("text"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty tokenizer returned %v", err)
	}
	if _, err := NewTiktokenCounter(); err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}

	assembler, err := NewDefaultContextAssembler()
	if err != nil {
		t.Fatalf("new assembler: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*ContextInput)
		code   ValidationCode
	}{
		{name: "locale", mutate: func(input *ContextInput) { input.Locale = "fr" }, code: CodeUnsupported},
		{name: "strategy", mutate: func(input *ContextInput) { input.Strategy = "unknown" }, code: CodeUnsupported},
		{name: "question", mutate: func(input *ContextInput) { input.Question = "" }, code: CodeInvalidValue},
		{name: "topic", mutate: func(input *ContextInput) { input.Topic = "sales/topic" }, code: CodeInvalidValue},
		{name: "topic version", mutate: func(input *ContextInput) { input.TopicVersion = "v/1" }, code: CodeInvalidValue},
		{name: "evidence", mutate: func(input *ContextInput) { input.Evidence = []Evidence{{ID: "", Text: "evidence"}} }, code: CodeInvalidValue},
		{name: "evidence source", mutate: func(input *ContextInput) {
			input.Evidence = []Evidence{{ID: "evidence", Text: "evidence", Source: "catalog/source"}}
		}, code: CodeInvalidValue},
		{name: "evidence confidence", mutate: func(input *ContextInput) {
			value := math.NaN()
			input.Evidence = []Evidence{{ID: "evidence", Text: "evidence", Confidence: &value}}
		}, code: CodeInvalidValue},
		{name: "metric", mutate: func(input *ContextInput) { input.Metrics = []PinnedMetric{{ID: "", Text: "metric"}} }, code: CodeInvalidValue},
		{name: "advisory", mutate: func(input *ContextInput) { input.Advisory = []OptionalItem{{ID: "advisory", Text: "line\nbreak"}} }, code: CodeInvalidValue},
		{name: "duplicate metric", mutate: func(input *ContextInput) {
			input.Evidence = []Evidence{{ID: "shared", Text: "evidence"}}
			input.Metrics = []PinnedMetric{{ID: "shared", Text: "metric"}}
		}, code: CodeDuplicateID},
		{name: "constraint", mutate: func(input *ContextInput) {
			input.Constraints = &ConstraintState{Allowed: true, Required: []MandatoryConstraint{{ID: "required", Kind: "", Text: "constraint"}}}
		}, code: CodeInvalidValue},
		{name: "duplicate constraint", mutate: func(input *ContextInput) {
			input.Constraints = &ConstraintState{Allowed: true, Required: []MandatoryConstraint{{ID: "shared", Kind: "required", Text: "one"}}, Excluded: []MandatoryConstraint{{ID: "shared", Kind: "excluded", Text: "two"}}}
		}, code: CodeDuplicateID},
		{name: "lane limit", mutate: func(input *ContextInput) { input.Evidence = make([]Evidence, 257) }, code: CodeLimit},
		{name: "constraint limit", mutate: func(input *ContextInput) {
			input.Constraints = &ConstraintState{Allowed: true, Required: make([]MandatoryConstraint, MaxConstraints+1)}
		}, code: CodeLimit},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := minimalInput()
			test.mutate(&input)
			_, err := assembler.Assemble(context.Background(), input, TierLow)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != test.code {
				t.Fatalf("assemble returned %v, want validation code %q", err, test.code)
			}
			if validationErr.Error() == "" {
				t.Fatal("validation error had empty text")
			}
		})
	}

	//nolint:staticcheck // Deliberately verify that nil context is rejected.
	if _, err := assembler.Assemble(nil, minimalInput(), TierLow); err == nil {
		t.Fatal("nil context accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := assembler.Assemble(canceled, minimalInput(), TierLow); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context returned %v", err)
	}
	if _, err := assembler.Assemble(context.Background(), minimalInput(), Tier("unknown")); err == nil {
		t.Fatal("unknown tier accepted")
	}
	if _, err := counter.Count(string([]byte{0xff})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid UTF-8 returned %v", err)
	}
}

type failAfterCounter struct {
	base   TokenCounter
	calls  int
	failAt int
}

func (c *failAfterCounter) Count(text string) (int, error) {
	c.calls++
	if c.calls == c.failAt {
		return 0, errors.New("counter failure")
	}
	return c.base.Count(text)
}
