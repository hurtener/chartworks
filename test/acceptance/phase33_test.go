package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase33(t *testing.T) {
	t.Run("AC01", testPhase33APIJourney)
	t.Run("AC02", testPhase33RecoveryAndCancellation)
	t.Run("AC03", testPhase33EvidenceAndUnresolved)
	t.Run("AC04", testPhase33Authority)
	t.Run("AC05", testPhase33HumanGates)
	t.Run("AC06", testPhase33TransformationChoice)
	t.Run("AC07", testPhase33DriftAmendment)
	t.Run("AC08", testPhase33BudgetsLocaleAndConcurrency)
}

type phase33Adapter struct {
	mu    sync.Mutex
	calls map[onboarding.Stage]int
	fail  map[onboarding.Stage]bool
	usage onboarding.Usage
}

func newPhase33Adapter() *phase33Adapter {
	return &phase33Adapter{calls: map[onboarding.Stage]int{}, fail: map[onboarding.Stage]bool{}}
}
func (a *phase33Adapter) step(stage onboarding.Stage, r onboarding.Run) (onboarding.StepResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[stage]++
	if a.fail[stage] {
		a.fail[stage] = false
		return onboarding.StepResult{}, store.ErrUnavailable
	}
	result := onboarding.StepResult{Usage: a.usage}
	switch stage {
	case onboarding.StageConnect:
		result.References = []onboarding.Reference{{Kind: "source", ID: r.Input.Source, Revision: 1}}
	case onboarding.StageInspect:
		result.References = []onboarding.Reference{{Kind: "dataset", ID: r.Input.Dataset, Revision: 1}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "column", Basis: []string{"catalog_revision:1"}, Confidence: "observed"}}
	case onboarding.StageProfile:
		result.References = []onboarding.Reference{{Kind: "profile", ID: r.Input.Profile, Revision: 1, Private: true}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "profile_column", Basis: []string{"bounded_sample"}, Confidence: "observed", Uncertainty: "currency and null meaning require review"}}
		if r.Input.Transformation && phase33AnswerValue(r.Answers, "approve_transformation") == "" {
			result.Questions = []onboarding.Question{{ID: "approve_transformation", Prompt: "Review managed transformation", Evidence: []string{"managed_write_review_required"}, Required: true}}
		}
	case onboarding.StageSemantic:
		result.References = []onboarding.Reference{{Kind: "topic_draft", ID: r.Input.Topic, Revision: 1, Digest: strings.Repeat("a", 64), Private: true}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "measure", Basis: []string{"profile:" + r.Input.Profile}, Confidence: "unresolved", Uncertainty: "unit, grain and null semantics require review", Sensitive: true}}
		if phase33AnswerValue(r.Answers, "grain") == "" {
			result.Questions = []onboarding.Question{{ID: "grain", Prompt: "Confirm grain", Evidence: []string{"profile:" + r.Input.Profile}, Required: true}, {ID: "kpis", Prompt: "Confirm units and KPIs", Evidence: []string{"profile:" + r.Input.Profile}, Required: true}}
		}
	case onboarding.StageProposals:
		result.References = []onboarding.Reference{{Kind: "query_proposal", ID: r.ID + "-query", Private: true}, {Kind: "block_proposal", ID: r.Input.Block, Private: true}, {Kind: "report_proposal", ID: r.Input.Report, Private: true}}
	}
	return result, nil
}
func (a *phase33Adapter) Connect(_ context.Context, _ identity.Envelope, in onboarding.StartRequest, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageConnect, onboarding.Run{Input: in})
}
func (a *phase33Adapter) Inspect(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageInspect, r)
}
func (a *phase33Adapter) Profile(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageProfile, r)
}
func (a *phase33Adapter) DraftSemantics(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageSemantic, r)
}
func (a *phase33Adapter) PublishReviewed(_ context.Context, _ identity.Envelope, r onboarding.Run, review onboarding.ReviewReference, _ string) (onboarding.StepResult, error) {
	a.mu.Lock()
	a.calls[onboarding.StageReview]++
	a.mu.Unlock()
	return onboarding.StepResult{References: []onboarding.Reference{{Kind: "topic", ID: r.Input.Topic, Revision: 1, Digest: review.Digest}}, Evidence: []onboarding.Evidence{{Entity: r.Input.Topic, Kind: "publication", Basis: []string{"independent_review:" + review.ID}, Confidence: "observed"}}}, nil
}
func (a *phase33Adapter) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageProposals, r)
}
func (a *phase33Adapter) ProposeDriftAmendment(_ context.Context, _ identity.Envelope, r onboarding.Run, in onboarding.DriftRequest, _ string) (onboarding.Amendment, error) {
	return onboarding.Amendment{Run: r.ID, SourceRevision: in.SourceRevision, Affected: []onboarding.Reference{{Kind: "topic", ID: r.Input.Topic, Revision: 1}}, Proposal: onboarding.Reference{Kind: "topic_amendment", ID: r.Input.Topic + "-amend", Private: true}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

type phase33Fixture struct {
	service *onboarding.Service
	adapter *phase33Adapter
	e       identity.Envelope
	client  *sdk.Client
	other   identity.Envelope
}

func phase33Scopes(id string) []string {
	return []string{"onboarding.read", "onboarding.write", "onboarding.cancel", "cw.tenant.write:*", "cw.onboarding.read:" + id, "cw.onboarding.write:" + id, "cw.onboarding.cancel:" + id, "cw.source.read:source-a", "cw.execution_context.use:context-a"}
}
func newPhase33Fixture(t *testing.T, id, locale string) *phase33Fixture {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	tokens := newTokenFixture(t)
	scopes := phase33Scopes(id)
	e := tokens.envelope(t, "tenant-a", "author", scopes...)
	other := tokens.envelope(t, "tenant-a", "other", scopes...)
	adapter := newPhase33Adapter()
	service, err := onboarding.New(db, adapter, onboarding.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	handler := onboardingapi.Handler(tokens.verifier, service, http.NotFoundHandler())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	bearer := tokens.sign(t, tokens.claims("tenant-a", "author", scopes), nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	return &phase33Fixture{service: service, adapter: adapter, e: e, client: client, other: other}
}
func phase33Start(id, locale string) onboarding.StartRequest {
	return onboarding.StartRequest{ID: id, Key: id + "-key", Mode: onboarding.ModeConnect, Locale: locale, Source: "source-a", Context: "context-a", Dataset: "sales", Profile: "profile-a", Topic: "commerce", TopicVersion: "commerce-v1", Block: "sales-summary", Report: "weekly-sales"}
}

func phase33AnswerValue(answers []onboarding.Answer, id string) string {
	for _, answer := range answers {
		if answer.ID == id {
			return answer.Value
		}
	}
	return ""
}

func phase33Answers(questions []onboarding.Question, value string) []onboarding.Answer {
	answers := make([]onboarding.Answer, 0, len(questions))
	for _, question := range questions {
		answers = append(answers, onboarding.Answer{ID: question.ID, Value: value})
	}
	return answers
}

func advancePhase33(t *testing.T, f *phase33Fixture, r onboarding.Run) onboarding.Run {
	t.Helper()
	ctx := context.Background()
	var err error
	for r.Status != onboarding.StatusComplete {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "reviewed explicit answer")
			if r.Stage == onboarding.StageReview {
				r, err = f.service.Answer(ctx, f.e, r.ID, onboarding.AnswerRequest{ExpectedVersion: r.Version, Review: &onboarding.ReviewReference{ID: "review-a", Revision: 1, Digest: strings.Repeat("a", 64)}})
			} else {
				r, err = f.service.Answer(ctx, f.e, r.ID, onboarding.AnswerRequest{ExpectedVersion: r.Version, Answers: answers})
			}
		} else {
			r, err = f.service.Resume(ctx, f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
		}
		if err != nil {
			t.Fatal("advance", r.Stage, r.Status, err)
		}
	}
	return r
}

func testPhase33APIJourney(t *testing.T) {
	f := newPhase33Fixture(t, "journey", "en")
	r, err := f.client.StartOnboarding(context.Background(), phase33Start("journey", "en"))
	if err != nil {
		t.Fatal(err)
	}
	for r.Status != onboarding.StatusComplete {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "reviewed")
			request := onboarding.AnswerRequest{ExpectedVersion: r.Version, Answers: answers}
			if r.Stage == onboarding.StageReview {
				request.Review = &onboarding.ReviewReference{ID: "review-a", Revision: 1, Digest: strings.Repeat("a", 64)}
			}
			r, err = f.client.AnswerOnboarding(context.Background(), r.ID, request)
		} else {
			r, err = f.client.ResumeOnboarding(context.Background(), r.ID, r.Version)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if r.RequiredAction != "review_blocks" || r.Progress.Percent != 100 || len(r.References) < 6 {
		t.Fatal("incomplete API journey", r)
	}
}
func testPhase33RecoveryAndCancellation(t *testing.T) {
	f := newPhase33Fixture(t, "recovery", "en")
	in := phase33Start("recovery", "en")
	a, err := f.service.Start(context.Background(), f.e, in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.service.Start(context.Background(), f.e, in)
	if err != nil || a.Version != b.Version {
		t.Fatal("start replay duplicated", b, err)
	}
	a, err = f.service.Resume(context.Background(), f.e, a.ID, onboarding.ResumeRequest{ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	f.adapter.fail[onboarding.StageInspect] = true
	failed, err := f.service.Resume(context.Background(), f.e, a.ID, onboarding.ResumeRequest{ExpectedVersion: a.Version})
	if err == nil || failed.Status != onboarding.StatusFailed {
		t.Fatal("failure not durable", failed, err)
	}
	resumed, err := f.service.Resume(context.Background(), f.e, failed.ID, onboarding.ResumeRequest{ExpectedVersion: failed.Version})
	if err != nil || resumed.Stage != onboarding.StageProfile {
		t.Fatal("failed stage not resumable", resumed, err)
	}
	cancelled, err := f.service.Cancel(context.Background(), f.e, resumed.ID, onboarding.CancelRequest{ExpectedVersion: resumed.Version, Reason: "stop requested"})
	if err != nil || cancelled.Status != onboarding.StatusCancelled {
		t.Fatal(cancelled, err)
	}
	if _, err = f.service.Resume(context.Background(), f.e, cancelled.ID, onboarding.ResumeRequest{ExpectedVersion: cancelled.Version}); !errors.Is(err, onboarding.ErrCancelled) {
		t.Fatal("cancelled work resumed", err)
	}
}
func testPhase33EvidenceAndUnresolved(t *testing.T) {
	f := newPhase33Fixture(t, "evidence", "en")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("evidence", "en"))
	for r.Stage != onboarding.StageSemantic || r.Status != onboarding.StatusAttention {
		var err error
		r, err = f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(r.Evidence) < 3 || len(r.Questions) < 2 || r.Evidence[len(r.Evidence)-1].Confidence != "unresolved" || !r.Evidence[len(r.Evidence)-1].Sensitive {
		t.Fatal("rich uncertainty missing", r.Evidence, r.Questions)
	}
}
func testPhase33Authority(t *testing.T) {
	f := newPhase33Fixture(t, "authority", "en")
	if _, err := f.service.Start(context.Background(), identity.Envelope{}, phase33Start("authority", "en")); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("unsigned start", err)
	}
	r, err := f.service.Start(context.Background(), f.e, phase33Start("authority", "en"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Get(context.Background(), f.other, r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("other actor read private run", err)
	}
	narrow, _ := identity.FromVerified("tenant-a", "author", "test-session", []string{"onboarding.write", "cw.tenant.write:*", "cw.onboarding.write:authority"}, time.Now().Add(time.Minute), nil)
	if _, err = f.service.Start(context.Background(), narrow, phase33Start("authority", "en")); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("source reach widened", err)
	}
	if _, err = f.service.Resume(context.Background(), narrow, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("continued run after source reach was removed", err)
	}
}
func testPhase33HumanGates(t *testing.T) {
	f := newPhase33Fixture(t, "gates", "en")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("gates", "en"))
	for r.Stage != onboarding.StageReview {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "reviewed")
			r, _ = f.service.Answer(context.Background(), f.e, r.ID, onboarding.AnswerRequest{ExpectedVersion: r.Version, Answers: answers})
		} else {
			r, _ = f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
		}
	}
	if r.Status != onboarding.StatusAttention || r.RequiredAction != "review_topic" {
		t.Fatal("publication was automatic", r)
	}
	if _, err := f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version}); !errors.Is(err, onboarding.ErrAttention) {
		t.Fatal("review gate bypassed", err)
	}
	r, err := f.service.Answer(context.Background(), f.e, r.ID, onboarding.AnswerRequest{ExpectedVersion: r.Version, Review: &onboarding.ReviewReference{ID: "review-a", Revision: 1, Digest: strings.Repeat("a", 64)}})
	if err != nil || r.Stage != onboarding.StageProposals {
		t.Fatal(r, err)
	}
	r, err = f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
	if err != nil || r.Status != onboarding.StatusComplete || r.RequiredAction != "review_blocks" {
		t.Fatal("block certification was not separate", r, err)
	}
}
func testPhase33TransformationChoice(t *testing.T) {
	f := newPhase33Fixture(t, "transform", "en")
	in := phase33Start("transform", "en")
	in.Transformation = true
	in.TransformationProposal = "managed-transform"
	r, _ := f.service.Start(context.Background(), f.e, in)
	for r.Stage != onboarding.StageProfile || r.Status != onboarding.StatusAttention {
		var err error
		r, err = f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	if r.RequiredAction != "review_transformation" || len(r.Questions) == 0 {
		t.Fatal("managed write was not reviewed", r)
	}
	direct := newPhase33Fixture(t, "direct", "en")
	d, _ := direct.service.Start(context.Background(), direct.e, phase33Start("direct", "en"))
	for d.Stage != onboarding.StageSemantic {
		d, _ = direct.service.Resume(context.Background(), direct.e, d.ID, onboarding.ResumeRequest{ExpectedVersion: d.Version})
	}
	if d.RequiredAction == "review_transformation" {
		t.Fatal("usable source forced materialization")
	}
}
func testPhase33DriftAmendment(t *testing.T) {
	f := newPhase33Fixture(t, "drift", "en")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("drift", "en"))
	r = advancePhase33(t, f, r)
	a, err := f.service.Drift(context.Background(), f.e, r.ID, onboarding.DriftRequest{ExpectedVersion: r.Version, SourceRevision: 2, Observation: "schema-observation-2"})
	if err != nil || !a.ExistingIntact || a.RequiredAction != "review_amendment" || !a.Proposal.Private || len(a.Affected) == 0 {
		t.Fatal(a, err)
	}
	current, err := f.service.Get(context.Background(), f.e, r.ID)
	if err != nil || current.Version != r.Version+1 || current.Status != onboarding.StatusComplete || len(current.Amendments) != 1 || current.Amendments[0].Proposal.ID != a.Proposal.ID {
		t.Fatal("drift amendment was not durably isolated from active definitions", current, err)
	}
}
func testPhase33BudgetsLocaleAndConcurrency(t *testing.T) {
	f := newPhase33Fixture(t, "bounded", "es")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("bounded", "es"))
	if !strings.Contains(r.Message, "fuente") {
		t.Fatal("Spanish status missing", r.Message)
	}
	f.adapter.usage = onboarding.Usage{Tokens: 25000}
	if _, err := f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version}); !errors.Is(err, onboarding.ErrBudget) {
		t.Fatal("token budget bypass", err)
	}
	f.adapter.usage = onboarding.Usage{}
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, err := f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, store.ErrConflict) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent CAS", success, conflict)
	}
}
