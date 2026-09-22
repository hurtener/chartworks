package onboarding

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
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
	fail        bool
	tokens      int
	authorities []RunAuthority
}

func (a *serviceAdapter) ResolveRunAuthority(_ context.Context, _ identity.Envelope, r Run) ([]RunAuthority, error) {
	if len(a.authorities) > 0 {
		return append([]RunAuthority(nil), a.authorities...), nil
	}
	revision, exact := int64(1), true
	if r.Status == StatusComplete {
		revision, exact = 2, false
	}
	return []RunAuthority{{Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Revision: revision, Exact: exact}}, nil
}

type blockingAdapter struct {
	serviceAdapter
	stage   Stage
	entered chan string
	release chan struct{}
	mu      sync.Mutex
	keys    []string
}

func (a *blockingAdapter) wait(stage Stage, key string) {
	if a.stage != stage {
		return
	}
	a.mu.Lock()
	a.keys = append(a.keys, key)
	a.mu.Unlock()
	a.entered <- key
	<-a.release
}
func (a *blockingAdapter) Connect(ctx context.Context, e identity.Envelope, in StartRequest, key string) (StepResult, error) {
	a.wait(StageConnect, key)
	return a.serviceAdapter.Connect(ctx, e, in, key)
}
func (a *blockingAdapter) Inspect(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	return a.serviceAdapter.Inspect(ctx, e, r, key)
}
func (a *blockingAdapter) Profile(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	return a.serviceAdapter.Profile(ctx, e, r, key)
}
func (a *blockingAdapter) DraftSemantics(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	return a.serviceAdapter.DraftSemantics(ctx, e, r, key)
}
func (a *blockingAdapter) PublishReviewed(ctx context.Context, e identity.Envelope, r Run, review ReviewReference, key string) (StepResult, error) {
	a.wait(StageReview, key)
	return a.serviceAdapter.PublishReviewed(ctx, e, r, review, key)
}
func (a *blockingAdapter) ProposeQueriesBlocksReports(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	return a.serviceAdapter.ProposeQueriesBlocksReports(ctx, e, r, key)
}
func (a *blockingAdapter) ProposeDriftAmendment(ctx context.Context, e identity.Envelope, r Run, in DriftRequest, key string) (Amendment, error) {
	return a.serviceAdapter.ProposeDriftAmendment(ctx, e, r, in, key)
}

func usageReceipt(tokens int) gateway.Receipt {
	if tokens == 0 {
		return gateway.Receipt{}
	}
	in, out := tokens/2, tokens-tokens/2
	return gateway.Receipt{Calls: []gateway.Usage{{Attempts: 1, InputTokens: &in, OutputTokens: &out}}}
}

