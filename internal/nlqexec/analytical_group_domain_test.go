package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestSQLRecoveryOrdinaryGroupDomainCompiler(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	a.publications[0].Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"A"}}}
	analyticalReseal(&a)
	var detail *exec.AnalyticalError
	if _, err := compileAnalytical(context.Background(), a); !errors.As(err, &detail) || detail.Code != exec.AnalyticalGroupDomainReviewCode {
		t.Fatal("absent filtered domain invented", err)
	}
	for _, version := range []int{6, 7} {
		c, err := compileAnalyticalVersion(context.Background(), a, version)
		if err != nil || c.GroupDomain != nil {
			t.Fatal("retained compiler changed", version, err)
		}
	}
	var previous string
	for _, domain := range []string{exec.AnalyticalGroupDomainRaw, exec.AnalyticalGroupDomainQualifying} {
		a.publications[0].Definition.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: domain}
		analyticalReseal(&a)
		c, err := compileAnalytical(context.Background(), a)
		if err != nil || c.GroupDomain == nil || c.GroupDomain.Domain != domain {
			t.Fatal("reviewed policy lost", err)
		}
		if !strings.Contains(analyticalGrainGuidance(c), domain) || strings.Contains(analyticalPopulationGuidance(c), "only a reviewed filter shared by every selected metric may be moved") {
			t.Fatal("contradictory guidance")
		}
		if previous == exec.Hash(c) {
			t.Fatal("domain omitted from proof digest")
		}
		previous = exec.Hash(c)
	}
	a = grainAdmission("Revenue by Region")
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || c.GroupDomain == nil || c.GroupDomain.Domain != exec.AnalyticalGroupDomainRaw {
		t.Fatal("unfiltered equivalence not canonical", err)
	}
	a.route.Request.Question = "Revenue"
	analyticalReseal(&a)
	c, err = compileAnalytical(context.Background(), a)
	if err != nil || c.GroupDomain != nil {
		t.Fatal("scalar acquired group policy", err)
	}
}

func TestSQLRecoveryOrdinaryGroupDomainConsensus(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	a.publications[0].Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"A"}}}
	a.publications[0].Definition.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: exec.AnalyticalGroupDomainQualifying}
	analyticalReseal(&a)
	c, err := compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	second := a.publications[0]
	second.Definition.Topic = "second_topic"
	pick := a.route.Selection.Topics[0]
	pick.Topic = second.Definition.Topic
	a.publications = append([]topics.Published{a.publications[0]}, second)
	a.route.Selection.Topics = append([]nlqroute.SelectedTopic{a.route.Selection.Topics[0]}, pick)
	for _, policy := range []*semantics.GroupDomainPolicy{nil, {Policy: semantics.MetricGroupDomainPolicy, Domain: exec.AnalyticalGroupDomainRaw}} {
		a.publications[1].Definition.GroupDomain = policy
		if err := compileAnalyticalGroupDomain(a, c); err == nil {
			t.Fatal("missing/disagreeing contributing topic inferred")
		}
	}
	a.publications[1].Definition.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: exec.AnalyticalGroupDomainQualifying}
	if err := compileAnalyticalGroupDomain(a, c); err != nil {
		t.Fatal("exact reviewed consensus", err)
	}
}
