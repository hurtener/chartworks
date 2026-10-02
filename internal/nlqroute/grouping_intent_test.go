package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
	"github.com/hurtener/chartworks/internal/semantics"
)

type groupingIntentEngine struct {
	gateway.Engine
	calls   int
	prompts []string
	respond func(string, []groupingIntentCard) conceptchoice.Proposal
}

func (g *groupingIntentEngine) Generate(ctx context.Context, _ gateway.Call, _ *gateway.Budget, role, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	g.calls++
	g.prompts = append(g.prompts, system+"\n"+prompt)
	if err := ctx.Err(); err != nil {
		return gateway.Generated{}, err
	}
	if role != "clarify" || schema.Name() != "nlq_grouping_intent" {
		return gateway.Generated{}, gateway.ErrInput
	}
	var in struct {
		Question   string               `json:"question"`
		Candidates []groupingIntentCard `json:"candidates"`
	}
	if json.Unmarshal([]byte(prompt), &in) != nil {
		return gateway.Generated{}, gateway.ErrInput
	}
	p := g.respond(in.Question, in.Candidates)
	p.Selected = append([]conceptchoice.Selection{}, p.Selected...)
	p.Alternatives = append([]string{}, p.Alternatives...)
	raw, _ := json.Marshal(map[string]any{"choice": p})
	return gateway.Generated{JSON: raw, Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: role}}}}, nil
}
func groupingProposal(question string, cards []groupingIntentCard, dimension string, grain semantics.TimeGrain) conceptchoice.Proposal {
	for _, card := range cards {
		if dimension == "" && card.Kind == "scalar_total" || card.Key != nil && card.Key.Dimension == dimension && card.Key.Grain == grain {
			return conceptchoice.Proposal{Decision: "select", Selected: []conceptchoice.Selection{{ID: card.ID, Quote: question}}}
		}
	}
	return conceptchoice.Proposal{Decision: "no_match"}
}
func newGroupingIntentFixture(t *testing.T) (*Service, *groupingIntentEngine) {
	t.Helper()
	p := recoveryPublication()
	p.Definition.Datasets[0].Columns = append(p.Definition.Datasets[0].Columns, semantics.Column{ID: "ordered", SourceName: "ordered_at", Name: "Order date", NativeType: "date", Category: "temporal"})
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "order_date", Name: "Order date", Aliases: []string{"fecha del pedido"}, Field: recoveryColumn("dataset", "ordered"), Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainDay, semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear}}})
	s, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	m := &groupingIntentEngine{Engine: engine, respond: func(q string, c []groupingIntentCard) conceptchoice.Proposal {
		return groupingProposal(q, c, "family", "")
	}}
	s.engine = m
	return s, m
}
func groupingIntentRequest(question string) RouteRequest {
	return RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{"revenue"}, GroupingIntentPolicy: GroundedGroupingIntentPolicy}
}

func TestSQLRecoveryGroundedGroupingRoutesRichLanguage(t *testing.T) {
	cases := []struct {
		question, dimension string
		grain               semantics.TimeGrain
		locale              nlq.Language
	}{
		{"Revenue grouped by product family, including all qualifying rows.", "family", "", nlq.LanguageEnglish},
		{"Ingresos por mes del calendario gregoriano en UTC; incluye todos los pedidos.", "order_date", semantics.GrainMonth, nlq.LanguageSpanish},
		{"Revenue by Gregorian calendar quarter in UTC, including every order.", "order_date", semantics.GrainQuarter, nlq.LanguageEnglish},
		{"What is the total revenue including all orders?", "", "", nlq.LanguageEnglish},
	}
	for _, tc := range cases {
		t.Run(tc.dimension+string(tc.grain)+string(tc.locale), func(t *testing.T) {
			s, m := newGroupingIntentFixture(t)
			m.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal {
				return groupingProposal(q, c, tc.dimension, tc.grain)
			}
			in := groupingIntentRequest(tc.question)
			in.Locale = tc.locale
			before, _ := json.Marshal(in)
			out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.Context == nil || out.GroupingIntent == nil || m.calls != 1 || out.Request.Grouping == nil {
				t.Fatal("grounded grouping route", err, out.Clarification)
			}
			want := []GroupingKey{}
			if tc.dimension != "" {
				want = append(want, GroupingKey{Topic: "topic", Dimension: tc.dimension, Grain: tc.grain})
			}
			if !reflect.DeepEqual(out.Request.Grouping.Keys, want) || !reflect.DeepEqual(out.GroupingIntent.Grouping.Keys, want) {
				t.Fatal("wrong reviewed grouping", out.Request.Grouping)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("caller request mutated")
			}
			if _, _, err := s.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err != nil || m.calls != 1 {
				t.Fatal("replay lost intent or called model", err)
			}
			if _, err := out.GenerationContext(); err != nil {
				t.Fatal("normal generation consumer has no sealed context", err)
			}
		})
	}
}

