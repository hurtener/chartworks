package reporting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestNarrativeExactScientificEvidence(t *testing.T) {
	spec := contractNarrative()
	spec.Fields = []string{"n"}
	spec.Reduction = "aggregate_evidence"
	result := exec.Result{Schema: []exec.Field{{Name: "n", Type: "float"}}, Rows: [][]json.RawMessage{{json.RawMessage(`1e-9`)}, {json.RawMessage(`2e-9`)}}, Outcome: "succeeded"}
	evidence, _, err := narrativeEvidence(result, spec)
	if err != nil || len(evidence) != 2 || evidence[0].Value != "0.000000003" {
		t.Fatal("small scientific values rounded", evidence, err)
	}
	spec.Reduction = "first_rows"
	evidence, _, err = narrativeEvidence(result, spec)
	if err != nil {
		t.Fatal(err)
	}
	text, err := groundedText(NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "difference", Evidence: []string{"e2", "e1"}}}}, evidence, spec)
	if err != nil || !strings.Contains(text, "= 0.000000001") {
		t.Fatal("scientific subtraction rounded", text, err)
	}
	for _, value := range []string{"1e999999999", "1e-999999999", "1/3", "NaN", "Inf", ".", "1e2e3", strings.Repeat("1", 4097)} {
		if _, _, ok := narrativeNumber(value); ok {
			t.Fatal("unbounded or nondecimal evidence accepted", value[:min(len(value), 20)])
		}
	}
}

func TestNarrativeReductionRedactionAndGrounding(t *testing.T) {
	spec := contractNarrative()
	spec.Fields = []string{"n"}
	spec.RedactedFields = []string{"secret"}
	spec.MaxRows = 1
	result := exec.Result{Schema: []exec.Field{{Name: "n", Type: "decimal"}, {Name: "secret", Type: "string"}}, Rows: [][]json.RawMessage{{json.RawMessage(`"1.250"`), json.RawMessage(`"PRIVATE_CANARY"`)}, {json.RawMessage(`"2.500"`), json.RawMessage(`"PRIVATE_CANARY"`)}}, Outcome: "truncated"}
	evidence, caveats, err := narrativeEvidence(result, spec)
	encoded, _ := json.Marshal(evidence)
	if err != nil || len(evidence) != 1 || strings.Contains(string(encoded), "PRIVATE_CANARY") || len(caveats) != 4 {
		t.Fatal("redaction or reduction caveats lost", evidence, caveats, err)
	}
	spec.Locale = "es-AR"
	text, err := groundedText(NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "value", Evidence: []string{"e1"}}}}, evidence, spec)
	if err != nil || !strings.Contains(text, "observación retenida") || !strings.Contains(text, "1.250") {
		t.Fatal("localized exact evidence lost", text, err)
	}
	for _, claims := range [][]NarrativeClaim{
		{}, {{Kind: "value", Evidence: []string{"invented"}}}, {{Kind: "cause", Evidence: []string{"e1"}}},
		{{Kind: "value", Evidence: []string{"e1", "e1"}}}, {{Kind: "difference", Evidence: []string{"e1", "e1"}}},
		{{Kind: "value", Evidence: []string{"e1"}}, {Kind: "value", Evidence: []string{"e1"}}},
	} {
		if _, err := groundedText(NarrativeAnswer{Claims: claims}, evidence, spec); err == nil {
			t.Fatal("ungrounded or duplicate claim accepted", claims)
		}
	}
	spec.MaxCharacters = 1
	if _, err := groundedText(NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "value", Evidence: []string{"e1"}}}}, evidence, spec); err == nil {
		t.Fatal("character budget bypassed")
	}
	spec.MaxCharacters = 1024
	spec.MaxBytes = 2
	if _, _, err := narrativeEvidence(result, spec); err == nil {
		t.Fatal("empty evidence after byte reduction accepted")
	}
	spec.MaxBytes = 4096
	spec.Fields = []string{"secret"}
	if _, _, err := narrativeEvidence(result, spec); err == nil {
		t.Fatal("redacted-only evidence accepted")
	}
	spec.Fields = []string{"n"}
	spec.Reduction = "aggregate_evidence"
	result.Rows = [][]json.RawMessage{{json.RawMessage(`null`), json.RawMessage(`"secret"`)}}
	if _, _, err := narrativeEvidence(result, spec); err == nil {
		t.Fatal("all-null aggregate invented a zero")
	}
	result.Rows = [][]json.RawMessage{{json.RawMessage(`"1e999999999"`), json.RawMessage(`"secret"`)}}
	if _, _, err := narrativeEvidence(result, spec); err == nil {
		t.Fatal("unbounded numeric evidence accepted")
	}
}

func TestNarrativeRejectsInconsistentRetainedRows(t *testing.T) {
	spec := contractNarrative()
	spec.Fields = []string{"n"}
	result := exec.Result{Schema: []exec.Field{{Name: "n", Type: "decimal"}}, Rows: [][]json.RawMessage{{}}, Outcome: "succeeded"}
	for _, reduction := range []string{"first_rows", "aggregate_evidence"} {
		spec.Reduction = reduction
		if _, _, err := narrativeEvidence(result, spec); err == nil {
			t.Fatalf("%s accepted a row inconsistent with its sealed schema", reduction)
		}
	}
}
