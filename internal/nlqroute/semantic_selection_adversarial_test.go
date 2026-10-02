package nlqroute

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

func TestSQLRecoveryPrivateAnswerIsNotASelectedCatalogTerm(t *testing.T) {
	p := recoveryPublication()
	secret := "Revenue"
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Find private label Revenue", Examples: []nlq.OptionalItem{{ID: "example", Text: "Label Revenue"}}, Answers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "private-label", Slot: "label", Value: &semantics.ClarificationValue{Text: &secret}}}}
	items := []admittedTopic{{id: "topic", publication: p, rules: rulesets.Published{Definition: semantics.RuleSetDefinition{Patterns: []semantics.ClarificationPattern{{ID: "private-label", Slots: []semantics.ClarificationSlot{{ID: "label", Sensitivity: semantics.LiteralSensitive}}}}}}}}
	before, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialSemanticSelection(context.Background(), in, items, nil); err != nil {
		t.Fatal(err)
	}
	if len(items[0].selection.roots) != 0 {
		t.Fatal("private scalar was interpreted as a metric")
	}
	safe := selectionFailureRequest(in, items)
	raw, err := json.Marshal(safe)
	if err != nil || strings.Contains(strings.ToLower(string(raw)), strings.ToLower(secret)) || len(safe.Answers)+len(safe.Choices) != 0 {
		t.Fatal("selection failure retained private answer text")
	}
	after, err := json.Marshal(in)
	if err != nil || string(before) != string(after) {
		t.Fatal("private selection redaction mutated caller input")
	}
}

func TestSQLRecoveryChoiceRootsMustBelongToRuleFacts(t *testing.T) {
	p := recoveryPublication()
	rules := selectionPolicy(t, p, nil, nil)
	metric := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	input := semantics.ClarificationInput{Locale: "en", Question: "Gross margin percentage", References: []semantics.Reference{metric}, Selection: &semantics.ClarificationReferenceSelection{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}}}
	result := semantics.ResolveClarifications(rules.model, input)
	if result.Outcome != semantics.ClarificationInvalid || len(result.Errors) == 0 {
		t.Fatal("a choice root outside the admitted rule facts was accepted")
	}
	input.Selection.References = []semantics.Reference{metric}
	result = semantics.ResolveClarifications(rules.model, input)
	if result.Outcome == semantics.ClarificationInvalid || result.Outcome == semantics.ClarificationConflicting {
		t.Fatal("valid selected root subset was rejected")
	}
}

func TestSQLRecoveryRequiredDependencyIsNotAClarificationAnswer(t *testing.T) {
	p := recoveryPublication()
	metric := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	cost := semantics.Reference{Kind: semantics.KindMeasure, ID: "costs"}
	pattern := semantics.ClarificationPattern{ID: "metric", Version: "v1", Targets: []semantics.Reference{metric, cost}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "selection-fixture"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"metric"}}, Why: "Choose the metric to inspect."}, Slots: []semantics.ClarificationSlot{{ID: "choice", Kind: semantics.SlotChoice, Required: true, Sensitivity: semantics.LiteralNonSensitive, Prompt: "Which metric?", Choices: []semantics.ClarificationChoice{{ID: "revenue-option", Label: "Revenue", Target: &metric}, {ID: "cost-option", Label: "Cost", Target: &cost}}}}}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	service.rules = selectionPolicy(t, p, []semantics.RuleDefinition{selectionRule("required-revenue", semantics.RuleScope{Kind: semantics.RuleScopeTopic}, metric)}, []semantics.ClarificationPattern{pattern})
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Choose metric"}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "required_answers" || engine.embeds != 0 {
		t.Fatal("a rule dependency answered an omitted user choice", err, out.Clarification)
	}
	in.MetricIDs = []string{"revenue"}
	out, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Context == nil || out.Clarification != nil || len(out.Resolutions) != 1 || out.Resolutions[0].Provenance != "typed_interpretation" {
		t.Fatal("explicitly selected metric failed to resolve the choice", err, out.Clarification)
	}
}
