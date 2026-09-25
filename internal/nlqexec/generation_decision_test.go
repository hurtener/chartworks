package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func decisionResponse(t *testing.T, kind string, questions ...string) gateway.Generated {
	t.Helper()
	raw, err := json.Marshal(generatedCandidate{Decision: kind, Questions: questions, Parameters: []exec.Parameter{}, Assumptions: []string{}, Ambiguities: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return gateway.Generated{JSON: raw, Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen", Attempts: 1}}}}
}
func TestSQLRecoveryDecisionBlocksBeforeValidation(t *testing.T) {
	for _, kind := range []string{generationdecision.Clarify, generationdecision.Insufficient} {
		e := testEnvelope(t)
		a, g, call, budget := testGeneration(t, e)
		model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{decisionResponse(t, kind, "Which reviewed metric?")}}}
		v := &sequenceValidator{}
		service := &Service{engine: model, validator: v}
		candidate, fixes, receipt, _, err := service.generateAndValidate(context.Background(), e, a, g, call, budget, "")
		problem := GenerationProblem(err)
		if problem == nil || problem.Outcome != kind || candidate.SQL != "" || fixes != 0 || v.calls != 0 || len(model.requests) != 1 || len(receipt.Calls) != 1 {
			t.Fatal("blocked decision reached native/correction or lost its attempt", err)
		}
		if !strings.Contains(model.requests[0].System, generationDecisionInstruction) {
			t.Fatal("decision instruction omitted")
		}
		if validationRepairable(err) {
			t.Fatal("clarification treated as SQL error")
		}
	}
}
func TestSQLRecoveryDecisionRejectsMissingAndContradictoryWire(t *testing.T) {
	bad := []string{
		`{"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`,
		`{"decision":"ready","questions":["Which metric?"],"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`,
		`{"decision":"clarify","questions":["Which metric?"],"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`,
		`{"decision":"insufficient_context","questions":[],"sql":"","parameters":[],"assumptions":[],"ambiguities":[]}`,
		`{"decision":"ready","questions":[],"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[],"bypass":true}`,
		`{"decision":"ready","decision":"clarify","questions":[],"sql":"SELECT id FROM analytics.sales","parameters":[],"assumptions":[],"ambiguities":[]}`,
	}
	for _, raw := range bad {
		e := testEnvelope(t)
		a, g, call, budget := testGeneration(t, e)
		model := &sequenceGateway{responses: []gateway.Generated{{JSON: json.RawMessage(raw)}}}
		v := &sequenceValidator{}
		service := &Service{engine: model, validator: v}
		_, _, _, _, err := service.generateAndValidate(context.Background(), e, a, g, call, budget, "")
		if !errors.Is(err, ErrGeneration) || v.calls != 0 || model.calls != 1 || GenerationProblem(err) != nil {
			t.Fatal("invalid wire acquired execution/clarification", err)
		}
	}
}
func TestSQLRecoveryDecisionStopsValidationRepair(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT missing FROM analytics.sales", nil, "sqlgen"), decisionResponse(t, generationdecision.Clarify, "Which metric was intended?")}}}
	v := &sequenceValidator{errors: []error{exec.ErrUnsafe}}
	s := &Service{engine: model, validator: v}
	_, fixes, receipt, _, err := s.generateAndValidate(context.Background(), e, a, g, call, budget, "")
	if !errors.Is(err, ErrGenerationClarification) || fixes != 1 || v.calls != 1 || len(model.requests) != 2 || len(receipt.Calls) != 2 {
		t.Fatal("decision exceeded repair or reached second validation", err)
	}
	if model.requests[1].Role != "sqlfix" || !strings.Contains(model.requests[1].System, generationDecisionInstruction) {
		t.Fatal("sqlfix missed readiness contract")
	}
}
func TestSQLRecoveryDecisionStopsExecutionCorrection(t *testing.T) {
	e := unitEnvelope(t)
	a, _, _, _ := testGeneration(t, e)
	q := unitQuery(e, "decision-query", "topic", "v1", "context", false)
	q.Generation.Context = a.assembled
	q.Route.Context = &nlqroute.ContextView{Relations: cloneRelations(a.assembled.Relations)}
	q.Assumptions = []string{"retained original note"}
	repo := newUnitRepository()
	repo.queries[q.ID] = q
	model := &sequenceGateway{responses: []gateway.Generated{decisionResponse(t, generationdecision.Insufficient, "Which reviewed dependency exists?")}}
	x := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}, unitResult("succeeded")}, errors: []error{exec.ErrQuery}}
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}, retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)}}
	s := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: x, engine: model, repo: repo}
	_, err := s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "decision-run"})
	if !errors.Is(err, ErrGenerationContext) || x.calls != 1 || model.calls != 1 || repo.queries[q.ID].SQL != q.SQL || repo.queries[q.ID].Assumptions[0] != q.Assumptions[0] {
		t.Fatal("blocked correction executed/substituted state", err)
	}
	_, _ = s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "decision-run"})
	if x.calls != 1 || model.calls != 1 {
		t.Fatal("terminal replay retried clarification")
	}
}
func TestSQLRecoveryDecisionPrivacyAndDetachment(t *testing.T) {
	raw := "alias-private-893"
	a := admission{route: nlqroute.RouteResult{Resolutions: []semantics.ClarificationResolution{{Topic: "topic", Pattern: "customer", Slot: "name", Value: "canonical-private-893", Sensitivity: semantics.LiteralSensitive}}}, decisionAnswers: []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "name", Value: &semantics.ClarificationValue{Text: &raw}}}, decisionParameters: []exec.Parameter{{Kind: "text", Value: "binding-private-412"}}, decisionParent: &QueryRecord{Parameters: []exec.Parameter{{Kind: "text", Value: "parent-private-613"}}}}
	c := generatedCandidate{Decision: generationdecision.Clarify, Questions: []string{"Clarify canonical-private-893, alias-private-893, binding-private-412 or parent-private-613?"}}
	before := exec.Hash(c)
	err := candidateDecision(a, c)
	p := GenerationProblem(errors.Join(ErrValidationBudget, err))
	if p == nil || strings.Contains(p.Questions[0], "private-") || !strings.Contains(p.Questions[0], "[redacted answer]") || exec.Hash(c) != before {
		t.Fatal("private or shared decision content", err)
	}
	if strings.Contains(fmt.Sprintf("%v %#v", err, err), "private-") {
		t.Fatal("error formatting leaked input")
	}
	p.Questions[0] = "mutated"
	if GenerationProblem(err).Questions[0] == "mutated" {
		t.Fatal("public problem aliases private error")
	}
	if GenerationProblem(errors.New("arbitrary upstream body")) != nil {
		t.Fatal("untrusted provider error became questions")
	}
	a.decisionParameters = []exec.Parameter{{Kind: "text", Value: "x"}}
	c.Questions = []string{strings.Repeat("x", 512)}
	p = GenerationProblem(candidateDecision(a, c))
	if p == nil || len(p.Questions[0]) > 512 || strings.Contains(p.Questions[0], "[redacted answer]") {
		t.Fatal("redaction expansion not withheld")
	}
}
func TestSQLRecoveryDecisionReadyKeepsCaveatsAndLegacyReads(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	model := &sequenceGateway{responses: []gateway.Generated{explanationCandidate(t, "SELECT id FROM analytics.sales", []string{"reviewed units"}, []string{"No presentation order specified."})}}
	s := &Service{engine: model, validator: &sequenceValidator{}}
	c, _, _, _, err := s.generateAndValidate(context.Background(), e, a, g, call, budget, "")
	if err != nil || len(c.Ambiguities) != 1 {
		t.Fatal("advisory caveat became blocker", err)
	}
	q := QueryRecord{Assumptions: []string{"legacy"}, Ambiguities: []string{"historical unclassified note"}}
	out := s.runResult(q, exec.ExecutionReport{}, false)
	if len(out.Ambiguities) != 1 {
		t.Fatal("legacy notes reinterpreted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = s.generate(ctx, e, a, g, call, budget, "sqlgen", "")
	if !errors.Is(err, context.Canceled) || model.calls != 1 {
		t.Fatal("cancelled request reached generation", err)
	}
}