func (a *serviceAdapter) result(stage Stage, run Run) (StepResult, error) {
	if a.fail {
		a.fail = false
		return StepResult{}, store.ErrUnavailable
	}
	switch stage {
	case StageConnect:
		return StepResult{References: []Reference{{Kind: "source", ID: run.Input.Source, Revision: 1, SourceRevision: 1, Source: run.Input.Source, Context: run.Input.Context}}, Evidence: []Evidence{{Entity: run.Input.Source, Kind: "connectivity", Basis: []string{"registered"}, Confidence: "observed"}}, Receipt: usageReceipt(a.tokens)}, nil
	case StageInspect:
		return StepResult{References: []Reference{{Kind: "dataset", ID: run.Input.Dataset, Revision: 1, SourceRevision: 1, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Columns: []string{"amount"}}}, Evidence: []Evidence{{Entity: "amount", Kind: "column", Basis: []string{"catalog", "source:" + run.Input.Source, "dataset:" + run.Input.Dataset, "schema_digest:old"}, Confidence: "observed"}}}, nil
	case StageProfile:
		return StepResult{References: []Reference{{Kind: "profile", ID: run.Input.Profile, Revision: 1, SourceRevision: 1, Private: true, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Columns: []string{"amount"}}}}, nil
	case StageSemantic:
		questions := []Question{}
		if answerValue(run.Answers, "grain") == "" {
			questions = append(questions, Question{ID: "grain", Prompt: "Confirm the grain", Evidence: []string{"profile"}, Required: true})
		}
		return StepResult{References: []Reference{{Kind: "topic_draft", ID: run.Input.Topic, Revision: 1, SourceRevision: 1, Digest: strings.Repeat("a", 64), Private: true, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Columns: []string{"amount"}, DependsOn: []string{"profile:" + run.Input.Profile}}}, Questions: questions}, nil
	case StageProposals:
		return StepResult{References: []Reference{{Kind: "onboarding_query_intent", ID: run.ID + "-query", SourceRevision: 1, Private: true, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, DependsOn: []string{"topic:" + run.Input.Topic}}, {Kind: "onboarding_block_intent", ID: run.Input.Block, SourceRevision: 1, Private: true, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, DependsOn: []string{"topic:" + run.Input.Topic}}, {Kind: "onboarding_report_intent", ID: run.Input.Report, SourceRevision: 1, Private: true, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, DependsOn: []string{"topic:" + run.Input.Topic}}}}, nil
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
	return StepResult{References: []Reference{{Kind: "topic", ID: run.Input.Topic, Revision: 1, SourceRevision: 1, Digest: review.Digest, Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Columns: []string{"amount"}, DependsOn: []string{"topic_draft:" + run.Input.Topic}}}, Evidence: []Evidence{{Entity: run.Input.Topic, Kind: "publication", Basis: []string{"review:" + review.ID}, Confidence: "observed"}}, Receipt: usageReceipt(max(a.tokens, 1))}, nil
}
func (a *serviceAdapter) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, run Run, _ string) (StepResult, error) {
	return a.result(StageProposals, run)
}
func (a *serviceAdapter) ProposeDriftAmendment(_ context.Context, _ identity.Envelope, run Run, in DriftRequest, _ string) (Amendment, error) {
	effective, _ := effectiveDriftReference(run)
	contextID, revision := effective.Context, effective.SourceRevision+1
	for _, coordinate := range a.authorities {
		if coordinate.Source == effective.Source && coordinate.Dataset == effective.Dataset {
			contextID, revision = coordinate.Context, coordinate.Revision
		}
	}
	return Amendment{Run: run.ID, Observation: "schema_changed", Source: effective.Source, Context: contextID, Dataset: effective.Dataset, SourceRevision: revision, Changes: []string{"amount"}, Affected: []Reference{{Kind: "topic", ID: run.Input.Topic}}, ImpactEvidence: []ImpactEvidence{{Kind: "topic", ID: run.Input.Topic, Basis: []string{"column:amount"}}}, Proposal: Reference{Kind: "topic_amendment", ID: run.Input.Topic + "-amend", SourceRevision: revision, Private: true, Source: effective.Source, Context: contextID, Dataset: effective.Dataset}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

func serviceEnvelope(t *testing.T, id string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"onboarding.read", "onboarding.write", "onboarding.cancel", "cw.tenant.write:*", "cw.onboarding.read:" + id, "cw.onboarding.write:" + id, "cw.onboarding.cancel:" + id, "cw.source.read:*", "cw.execution_context.use:*", "cw.dataset.query:*"}, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func serviceRevokedEnvelope(t *testing.T, id string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"onboarding.read", "onboarding.cancel", "cw.onboarding.read:" + id, "cw.onboarding.cancel:" + id, "cw.source.read:other-source", "cw.execution_context.use:other-context"}, time.Now().Add(time.Hour), nil)
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
	if _, answerErr := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "unrequested", Decision: "confirmed_external"}}}); !errors.Is(answerErr, ErrInvalid) {
		t.Fatal("unrequested answer accepted", answerErr)
	}
	if _, answerErr := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "grain", Decision: "SELECT * FROM secret"}}}); !errors.Is(answerErr, ErrInvalid) {
		t.Fatal("free-text semantic answer accepted", answerErr)
	}
	if _, answerErr := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "grain", Decision: "confirmed_external", Reference: "postgres://credential"}}}); !errors.Is(answerErr, ErrInvalid) {
		t.Fatal("credential-like reference accepted", answerErr)
	}
	run, err = service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "grain", Decision: "confirmed_external", Reference: "reviewed-grain"}}})
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
	amendment, err := service.Drift(t.Context(), e, run.ID, DriftRequest{ExpectedVersion: run.Version})
	if err != nil || amendment.RunVersion != run.Version+1 {
		t.Fatal("drift", amendment, err)
	}
	if replay, replayErr := service.Drift(t.Context(), e, run.ID, DriftRequest{ExpectedVersion: run.Version}); replayErr != nil || replay.Proposal.ID != amendment.Proposal.ID {
		t.Fatal("drift replay", replay, replayErr)
	}
}

