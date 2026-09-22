package onboarding

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type memoryRepository struct {
	mu      sync.Mutex
	runs    map[string]Run
	digests map[string]string
}

func (m *memoryRepository) CreateOnboarding(_ context.Context, _ identity.Envelope, run Run, digest string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prior, ok := m.runs[run.ID]; ok {
		if m.digests[run.ID] != digest {
			return Run{}, store.ErrConflict
		}
		return prior, nil
	}
	m.runs[run.ID], m.digests[run.ID] = run, digest
	return run, nil
}
func (m *memoryRepository) ReadOnboarding(_ context.Context, _ identity.Envelope, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return Run{}, store.ErrNotFound
	}
	return run, nil
}
func (m *memoryRepository) SaveOnboarding(_ context.Context, _ identity.Envelope, run Run, expected int64) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runs[run.ID]
	if !ok {
		return Run{}, store.ErrNotFound
	}
	if current.Version != expected || run.Version != expected {
		return Run{}, store.ErrConflict
	}
	run.Version++
	m.runs[run.ID] = run
	return run, nil
}

type serviceAdapter struct {
	fail   bool
	tokens int
}

func (a *serviceAdapter) result(stage Stage, run Run) (StepResult, error) {
	if a.fail {
		a.fail = false
		return StepResult{}, store.ErrUnavailable
	}
	switch stage {
	case StageConnect:
		return StepResult{References: []Reference{{Kind: "source", ID: run.Input.Source, Revision: 1}}, Evidence: []Evidence{{Entity: run.Input.Source, Kind: "connectivity", Basis: []string{"registered"}, Confidence: "observed"}}, Usage: Usage{Entities: 1, Tokens: a.tokens}}, nil
	case StageInspect:
		return StepResult{References: []Reference{{Kind: "dataset", ID: run.Input.Dataset, Revision: 1}}, Evidence: []Evidence{{Entity: "amount", Kind: "column", Basis: []string{"catalog"}, Confidence: "observed"}}, Usage: Usage{Entities: 1}}, nil
	case StageProfile:
		return StepResult{References: []Reference{{Kind: "profile", ID: run.Input.Profile, Revision: 1, Private: true}}, Usage: Usage{Entities: 1}}, nil
	case StageSemantic:
		questions := []Question{}
		if answerValue(run.Answers, "grain") == "" {
			questions = append(questions, Question{ID: "grain", Prompt: "Confirm the grain", Evidence: []string{"profile"}, Required: true})
		}
		return StepResult{References: []Reference{{Kind: "topic_draft", ID: run.Input.Topic, Revision: 1, Digest: strings.Repeat("a", 64), Private: true}}, Questions: questions, Usage: Usage{Entities: 1}}, nil
	case StageProposals:
		return StepResult{References: []Reference{{Kind: "query_proposal", ID: run.ID + "-query", Private: true}, {Kind: "block_proposal", ID: run.Input.Block, Private: true}, {Kind: "report_proposal", ID: run.Input.Report, Private: true}}, Usage: Usage{Entities: 3}}, nil
	default:
		return StepResult{}, ErrInvalid
	}
}
func (a *serviceAdapter) Connect(_ context.Context, _ identity.Envelope, in StartRequest, _ string) (StepResult, error) {
	return a.result(StageConnect, Run{Input: in})
}
func (a *serviceAdapter) Inspect(_ context.Context, _ identity.Envelope, run Run, _ string) (StepResult, error) {
	return a.result(StageInspect, run)
}
func (a *serviceAdapter) Profile(_ context.Context, _ identity.Envelope, run Run, _ string) (StepResult, error) {
	return a.result(StageProfile, run)
}
func (a *serviceAdapter) DraftSemantics(_ context.Context, _ identity.Envelope, run Run, _ string) (StepResult, error) {
	return a.result(StageSemantic, run)
}
func (a *serviceAdapter) PublishReviewed(_ context.Context, _ identity.Envelope, run Run, review ReviewReference, _ string) (StepResult, error) {
	return StepResult{References: []Reference{{Kind: "topic", ID: run.Input.Topic, Revision: 1, Digest: review.Digest}}, Evidence: []Evidence{{Entity: run.Input.Topic, Kind: "publication", Basis: []string{"review:" + review.ID}, Confidence: "observed"}}, Usage: Usage{Entities: 1}}, nil
}
func (a *serviceAdapter) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, run Run, _ string) (StepResult, error) {
	return a.result(StageProposals, run)
}
func (a *serviceAdapter) ProposeDriftAmendment(_ context.Context, _ identity.Envelope, run Run, in DriftRequest, _ string) (Amendment, error) {
	return Amendment{Run: run.ID, SourceRevision: in.SourceRevision, Affected: []Reference{{Kind: "topic", ID: run.Input.Topic}}, Proposal: Reference{Kind: "topic_amendment", ID: run.Input.Topic + "-amend", Private: true}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

func serviceEnvelope(t *testing.T, id string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"onboarding.read", "onboarding.write", "onboarding.cancel", "cw.tenant.write:*", "cw.onboarding.read:" + id, "cw.onboarding.write:" + id, "cw.onboarding.cancel:" + id, "cw.source.read:source", "cw.execution_context.use:context"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func serviceStart(id string) StartRequest {
	return StartRequest{ID: id, Key: id + "-key", Mode: ModeConnect, Locale: "en", Source: "source", Context: "context", Dataset: "dataset", Profile: "profile", Topic: "topic", TopicVersion: "topic-v1", Block: "block", Report: "report"}
}

func TestServiceJourneyRecoveryAndDrift(t *testing.T) {
	repo := &memoryRepository{runs: map[string]Run{}, digests: map[string]string{}}
	adapter := &serviceAdapter{fail: true}
	service, err := New(repo, adapter, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	e := serviceEnvelope(t, "journey")
	run, err := service.Start(t.Context(), e, serviceStart("journey"))
	if err != nil {
		t.Fatal(err)
	}
	if replay, replayErr := service.Start(t.Context(), e, serviceStart("journey")); replayErr != nil || replay.Version != run.Version {
		t.Fatal("start replay", replay, replayErr)
	}
	failed, err := service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
	if !errors.Is(err, store.ErrUnavailable) || failed.Status != StatusFailed {
		t.Fatal("failed stage receipt", failed, err)
	}
	run = failed
	for run.Stage != StageSemantic {
		run, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
	if err != nil || run.Status != StatusAttention || run.RequiredAction != "answer_questions" {
		t.Fatal("semantic gate", run, err)
	}
	if _, answerErr := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "unrequested", Value: "invented"}}}); !errors.Is(answerErr, ErrInvalid) {
		t.Fatal("unrequested answer accepted", answerErr)
	}
	run, err = service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "grain", Value: "one row per order"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
	if err != nil || run.Stage != StageReview || run.RequiredAction != "review_topic" {
		t.Fatal("topic review gate", run, err)
	}
	if _, reviewErr := service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version}); !errors.Is(reviewErr, ErrAttention) {
		t.Fatal("review gate resumed without review", reviewErr)
	}
	run, err = service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Review: &ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
	if err != nil || run.Status != StatusComplete || run.RequiredAction != "review_blocks" || run.Usage.Stages != 7 {
		t.Fatal("completion", run, err)
	}
	amendment, err := service.Drift(t.Context(), e, run.ID, DriftRequest{ExpectedVersion: run.Version, SourceRevision: 2, Observation: "catalog-v2"})
	if err != nil || amendment.RunVersion != run.Version+1 {
		t.Fatal("drift", amendment, err)
	}
	if replay, replayErr := service.Drift(t.Context(), e, run.ID, DriftRequest{ExpectedVersion: run.Version, SourceRevision: 2, Observation: "catalog-v2"}); replayErr != nil || replay.Proposal.ID != amendment.Proposal.ID {
		t.Fatal("drift replay", replay, replayErr)
	}
}

