package semantics

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestClarificationMergeRejectsUnownedAndStaleDeltasAtomically(t *testing.T) {
	_, definition := cw01Definition(t, cw01NumberSlot())
	original := cw01Answer(definition, "order_period", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}})
	copyOf := func(a ClarificationAnswer) ClarificationAnswer { return CloneClarificationAnswers([]ClarificationAnswer{a})[0] }
	removal := copyOf(original)
	removal.Remove, removal.Value = true, nil
	missing := copyOf(removal)
	missing.Slot = "unowned"
	noValue := copyOf(original)
	noValue.Value = nil
	both := copyOf(original)
	both.Remove = true
	cases := []struct {
		name  string
		base  []ClarificationAnswer
		delta []ClarificationAnswer
	}{
		{"duplicate-base", []ClarificationAnswer{original, original}, nil},
		{"base-removal", []ClarificationAnswer{removal}, nil},
		{"base-no-value", []ClarificationAnswer{noValue}, nil},
		{"delta-no-value", []ClarificationAnswer{original}, []ClarificationAnswer{noValue}},
		{"delta-two-actions", []ClarificationAnswer{original}, []ClarificationAnswer{both}},
		{"duplicate-delta", []ClarificationAnswer{original}, []ClarificationAnswer{removal, removal}},
		{"unowned-removal", []ClarificationAnswer{original}, []ClarificationAnswer{missing}},
		{"oversized-base", make([]ClarificationAnswer, 65), nil},
		{"oversized-delta", nil, make([]ClarificationAnswer, 65)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeBase, beforeDelta := CloneClarificationAnswers(tc.base), CloneClarificationAnswers(tc.delta)
			out, err := MergeClarificationAnswers(tc.base, tc.delta)
			if !errors.Is(err, ErrInvalid) || out != nil {
				t.Fatal("invalid delta returned partially accepted answers", err)
			}
			if !reflect.DeepEqual(tc.base, beforeBase) || !reflect.DeepEqual(tc.delta, beforeDelta) {
				t.Fatal("rejected delta mutated session input")
			}
		})
	}
	for _, mutate := range []func(*ClarificationAnswer){
		func(a *ClarificationAnswer) { a.TopicVersion = "stale-topic" },
		func(a *ClarificationAnswer) { a.RulesetVersion = "stale-rules" },
		func(a *ClarificationAnswer) { a.PatternVersion = "stale-pattern" },
	} {
		for _, delta := range []ClarificationAnswer{copyOf(original), copyOf(removal)} {
			mutate(&delta)
			out, err := MergeClarificationAnswers([]ClarificationAnswer{original}, []ClarificationAnswer{delta})
			var typed *ValidationError
			if !errors.As(err, &typed) || typed.Code != CodeEvidenceMismatch || out != nil {
				t.Fatal("stale replacement or removal accepted", err)
			}
		}
	}
	base := make([]ClarificationAnswer, 64)
	for i := range base {
		base[i] = copyOf(original)
		base[i].Slot = fmt.Sprintf("field-%02d", i)
	}
	out, err := MergeClarificationAnswers(base, []ClarificationAnswer{original})
	var typed *ValidationError
	if !errors.As(err, &typed) || typed.Code != CodeLimit || out != nil {
		t.Fatal("combined group exceeded the answer limit", err)
	}
}

