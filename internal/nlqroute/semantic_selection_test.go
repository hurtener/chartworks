package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// The reader uses the actual pure rule compiler/evaluator, not a fake Allowed
// boolean. Only publication/source I/O and learned retrieval are fixture seams.
type selectionRules struct {
	testRules
	subject semantics.RuleSubject
	model   semantics.RuleModel
	inputs  [][]semantics.Reference
}

func (r *selectionRules) Evaluate(ctx context.Context, e identity.Envelope, topic string, in rulesets.EvaluateRequest) (rulesets.Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return rulesets.Evaluation{}, err
	}
	r.inputs = append(r.inputs, append([]semantics.Reference(nil), in.References...))
	template := ""
	if in.Template != nil {
		template = in.Template.ID
	}
	result, err := semantics.EvaluateSelectedConstraints(r.subject, r.model, semantics.RuleSelectionInput{References: in.References, Template: template})
	p := r.published
	return rulesets.Evaluation{Topic: topic, TopicVersion: p.Definition.TopicVersion, RuleVersion: p.State.Version, PackDigest: p.Definition.PackDigest, RuleDigest: p.Digest, Result: result}, err
}

func selectionPolicy(t *testing.T, p topics.Published, rules []semantics.RuleDefinition, patterns []semantics.ClarificationPattern) *selectionRules {
	t.Helper()
	d := p.Definition
	pack := semantics.TopicPack{SchemaVersion: 1, Topic: d.Topic, Version: d.Version, Measures: d.Measures, KPIs: d.KPIs, Dimensions: d.Dimensions, Joins: d.Joins, CanonicalEntities: d.CanonicalEntities}
	for _, dataset := range d.Datasets {
		pack.Datasets = append(pack.Datasets, semantics.Dataset{ID: dataset.ID, Columns: dataset.Columns})
	}
	subject, err := semantics.NewRuleSubject(pack, p.Digest)
	if err != nil {
		t.Fatal(err)
	}
	definition := semantics.RuleSetDefinition{SchemaVersion: 1, ID: "selection-rules", Version: "r1", Topic: d.Topic, TopicVersion: d.Version, PackDigest: p.Digest, Rules: rules, Patterns: patterns}
	model, err := semantics.CompilePublishedRules(subject, definition)
	if err != nil {
		t.Fatal(err)
	}
	return &selectionRules{testRules: testRules{published: rulesets.Published{State: rulesets.State{Topic: d.Topic, Version: "r1", Revision: 1, Active: true}, Definition: model.Definition(), Digest: model.Digest()}}, subject: subject, model: model}
}

func selectionRule(id string, scope semantics.RuleScope, target semantics.Reference) semantics.RuleDefinition {
	return semantics.RuleDefinition{ID: id, Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: scope, Priority: 10, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-selection-review"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: target}}
}

func TestSQLRecoverySelectedQuestionPinsMetricDimensionAndJoin(t *testing.T) {
	for _, input := range []struct {
		locale   nlq.Language
		question string
	}{{nlq.LanguageEnglish, "Gross margin percentage by product family"}, {nlq.LanguageSpanish, "Porcentaje de margen bruto por familia de producto"}} {
		t.Run(string(input.locale), func(t *testing.T) {
			p := recoveryPublication()
			p.Definition.KPIs[1].Aliases = []string{"porcentaje de margen bruto"}
			p.Definition.Dimensions[0].Aliases = []string{"familia de producto"}
			hit := recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1])
			service, _ := recoveryRouteService(t, p, hit)
			in := RouteRequest{Topic: "topic", Context: "ctx", Locale: input.locale, Question: input.question}
			before, _ := json.Marshal(in)
			out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.Context == nil || out.Selection == nil {
				t.Fatalf("selected route: %v / %#v", err, out.Clarification)
			}
			if len(out.Context.Metrics) != 1 || out.Context.Metrics[0].ID != "topic:margin_pct" || len(out.Selection.Topics[0].Roots) != 2 {
				t.Fatal("longest selected KPI/grouping not pinned")
			}
			for _, required := range []string{"sales_products", "family_name", "sales_cost", "100 * margin"} {
				if !strings.Contains(out.Context.Prompt, required) {
					t.Fatalf("missing mandatory dependency %s", required)
				}
			}
			if len(in.MetricIDs) != 0 || len(out.Request.MetricIDs) != 0 {
				t.Fatal("inference rewrote caller selections")
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("selection mutated request")
			}
			// Remove all optional evidence: the complete answer dependencies must remain.
			assembled, _ := out.GenerationContext()
			mandatory, err := service.assembler.Assemble(context.Background(), nlq.ContextInput{Locale: assembled.Locale, Strategy: assembled.Strategy, Topic: assembled.Topic, TopicVersion: assembled.TopicVersion, Topics: assembled.Topics, Question: assembled.Question, Relations: assembled.Relations, Metrics: assembled.Metrics, Constraints: assembled.Constraints}, nlq.TierHigh)
			if err != nil || !strings.Contains(mandatory.Prompt, "sales_products") || !strings.Contains(mandatory.Prompt, "100 * margin") {
				t.Fatal("selected semantics depended on optional retrieval", err)
			}
		})
	}
}

