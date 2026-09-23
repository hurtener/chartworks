package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase33(t *testing.T) {
	t.Run("AC01", testPhase33RealDomainBoundary)
	t.Run("AC02", testPhase33RecoveryAndCancellation)
	t.Run("AC03", testPhase33EvidenceAndUnresolved)
	t.Run("AC04", testPhase33Authority)
	t.Run("AC05", testPhase33HumanGates)
	t.Run("AC06", testPhase33TransformationChoice)
	t.Run("AC07", testPhase33DriftAmendment)
	t.Run("AC08", testPhase33BudgetsLocaleAndConcurrency)
	t.Run("AC10", testPhase33PostgresLeaseRace)
}

type phase33NoAutopilot struct{}

func (phase33NoAutopilot) Get(context.Context, identity.Envelope, string) (engineering.AutopilotProposal, error) {
	return engineering.AutopilotProposal{}, store.ErrNotFound
}

type phase33RaceAdapter struct {
	*phase33FailureAdapter
	block   onboarding.Stage
	entered chan string
	release chan struct{}
}

func (a *phase33RaceAdapter) Connect(ctx context.Context, e identity.Envelope, in onboarding.StartRequest, key string) (onboarding.StepResult, error) {
	if a.block == onboarding.StageConnect {
		a.entered <- key
		<-a.release
	}
	return a.phase33FailureAdapter.Connect(ctx, e, in, key)
}

func (a *phase33RaceAdapter) PublishReviewed(ctx context.Context, e identity.Envelope, r onboarding.Run, review onboarding.ReviewReference, key string) (onboarding.StepResult, error) {
	if a.block == onboarding.StageReview {
		a.entered <- key
		<-a.release
	}
	return a.phase33FailureAdapter.PublishReviewed(ctx, e, r, review, key)
}

type phase33FailureAdapter struct {
	mu     sync.Mutex
	calls  map[onboarding.Stage]int
	fail   map[onboarding.Stage]bool
	tokens int
}

func (a *phase33FailureAdapter) ResolveRunAuthority(_ context.Context, _ identity.Envelope, r onboarding.Run) ([]onboarding.RunAuthority, error) {
	revision, exact := int64(1), true
	if r.Status == onboarding.StatusComplete {
		revision, exact = 2, false
	}
	return []onboarding.RunAuthority{{Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Revision: revision, Exact: exact}}, nil
}

