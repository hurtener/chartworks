package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func pendingFixture(t *testing.T) (*Service, QueryRecord, QuestionRequest) {
	t.Helper()
	e := unitEnvelope(t)
	request := QuestionRequest{Topic: "topic", Context: "context", Locale: nlq.LanguageEnglish, Question: "What is revenue?"}
	contract := unitContract("topic", "v1", "source", "context", "dataset", true, false)
	contract.Publication.Definition.Measures = []semantics.Measure{{ID: "revenue"}}
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": contract}}
	repo := newUnitRepository()
	s := &Service{repo: repo, topics: reader, sources: retainedSourceReader{}}
	q := unitQuery(e, "pending", "topic", "v1", "context", false)
	q.SQL = ""
	q.Status = "preflight"
	q.Route.Request = request.routeRequest()
	a, err := s.resolveCurrentAdmission(context.Background(), e, q, s.topics.Contract, s.sources.Binding)
	if err != nil {
		t.Fatal(err)
	}
	a.route = q.Route
	err = s.persistGenerationPending(context.Background(), e, request, "pending-operation", "", nil, a, nlq.GenerationContext{}, gateway.Receipt{Calls: []gateway.Usage{{Role: "sqlgen", Attempts: 1}}}, 0, &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Clarify, Questions: []string{"Which reviewed metric?"}}})
	problem := GenerationProblem(err)
	if problem == nil {
		t.Fatal(err)
	}
	q = repo.queries[problem.QueryID]
	return s, q, request
}

func TestGenerationPendingPersistenceAndRestartReplay(t *testing.T) {
	s, q, in := pendingFixture(t)
	if !GenerationPendingValid(q) || q.SQL != "" || q.Receipt.Calls[0].Attempts != 1 {
		t.Fatal("pending evidence lost")
	}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var restarted QueryRecord
	if err = json.Unmarshal(raw, &restarted); err != nil {
		t.Fatal(err)
	}
	_, err = s.reusablePlanResult(context.Background(), restarted, unitEnvelope(t), in, q.Operation, "")
	p := GenerationProblem(err)
	if p == nil || !reflect.DeepEqual(p, generationdecision.Public(q.GenerationPending.Problem)) {
		t.Fatal("restart changed pending outcome", err)
	}
	p.Choices[0].ID = "mutated"
	p.Questions[0] = "mutated"
	if q.GenerationPending.Problem.Choices[0].ID == "mutated" || q.GenerationPending.Problem.Questions[0] == "mutated" {
		t.Fatal("public projection aliases evidence")
	}
}

func TestGenerationPendingOriginRejectsBeforeDependencies(t *testing.T) {
	s, q, in := pendingFixture(t)
	in.GenerationQuery = q.ID
	in.GenerationContext = q.GenerationPending.Problem.AnswerContext
	in.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}
	request := PlanRequest{QuestionRequest: in, Operation: "resume:" + q.ID}
	for _, tc := range []struct {
		name string
		edit func(*PlanRequest, *QueryRecord)
	}{
		{"wrong context", func(r *PlanRequest, q *QueryRecord) { r.GenerationContext = "other" }},
		{"wrong operation", func(r *PlanRequest, q *QueryRecord) { r.Operation = "new-call" }},
		{"foreign session", func(r *PlanRequest, q *QueryRecord) { q.Session = "other" }},
		{"expired", func(r *PlanRequest, q *QueryRecord) {
			q.GenerationPending.Problem.ExpiresAt = time.Now().Add(-time.Second)
		}},
		{"stale", func(r *PlanRequest, q *QueryRecord) { q.EvidenceStale = true }},
		{"round bound", func(r *PlanRequest, q *QueryRecord) { q.GenerationPending.Round = maxGenerationRounds }},
		{"unreviewed", func(r *PlanRequest, q *QueryRecord) {
			r.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: "invented"}}
		}},
		{"question substitution", func(r *PlanRequest, q *QueryRecord) { r.Question = "different question" }},
		{"instructions", func(r *PlanRequest, q *QueryRecord) {
			r.Hints = []nlq.Instruction{{Key: "override", Text: "new intent"}}
		}},
		{"duplicate choice", func(r *PlanRequest, q *QueryRecord) { r.References = append(r.References, r.References[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := q
			pending := *q.GenerationPending
			copy.GenerationPending = &pending
			r := request
			r.References = append([]semantics.Reference(nil), request.References...)
			tc.edit(&r, &copy)
			repo := newUnitRepository()
			repo.queries[q.ID] = copy
			denied := &Service{repo: repo} // Any topic/source/model access would panic.
			if _, err := denied.Plan(context.Background(), unitEnvelope(t), r); err == nil {
				t.Fatal("invalid origin accepted")
			}
		})
	}
	_ = s
}

