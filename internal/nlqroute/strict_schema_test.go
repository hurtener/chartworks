package nlqroute

import (
	"github.com/hurtener/chartworks/internal/gateway"
	"testing"
)

func TestActualConceptStrictProviderSchema(t *testing.T) {
	schema, err := conceptSchema([]string{"candidate-a", "candidate-b"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := gateway.NewStrictSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Normalize([]byte(`{"decision":"clarify","selected":[],"alternatives":["candidate-a","candidate-b"]}`), 4096); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Normalize([]byte(`{"decision":"clarify","selected":[],"alternatives":["candidate-a","candidate-a"]}`), 4096); err == nil {
		t.Fatal("duplicate alternative accepted")
	}
}
