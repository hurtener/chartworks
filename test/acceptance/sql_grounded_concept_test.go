package acceptance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func groundedOptionID(p topics.Published, ref semantics.Reference) string {
	return readexec.Hash([]any{p.Definition.Topic, p.State.Version, p.Digest, ref})
}
func groundedWireResponse(t *testing.T, body any) string {
	t.Helper()
	content, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{"id": "grounded-fixture", "object": "chat.completion", "model": "model-clarify", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(response)
}
func groundedSelectionResponse(t *testing.T, p topics.Published, question, id string) string {
	return groundedWireResponse(t, map[string]any{"decision": "select", "selected": []any{map[string]any{"id": groundedOptionID(p, semantics.Reference{Kind: semantics.KindMeasure, ID: id}), "quote": question}}, "alternatives": []string{}})
}
func TestSQLRecoveryGroundedConceptLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			text := "How much did customers pay in total?"
			if locale == nlq.LanguageSpanish {
				text = "¿Cuánto pagaron los clientes en total?"
			}
			q := f.question(text, locale)
			q.ConceptPolicy = nlqroute.GroundedConceptPolicy
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.chatSequence = []string{groundedSelectionResponse(t, f.published, text, "revenue"), phase18RawResponse(t, `SELECT sum(amount) AS paid FROM analytics.sales`)}
			f.model.mu.Unlock()
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if err != nil || p.QueryID == "" || p.Analytical == nil || p.Route.Concepts == nil {
				t.Fatal("grounded concept plan", err)
			}
			if len(p.Route.Selection.Topics) != 1 || len(p.Route.Selection.Topics[0].Roots) != 1 || p.Route.Selection.Topics[0].Roots[0].Reason != "grounded_model" {
				t.Fatal("not selected via concept policy")
			}
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			stored, err := f.f.db.ReadQuery(ctx, scope, p.QueryID)
			if err != nil || stored.Route.Concepts == nil || readexec.Hash(stored.Route.Concepts) != readexec.Hash(p.Route.Concepts) {
				t.Fatal("lost protected selection", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			calls := f.model.requests.Load()
			out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls {
				t.Fatal("restart repeated selector", err)
			}
			requireExplanationSum(t, out, "9007199254740998.625")
			projected, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
			if err != nil || projected.Result != nil || readexec.Hash(projected.Route.Concepts) != readexec.Hash(stored.Route.Concepts) {
				t.Fatal("saved proof/privacy", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
			if err != nil || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts || f.model.requests.Load() != calls {
				t.Fatal("terminal replay repeated work", err)
			}
			requireExplanationSum(t, replay, "9007199254740998.625")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if !strings.Contains(wire, "nlq_concept_choice") || !strings.Contains(wire, "model-clarify") || !strings.Contains(wire, "sum(amount)") {
				t.Fatal("not the real structured provider/generation path")
			}
			// The previous independent model choice is retained as a reviewed typed root.
			f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS paid FROM analytics.sales`))
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
			if err != nil || child.Route.Concepts != nil || len(child.Route.Request.MetricIDs) != 1 || child.Route.Request.MetricIDs[0] != "revenue" {
				t.Fatal("refinement reran independent model choice", err)
			}
			f.model.mu.Lock()
			clarifies := 0
			for _, model := range f.model.models {
				if model == "model-clarify" {
					clarifies++
				}
			}
			f.model.mu.Unlock()
			if clarifies != 1 {
				t.Fatal("unexpected extra concept operation", clarifies)
			}
		})
	}
}
func TestSQLRecoveryGroundedConceptAmbiguityAcceptance(t *testing.T) {
	f := newCW01FixtureWithPack(t, func(p *semantics.TopicPack) {
		m := p.Measures[0]
		m.ID = "average_payment"
		m.Name = "Average payment"
		m.Description = "Mean amount per sales record"
		m.Aggregation = semantics.AggregationAverage
		p.Measures = append(p.Measures, m)
	})
	ctx := context.Background()
	q := f.question("What payment measure should we use?", nlq.LanguageEnglish)
	q.ConceptPolicy = nlqroute.GroundedConceptPolicy
	ids := []string{groundedOptionID(f.published, semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}), groundedOptionID(f.published, semantics.Reference{Kind: semantics.KindMeasure, ID: "average_payment"})}
	metadata := support.Raw(t, f.f.dsn)
	for _, decision := range []string{"clarify", "no_match"} {
		alternatives := []string{}
		if decision == "clarify" {
			alternatives = ids
		}
		f.model.mode.Store(groundedWireResponse(t, map[string]any{"decision": decision, "selected": []any{}, "alternatives": alternatives}))
		before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil || p.QueryID != "" || p.Route.Clarification == nil || p.Route.Context != nil || p.Route.Concepts == nil {
			t.Fatal("model ambiguity became executable", decision, err)
		}
		if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
			t.Fatal("blocked selector executed")
		}
		if decision == "clarify" {
			if len(p.Route.Concepts.Options) != 2 {
				t.Fatal("missing reviewed alternatives")
			}
			explicit := q
			explicit.References = []semantics.Reference{p.Route.Concepts.Options[0].Reference}
			if explicit.References[0].ID != "revenue" {
				explicit.References[0] = p.Route.Concepts.Options[1].Reference
			}
			f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS paid FROM analytics.sales`))
			p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: explicit})
			if err != nil || p.QueryID == "" || p.Route.Concepts != nil {
				t.Fatal("reviewed alternative cannot be used", err)
			}
		}
	}
	f.model.mu.Lock()
	defer f.model.mu.Unlock()
	for _, model := range f.model.models {
		if model == "model-sqlfix" {
			t.Fatal("ambiguity sent through SQL repair")
		}
	}
}
func TestSQLRecoveryGroundedConceptPrivatePendingAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := f.question("How much did customers pay for named sales?", nlq.LanguageEnglish)
	q.ConceptPolicy = nlqroute.GroundedConceptPolicy
	f.model.mode.Store(groundedSelectionResponse(t, f.published, q.Question, "revenue"))
	pending, err := f.query.Preflight(ctx, f.e, nlqexec.PreflightRequest{QuestionRequest: q})
	if err != nil || pending.QueryID == "" || pending.Route.Clarification == nil || pending.Route.Concepts == nil {
		t.Fatal("grounded concept did not activate reviewed private form", err)
	}
	q.ClarificationQuery, q.AnswerContext = pending.QueryID, pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	// Restart and submit a typed answer; the model selection is replayed, not rerun.
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	f.model.mode.Store(phase18RawResponse(t, `SELECT sum(amount) AS paid FROM analytics.sales`))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil || p.QueryID == "" || p.Bindings == nil || p.Route.Concepts == nil || readexec.Hash(p.Route.Concepts) != readexec.Hash(pending.Route.Concepts) {
		t.Fatal("pending selection origin changed", err)
	}
	out := f.run(t, p, 1, true)
	requireExplanationSum(t, out, "9007199254740993.125")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies, "\n")
	clarifies := 0
	for _, m := range f.model.models {
		if m == "model-clarify" {
			clarifies++
		}
	}
	f.model.mu.Unlock()
	if clarifies != 1 || strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "alias-secret-731") {
		t.Fatal("private values leaked or model proof regenerated", clarifies)
	}
	modified := p.Route
	copy := *p.Route.Concepts
	modified.Concepts = &copy
	modified.Concepts.Catalog = strings.Repeat("0", 64)
	calls := f.model.requests.Load()
	if _, _, err := f.service.ReplayClarifications(ctx, f.e, modified); err == nil || f.model.requests.Load() != calls {
		t.Fatal("substituted evidence reselected rather than rejected")
	}
}