func TestSQLRecoveryGroundedGroupingManualAndLegacyNoModel(t *testing.T) {
	for _, policy := range []string{"", GroundedGroupingIntentPolicy} {
		s, m := newGroupingIntentFixture(t)
		in := groupingIntentRequest("Revenue by product family")
		in.Grouping = &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "family"}}}
		in.GroupingIntentPolicy = policy
		out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
		if err != nil || out.Context == nil || m.calls != 0 || out.GroupingIntent != nil || out.Request.GroupingIntentPolicy != "" {
			t.Fatal("manual grouping invoked model", err)
		}
	}
	s, m := newGroupingIntentFixture(t)
	in := groupingIntentRequest("Revenue")
	in.GroupingIntentPolicy = ""
	if out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in); err != nil || out.Context == nil || m.calls != 0 {
		t.Fatal("legacy costs changed", err)
	}
	in.GroupingIntentPolicy = "unknown"
	if _, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in); err == nil || m.calls != 0 {
		t.Fatal("unknown policy admitted")
	}
	in.GroupingIntentPolicy = GroundedGroupingIntentPolicy
	if _, err := s.Route(t.Context(), testEnvelope(t, false), in); err == nil || m.calls != 0 {
		t.Fatal("unauthorized model call")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Route(ctx, cw07DiscoveryEnvelope(t), in); !errors.Is(err, context.Canceled) || m.calls != 0 {
		t.Fatal("cancelled call admitted", err)
	}
}

func TestSQLRecoveryGroundedGroupingReplayRejectsTamper(t *testing.T) {
	s, m := newGroupingIntentFixture(t)
	out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), groupingIntentRequest("Revenue grouped by product family, with all rows included."))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	cases := map[string]func(*RouteResult){
		"missing_proof":     func(o *RouteResult) { o.GroupingIntent = nil },
		"missing_policy":    func(o *RouteResult) { o.Request.GroupingIntentPolicy = "" },
		"missing_both":      func(o *RouteResult) { o.GroupingIntent = nil; o.Request.GroupingIntentPolicy = "" },
		"changed_question":  func(o *RouteResult) { o.Request.Question += " instead" },
		"changed_grouping":  func(o *RouteResult) { o.Request.Grouping.Keys[0].Dimension = "order_date" },
		"changed_catalog":   func(o *RouteResult) { o.GroupingIntent.Catalog = strings.Repeat("a", 64) },
		"changed_output":    func(o *RouteResult) { o.GroupingIntent.Grouping.Keys = nil },
		"changed_option":    func(o *RouteResult) { o.GroupingIntent.Options[0].Label = "forged" },
		"missing_selection": func(o *RouteResult) { o.Selection = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var bad RouteResult
			_ = json.Unmarshal(raw, &bad)
			change(&bad)
			if _, _, err := s.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), bad); err == nil {
				t.Fatal("tampered grouping replay accepted")
			}
			if m.calls != 1 {
				t.Fatal("replay re-inferred intent")
			}
		})
	}
	out.GroupingIntent.Grouping.Keys = nil
	if _, err := out.ResolvedBusinessConstraints(); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("grouping escaped in-process seal", err)
	}
}