func TestSQLRecoverySelectedFactsActivateCompoundRules(t *testing.T) {
	p := recoveryPublication()
	metric := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	family := semantics.Reference{Kind: semantics.KindDimension, ID: "family"}
	rules := selectionPolicy(t, p, []semantics.RuleDefinition{selectionRule("require-family", semantics.RuleScope{Kind: semantics.RuleScopeEntities, Targets: []semantics.Reference{metric, family}}, family)}, nil)
	service, _ := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	service.rules = rules
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage"})
	if err != nil || out.Context == nil || out.Selection == nil {
		t.Fatal("required-reference expansion failed", err, out.Clarification)
	}
	found := false
	for _, root := range out.Selection.Topics[0].Roots {
		found = found || root.Reference == family && root.Reason == "required_rule"
	}
	if !found || len(rules.inputs) < 2 || !strings.Contains(out.Context.Prompt, "sales_products") {
		t.Fatal("required root was not expanded to a joined mandatory graph")
	}
	// A compound rule may depend on a KPI ingredient and its grouping dimension.
	compound := selectionRule("compound", semantics.RuleScope{Kind: semantics.RuleScopeCompound, Targets: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, family}}, metric)
	compound.Class = semantics.RuleAdvisoryContext
	compound.Constraint = nil
	compound.Guidance = &semantics.AdvisoryGuidance{Text: "Apply reviewed family comparison guidance", Sensitivity: semantics.LiteralNonSensitive}
	service.rules = selectionPolicy(t, p, []semantics.RuleDefinition{compound}, nil)
	out, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage by product family"})
	if err != nil || out.Context == nil || len(out.Context.Advisory) != 1 {
		t.Fatal("compound rule did not see the selected closure", err)
	}
}

func TestSQLRecoverySelectedMetricActivatesClarificationBeforeModel(t *testing.T) {
	p := recoveryPublication()
	metric := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	family := semantics.Reference{Kind: semantics.KindDimension, ID: "family"}
	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	pattern := semantics.ClarificationPattern{ID: "grouping", Version: "v1", Targets: []semantics.Reference{metric, family, revenue}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "selection-fixture"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyReferences: []semantics.Reference{metric}}, Why: "Choose a grouping for the selected KPI."}, Slots: []semantics.ClarificationSlot{{ID: "group", Kind: semantics.SlotChoice, Required: true, Sensitivity: semantics.LiteralNonSensitive, Prompt: "Which grouping?", Choices: []semantics.ClarificationChoice{{ID: "family-option", Label: "Family", Target: &family}, {ID: "revenue-option", Label: "Revenue", Target: &revenue}}}}}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	service.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{pattern})
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage"}
	pending, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || pending.Clarification == nil || pending.Outcome != nlq.StrategyClarify || engine.embeds != 0 {
		t.Fatal("metric-scoped clarification reached model work", err)
	}
	in.AnswerContext = pending.AnswerContext
	in.Answers = []semantics.ClarificationAnswer{{Topic: "topic", TopicVersion: "v1", RulesetVersion: "r1", Pattern: "grouping", PatternVersion: "v1", Slot: "group", Value: &semantics.ClarificationValue{OptionID: "family-option"}}}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Context == nil || len(out.Resolutions) != 1 || !strings.Contains(out.Context.Prompt, "sales_products") {
		t.Fatal("clarified grouping did not join the selected graph", err, out.Clarification)
	}
	if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil {
		t.Fatal("selected clarification did not replay", err)
	}
}

