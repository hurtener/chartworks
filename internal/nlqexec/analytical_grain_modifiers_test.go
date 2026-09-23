package nlqexec

import (
	"context"
	"testing"
)

func TestSQLRecoveryGrainExplicitFilterIsNotGrouping(t *testing.T) {
	for _, question := range []string{
		"Revenue filtered by Region", "Revenue filter by Region", "Revenue filtering by Region",
		"Ingresos filtrados por región", "Ingresos filtradas por región", "Ingresos filtrado por región", "Ingresos filtrada por región", "Ingresos filtrar por región",
	} {
		t.Run(question, func(t *testing.T) {
			c, err := compileAnalytical(context.Background(), grainAdmission(question))
			if err != nil || c == nil || c.Grain != nil {
				t.Fatal("filter phrase acquired grouping proof", err)
			}
		})
	}
}