func newPhase33FailureAdapter() *phase33FailureAdapter {
	return &phase33FailureAdapter{calls: map[onboarding.Stage]int{}, fail: map[onboarding.Stage]bool{}}
}
func (a *phase33FailureAdapter) step(stage onboarding.Stage, r onboarding.Run) (onboarding.StepResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[stage]++
	if a.fail[stage] {
		a.fail[stage] = false
		return onboarding.StepResult{}, store.ErrUnavailable
	}
	result := onboarding.StepResult{}
	if a.tokens > 0 {
		input, output := a.tokens/2, a.tokens-a.tokens/2
		result.Receipt = gateway.Receipt{Calls: []gateway.Usage{{Attempts: 1, InputTokens: &input, OutputTokens: &output}}}
	}
	switch stage {
	case onboarding.StageConnect:
		result.References = []onboarding.Reference{{Kind: "source", ID: r.Input.Source, Revision: 1, SourceRevision: 1, Source: r.Input.Source, Context: r.Input.Context}}
	case onboarding.StageInspect:
		result.References = []onboarding.Reference{{Kind: "dataset", ID: r.Input.Dataset, Revision: 1, SourceRevision: 1, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Columns: []string{"amount"}}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "column", Basis: []string{"catalog_revision:1", "source:" + r.Input.Source, "dataset:" + r.Input.Dataset, "schema_digest:old"}, Confidence: "observed"}}
	case onboarding.StageProfile:
		result.References = []onboarding.Reference{{Kind: "profile", ID: r.Input.Profile, Revision: 1, SourceRevision: 1, Private: true, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Columns: []string{"amount"}}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "profile_column", Basis: []string{"bounded_sample"}, Confidence: "observed", Uncertainty: "currency and null meaning require review"}}
		if r.Input.Transformation && phase33AnswerValue(r.Answers, "approve_transformation") == "" {
			result.Questions = []onboarding.Question{{ID: "approve_transformation", Prompt: "Review managed transformation", Evidence: []string{"managed_write_review_required"}, Required: true}}
		}
	case onboarding.StageSemantic:
		result.References = []onboarding.Reference{{Kind: "topic_draft", ID: r.Input.Topic, Revision: 1, SourceRevision: 1, Digest: strings.Repeat("a", 64), Private: true, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Columns: []string{"amount"}, DependsOn: []string{"profile:" + r.Input.Profile}}}
		result.Evidence = []onboarding.Evidence{{Entity: "amount", Kind: "measure", Basis: []string{"profile:" + r.Input.Profile}, Confidence: "unresolved", Uncertainty: "unit, grain and null semantics require review", Sensitive: true}}
		if phase33AnswerValue(r.Answers, "grain") == "" {
			result.Questions = []onboarding.Question{{ID: "grain", Prompt: "Confirm grain", Evidence: []string{"profile:" + r.Input.Profile}, Required: true}, {ID: "kpis", Prompt: "Confirm units and KPIs", Evidence: []string{"profile:" + r.Input.Profile}, Required: true}}
		}
	case onboarding.StageProposals:
		result.References = []onboarding.Reference{{Kind: "onboarding_query_intent", ID: r.ID + "-query", SourceRevision: 1, Private: true, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, DependsOn: []string{"topic:" + r.Input.Topic}}, {Kind: "onboarding_block_intent", ID: r.Input.Block, SourceRevision: 1, Private: true, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, DependsOn: []string{"topic:" + r.Input.Topic}}, {Kind: "onboarding_report_intent", ID: r.Input.Report, SourceRevision: 1, Private: true, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, DependsOn: []string{"topic:" + r.Input.Topic}}}
	}
	return result, nil
}
func (a *phase33FailureAdapter) Connect(_ context.Context, _ identity.Envelope, in onboarding.StartRequest, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageConnect, onboarding.Run{Input: in})
}
func (a *phase33FailureAdapter) Inspect(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageInspect, r)
}
func (a *phase33FailureAdapter) Profile(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageProfile, r)
}
func (a *phase33FailureAdapter) DraftSemantics(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageSemantic, r)
}
func (a *phase33FailureAdapter) PublishReviewed(_ context.Context, _ identity.Envelope, r onboarding.Run, review onboarding.ReviewReference, _ string) (onboarding.StepResult, error) {
	a.mu.Lock()
	a.calls[onboarding.StageReview]++
	a.mu.Unlock()
	return onboarding.StepResult{References: []onboarding.Reference{{Kind: "topic", ID: r.Input.Topic, Revision: 1, SourceRevision: 1, Digest: review.Digest, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, Columns: []string{"amount"}, DependsOn: []string{"topic_draft:" + r.Input.Topic}}}, Evidence: []onboarding.Evidence{{Entity: r.Input.Topic, Kind: "publication", Basis: []string{"independent_review:" + review.ID}, Confidence: "observed"}}}, nil
}
func (a *phase33FailureAdapter) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, r onboarding.Run, _ string) (onboarding.StepResult, error) {
	return a.step(onboarding.StageProposals, r)
}
func (a *phase33FailureAdapter) ProposeDriftAmendment(_ context.Context, _ identity.Envelope, r onboarding.Run, in onboarding.DriftRequest, _ string) (onboarding.Amendment, error) {
	effective := r.References[len(r.References)-4]
	revision := effective.SourceRevision + 1
	return onboarding.Amendment{Run: r.ID, Observation: "schema_changed", Source: effective.Source, Context: effective.Context, Dataset: effective.Dataset, SourceRevision: revision, Changes: []string{"amount"}, Affected: []onboarding.Reference{{Kind: "topic", ID: r.Input.Topic, Revision: 1}}, ImpactEvidence: []onboarding.ImpactEvidence{{Kind: "topic", ID: r.Input.Topic, Basis: []string{"column:amount"}}}, Proposal: onboarding.Reference{Kind: "topic_amendment", ID: r.Input.Topic + "-amend", SourceRevision: revision, Private: true, Source: effective.Source, Context: effective.Context, Dataset: effective.Dataset}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

type phase33Fixture struct {
	service *onboarding.Service
	adapter *phase33FailureAdapter
	e       identity.Envelope
	client  *sdk.Client
	other   identity.Envelope
}

func phase33Scopes(id string) []string {
	return []string{"onboarding.read", "onboarding.write", "onboarding.cancel", "cw.tenant.write:*", "cw.onboarding.read:" + id, "cw.onboarding.write:" + id, "cw.onboarding.cancel:" + id, "cw.source.read:source-a", "cw.execution_context.use:context-a", "cw.dataset.query:sales"}
}
func newPhase33Fixture(t *testing.T, id, locale string) *phase33Fixture {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	tokens := newTokenFixture(t)
	scopes := phase33Scopes(id)
	e := tokens.envelope(t, "tenant-a", "author", scopes...)
	other := tokens.envelope(t, "tenant-a", "other", scopes...)
	adapter := newPhase33FailureAdapter()
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
			return answer.Decision
		}
	}
	return ""
}