func TestServiceAuthorityBudgetAndCancellation(t *testing.T) {
	repo := &memoryRepository{runs: map[string]Run{}, digests: map[string]string{}}
	adapter := &serviceAdapter{}
	limits := DefaultLimits()
	limits.MaxTokens = 1
	service, _ := New(repo, adapter, limits)
	e := serviceEnvelope(t, "bounded")
	run, err := service.Start(t.Context(), e, serviceStart("bounded"))
	if err != nil {
		t.Fatal(err)
	}
	adapter.tokens = 2
	if _, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version}); !errors.Is(err, ErrBudget) {
		t.Fatal("token budget widened", err)
	}
	cancelled, err := service.Cancel(t.Context(), e, run.ID, CancelRequest{ExpectedVersion: run.Version, Reason: "operator stopped setup"})
	if err != nil || cancelled.CancellationReason == "" || cancelled.Status != StatusCancelled {
		t.Fatal(cancelled, err)
	}
	if _, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: cancelled.Version}); !errors.Is(err, ErrCancelled) {
		t.Fatal("cancelled run resumed", err)
	}
	narrow, _ := identity.FromVerified("tenant", "actor", "session", []string{"onboarding.write", "cw.tenant.write:*", "cw.onboarding.write:denied"}, time.Now().Add(time.Hour), nil)
	if _, err = service.Start(t.Context(), narrow, serviceStart("denied")); err == nil {
		t.Fatal("source/context authority widened")
	}
}

func TestServiceValidationAndReads(t *testing.T) {
	if _, err := New(nil, nil, DefaultLimits()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	repo := &memoryRepository{runs: map[string]Run{}, digests: map[string]string{}}
	service, _ := New(repo, &serviceAdapter{}, DefaultLimits())
	e := serviceEnvelope(t, "readable")
	in := serviceStart("readable")
	run, err := service.Start(t.Context(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	if got, getErr := service.Get(t.Context(), e, run.ID); getErr != nil || got.ID != run.ID {
		t.Fatal(got, getErr)
	}
	changed := in
	changed.Report = "different"
	if _, err = service.Start(t.Context(), e, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	bad := serviceStart("bad")
	bad.Transformation = true
	if _, err = service.Start(t.Context(), serviceEnvelope(t, "bad"), bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbound transformation accepted", err)
	}
	answers := []Answer{{ID: "same", Value: "old"}}
	answers = setAnswer(answers, Answer{ID: "same", Value: "new"})
	if len(answers) != 1 || answerValue(answers, "same") != "new" || validText("bad\x00", 10) {
		t.Fatal("bounded answer helpers")
	}
}