func TestServiceTransformedOutputAuthorityGuardsEveryContinuation(t *testing.T) {
	makeRun := func(id string, stage Stage, status Status) Run {
		in := serviceStart(id)
		return Run{ID: id, Key: id + "-key", Version: 1, Stage: stage, Status: status, Locale: "en", Input: in, References: []Reference{
			{Kind: "dataset", ID: in.Dataset, Revision: 1, SourceRevision: 1, Source: in.Source, Context: in.Context, Dataset: in.Dataset, Columns: []string{"amount"}},
			{Kind: "profile", ID: in.Profile, Revision: 2, SourceRevision: 2, Private: true, Source: "managed-output", Context: "managed-output:v2", Dataset: "managed-dataset", Columns: []string{"amount"}},
			{Kind: "topic", ID: in.Topic, Revision: 1, SourceRevision: 2, Source: "managed-output", Context: "managed-output:v2", Dataset: "managed-dataset", Columns: []string{"amount"}, DependsOn: []string{"profile:" + in.Profile}},
		}, Evidence: []Evidence{{Entity: "amount", Kind: "profile_column", Basis: []string{"source:managed-output", "dataset:managed-dataset", "schema_digest:old"}, Confidence: "observed"}}, Limits: DefaultLimits(), Deadline: time.Now().Add(time.Hour), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}
	methods := []struct {
		name   string
		stage  Stage
		status Status
		call   func(*Service, identity.Envelope, Run) error
	}{
		{"resume", StageProfile, StatusReady, func(s *Service, e identity.Envelope, r Run) error {
			_, err := s.Resume(t.Context(), e, r.ID, ResumeRequest{ExpectedVersion: r.Version})
			return err
		}},
		{"answer-review", StageReview, StatusAttention, func(s *Service, e identity.Envelope, r Run) error {
			_, err := s.Answer(t.Context(), e, r.ID, AnswerRequest{ExpectedVersion: r.Version, Review: &ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}})
			return err
		}},
		{"drift", StageComplete, StatusComplete, func(s *Service, e identity.Envelope, r Run) error {
			_, err := s.Drift(t.Context(), e, r.ID, DriftRequest{ExpectedVersion: r.Version})
			return err
		}},
	}
	for _, tc := range methods {
		t.Run(tc.name+"-revoked", func(t *testing.T) {
			run := makeRun("output-"+strings.ReplaceAll(tc.name, "-", "_"), tc.stage, tc.status)
			repo := &memoryRepository{runs: map[string]Run{run.ID: run}, digests: map[string]string{}}
			adapter := &serviceAdapter{authorities: []RunAuthority{{Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Revision: 1, Exact: true}, {Source: "managed-output", Context: "managed-output:v2", Dataset: "managed-dataset", Revision: 2, Exact: true}}}
			service, _ := New(repo, adapter, DefaultLimits())
			scopes := []string{"onboarding.write", "cw.onboarding.write:" + run.ID, "cw.source.read:" + run.Input.Source, "cw.execution_context.use:" + run.Input.Context, "cw.dataset.query:" + run.Input.Dataset}
			e, _ := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), nil)
			if err := tc.call(service, e, run); !errors.Is(err, access.ErrNotFound) {
				t.Fatal("continuation accepted after managed output reach revocation", err)
			}
		})
	}
	for _, tc := range methods[:2] {
		t.Run(tc.name+"-rotated", func(t *testing.T) {
			run := makeRun("rotated-"+strings.ReplaceAll(tc.name, "-", "_"), tc.stage, tc.status)
			repo := &memoryRepository{runs: map[string]Run{run.ID: run}, digests: map[string]string{}}
			adapter := &serviceAdapter{authorities: []RunAuthority{{Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Revision: 1, Exact: true}, {Source: "managed-output", Context: "managed-output:v3", Dataset: "managed-dataset", Revision: 3, Exact: false}}}
			service, _ := New(repo, adapter, DefaultLimits())
			scopes := []string{"onboarding.write", "cw.onboarding.write:" + run.ID, "cw.source.read:*", "cw.execution_context.use:*", "cw.dataset.query:*"}
			e, _ := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), nil)
			if err := tc.call(service, e, run); !errors.Is(err, store.ErrConflict) {
				t.Fatal("continuation crossed transformed output revision fence", err)
			}
		})
	}
	t.Run("drift-uses-current-rotated-output", func(t *testing.T) {
		run := makeRun("rotated-drift", StageComplete, StatusComplete)
		repo := &memoryRepository{runs: map[string]Run{run.ID: run}, digests: map[string]string{}}
		adapter := &serviceAdapter{authorities: []RunAuthority{{Source: run.Input.Source, Context: run.Input.Context, Dataset: run.Input.Dataset, Revision: 1, Exact: true}, {Source: "managed-output", Context: "managed-output:v3", Dataset: "managed-dataset", Revision: 3, Exact: false}}}
		service, _ := New(repo, adapter, DefaultLimits())
		scopes := []string{"onboarding.write", "cw.onboarding.write:" + run.ID, "cw.source.read:*", "cw.execution_context.use:*", "cw.dataset.query:*"}
		e, _ := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), nil)
		amendment, err := service.Drift(t.Context(), e, run.ID, DriftRequest{ExpectedVersion: run.Version})
		if err != nil || amendment.Source != "managed-output" || amendment.Context != "managed-output:v3" || amendment.Dataset != "managed-dataset" || amendment.SourceRevision != 3 {
			t.Fatal("drift did not bind server-resolved managed output", amendment, err)
		}
	})
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
	if _, err = service.Cancel(t.Context(), e, run.ID, CancelRequest{ExpectedVersion: run.Version, Reason: "raw operator explanation"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("free-text cancellation reason accepted", err)
	}
	if _, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version}); !errors.Is(err, ErrBudget) {
		t.Fatal("token budget widened", err)
	}
	// The budget rejection leaves a durable lease that must reconcile before
	// cancellation becomes final.
	current, _ := service.Get(t.Context(), e, run.ID)
	cancelled, err := service.Cancel(t.Context(), e, run.ID, CancelRequest{ExpectedVersion: current.Version, Reason: "budget"})
	if err != nil || !cancelled.CancelRequested || cancelled.RequiredAction != "reconcile_cancellation" {
		t.Fatal(cancelled, err)
	}
	adapter.tokens = 0
	cancelled, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: cancelled.Version})
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
	revoked := serviceRevokedEnvelope(t, run.ID)
	if _, getErr := service.Get(t.Context(), revoked, run.ID); !errors.Is(getErr, access.ErrNotFound) {
		t.Fatal("run evidence survived source/context reach revocation", getErr)
	}
	if _, cancelErr := service.Cancel(t.Context(), revoked, run.ID, CancelRequest{ExpectedVersion: run.Version, Reason: "user_requested"}); !errors.Is(cancelErr, access.ErrNotFound) {
		t.Fatal("cancellation replay exposed run after reach revocation", cancelErr)
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
	answers := []Answer{{ID: "same", Decision: "unresolved"}}
	answers = setAnswer(answers, Answer{ID: "same", Decision: "confirmed_external"})
	if len(answers) != 1 || answerValue(answers, "same") != "confirmed_external" || validText("bad\x00", 10) {
		t.Fatal("bounded answer helpers")
	}
}