func TestSQLRecoveryGroundedGroupingClarificationsAndBounds(t *testing.T) {
	for _, mode := range []string{"clarify", "no_match", "repeated_span", "conflicting_grains"} {
		t.Run(mode, func(t *testing.T) {
			s, m := newGroupingIntentFixture(t)
			in := groupingIntentRequest("Revenue month then quarter, with month repeated.")
			m.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal {
				p := conceptchoice.Proposal{Decision: mode}
				if mode == "clarify" {
					p.Alternatives = []string{c[0].ID, c[1].ID}
				}
				if mode == "repeated_span" {
					p = groupingProposal(q, c, "order_date", semantics.GrainMonth)
					p.Selected[0].Quote = "month"
				}
				if mode == "conflicting_grains" {
					p = groupingProposal(q, c, "order_date", semantics.GrainMonth)
					p.Selected[0].Quote = "month then"
					other := groupingProposal(q, c, "order_date", semantics.GrainQuarter)
					other.Selected[0].Quote = "quarter"
					p.Selected = append(p.Selected, other.Selected...)
				}
				return p
			}
			out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.Clarification == nil || out.Context != nil || m.calls != 1 {
				t.Fatal("uncertain grouping executable", err, out.Clarification)
			}
		})
	}
	s, m := newGroupingIntentFixture(t)
	in := groupingIntentRequest("¿Cuál es el valor bruto por hora en America/New_York?")
	in.Locale = nlq.LanguageSpanish
	out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "unsupported_temporal_grain" || !strings.Contains(out.Clarification.Prompt, "Elegí") || m.calls != 0 {
		t.Fatal("unsupported Spanish grain not localized or called model", err)
	}
}

func TestSQLRecoveryGroundedGroupingAmbiguityAndSourceDrift(t *testing.T) {
	s, m := newGroupingIntentFixture(t)
	original := s.topics.(*testTopics).contract
	second := original.Publication.Definition.Dimensions[len(original.Publication.Definition.Dimensions)-1]
	second.ID = "ship_date"
	second.Name = "Ship date"
	second.Aliases = []string{"fecha de envío"}
	s.topics.(*testTopics).contract.Publication.Definition.Dimensions = append(s.topics.(*testTopics).contract.Publication.Definition.Dimensions, second)
	m.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal {
		return groupingProposal(q, c, "order_date", semantics.GrainMonth)
	}
	out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), groupingIntentRequest("Revenue by month, with all rows included."))
	if err != nil || out.Clarification == nil || out.Clarification.Reason != "conflicting_grouping_intent" || out.Context != nil {
		t.Fatal("competing date basis admitted", err)
	}
	out, err = s.Route(t.Context(), cw07DiscoveryEnvelope(t), groupingIntentRequest("Revenue by order date month, with all rows included."))
	if err != nil || out.Context == nil {
		t.Fatal("unique date basis not admitted", err, out.Clarification)
	}
	calls := m.calls
	s.topics.(*testTopics).contract.Relations[0].Columns[0].Nullable = !s.topics.(*testTopics).contract.Relations[0].Columns[0].Nullable
	if _, _, err = s.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err == nil || m.calls != calls {
		t.Fatal("source drift borrowed grouping", err)
	}
	// Equal local IDs from different publications stay distinct; shared aliases
	// do not authorize a model to choose one topic by preference.
	a := admittedTopic{id: "one", publication: original.Publication, relations: original.Relations}
	b := admittedTopic{id: "two", publication: original.Publication, relations: original.Relations}
	cards, err := groupingIntentCards(t.Context(), groupingIntentRequest("Revenue by product family"), []admittedTopic{a, b})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	var chosen string
	for _, c := range cards {
		ids = append(ids, c.ID)
		if c.Key != nil && c.Key.Topic == "one" && c.Key.Dimension == "family" {
			chosen = c.ID
		}
	}
	q := "Revenue by product family"
	proof, err := conceptchoice.Resolve(t.Context(), q, ids, conceptchoice.Proposal{Decision: "select", Selected: []conceptchoice.Selection{{ID: chosen, Quote: "product family"}}})
	if err != nil || !ambiguousGroupingChoice(cards, q, proof) {
		t.Fatal("cross-topic alias collision admitted", err)
	}
}

