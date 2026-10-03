package nlqroute

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
)

func TestSQLRecoveryGroundedGroupingDecisionBoundSchema(t *testing.T) {
	ids := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	schema, err := groupingIntentSchema(ids)
	if err != nil {
		t.Fatal(err)
	}
	strict, err := gateway.NewStrictSchema(schema)
	if err != nil {
		t.Fatal("strict provider projection", err)
	}
	wire, err := gateway.NewSchema("wire", strict.Document())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body string
		valid      bool
	}{
		// Exact semantic body observed in the PR70 scalar failure. Wrapping it does
		// not repair the invalid cardinality; both schemas must reject it.
		{"captured_empty_select", `{"choice":{"alternatives":[],"decision":"select","selected":[]}}`, false},
		{"old_unwrapped_body", `{"alternatives":[],"decision":"select","selected":[]}`, false},
		{"scalar", `{"choice":{"alternatives":[],"decision":"select","selected":[{"id":"` + ids[0] + `","quote":"valor bruto total"}]}}`, true},
		{"clarify", `{"choice":{"alternatives":["` + ids[0] + `","` + ids[1] + `"],"decision":"clarify","selected":[]}}`, true},
		{"no_match", `{"choice":{"alternatives":[],"decision":"no_match","selected":[]}}`, true},
		{"clarify_single", `{"choice":{"alternatives":["` + ids[0] + `"],"decision":"clarify","selected":[]}}`, false},
		{"no_match_with_selection", `{"choice":{"alternatives":[],"decision":"no_match","selected":[{"id":"` + ids[0] + `","quote":"total"}]}}`, false},
		{"select_with_alternative", `{"choice":{"alternatives":["` + ids[1] + `"],"decision":"select","selected":[{"id":"` + ids[0] + `","quote":"total"}]}}`, false},
		{"invented_id", `{"choice":{"alternatives":[],"decision":"select","selected":[{"id":"` + strings.Repeat("c", 64) + `","quote":"total"}]}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			if (schema.Validate(raw, 32<<10) == nil) != tc.valid || (wire.Validate(raw, 32<<10) == nil) != tc.valid {
				t.Fatal("domain/wire cardinality disagrees", tc.valid)
			}
			normalized, err := strict.Normalize(raw, 32<<10)
			if (err == nil) != tc.valid {
				t.Fatal("normalization bypassed cardinality", err)
			}
			if tc.valid {
				var proposal struct {
					Choice conceptchoice.Proposal `json:"choice"`
				}
				if json.Unmarshal(normalized, &proposal) != nil {
					t.Fatal("normalized shape lost")
				}
				proof, err := conceptchoice.Resolve(t.Context(), "¿Cuál es el valor bruto total?", ids, proposal.Choice)
				if err != nil {
					t.Fatal("original proof rejected valid choice", err)
				}
				// The previous producer's flat valid response remains a valid
				// canonical proposal. Persisted proof uses its existing shape,
				// without the new provider-only envelope.
				oldSchema, err := conceptSchema(ids)
				if err != nil {
					t.Fatal(err)
				}
				oldBody, _ := json.Marshal(proposal.Choice)
				if oldSchema.Validate(oldBody, 32<<10) != nil {
					t.Fatal("valid previous producer shape lost")
				}
				retained, _ := json.Marshal(proof)
				var reloaded conceptchoice.Proof
				if json.Unmarshal(retained, &reloaded) != nil || conceptchoice.Verify(t.Context(), "¿Cuál es el valor bruto total?", ids, reloaded) != nil {
					t.Fatal("previous retained proof cannot replay")
				}
			}
		})
	}
	// Cardinality projection does not replace uniqueItems in the original domain.
	duplicate := []byte(`{"choice":{"alternatives":["` + ids[0] + `","` + ids[0] + `"],"decision":"clarify","selected":[]}}`)
	if schema.Validate(duplicate, 32<<10) == nil {
		t.Fatal("domain duplicate accepted")
	}
	if _, err := strict.Normalize(duplicate, 32<<10); err == nil {
		t.Fatal("strict normalization bypassed original uniqueness")
	}
}
