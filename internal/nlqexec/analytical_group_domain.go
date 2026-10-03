package nlqexec

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/semantics"
)

// compileAnalyticalGroupDomain never creates a group-domain preference from
// SQL, examples or source samples. Every contributing topic must explicitly
// agree when all selected aggregate leaves have filtered populations.
func compileAnalyticalGroupDomain(a admission, c *exec.AnalyticalContract) error {
	if c.Version != exec.AnalyticalGroupedProgramsVersion || c.GroupedPopulations != nil || len(c.Populations) != 0 || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0 {
		return nil
	}
	unfiltered := false
	var visit func(exec.AnalyticalExpression)
	visit = func(e exec.AnalyticalExpression) {
		if e.Column != "" {
			unfiltered = unfiltered || len(e.Filters) == 0
		}
		for _, arg := range e.Args {
			visit(arg)
		}
	}
	for _, metric := range c.Metrics {
		visit(metric.Expression)
	}
	domain := ""
	missing, count := false, 0
	for _, selected := range a.route.Selection.Topics {
		contributes := false
		for _, root := range selected.Roots {
			contributes = contributes || root.Reason != "required_rule" && (root.Reference.Kind == semantics.KindMeasure || root.Reference.Kind == semantics.KindKPI)
		}
		if !contributes {
			continue
		}
		for _, publication := range a.publications {
			if publication.Definition.Topic != selected.Topic {
				continue
			}
			count++
			policy := publication.Definition.GroupDomain
			if policy == nil {
				missing = true
				continue
			}
			if policy.Policy != semantics.MetricGroupDomainPolicy || policy.Domain != exec.AnalyticalGroupDomainRaw && policy.Domain != exec.AnalyticalGroupDomainQualifying {
				return exec.ErrBinding
			}
			if domain != "" && domain != policy.Domain && !unfiltered {
				return analyticalUnsupported(exec.AnalyticalGroupDomainReviewCode)
			}
			domain = policy.Domain
		}
	}
	if count == 0 {
		return exec.ErrBinding
	}
	if unfiltered {
		// Union with a TRUE leaf is TRUE. Canonicalizing this equivalence neither
		// fills missing groups nor chooses a domain for filtered-only metrics.
		domain = exec.AnalyticalGroupDomainRaw
	} else if missing || domain == "" {
		return analyticalUnsupported(exec.AnalyticalGroupDomainReviewCode)
	}
	c.GroupDomain = &exec.AnalyticalGroupDomain{Policy: exec.AnalyticalGroupDomainPolicy, Domain: domain}
	return nil
}

func analyticalGroupDomainGuidance(c *exec.AnalyticalContract) string {
	if c == nil || c.GroupDomain == nil {
		return ""
	}
	placement := "Keep all metric population filters inside their own FILTER/CASE aggregates, with no metric population WHERE; groups come from the complete admitted source/join rows. Nonqualifying-only groups remain present with normal empty-aggregate results."
	if c.GroupDomain.Domain == exec.AnalyticalGroupDomainQualifying {
		placement = "The metric population WHERE must be exactly the union (OR) of the complete reviewed filter populations of every selected aggregate leaf. Each aggregate must still use its own reviewed population; a shared predicate may be enforced in WHERE. Nonqualifying-only groups must not appear."
	}
	return " Reviewed ordinary group-domain policy " + c.GroupDomain.Policy + ": " + c.GroupDomain.Domain + ". " + placement + " Group existence is determined before aggregate NULL treatment: qualifying all-NULL inputs still form a group. Do not relocate predicates between WHERE and FILTER/CASE in a way that changes this domain. Query-owned predicates remain separate and are bound and verified by the service; do not inline or invent them."
}

// A fresh missing policy is reviewed semantic context, not a SQL repair or a
// retained-v7 intent confirmation. Questions are fixed and contain no values.
func ordinaryGroupDomainReview(locale nlq.Language) error {
	questions := []string{"Should grouping include every admitted source group, or only groups containing rows that qualify for the selected metrics?", "Ask the topic owner to review and publish group_domain (or per-fact group_domains for independent populations), then submit a new question."}
	if locale == nlq.LanguageSpanish {
		questions = []string{"¿La agrupación debe incluir todos los grupos de la fuente admitida, o solo los grupos con filas que cumplen los filtros de las métricas seleccionadas?", "Pide al responsable del tema que revise y publique group_domain (o group_domains por hecho para poblaciones independientes), y después envía una consulta nueva."}
	}
	return &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Insufficient, Questions: questions}}
}