func TestSQLRecoveryGroundedGroupingPendingAndScalarCustody(t *testing.T) {
	for _, mode := range []string{"clarify", "no_match"} {
		t.Run(mode, func(t *testing.T) {
			s, m := newGroupingIntentFixture(t)
			m.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal {
				p := conceptchoice.Proposal{Decision: mode}
				if mode == "clarify" {
					p.Alternatives = []string{c[0].ID, c[1].ID}
				}
				return p
			}
			in := groupingIntentRequest("Revenue grouped by product family, all rows included.")
			out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.GroupingIntent == nil || out.Clarification == nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(out)
			var retained RouteResult
			_ = json.Unmarshal(raw, &retained)
			if _, _, err = s.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), retained); err != nil || m.calls != 1 {
				t.Fatal("pending replay inferred or lost decision", err)
			}
			if _, err = s.GroundedOrigin(t.Context(), cw07DiscoveryEnvelope(t), retained); !errors.Is(err, nlq.ErrInsufficient) {
				t.Fatal("unresolved choice promoted to origin", err)
			}
			// Options are presentation coordinates for a fresh explicit Plan, not a
			// reusable authority token or a business-answer continuation.
			in.Grouping = &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{{Topic: "topic", Dimension: "family"}}}
			out, err = s.Route(t.Context(), cw07DiscoveryEnvelope(t), in)
			if err != nil || out.Context == nil || out.GroupingIntent != nil || m.calls != 1 {
				t.Fatal("manual resolution re-inferred", err)
			}
		})
	}
	s, m := newGroupingIntentFixture(t)
	m.respond = func(q string, c []groupingIntentCard) conceptchoice.Proposal { return groupingProposal(q, c, "", "") }
	out, err := s.Route(t.Context(), cw07DiscoveryEnvelope(t), groupingIntentRequest("Total revenue including all orders"))
	if err != nil || out.Selection == nil || out.Selection.GroupingIntent == "" {
		t.Fatal("scalar lost model origin", err)
	}
	out.GroupingIntent = nil
	out.Request.GroupingIntentPolicy = ""
	if _, _, err = s.ReplayClarifications(t.Context(), cw07DiscoveryEnvelope(t), out); err == nil || m.calls != 1 {
		t.Fatal("scalar intent downgraded to legacy", err)
	}
}

func TestSQLRecoveryGroundedGroupingProtectedOrigin(t *testing.T) {
	s, m := newGroupingIntentFixture(t)
	e := cw07DiscoveryEnvelope(t)
	in := groupingIntentRequest("Revenue grouped by product family, all rows included.")
	out, err := s.Route(t.Context(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := s.GroundedOrigin(t.Context(), e, out)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Route(ctx, e, out.Request)
	if err != nil || replay.Context == nil || replay.GroupingIntent == nil || m.calls != 1 {
		t.Fatal("origin re-inferred", err)
	}
	changed := out.Request
	changed.Question += " instead"
	if _, err = s.Route(ctx, e, changed); err == nil || m.calls != 1 {
		t.Fatal("changed question borrowed origin", err)
	}
	changed = out.Request
	changed.Grouping = &GroupingSelection{Policy: GroupingPolicy, Keys: []GroupingKey{}}
	if _, err = s.Route(ctx, e, changed); err == nil || m.calls != 1 {
		t.Fatal("changed manual choice borrowed origin", err)
	}
}
