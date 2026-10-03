package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

type fixedPointTopics struct {
	*testTopics
	bindings int
}

func (p *fixedPointTopics) ClarificationBinding(ctx context.Context, e identity.Envelope, source, executionContext string) (readexec.Binding, error) {
	p.bindings++
	return p.testTopics.ClarificationBinding(ctx, e, source, executionContext)
}

func fixedPointRouteFixture(t *testing.T) (*Service, *testEngine, *fixedPointTopics, RouteRequest, []semantics.ClarificationAnswer) {
	t.Helper()
	p := recoveryPublication()
	amount, cost := recoveryColumn("dataset", "amount"), recoveryColumn("dataset", "cost")
	for i := range p.Definition.Datasets[0].Columns {
		c := &p.Definition.Datasets[0].Columns[i]
		if c.NativeType == "numeric" {
			c.Category = "numeric"
			c.NativeType = "numeric(38,6)"
		}
	}
	gate := semantics.ClarificationPattern{ID: "basis", Version: "v1", Targets: []semantics.Reference{amount, cost}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-dependent-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"choose inspection", "elegir inspección"}}, Why: "Choose a reviewed basis."}, Slots: []semantics.ClarificationSlot{{ID: "basis", Kind: semantics.SlotChoice, Required: true, Sensitivity: semantics.LiteralNonSensitive, Prompt: "Which basis?", Choices: []semantics.ClarificationChoice{{ID: "amount-option", Label: "First basis", Target: &amount}, {ID: "cost-option", Label: "Second basis", Target: &cost}}}}}
	child := semantics.ClarificationPattern{ID: "threshold", Version: "v1", Targets: []semantics.Reference{amount}, Provenance: gate.Provenance, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyReferences: []semantics.Reference{amount}}, Why: "The first basis requires a reviewed threshold."}, Slots: []semantics.ClarificationSlot{{ID: "minimum", Kind: semantics.SlotNumber, Required: true, Sensitivity: semantics.LiteralSensitive, Prompt: "Which minimum?", PromptES: "¿Qué mínimo?", Effect: &semantics.ClarificationEffect{Kind: "number", Target: amount, Operator: "gte", Nulls: "exclude", Unit: "USD", Precision: 38, Scale: 6}}}}
	service, engine := recoveryRouteService(t, p, recoveryFacet(t, p, "measure", "revenue", p.Definition.Measures[0]))
	reader := &fixedPointTopics{testTopics: service.topics.(*testTopics)}
	reader.binding = cw07Binding(1)
	reader.binding.Relations = reader.contract.Relations
	service.topics = reader
	service.rules = selectionPolicy(t, p, nil, []semantics.ClarificationPattern{child, gate})
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Choose inspection"}
	answer := func(pattern, slot string, value semantics.ClarificationValue) semantics.ClarificationAnswer {
		return semantics.ClarificationAnswer{Topic: "topic", TopicVersion: "v1", RulesetVersion: "r1", Pattern: pattern, PatternVersion: "v1", Slot: slot, Value: &value}
	}
	return service, engine, reader, in, []semantics.ClarificationAnswer{answer("basis", "basis", semantics.ClarificationValue{OptionID: "amount-option"}), answer("threshold", "minimum", semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "9007199254740993.125", Unit: "USD"}})}
}

