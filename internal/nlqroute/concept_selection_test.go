package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

type groundedEngine struct {
	gateway.Engine
	calls    int
	prompts  []string
	response func(string, []conceptCard) conceptchoice.Proposal
}

func (g *groundedEngine) Generate(ctx context.Context, _ gateway.Call, _ *gateway.Budget, role, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	g.calls++
	g.prompts = append(g.prompts, system+"\n"+prompt)
	if err := ctx.Err(); err != nil {
		return gateway.Generated{}, err
	}
	if role != "clarify" || schema.Name() != "nlq_concept_choice" {
		return gateway.Generated{}, gateway.ErrInput
	}
	var body struct {
		Question   string        `json:"question"`
		Candidates []conceptCard `json:"candidates"`
	}
	if json.Unmarshal([]byte(prompt), &body) != nil {
		return gateway.Generated{}, gateway.ErrOutput
	}
	proposal := g.response(body.Question, body.Candidates)
	proposal.Selected = append([]conceptchoice.Selection{}, proposal.Selected...)
	proposal.Alternatives = append([]string{}, proposal.Alternatives...)
	raw, err := json.Marshal(proposal)
	return gateway.Generated{JSON: raw, Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: role}}}}, err
}
func choiceByID(q string, cards []conceptCard, id string) conceptchoice.Proposal {
	for _, c := range cards {
		if c.Reference.ID == id {
			return conceptchoice.Proposal{Decision: "select", Selected: []conceptchoice.Selection{{ID: c.ID, Quote: q}}}
		}
	}
	return conceptchoice.Proposal{Decision: "no_match"}
}
func newGroundedFixture(t *testing.T) (*Service, *groundedEngine) {
	t.Helper()
	p := recoveryPublication()
	hit := recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1])
	s, engine := recoveryRouteService(t, p, hit)
	model := &groundedEngine{Engine: engine, response: func(q string, cards []conceptCard) conceptchoice.Proposal { return choiceByID(q, cards, "margin_pct") }}
	s.engine = model
	return s, model
}
func groundedRequest(question string) RouteRequest {
	return RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: question, ConceptPolicy: GroundedConceptPolicy}
}
func TestSQLRecoveryGroundedConceptRoutesFullKPIClosure(t *testing.T) {
	for _, question := range []string{"What share remains after costs?", "¿Qué porcentaje queda de las ventas luego de los costes?"} {
		s, model := newGroundedFixture(t)
		in := groundedRequest(question)
		if strings.HasPrefix(question, "¿") {
			in.Locale = nlq.LanguageSpanish
		}
		out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), in)
		if err != nil || out.Context == nil || out.Concepts == nil || model.calls != 1 || len(out.Context.Metrics) != 1 {
			t.Fatal("grounded route", err)
		}
		roots := out.Selection.Topics[0].Roots
		if len(roots) != 1 || roots[0].Reference.ID != "margin_pct" || roots[0].Reason != "grounded_model" {
			t.Fatal("incidental constituent became root", roots)
		}
		assertRecoveryDependencies(t, out.Context.Metrics[0].Dependencies, "kpi:margin_pct", "kpi:margin", "measure:revenue", "measure:costs", "column:dataset:amount", "column:dataset:cost")
		assembled, err := out.GenerationContext()
		if err != nil {
			t.Fatal(err)
		}
		generation, err := s.assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: assembled})
		if err != nil || !strings.Contains(generation.Prompt, "sales_cost") || !strings.Contains(generation.Prompt, "100 * margin") {
			t.Fatal("not in actual generator packet", err)
		}
		if _, _, err = s.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err != nil || model.calls != 1 {
			t.Fatal("replay called model or changed meaning", err)
		}
	}
}
func TestSQLRecoveryGroundedConceptExplicitAndLegacyNoCall(t *testing.T) {
	for _, request := range []RouteRequest{
		{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue"},
		{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "unknown wording", ConceptPolicy: GroundedConceptPolicy, MetricIDs: []string{"revenue"}},
		{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "unknown wording", ConceptPolicy: GroundedConceptPolicy, References: []semantics.Reference{{Kind: semantics.KindKPI, ID: "margin_pct"}}},
	} {
		s, model := newGroundedFixture(t)
		out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), request)
		if err != nil || out.Context == nil || out.Concepts != nil || model.calls != 0 {
			t.Fatal("legacy/explicit path changed", err)
		}
	}
}
func TestSQLRecoveryGroundedConceptAmbiguityHasReviewedOptions(t *testing.T) {
	for _, decision := range []string{"clarify", "no_match"} {
		s, model := newGroundedFixture(t)
		model.response = func(_ string, cards []conceptCard) conceptchoice.Proposal {
			p := conceptchoice.Proposal{Decision: decision}
			if decision == "clarify" {
				p.Alternatives = []string{cards[0].ID, cards[1].ID}
			}
			return p
		}
		out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), groundedRequest("Which return should count?"))
		if err != nil || out.Clarification == nil || out.Context != nil || out.Selection != nil || out.Concepts == nil || model.calls != 1 {
			t.Fatal("ambiguous result could generate", err)
		}
		if decision == "clarify" {
			if len(out.Concepts.Options) != 2 || len(out.Clarification.Choices) != 2 {
				t.Fatal("missing reviewed option coordinates")
			}
			for _, option := range out.Concepts.Options {
				if !option.Reference.Valid() || option.Topic != "topic" || option.Label == "" {
					t.Fatal("invalid reviewed option")
				}
			}
			request := groundedRequest("Use this reviewed concept")
			request.References = []semantics.Reference{out.Concepts.Options[0].Reference}
			explicit, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), request)
			if err != nil || explicit.Context == nil || model.calls != 1 {
				t.Fatal("options cannot drive explicit Plan", err)
			}
		}
	}
}
func TestSQLRecoveryGroundedConceptMalformedOutputAndAdmission(t *testing.T) {
	for _, response := range []func(string, []conceptCard) conceptchoice.Proposal{
		func(q string, c []conceptCard) conceptchoice.Proposal {
			return conceptchoice.Proposal{Decision: "select", Selected: []conceptchoice.Selection{{ID: strings.Repeat("b", 64), Quote: q}}}
		},
		func(q string, c []conceptCard) conceptchoice.Proposal {
			return conceptchoice.Proposal{Decision: "select", Selected: []conceptchoice.Selection{{ID: c[0].ID, Quote: "not present"}}}
		},
		func(q string, c []conceptCard) conceptchoice.Proposal {
			return conceptchoice.Proposal{Decision: "no_match", Selected: []conceptchoice.Selection{{ID: c[0].ID, Quote: q}}}
		},
	} {
		s, m := newGroundedFixture(t)
		m.response = response
		if out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), groundedRequest("income")); err == nil || out.Context != nil {
			t.Fatal("bad model response accepted")
		}
	}
	s, m := newGroundedFixture(t)
	if _, err := s.Route(context.Background(), testEnvelope(t, false), groundedRequest("income")); err == nil || m.calls != 0 {
		t.Fatal("unauthorized provider call")
	}
	in := groundedRequest("income")
	in.ConceptPolicy = "unknown"
	if _, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), in); err == nil || m.calls != 0 {
		t.Fatal("unknown policy reached provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Route(ctx, cw07DiscoveryEnvelope(t), groundedRequest("income")); !errors.Is(err, context.Canceled) || m.calls != 0 {
		t.Fatal("cancelled selector", err)
	}
}
func TestSQLRecoveryGroundedConceptReplayRejectsTamperWithoutModel(t *testing.T) {
	s, m := newGroundedFixture(t)
	out, err := s.Route(context.Background(), cw07DiscoveryEnvelope(t), groundedRequest("show the residual share"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	for _, change := range []func(*RouteResult){
		func(o *RouteResult) { o.Concepts = nil }, func(o *RouteResult) { o.Request.ConceptPolicy = "" }, func(o *RouteResult) { o.Request.Question += " and something else" },
		func(o *RouteResult) { o.Concepts.Catalog = strings.Repeat("0", 64) }, func(o *RouteResult) { o.Concepts.Choice.Selected[0].ID = strings.Repeat("0", 64) }, func(o *RouteResult) { o.Selection = nil },
		func(o *RouteResult) { o.Concepts.Options = []ConceptOption{{ID: "forged"}} },
	} {
		var modified RouteResult
		if json.Unmarshal(raw, &modified) != nil {
			t.Fatal("fixture")
		}
		change(&modified)
		if _, _, err := s.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), modified); err == nil {
			t.Fatal("modified proof accepted")
		}
		if m.calls != 1 {
			t.Fatal("replay model fallback")
		}
	}
	before := m.calls
	s.topics.(*testTopics).contract.Publication.Digest = strings.Repeat("b", 64)
	if _, _, err := s.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), out); err == nil || m.calls != before {
		t.Fatal("stale catalog borrowed proof")
	}
}
func TestSQLRecoveryGroundedConceptBoundsAndPrivateCatalog(t *testing.T) {
	p := recoveryPublication()
	p.Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "secret", Field: recoveryColumn("dataset", "amount"), Operator: "eq", Values: []string{"private-filter-1"}}}
	admitted := []admittedTopic{{id: "topic", publication: p}}
	in := groundedRequest("private-value-22 and customer-alias-22")
	admitted[0].rules = rulesets.Published{Definition: semantics.RuleSetDefinition{Patterns: []semantics.ClarificationPattern{{ID: "customer", Slots: []semantics.ClarificationSlot{{ID: "value", Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical: "private-value-22", Aliases: []string{"customer-alias-22"}}}}}}}}}}
	safe := groundedQuestion(in, admitted)
	if safe != "[redacted answer] and [redacted answer]" {
		t.Fatal("selector saw known private values")
	}
	cards, err := conceptCards(context.Background(), in, admitted)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cards)
	if strings.Contains(string(raw), "private-filter-1") || strings.Contains(string(raw), "private-value-22") {
		t.Fatal("compact selector cards exposed policy literals")
	}
	expected, _ := conceptCards(context.Background(), in, admitted)
	cards[0].Name = "mutated"
	again, _ := conceptCards(context.Background(), in, admitted)
	if !reflect.DeepEqual(expected, again) {
		t.Fatal("candidate cards shared metadata")
	}
	for len(admitted[0].publication.Definition.Measures) <= conceptchoice.MaxCandidates {
		v := p.Definition.Measures[0]
		v.ID = strings.Repeat("x", len(admitted[0].publication.Definition.Measures)+1)
		admitted[0].publication.Definition.Measures = append(admitted[0].publication.Definition.Measures, v)
	}
	if _, err := conceptCards(context.Background(), in, admitted); !errors.Is(err, nlq.ErrInsufficient) {
		t.Fatal("unbounded candidate enumeration", err)
	}
}
