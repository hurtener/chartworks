package reporting

import (
	"github.com/hurtener/chartworks/internal/gateway"
	"testing"
)

func TestActualNarrativeStrictProviderSchemas(t *testing.T) {
	for name, raw := range map[string]string{"grounded": narrativeSchema, "statistics": statisticalClaimSchema(Narrative{MaxClaims: 3, Statistics: []NarrativeStatistic{{Kind: "trend"}, {Kind: "extrema"}}})} {
		schema, err := gateway.NewSchema(name, []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		p, err := gateway.NewStrictSchema(schema)
		if err != nil {
			t.Fatal(err)
		}
		if name == "grounded" {
			if _, err = p.Normalize([]byte(`{"claims":[{"kind":"difference","evidence":["same","same"]}]}`), 4096); err == nil {
				t.Fatal("duplicate narrative evidence accepted")
			}
		}
	}
}
