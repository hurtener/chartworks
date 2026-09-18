package semantics

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestClarificationRedactionMatchesAcceptedWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name, answer, question string
	}{
		{"padded", "  north coast  ", "Count orders for north coast"},
		{"collapsed", "north   coast", "Count orders for north coast"},
		{"question-tabs", "north coast", "Count orders for NORTH\tCOAST"},
		{"unicode-space", "north\u00a0coast", "Count orders for north\u2003coast"},
		{"leading-tab", "\tnorth coast\t", "Count orders for north coast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot := cw01BoundaryTextSlot()
			slot.Effect.Values[0].Aliases = []string{"north coast"}
			subject, definition := cw01Definition(t, slot)
			answer := cw01Answer(definition, "order_period", "region", ClarificationValue{Text: &tc.answer})
			input := ClarificationInput{Locale: "en", Question: "Count orders", Answers: []ClarificationAnswer{answer}}
			evaluation := ResolveClarifications(cw01Compile(t, subject, definition), input)
			if evaluation.Outcome != ClarificationSatisfied || len(evaluation.Resolutions) != 1 {
				t.Fatal("reviewed whitespace-equivalent input did not resolve")
			}
			beforeAnswers := CloneClarificationAnswers(input.Answers)
			beforeResolutions := CloneClarificationResolutions(evaluation.Resolutions)
			got := RedactClarificationText(tc.question, input.Answers, evaluation.Resolutions)
			if got != "Count orders for [redacted answer]" {
				t.Fatal("accepted sensitive spelling was not redacted")
			}
			if !reflect.DeepEqual(input.Answers, beforeAnswers) || !reflect.DeepEqual(evaluation.Resolutions, beforeResolutions) {
				t.Fatal("redaction mutated protected answer evidence")
			}
		})
	}
}

func TestClarificationRedactionKeepsLiteralMatchingAndLongestPhrase(t *testing.T) {
	r := []ClarificationResolution{
		{Sensitivity: LiteralSensitive, Value: "north          coast"},
		{Sensitivity: LiteralSensitive, Value: "north coast team"},
		{Sensitivity: LiteralSensitive, Value: "unit [x].*"},
	}
	got := RedactClarificationText("north coast team; unit [x].*; unit x; public text", nil, r)
	if got != "[redacted answer]; [redacted answer]; unit x; public text" {
		t.Fatal("normalized matching leaked a suffix or treated literal input as an expression")
	}
	if strings.Contains(got, "team") {
		t.Fatal("shorter whitespace-padded value won over the full known phrase")
	}
	r[0].Sensitivity, r[1].Sensitivity, r[2].Sensitivity = LiteralNonSensitive, LiteralNonSensitive, LiteralNonSensitive
	plain := "north coast team; unit [x].*"
	if RedactClarificationText(plain, nil, r) != plain {
		t.Fatal("nonsensitive values were redacted")
	}
}

func TestClarificationRedactionUnicodeWhitespaceAlphabet(t *testing.T) {
	resolution := []ClarificationResolution{{Sensitivity: LiteralSensitive, Value: "north coast"}}
	for _, separator := range []rune{' ', '\t', '\n', '\v', '\f', '\r', '\u0085', '\u00a0', '\u1680', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000'} {
		t.Run(fmt.Sprintf("U+%04X", separator), func(t *testing.T) {
			if !unicode.IsSpace(separator) {
				t.Fatal("test separator is not recognized by the answer normalizer")
			}
			if RedactClarificationText("north"+string(separator)+"coast", nil, resolution) != "[redacted answer]" {
				t.Fatal("a resolver-equivalent whitespace spelling escaped redaction")
			}
		})
	}
	for _, separator := range []rune{'-', '.', '\u180e', '\u200b'} {
		text := "north" + string(separator) + "coast"
		if RedactClarificationText(text, nil, resolution) != text {
			t.Fatal("redaction interpreted punctuation or a non-whitespace rune as an alias")
		}
	}
}

func FuzzClarificationRedactionWhitespace(f *testing.F) {
	f.Add("north", "coast", uint8(0), uint8(1))
	f.Add("unit[x]", ".*", uint8(2), uint8(3))
	f.Add("región", "norte", uint8(4), uint8(0))
	f.Fuzz(func(t *testing.T, first, second string, a, b uint8) {
		for _, word := range []string{first, second} {
			if !validClarificationText(word, 32) || len(strings.Fields(word)) != 1 || strings.TrimSpace(word) != word {
				return
			}
		}
		spaces := []string{" ", "\t", "\u00a0", "\u2003", "   "}
		raw := " " + first + spaces[int(a)%len(spaces)] + second + " "
		answer := ClarificationAnswer{Topic: "topic", Pattern: "policy", Slot: "field", Value: &ClarificationValue{Text: &raw}}
		resolution := ClarificationResolution{Topic: "topic", Pattern: "policy", Slot: "field", Sensitivity: LiteralSensitive}
		question := first + spaces[int(b)%len(spaces)] + second
		redacted := RedactClarificationText(question, []ClarificationAnswer{answer}, []ClarificationResolution{resolution})
		if redacted != "[redacted answer]" {
			t.Fatal("known normalized sensitive spelling was not redacted")
		}
		if RedactClarificationText(redacted, []ClarificationAnswer{answer}, []ClarificationResolution{resolution}) != redacted {
			t.Fatal("redacting the same evidence again changed the public marker")
		}
	})
}

func TestClarificationRedactionIsStableOnReplay(t *testing.T) {
	for _, value := range []string{"red", "answer", "[red", "["} {
		t.Run(value, func(t *testing.T) {
			resolutions := []ClarificationResolution{{Sensitivity: LiteralSensitive, Value: value}}
			first := RedactClarificationText("Customer: "+value, nil, resolutions)
			if first != "Customer: [redacted answer]" {
				t.Fatal("known value was not redacted")
			}
			if RedactClarificationText(first, nil, resolutions) != first {
				t.Fatal("replaying canonical evidence changed the redacted question")
			}
		})
	}
	// Reserving the marker must not leave the suffix of a longer known value.
	resolutions := []ClarificationResolution{{Sensitivity: LiteralSensitive, Value: "[redacted answer] private suffix"}}
	if RedactClarificationText("Customer: [redacted answer] private suffix", nil, resolutions) != "Customer: [redacted answer]" {
		t.Fatal("marker preservation exposed the suffix of a longer sensitive value")
	}
}