func TestSQLRecoveryDependentAnswersKeepPreflightBinding(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			service, engine, reader, in, answers := fixedPointRouteFixture(t)
			in.Locale = locale
			if locale == nlq.LanguageSpanish {
				in.Question = "Elegir inspección"
				answers[1].Value.Number.Value = "9007199254740993,125"
			}
			ctx := context.Background()
			e := cw07DiscoveryEnvelope(t)
			pending, err := service.Route(ctx, e, in)
			if err != nil || pending.Clarification == nil || len(pending.Clarification.Questions) != 1 || pending.Clarification.Questions[0].Pattern != "basis" || reader.bindings == 0 || engine.embeds != 0 {
				t.Fatal("initial choice was not source-bound before answers", err, pending.Clarification, reader.bindings)
			}
			in.AnswerContext = pending.AnswerContext
			in.Answers = answers[:1]
			child, err := service.Route(ctx, e, in)
			if err != nil || child.Clarification == nil || child.AnswerContext != pending.AnswerContext || len(child.Clarification.Questions) != 1 || child.Clarification.Questions[0].Pattern != "threshold" || engine.embeds != 0 {
				t.Fatal("dependent question broke original answer context", err, child.Clarification)
			}
			if locale == nlq.LanguageSpanish && child.Clarification.Questions[0].Prompt != "¿Qué mínimo?" {
				t.Fatal("dependent locale lost")
			}
			// Both answers may arrive together against the first protected context.
			in.Answers = answers
			ready, err := service.Route(ctx, e, in)
			if err != nil || ready.Context == nil || ready.Clarification != nil || len(ready.Resolutions) != 2 || ready.AnswerContext != pending.AnswerContext {
				t.Fatal("complete dependent route", err, ready.Clarification)
			}
			constraints, err := ready.ResolvedBusinessConstraints()
			if err != nil || len(constraints) != 1 || constraints[0].Value != "9007199254740993.125" {
				t.Fatal("exact owned predicate not emitted", err, constraints)
			}
			if strings.Contains(ready.Context.Prompt, "9007199254740993") {
				t.Fatal("private answer leaked to provider context")
			}
			before := engine.embeds
			for _, prior := range []RouteResult{child, ready} {
				raw, _ := json.Marshal(prior)
				var restored RouteResult
				if err := json.Unmarshal(raw, &restored); err != nil {
					t.Fatal(err)
				}
				if _, _, err := service.ReplayClarifications(ctx, e, restored); err != nil {
					t.Fatal("durable dependent state cannot replay", err)
				}
			}
			if engine.embeds != before {
				t.Fatal("replay dispatched model work")
			}
			reader.binding.Revision++
			if _, _, err := service.ReplayClarifications(ctx, e, ready); !errors.Is(err, readexec.ErrBinding) {
				t.Fatal("changed source borrowed dependent proof", err)
			}
		})
	}
}

func TestSQLRecoveryDependentAnswerSourceRotationAndInactiveIsolation(t *testing.T) {
	service, engine, reader, in, answers := fixedPointRouteFixture(t)
	ctx := context.Background()
	e := cw07DiscoveryEnvelope(t)
	unrelated := in
	unrelated.Question = "List records"
	if out, err := service.Route(ctx, e, unrelated); err != nil || out.Clarification != nil || reader.bindings != 0 {
		t.Fatal("unrelated policy caused binding/clarification", err, out.Clarification, reader.bindings)
	}
	pending, err := service.Route(ctx, e, in)
	if err != nil {
		t.Fatal(err)
	}
	in.AnswerContext = pending.AnswerContext
	in.Answers = answers
	reader.binding.Fingerprint = strings.Repeat("b", 64)
	before := engine.embeds
	_, err = service.Route(ctx, e, in)
	var invalid *Clarification
	if !errors.As(err, &invalid) || invalid.Reason != "stale_answer_context" || engine.embeds != before {
		t.Fatal("source rotation accepted original answer context", err)
	}
	reader.binding.Fingerprint = strings.Repeat("a", 64)
	in.Answers[0].Value.OptionID = "cost-option"
	_, err = service.Route(ctx, e, in)
	if !errors.As(err, &invalid) || engine.embeds != before {
		t.Fatal("inactive retained child silently applied", err)
	}
	in.Answers = answers[:1]
	out, err := service.Route(ctx, e, in)
	if err != nil || out.Context == nil || len(out.Resolutions) != 1 {
		t.Fatal("switching branch without stale child failed", err, out.Clarification)
	}
	if constraints, err := out.ResolvedBusinessConstraints(); err != nil || len(constraints) != 0 {
		t.Fatal("old branch predicate survived", err, constraints)
	}
}
