package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"testing"
)

func TestSQLRecoveryReviewedNullPolicyCompiler(t *testing.T) {
	a := analyticalAdmission()
	a.publications[0].Definition.KPIs[0].Expression = "coalesce(revenue,0)"
	a.publications[0].Definition.KPIs[0].Inputs = []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}
	analyticalReseal(&a)
	c, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || c == nil || c.Metrics[0].Expression.Op != "coalesce" || c.Metrics[0].Expression.Args[1].Value != "0" {
		t.Fatal("reviewed fallback", err)
	}
	if c.Version != exec.AnalyticalGroupedProgramsVersion || c.Intent == nil || c.Intent.Order != nil || c.Intent.Limit != 0 || c.QueryPopulation == nil || len(c.QueryPopulation.Constraints) != 0 {
		t.Fatal("missing default non-narrowing intent")
	}
	if _, err := compileAnalyticalVersion(context.Background(), a, 6); err == nil {
		t.Fatal("retained v6 grammar widened")
	}
}
func TestSQLRecoveryDefaultIntentPreservesV6Replay(t *testing.T) {
	a := grainAdmission("Revenue")
	old, err := compileAnalyticalVersion(context.Background(), a, 6)
	if err != nil || old.Intent != nil || old.QueryPopulation != nil {
		t.Fatal("v6 upgraded", err)
	}
	current, err := compileCurrentAnalytical(context.Background(), a)
	if err != nil || current.Intent == nil || current.QueryPopulation == nil {
		t.Fatal("v7 absent intent not measured", err)
	}
}