func TestSQLRecoverySelectionDoesNotAnswerWithMetricIngredients(t *testing.T) {
	p := recoveryPublication()
	metric := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	cost := semantics.Reference{Kind: semantics.KindMeasure, ID: "costs"}
	pattern := semantics.ClarificationPattern{ID: "measure", Version: "v1", Targets: []semantics.Reference{metric, revenue, cost}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "selection-fixture"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyReferences: []semantics.Reference{metric}}, Why: "Select one ingredient to inspect."}, Slots: []semantics.ClarificationSlot{{ID: "ingredient", Kind: semantics.SlotChoice, Required: true, Sensitivity: semantics.LiteralNonSensitive, Prompt: "Which ingredient?", Choices: []semantics.ClarificationChoice{{ID: "revenue-option", Label: "Revenue", Target: &revenue}, {ID: "cost-option", Label: "Cost", Target: &cost}}}}}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	service.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{pattern})
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage"})
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "required_answers" || engine.embeds != 0 {
		t.Fatal("constituents were mistaken for user choices", err, out.Clarification)
	}
}

func TestSQLRecoverySelectionRejectsAmbiguityAndHonorsOmissions(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Measures[0].Aliases = []string{"balance"}
	p.Definition.Measures[1].Aliases = []string{"balance"}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Balance"}
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "ambiguous_semantic_root" || engine.embeds != 0 {
		t.Fatal("ambiguous alias selected a metric", err)
	}
	in.MetricIDs = []string{"revenue"}
	out, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Context == nil || len(out.Context.Metrics) != 1 {
		t.Fatal("explicit selection could not disambiguate", err)
	}
	in.MetricIDs = nil
	in.Question = "Gross margin percentage by product family"
	in.OmittedRoots = []semantics.Reference{{Kind: semantics.KindKPI, ID: "margin_pct"}}
	out, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Context == nil || len(out.Context.Metrics) != 0 || len(out.Selection.Topics[0].Roots) != 1 {
		t.Fatal("omitted long name selected its shorter KPI constituent", err)
	}
	in.Question = "Not gross margin percentage"
	in.OmittedRoots = nil
	out, err = service.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
	if err != nil || len(out.Context.Metrics) != 0 {
		t.Fatal("negated concept was auto-selected", err)
	}
}

func TestSQLRecoverySelectedGraphDoesNotGuessJoinPaths(t *testing.T) {
	for _, shape := range []string{"disconnected", "ambiguous"} {
		t.Run(shape, func(t *testing.T) {
			p := recoveryPublication()
			if shape == "disconnected" {
				p.Definition.Joins = nil
			} else {
				j := p.Definition.Joins[0]
				j.ID = "second-path"
				p.Definition.Joins = append(p.Definition.Joins, j)
			}
			service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
			out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage by product family"})
			if err != nil || out.Clarification == nil || out.Clarification.Reason != "ambiguous_semantic_relationship" || engine.embeds != 0 {
				t.Fatal("unsafe connecting path reached provider", err)
			}
		})
	}
}

func TestSQLRecoverySelectionReplayDetectsChangedEvidence(t *testing.T) {
	p := recoveryPublication()
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage by product family"})
	if err != nil || out.Selection == nil {
		t.Fatal(err)
	}
	calls := engine.embeds
	if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil {
		t.Fatal("fresh selection failed replay", err)
	}
	if engine.embeds != calls {
		t.Fatal("selection replay called a model")
	}
	out.Selection.Topics[0].DependencyDigest = strings.Repeat("b", 64)
	if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("changed selection replayed", err)
	}
	out.Selection = nil
	if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("new context downgraded to legacy replay", err)
	}
}

func TestSQLRecoverySelectionOmissionsCannotDisableRequiredRules(t *testing.T) {
	p := recoveryPublication()
	metric := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	family := semantics.Reference{Kind: semantics.KindDimension, ID: "family"}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1]))
	service.rules = selectionPolicy(t, p, []semantics.RuleDefinition{selectionRule("required", semantics.RuleScope{Kind: semantics.RuleScopeEntities, Targets: []semantics.Reference{metric, family}}, family)}, nil)
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage", OmittedRoots: []semantics.Reference{family}})
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "conflicting_semantic_rules" || engine.embeds != 0 {
		t.Fatal("omission disabled a hard requirement", err)
	}
}