func TestGenerationPendingReviewedChoicesAndOrigin(t *testing.T) {
	_, q, in := pendingFixture(t)
	answer := in
	answer.GenerationQuery = q.ID
	answer.GenerationContext = q.GenerationPending.Problem.AnswerContext
	answer.References = []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}
	if !generationResumeRequest(in, answer, q.GenerationPending) {
		t.Fatal("reviewed choice rejected")
	}
	p := topics.Published{Definition: topics.Definition{Measures: []semantics.Measure{{ID: "duplicate"}, {ID: "unique"}}}}
	other := topics.Published{Definition: topics.Definition{Measures: []semantics.Measure{{ID: "duplicate"}}, Dimensions: []semantics.Dimension{{ID: "region"}}}}
	choices := generationChoices(admission{publications: []topics.Published{p, other}})
	if !reflect.DeepEqual(choices, []generationdecision.Choice{{Kind: "dimension", ID: "region"}, {Kind: "measure", ID: "unique"}}) {
		t.Fatal("ambiguous catalog ID exposed", choices)
	}
}

func TestGenerationPendingCurrentSourceAndExpiry(t *testing.T) {
	s, q, _ := pendingFixture(t)
	if err := s.checkGenerationPending(context.Background(), unitEnvelope(t), q); err != nil {
		t.Fatal(err)
	}
	q.GenerationPending.Binding = "changed"
	if err := s.checkGenerationPending(context.Background(), unitEnvelope(t), q); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("changed source accepted", err)
	}
	q.GenerationPending.Problem.ExpiresAt = time.Now().Add(-time.Second)
	s.topics = nil
	s.sources = nil
	if err := s.checkGenerationPending(context.Background(), unitEnvelope(t), q); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("expired origin touched dependency", err)
	}
}

func TestGenerationPendingRejectsExecutableState(t *testing.T) {
	_, q, _ := pendingFixture(t)
	for _, mutate := range []func(*QueryRecord){func(q *QueryRecord) { q.SQL = "SELECT 1" }, func(q *QueryRecord) { q.Status = "planned" }, func(q *QueryRecord) { q.Parameters = []exec.Parameter{{}} }, func(q *QueryRecord) { q.GenerationPending.Round = 0 }} {
		copy := q
		p := *q.GenerationPending
		copy.GenerationPending = &p
		mutate(&copy)
		if GenerationPendingValid(copy) {
			t.Fatal("pending acquired executable state")
		}
	}
	if validateQuestion(QuestionRequest{GenerationQuery: "pending"}) == nil {
		t.Fatal("resume markers accepted by unrelated flow")
	}
}

func TestGenerationPendingRunCannotEraseResumeOperation(t *testing.T) {
	e := unitEnvelope(t)
	q := unitQuery(e, "child", "topic", "v1", "context", false)
	q.Operation = "resume:pending"
	repo := newUnitRepository()
	repo.queries[q.ID] = q
	s := &Service{repo: repo}
	if _, err := s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "different"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("resume operation replaced before dependencies", err)
	}
}

func TestGenerationPendingChildReplayRevalidatesEvidence(t *testing.T) {
	s, child, request := pendingFixture(t)
	origin := child
	origin.ID = "original-pending"
	origin.Revision = 1
	child.Parent = origin.ID
	child.ParentRevision = 1
	child.ParentDigest = QueryLineageDigest(origin)
	answerDigest := exec.Hash("reviewed-answer")
	child.GenerationResolution = &GenerationResolution{QueryID: origin.ID, AnswerDigest: answerDigest}
	ctx := withGenerationContinuation(context.Background(), &generationContinuation{origin: &origin, answerDigest: answerDigest})
	if _, err, handled := generationResolutionReplay(ctx, child); !handled || GenerationProblem(err) == nil {
		t.Fatal("fixture does not exercise pending decision branch", err)
	}
	for _, mutate := range []func(*QueryRecord){
		func(q *QueryRecord) { q.EvidenceStale = true },
		func(q *QueryRecord) { q.GenerationPending.Problem.ExpiresAt = time.Now().Add(-time.Second) },
		func(q *QueryRecord) { q.GenerationPending.Binding = exec.Hash("foreign-source") },
	} {
		q := child
		p := *child.GenerationPending
		q.GenerationPending = &p
		mutate(&q)
		if _, err := s.reusablePlanResult(ctx, q, unitEnvelope(t), request, q.Operation, q.Parent); err == nil || GenerationProblem(err) != nil {
			t.Fatal("stale child decision replayed", err)
		}
	}
}