func TestServiceEffectLeaseCoordinatesCancellationAndRecovery(t *testing.T) {
	repo := &memoryRepository{runs: map[string]Run{}, digests: map[string]string{}}
	adapter := &blockingAdapter{stage: StageConnect, entered: make(chan string, 2), release: make(chan struct{}, 2)}
	service, _ := New(repo, adapter, DefaultLimits())
	e := serviceEnvelope(t, "leased")
	run, _ := service.Start(t.Context(), e, serviceStart("leased"))
	done := make(chan error, 1)
	go func() {
		_, err := service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
		done <- err
	}()
	firstKey := <-adapter.entered
	leased, err := service.Get(t.Context(), e, run.ID)
	if err != nil || leased.Lease == nil || leased.Status != StatusRunning {
		t.Fatal("effect ran without durable lease", leased, err)
	}
	cancelling, err := service.Cancel(t.Context(), e, run.ID, CancelRequest{ExpectedVersion: leased.Version, Reason: "user_requested"})
	if err != nil || !cancelling.CancelRequested || cancelling.Status != StatusRunning {
		t.Fatal("cancellation erased in-flight lease", cancelling, err)
	}
	adapter.release <- struct{}{}
	if err = <-done; !errors.Is(err, store.ErrConflict) {
		t.Fatal("in-flight result crossed cancellation fence", err)
	}
	go func() {
		_, err := service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: cancelling.Version})
		done <- err
	}()
	secondKey := <-adapter.entered
	if secondKey != firstKey {
		t.Fatal("recovery changed idempotency operation", firstKey, secondKey)
	}
	adapter.release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	final, _ := service.Get(t.Context(), e, run.ID)
	if final.Status != StatusCancelled || final.Lease != nil || !final.CancelRequested {
		t.Fatal("reconciled effect did not finalize cancellation", final)
	}
}