func TestSQLRecoverySelectionExcludesDescriptiveAssociationsFromRuleFacts(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "amount_band", Name: "Amount band", Field: recoveryColumn("dataset", "amount"), Role: semantics.DimensionNumeric})
	admitted := []admittedTopic{{id: "topic", publication: p}}
	in := RouteRequest{Locale: nlq.LanguageEnglish, Question: "Revenue"}
	if err := initialSemanticSelection(context.Background(), in, admitted, nil); err != nil {
		t.Fatal(err)
	}
	if err := expandSelectedFacts(context.Background(), &admitted[0]); err != nil {
		t.Fatal(err)
	}
	for _, ref := range admitted[0].selection.facts {
		if ref.Kind == semantics.KindDimension && ref.ID == "amount_band" {
			t.Fatal("descriptive dimension became selected rule fact")
		}
	}
	assertRecoveryDependencies(t, admitted[0].selection.dependencies, "dimension:amount_band")
}

func TestSQLRecoverySelectionCancellationAndDetachedOrdering(t *testing.T) {
	p := recoveryPublication()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := initialSemanticSelection(ctx, RouteRequest{Question: "Revenue"}, []admittedTopic{{id: "topic", publication: p}}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("selection ignored cancellation", err)
	}
	left := []admittedTopic{{id: "topic", publication: p}}
	right := []admittedTopic{{id: "topic", publication: p}}
	a := RouteRequest{Question: "Gross margin percentage by product family", Locale: nlq.LanguageEnglish, References: []semantics.Reference{{Kind: semantics.KindDimension, ID: "family"}, {Kind: semantics.KindKPI, ID: "margin_pct"}}}
	b := a
	b.References = []semantics.Reference{a.References[1], a.References[0]}
	for i, input := range []RouteRequest{a, b} {
		target := left
		if i == 1 {
			target = right
		}
		if err := initialSemanticSelection(context.Background(), input, target, nil); err != nil {
			t.Fatal(err)
		}
		if err := expandSelectedFacts(context.Background(), &target[0]); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(selectionView(left), selectionView(right)) {
		t.Fatal("selection digest depends on reference order")
	}
	view := selectionView(left)
	view.Topics[0].References[0].ID = "changed"
	if reflect.DeepEqual(view, selectionView(left)) {
		t.Fatal("selection view aliases working state")
	}
}

func TestSQLRecoverySelectedTypedMetricCannotResolveAnotherKind(t *testing.T) {
	p := recoveryPublication()
	// Distinct semantic kinds have distinct coordinates, even with one bare ID.
	p.Definition.KPIs = append(p.Definition.KPIs, semantics.KPI{ID: "revenue", Name: "Double revenue", Expression: "2 * revenue", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
	items := []admittedTopic{{id: "topic", publication: p}}
	input := RouteRequest{Locale: nlq.LanguageEnglish, Question: "Double revenue"}
	if err := initialSemanticSelection(context.Background(), input, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := expandSelectedFacts(context.Background(), &items[0]); err != nil {
		t.Fatal(err)
	}
	metrics, err := applySelectedContext(items, nil)
	if err != nil || len(metrics) != 1 || !strings.Contains(metrics[0].Text, "2 * revenue") {
		t.Fatal("typed KPI resolved the measure with the same bare ID", err)
	}
}

func TestSQLRecoveryReviewedReferenceWithoutRulesIsValid(t *testing.T) {
	p := recoveryPublication()
	service, _ := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Show this metric", References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
	if err != nil || out.Context == nil || len(out.Context.Metrics) != 1 {
		t.Fatal("reviewed explicit reference incorrectly requires a ruleset", err)
	}
}

func TestSQLRecoverySharedConceptRequiresIdenticalGraphAndSource(t *testing.T) {
	p := recoveryPublication()
	q := recoveryPublication()
	q.State.Topic, q.Definition.Topic = "related", "related"
	ref := semantics.Reference{Kind: semantics.KindKPI, ID: "margin_pct"}
	for _, damage := range []string{"none", "formula", "source"} {
		t.Run(damage, func(t *testing.T) {
			var changed topics.Published
			raw, _ := json.Marshal(q)
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			if damage == "formula" {
				changed.Definition.KPIs[0].Expression = "revenue + costs"
			} else if damage == "source" {
				changed.Definition.Datasets[0].Source.Source = "another-source"
			}
			items := []admittedTopic{{id: "topic", publication: p}, {id: "related", publication: changed}}
			same, err := sameSelectedConcept(context.Background(), items, []selectionTerm{{topic: 0, ref: ref}, {topic: 1, ref: ref}})
			if err != nil || same != (damage == "none") {
				t.Fatal("shared-topic meaning was not established from exact closure", same, err)
			}
		})
	}
}
