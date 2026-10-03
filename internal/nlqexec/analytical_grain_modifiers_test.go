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

func TestSQLRecoveryGrainUnicodeLiteralAndNegation(t *testing.T) {
	for _, question := range []string{
		"Revenue for “by Region”", "Revenue for ‘by Region’", "Revenue for «by Region»",
		"Revenue by “Region”", "Ingresos para «por región»",
		"Ingresos no agrupados por región", "Ingresos no agrupada por región", "Ingresos no agrupes por región",
	} {
		t.Run(question, func(t *testing.T) {
			c, err := compileAnalytical(context.Background(), grainAdmission(question))
			if err != nil || c == nil || c.Grain != nil {
				t.Fatal("quoted or negated phrase acquired grouping proof", err)
			}
		})
	}
	c, err := compileAnalytical(context.Background(), grainAdmission("Revenue for «by Order» by Region"))
	if err != nil || c == nil || c.Grain == nil || len(c.Grain.Columns) != 1 || c.Grain.Columns[0] != "region_native" {
		t.Fatal("masking a quoted value erased the affirmative grouping", err)
	}
}
