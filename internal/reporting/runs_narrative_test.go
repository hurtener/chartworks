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