func TestServiceReviewLeaseCoordinatesAnswerAndCancel(t *testing.T) {
	repo := &memoryRepository{runs: map[string]Run{}, digests: map[string]string{}}
	adapter := &blockingAdapter{stage: StageComplete, entered: make(chan string, 2), release: make(chan struct{}, 2)}
	service, _ := New(repo, adapter, DefaultLimits())
	e := serviceEnvelope(t, "review-race")
	run, _ := service.Start(t.Context(), e, serviceStart("review-race"))
	for run.Stage != StageReview {
		var err error
		if run.Status == StatusAttention {
			run, err = service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: run.Version, Answers: []Answer{{ID: "grain", Decision: "confirmed_external"}}})
		} else {
			run, err = service.Resume(t.Context(), e, run.ID, ResumeRequest{ExpectedVersion: run.Version})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	adapter.stage = StageReview
	review := ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}
	done := make(chan error, 1)
	go func(expected int64) {
		_, err := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: expected, Review: &review})
		done <- err
	}(run.Version)
	firstKey := <-adapter.entered
	leased, _ := service.Get(t.Context(), e, run.ID)
	cancelling, err := service.Cancel(t.Context(), e, run.ID, CancelRequest{ExpectedVersion: leased.Version, Reason: "superseded"})
	if err != nil || cancelling.Lease == nil {
		t.Fatal(cancelling, err)
	}
	adapter.release <- struct{}{}
	if err = <-done; !errors.Is(err, store.ErrConflict) {
		t.Fatal("publication result crossed cancellation fence", err)
	}
	go func(expected int64) {
		_, err := service.Answer(t.Context(), e, run.ID, AnswerRequest{ExpectedVersion: expected, Review: &review})
		done <- err
	}(cancelling.Version)
	if secondKey := <-adapter.entered; secondKey != firstKey {
		t.Fatal("publication reconciliation changed operation", firstKey, secondKey)
	}
	adapter.release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	final, _ := service.Get(t.Context(), e, run.ID)
	if final.Status != StatusCancelled || len(final.Receipts) != 1 || final.Usage.ModelCalls != final.Limits.MaxModelCalls || final.Usage.Tokens != final.Limits.MaxTokens {
		t.Fatal("publication receipt not attached before final cancellation", final)
	}
}

func TestAccountingValidationAndLocalizedStatuses(t *testing.T) {
	negative := -1
	zero := 0
	if _, _, err := delegatedUsage(StepResult{Receipt: gateway.Receipt{Calls: []gateway.Usage{{Attempts: 1, InputTokens: &negative, OutputTokens: &zero}}}}, &Lease{ReservedCalls: 1, ReservedTokens: 10}); !errors.Is(err, ErrInvalid) {
		t.Fatal("negative provider usage accepted", err)
	}
	if _, _, err := delegatedUsage(StepResult{Receipt: gateway.Receipt{Calls: []gateway.Usage{{Attempts: 1}}}}, nil); !errors.Is(err, ErrBudget) {
		t.Fatal("unknown unreserved usage accepted", err)
	}
	if usage, receipt, err := delegatedUsage(StepResult{References: []Reference{{Kind: "source", ID: "source"}}}, &Lease{Stage: StageConnect}); err != nil || usage.Entities != 1 || receipt != nil {
		t.Fatal(usage, receipt, err)
	}
	limits := DefaultLimits()
	run := Run{Limits: limits, Usage: Usage{ModelCalls: 1, Tokens: 100}, Stage: StageReview}
	if calls, tokens, err := leaseReservation(run, StageReview); err != nil || calls != limits.MaxModelCalls-1 || tokens != limits.MaxTokens-100 {
		t.Fatal("review lease did not reserve the actual remaining gateway allowance", calls, tokens, err)
	}
	for _, step := range []StepResult{
		{References: []Reference{{Kind: "source", ID: "bad/id"}}},
		{Evidence: []Evidence{{Entity: "entity", Kind: "column", Basis: []string{"catalog"}, Confidence: "certain"}}},
		{Questions: []Question{{ID: "question", Prompt: "bad\x00", Evidence: []string{}}}},
	} {
		if err := validateStep(step, limits, Usage{}, Usage{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid delegated result accepted", step, err)
		}
	}
	if statusMessage("es", "cancelling") == statusMessage("en", "cancelling") || statusMessage("es", "cancelled") == statusMessage("en", "cancelled") || statusMessage("es", "failed") == statusMessage("en", "failed") {
		t.Fatal("localized statuses collapsed")
	}
}