func phase33Answers(questions []onboarding.Question, value string) []onboarding.Answer {
	answers := make([]onboarding.Answer, 0, len(questions))
	for _, question := range questions {
		answers = append(answers, onboarding.Answer{ID: question.ID, Decision: value})
	}
	return answers
}

func advancePhase33(t *testing.T, f *phase33Fixture, r onboarding.Run) onboarding.Run {
	t.Helper()
	ctx := context.Background()
	var err error
	for r.Status != onboarding.StatusComplete {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "confirmed_external")
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

func testPhase33InjectedSurfaceParity(t *testing.T) {
	f := newPhase33Fixture(t, "journey", "en")
	r, err := f.client.StartOnboarding(context.Background(), phase33Start("journey", "en"))
	if err != nil {
		t.Fatal(err)
	}
	for r.Status != onboarding.StatusComplete {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "confirmed_external")
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
	cancelled, err := f.service.Cancel(context.Background(), f.e, resumed.ID, onboarding.CancelRequest{ExpectedVersion: resumed.Version, Reason: "user_requested"})
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
	revoked := f.e
	revokedScopes := []string{"onboarding.read", "onboarding.cancel", "cw.onboarding.read:authority", "cw.onboarding.cancel:authority", "cw.source.read:other-source", "cw.execution_context.use:other-context"}
	revoked, _ = identity.FromVerified(revoked.Tenant(), revoked.User(), revoked.Session(), revokedScopes, time.Now().Add(time.Minute), nil)
	if _, err = f.service.Get(context.Background(), revoked, r.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("GET exposed persisted evidence after source/context reach revocation", err)
	}
	if _, err = f.service.Cancel(context.Background(), revoked, r.ID, onboarding.CancelRequest{ExpectedVersion: r.Version, Reason: "user_requested"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("cancel exposed persisted evidence after source/context reach revocation", err)
	}
}
func testPhase33HumanGates(t *testing.T) {
	f := newPhase33Fixture(t, "gates", "en")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("gates", "en"))
	for r.Stage != onboarding.StageReview {
		if r.Status == onboarding.StatusAttention {
			answers := phase33Answers(r.Questions, "confirmed_external")
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
	f := newPhase26Fixture(t)
	applied := f.apply(t, f.approve(t, f.propose(t)))
	draftService, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, f.s, index, f.model)
	if err != nil {
		t.Fatal(err)
	}
	domains, err := onboarding.NewDomains(f.s, f.service, draftService, topicService, f.auto)
	if err != nil {
		t.Fatal(err)
	}
	limits := onboarding.DefaultLimits()
	service, err := onboarding.New(f.db, domains, limits)
	if err != nil {
		t.Fatal(err)
	}
	id := "transform-real"
	scopes := append(phase26Scopes(), "onboarding.read", "onboarding.write", "onboarding.cancel", "cw.onboarding.read:"+id, "cw.onboarding.write:"+id, "cw.onboarding.cancel:"+id)
	e := f.token.envelope(t, f.author.Tenant(), f.author.User(), scopes...)
	if e.Session() != f.author.Session() {
		t.Fatal("transformation fixture lost exact apply session")
	}
	dataset := ""
	for _, relation := range applied.Material.Binding.Relations {
		if relation.Schema == "analytics" && relation.Name == "sales" {
			dataset = relation.ID
		}
	}
	if dataset == "" {
		t.Fatal("reviewed proposal lacks exact source dataset")
	}
	in := onboarding.StartRequest{ID: id, Key: id + "-key", Mode: onboarding.ModeConnect, Locale: "en", Source: applied.Material.Binding.Source, Context: applied.Material.Binding.Context, Dataset: dataset, Profile: "transform-output-profile", Topic: "transform-topic", TopicVersion: "transform-v1", Block: "transform-block", Report: "transform-report", Transformation: true, TransformationProposal: applied.ID}
	run, err := service.Start(t.Context(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	for run.Stage != onboarding.StageSemantic {
		run, err = service.Resume(t.Context(), e, run.ID, onboarding.ResumeRequest{ExpectedVersion: run.Version})
		if err != nil {
			t.Fatal("real reviewed transformation onboarding", run.Stage, err)
		}
	}
	var outputProfile onboarding.Reference
	for _, ref := range run.References {
		if ref.Kind == "profile" {
			outputProfile = ref
		}
	}
	last := applied.Material.Pipeline.Steps[len(applied.Material.Pipeline.Steps)-1]
	outputDiscovery, discoveryErr := f.s.Discover(t.Context(), e, outputProfile.Source)
	if outputProfile.ID == "" || outputProfile.Source != applied.Material.Pipeline.ID+"."+last.ID || discoveryErr != nil || len(outputDiscovery.Relations) != 1 || outputProfile.Dataset != outputDiscovery.Relations[0].ID || outputProfile.Source == in.Source || len(outputProfile.Columns) == 0 {
		t.Fatal("onboarding did not profile the exact approved managed output", outputProfile, applied.Effects)
	}
	profile, err := f.service.InspectProfile(t.Context(), e, in.Profile)
	if err != nil || profile.Profile == nil || profile.Profile.Source != outputProfile.Source || profile.Profile.Context != outputProfile.Context || profile.Profile.Dataset != outputProfile.Dataset {
		t.Fatal("managed output profile is not durable/exact", profile, err)
	}

	// The same genuinely applied proposal cannot satisfy a run bound to another
	// real source, context and dataset.
	unrelatedSource := f.create(t, "phase33-unrelated-source")
	unrelatedDiscovery, err := f.s.Discover(t.Context(), e, unrelatedSource.ID)
	if err != nil || len(unrelatedDiscovery.Relations) == 0 {
		t.Fatal(unrelatedDiscovery, err)
	}
	unrelatedID := "transform-unrelated"
	unrelatedScopes := append(scopes, "cw.onboarding.read:"+unrelatedID, "cw.onboarding.write:"+unrelatedID, "cw.onboarding.cancel:"+unrelatedID)
	unrelatedEnvelope := f.token.envelope(t, e.Tenant(), e.User(), unrelatedScopes...)
	unrelated := in
	unrelated.ID, unrelated.Key, unrelated.Profile = unrelatedID, unrelatedID+"-key", "unrelated-output-profile"
	unrelated.Source, unrelated.Context, unrelated.Dataset = unrelatedSource.ID, unrelatedSource.ContextID, unrelatedDiscovery.Relations[0].ID
	unrelatedRun, err := service.Start(t.Context(), unrelatedEnvelope, unrelated)
	if err != nil {
		t.Fatal(err)
	}
	for unrelatedRun.Stage != onboarding.StageProfile {
		unrelatedRun, err = service.Resume(t.Context(), unrelatedEnvelope, unrelatedRun.ID, onboarding.ResumeRequest{ExpectedVersion: unrelatedRun.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = service.Resume(t.Context(), unrelatedEnvelope, unrelatedRun.ID, onboarding.ResumeRequest{ExpectedVersion: unrelatedRun.Version}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("unrelated applied proposal satisfied the transformation gate", err)
	}
}
func testPhase33DriftAmendment(t *testing.T) {
	f := newPhase33Fixture(t, "drift", "en")
	r, _ := f.service.Start(context.Background(), f.e, phase33Start("drift", "en"))
	r = advancePhase33(t, f, r)
	a, err := f.service.Drift(context.Background(), f.e, r.ID, onboarding.DriftRequest{ExpectedVersion: r.Version})
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
	f.adapter.tokens = 25000
	if _, err := f.service.Resume(context.Background(), f.e, r.ID, onboarding.ResumeRequest{ExpectedVersion: r.Version}); !errors.Is(err, onboarding.ErrBudget) {
		t.Fatal("token budget bypass", err)
	}
	f.adapter.tokens = 0
	r, _ = f.service.Get(context.Background(), f.e, r.ID)
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

// AC01 exercises the production adapter over real PostgreSQL source discovery,
// deterministic profiling, topic draft/review/publication and the public SDK.
// Failure injection remains in the focused orchestration tests above; this test
// prevents those doubles from being mistaken for domain integration evidence.
func testPhase33RealDomainBoundary(t *testing.T) {
	f, draftsService, _, _, pack := publicationFixture(t)
	model := newGatewayFixture(t, func(cfg *config.Gateway) {
		embedding := cfg.Roles["embedding"]
		embedding.MaxBatchItems = 4
		embedding.MaxBatchBytes = 64 << 10
		cfg.Roles["embedding"] = embedding
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, f.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	id := "real-domain"
	scopes := []string{
		"onboarding.read", "onboarding.write", "onboarding.cancel",
		"sources.read", "sources.rotate", "engineering.profile", "engineering.read",
		"topics.write", "topics.read", "topics.review", "topics.publish",
		"cw.tenant.write:" + f.e.Tenant(),
		"cw.onboarding.read:" + id, "cw.onboarding.write:" + id, "cw.onboarding.cancel:" + id,
		"cw.onboarding.read:real-upload", "cw.onboarding.write:real-upload", "cw.onboarding.cancel:real-upload",
		"cw.source.read:*", "cw.source.write:*", "cw.execution_context.use:*", "cw.dataset.query:*",
		"cw.topic.write:*", "cw.topic.read:*", "cw.topic.publish:*",
	}
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
	domains, err := onboarding.NewDomains(f.s, f.service, draftsService, topicService, phase33NoAutopilot{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := onboarding.New(f.db, domains, onboarding.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	handler := onboardingapi.Handler(f.token.verifier, service, http.NotFoundHandler())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	claims := f.token.claims(e.Tenant(), e.User(), scopes)
	claims["session"] = e.Session()
	bearer := f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	uploadRaw := []byte("id,amount\n1,10.5\n2,20.0\n")
	upload := f.load(t, engineeringSpec("guided-upload", "csv", uploadRaw, []engineering.UploadColumn{{Name: "id", Type: "integer"}, {Name: "amount", Type: "number"}}), uploadRaw)
	if upload.Upload.Source == nil {
		t.Fatal("real upload omitted source")
	}
	uploadDiscovery, err := f.s.Discover(t.Context(), e, upload.Upload.Source.ID)
	if err != nil || len(uploadDiscovery.Relations) != 1 {
		t.Fatal("uploaded source discovery", uploadDiscovery, err)
	}
	uploadStart := onboarding.StartRequest{ID: "real-upload", Key: "real-upload-key", Mode: onboarding.ModeUpload, Locale: "en", Source: upload.Upload.Source.ID, Context: upload.Upload.Source.ContextID, Dataset: uploadDiscovery.Relations[0].ID, Profile: "guided-upload-profile", Topic: "guided-upload-topic", TopicVersion: "guided-upload-v1", Block: "guided-upload-block", Report: "guided-upload-report", Upload: upload.Upload.ID}
	uploadRun, err := client.StartOnboarding(t.Context(), uploadStart)
	if err != nil {
		t.Fatal(err)
	}
	uploadRun, err = client.ResumeOnboarding(t.Context(), uploadRun.ID, uploadRun.Version)
	if err != nil || uploadRun.Stage != onboarding.StageInspect || uploadRun.References[0].ID != upload.Upload.Source.ID {
		t.Fatal("real upload onboarding handoff", uploadRun, err)
	}
	in := onboarding.StartRequest{ID: id, Key: id + "-key", Mode: onboarding.ModeConnect, Locale: "en", Source: pack.Datasets[0].Source.Source, Context: pack.Datasets[0].Source.Context, Dataset: pack.Datasets[0].ID, Profile: pack.Datasets[0].Source.ProfileVersion, Topic: "guided-commerce", TopicVersion: "guided-v1", Block: "guided-block", Report: "guided-report"}
	run, err := client.StartOnboarding(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	var approvedReview topics.Review
	for run.Status != onboarding.StatusComplete {
		switch {
		case run.Stage == onboarding.StageReview:
			draft, readErr := draftsService.Read(t.Context(), e, in.Topic, 0)
			if readErr != nil {
				t.Fatal(readErr)
			}
			review, reviewErr := topicService.Review(t.Context(), e, in.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Independent synthetic review"})
			if reviewErr != nil {
				t.Fatal(reviewErr)
			}
			approvedReview = review
			run, err = client.AnswerOnboarding(t.Context(), id, onboarding.AnswerRequest{ExpectedVersion: run.Version, Review: &onboarding.ReviewReference{ID: review.ID, Revision: review.DraftRevision, Digest: review.Digest}})
		case run.Status == onboarding.StatusAttention:
			run, err = client.AnswerOnboarding(t.Context(), id, onboarding.AnswerRequest{ExpectedVersion: run.Version, Answers: phase33Answers(run.Questions, "confirmed_external")})
		default:
			run, err = client.ResumeOnboarding(t.Context(), id, run.Version)
		}
		if err != nil {
			var status *sdk.StatusError
			if errors.As(err, &status) {
				t.Fatal("real domain journey", run.Stage, status.Status, status.Code, err)
			}
			t.Fatal("real domain journey", run.Stage, err)
		}
	}
	if model.requests.Load() < 2 || run.Usage.ModelCalls != run.Limits.MaxModelCalls || run.Usage.Tokens != run.Limits.MaxTokens {
		t.Fatal("gateway reservation or recorded model boundary missing", model.requests.Load(), run.Usage)
	}
	var topicRef onboarding.Reference
	for _, kind := range []string{"topic", "onboarding_query_intent", "onboarding_block_intent", "onboarding_report_intent"} {
		found := false
		for _, ref := range run.References {
			found = found || ref.Kind == kind
			if ref.Kind == "topic" && ref.ID == in.Topic {
				topicRef = ref
			}
		}
		if !found {
			t.Fatal("missing durable handoff", kind, run.References)
		}
	}
	published, err := topicService.Read(t.Context(), e, in.Topic, "")
	if err != nil || !published.State.Active || published.State.Archived || published.State.Topic != in.Topic || published.State.Version != in.TopicVersion || published.State.Revision < 1 || published.State.Revision != topicRef.Revision || published.Digest != topicRef.Digest || published.Digest != approvedReview.Digest || published.Definition.Topic != in.Topic || published.Definition.Version != in.TopicVersion {
		t.Fatal("ledger topic does not match active reviewed publication", err)
	}
	metadata := support.Raw(t, f.dsn)
	var reviewID, digest string
	var draftRevision int64
	if err := metadata.QueryRow(t.Context(), `SELECT review_id,draft_revision,digest FROM chartworks.topic_published_versions WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, e.Tenant(), in.Topic, published.State.Version).Scan(&reviewID, &draftRevision, &digest); err != nil || reviewID != approvedReview.ID || draftRevision != approvedReview.DraftRevision || digest != published.Digest {
		t.Fatal("active publication lost its independent review link", err)
	}
	rotated, err := f.s.Rotate(t.Context(), e, in.Source, pack.Datasets[0].Source.SourceRevision)
	if err != nil || rotated.Revision <= pack.Datasets[0].Source.SourceRevision || rotated.ContextID == in.Context {
		t.Fatal("real source rotation", rotated, err)
	}
	amendment, err := service.Drift(t.Context(), e, id, onboarding.DriftRequest{ExpectedVersion: run.Version})
	if err != nil || amendment.Observation != "binding_changed" || amendment.Source != in.Source || amendment.Context != rotated.ContextID || amendment.SourceRevision != rotated.Revision || len(amendment.Changes) != 1 || amendment.Changes[0] != "execution_context" {
		t.Fatal("server-resolved drift amendment", amendment, err)
	}
	bindings, err := onboardingapi.MCPBindings(service)
	if err != nil || len(bindings) != 8 {
		t.Fatal("MCP consumer bindings", len(bindings), err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpService, err := mcpserver.New(f.token.verifier, mcpRegistry, config.Defaults().MCP, nil)
	if err != nil {
		t.Fatal(err)
	}
	mcpClaims := f.token.claims(e.Tenant(), e.User(), append(scopes, "mcp.use"))
	mcpClaims["aud"] = f.token.cfg.MCPAudience()
	mcpClaims["session"] = e.Session()
	mcpBearer := f.token.sign(t, mcpClaims, nil)
	mcpClient, err := mcpService.Client(func(context.Context) (string, error) { return mcpBearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	mcpBody, _ := json.Marshal(onboardingapi.IDRequest{ID: id})
	mcpResult, err := mcpClient.CallTool(t.Context(), "get_onboarding", mcpBody)
	if err != nil || mcpResult == nil || mcpResult.IsError {
		t.Fatal("real MCP progress read", mcpResult, err)
	}
}

func testPhase33PostgresLeaseRace(t *testing.T) {
	db := support.Open(t, support.Database(t))
	tokens := newTokenFixture(t)
	id := "postgres-race"
	scopes := phase33Scopes(id)
	e := tokens.envelope(t, "tenant-a", "author", scopes...)
	adapter := &phase33RaceAdapter{phase33FailureAdapter: newPhase33FailureAdapter(), block: onboarding.StageConnect, entered: make(chan string, 2), release: make(chan struct{}, 2)}
	service, err := onboarding.New(db, adapter, onboarding.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Start(t.Context(), e, phase33Start(id, "en"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, resumeErr := service.Resume(t.Context(), e, id, onboarding.ResumeRequest{ExpectedVersion: run.Version})
		done <- resumeErr
	}()
	firstKey := <-adapter.entered
	leased, err := service.Get(t.Context(), e, id)
	if err != nil || leased.Lease == nil {
		t.Fatal("PostgreSQL did not retain pre-effect lease", leased, err)
	}
	cancelling, err := service.Cancel(t.Context(), e, id, onboarding.CancelRequest{ExpectedVersion: leased.Version, Reason: "user_requested"})
	if err != nil || !cancelling.CancelRequested {
		t.Fatal(cancelling, err)
	}
	adapter.release <- struct{}{}
	if err = <-done; !errors.Is(err, store.ErrConflict) {
		t.Fatal("PostgreSQL fence allowed stale effect commit", err)
	}
	go func() {
		_, resumeErr := service.Resume(t.Context(), e, id, onboarding.ResumeRequest{ExpectedVersion: cancelling.Version})
		done <- resumeErr
	}()
	if secondKey := <-adapter.entered; secondKey != firstKey {
		t.Fatal("retry changed operation key", firstKey, secondKey)
	}
	adapter.release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	final, _ := service.Get(t.Context(), e, id)
	if final.Status != onboarding.StatusCancelled || final.Lease != nil {
		t.Fatal("PostgreSQL reconciliation did not finalize cancellation", final)
	}
}
