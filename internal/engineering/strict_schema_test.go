package engineering

import (
	"github.com/hurtener/chartworks/internal/gateway"
	"testing"
)

func TestActualEngineeringStrictProviderSchemas(t *testing.T) {
	profile, err := profileSummarySchema()
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := pipelineProposalSchema()
	if err != nil {
		t.Fatal(err)
	}
	blind, err := gateway.NewSchema("blind", []byte(blindPlanSchema))
	if err != nil {
		t.Fatal(err)
	}
	matched, err := gateway.NewSchema("matched", []byte(matchedProposalSchema))
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []*gateway.Schema{profile, pipeline, blind, matched} {
		if _, err := gateway.NewStrictSchema(schema); err != nil {
			t.Fatal(schema.Name(), err)
		}
	}
	p, err := gateway.NewStrictSchema(matched)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Normalize([]byte(`{"sql":"SELECT 1","columns":[{"name":"id","type":"bigint","primary_key":true}],"rationale":"Synthetic","evidence":["same","same"],"alternatives":[{"dataset":"alternate","rationale":"Synthetic"}]}`), 65536); err == nil {
		t.Fatal("duplicate engineering evidence accepted")
	}
}
