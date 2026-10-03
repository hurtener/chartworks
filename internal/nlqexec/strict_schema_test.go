package nlqexec

import (
	"github.com/hurtener/chartworks/internal/gateway"
	"testing"
)

func TestActualSQLStrictProviderSchema(t *testing.T) {
	if generationSchemaErr != nil {
		t.Fatal(generationSchemaErr)
	}
	projection, err := gateway.NewStrictSchema(generationSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"sqlgen", "sqlfix", "clarify"} {
		t.Run(role, func(t *testing.T) {
			good := []byte(`{"decision":"ready","questions":[],"sql":"SELECT 1","parameters":[],"assumptions":[],"ambiguities":[]}`)
			if _, err := projection.Normalize(good, 65536); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{`{"decision":"ready","questions":[],"parameters":[],"assumptions":[],"ambiguities":[]}`, `{"decision":"ready","questions":[],"sql":null,"parameters":[],"assumptions":[],"ambiguities":[]}`} {
				if _, err := projection.Normalize([]byte(bad), 65536); err == nil {
					t.Fatal("required SQL field lost")
				}
			}
		})
	}
}
