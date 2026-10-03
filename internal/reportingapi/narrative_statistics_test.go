package reportingapi

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestStatisticalNarrativeTransportClosedSchema(t *testing.T) {
	schema, err := api.SchemaFor("statisticalNarrative", reflect.TypeFor[reporting.Narrative](), false, api.NullableCollections)
	if err != nil {
		t.Fatal(err)
	}
	n := reporting.Narrative{PolicyVersion: reporting.StatisticalNarrativePolicyVersion, SchemaVersion: reporting.StatisticalNarrativeSchemaVersion, Type: "explanation", Instructions: "evidence_only", Fields: []string{"amount"}, RedactedFields: []string{}, Reduction: "statistical_evidence", MaxRows: 10, MaxBytes: 8192, MaxCharacters: 4000, MaxCalls: 1, MaxTokens: 1024, TimeoutMillis: 1000, PromptVersion: "summary-v1", ModelVersion: "policy-v1", Locale: "en", Tone: "concise", RequireEvidence: true, RequireCaveats: true, MaxClaims: 3, Statistics: []reporting.NarrativeStatistic{{ID: "spread", Kind: "population_variance", Value: reporting.NarrativeFieldRef{Column: 0, Field: exec.Field{Name: "amount", Type: "decimal", Encoding: "string", NativeType: "numeric"}}}}}
	body, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	def := api.Definition{Request: schema, MaxBodyBytes: 65536}
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{{"exact", string(body), false}, {"unreviewed formula", strings.Replace(string(body), `"kind":"population_variance"`, `"kind":"population_variance","formula":"amount * 9"`, 1), true}, {"unknown kind", strings.Replace(string(body), `"kind":"population_variance"`, `"kind":"forecast"`, 1), true}, {"duplicate coordinate", strings.Replace(string(body), `"column":0`, `"column":0,"column":1`, 1), true}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			var got reporting.Narrative
			err := decode(httptest.NewRecorder(), r, def, &got)
			if (err != nil) != tc.bad {
				t.Fatalf("closed statistical schema: %v", err)
			}
		})
	}
}