func TestClarificationMergeReplacementRemovalAndDetachedOrdering(t *testing.T) {
	_, definition := cw01Definition(t, cw01NumberSlot())
	answer := cw01Answer(definition, "order_period", "amount", ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}})
	replacement := CloneClarificationAnswers([]ClarificationAnswer{answer})[0]
	replacement.Value.Number.Value = "20"
	out, err := MergeClarificationAnswers([]ClarificationAnswer{answer}, []ClarificationAnswer{replacement})
	if err != nil || len(out) != 1 || out[0].Value.Number.Value != "20" || answer.Value.Number.Value != "10" {
		t.Fatal("replacement retained the stale scalar", err)
	}
	out[0].Value.Number.Value = "mutated"
	if replacement.Value.Number.Value != "20" {
		t.Fatal("returned answer aliases the submitted delta")
	}
	removal := replacement
	removal.Remove, removal.Value = true, nil
	out, err = MergeClarificationAnswers([]ClarificationAnswer{answer}, []ClarificationAnswer{removal})
	if err != nil || len(out) != 0 || answer.Value == nil {
		t.Fatal("removal retained a stale value or mutated history", err)
	}
	base := []ClarificationAnswer{answer, answer, answer, answer}
	base[0].Topic, base[1].Topic = "z-topic", "a-topic"
	base[2].Topic, base[3].Topic = "a-topic", "a-topic"
	base[1].Pattern, base[2].Pattern, base[3].Pattern = "z-pattern", "a-pattern", "a-pattern"
	base[2].Slot, base[3].Slot = "z-slot", "a-slot"
	ordered, err := MergeClarificationAnswers(base, nil)
	if err != nil || len(ordered) != 4 || ordered[0].Slot != "a-slot" || ordered[1].Slot != "z-slot" || ordered[2].Pattern != "z-pattern" || ordered[3].Topic != "z-topic" {
		t.Fatal("deterministic topic/policy/slot ordering changed", err)
	}
}

func TestClarificationProjectionProtectsValuesAndRejectsBrokenSeals(t *testing.T) {
	slot := cw01BoundaryTextSlot()
	slot.Effect.Values[0].Canonical = "cw-selected-private-721"
	slot.Effect.Values[0].Aliases = []string{"cw-alias-private-721"}
	subject, definition := cw01Definition(t, slot)
	text := "cw-alias-private-721"
	answer := cw01Answer(definition, "order_period", "region", ClarificationValue{Text: &text})
	evaluation := ResolveClarifications(cw01Compile(t, subject, definition), ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{answer}})
	if evaluation.Outcome != ClarificationSatisfied || len(evaluation.Resolutions) != 1 {
		t.Fatal("reviewed synthetic value did not resolve")
	}
	r := evaluation.Resolutions[0]
	provider, err := ProviderClarificationText(r)
	if err != nil || strings.Contains(provider, r.Value) || strings.Contains(provider, text) || strings.Contains(provider, "S-01") || !strings.Contains(provider, r.ID) {
		t.Fatal("provider projection leaked selected scalar/dictionary or lost its seal", err)
	}
	budget, err := ClarificationBudgetText(r)
	if err != nil || !strings.Contains(budget, r.Value) || strings.Contains(budget, text) || strings.Contains(budget, "S-01") || len(r.Effect.Values) != 2 {
		t.Fatal("budget omitted the selected value, retained raw aliases, or mutated the policy", err)
	}
	redacted := RedactClarificationText("Count orders for CW-ALIAS-PRIVATE-721 and cw-selected-private-721", []ClarificationAnswer{answer}, evaluation.Resolutions)
	if strings.Contains(strings.ToLower(redacted), "private-721") || strings.Count(redacted, "[redacted answer]") != 2 {
		t.Fatal("known answer spellings reached an ordinary projection")
	}
	if RedactClarificationText("unchanged question", nil, nil) != "unchanged question" {
		t.Fatal("empty redaction set changed unrelated text")
	}
	for _, project := range []func(ClarificationResolution) (string, error){ProviderClarificationText, ClarificationBudgetText} {
		for _, bad := range []ClarificationResolution{{}, {ID: r.ID, Value: "tampered"}} {
			value, err := project(bad)
			if !errors.Is(err, ErrInvalid) || value != "" {
				t.Fatal("broken resolution seal produced context", err)
			}
		}
		oversized := r
		oversized.Pattern = strings.Repeat("p", 17<<10)
		oversized.ID = ClarificationResolutionDigest(oversized)
		value, err := project(oversized)
		var typed *ValidationError
		if !errors.As(err, &typed) || typed.Code != CodeLimit || value != "" {
			t.Fatal("oversized sealed context bypassed the payload ceiling", err)
		}
	}
}
