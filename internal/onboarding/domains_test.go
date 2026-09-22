package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

type domainSources struct {
	source    sources.Source
	discovery sources.Discovery
}

func (f domainSources) Get(context.Context, identity.Envelope, string) (sources.Source, error) {
	return f.source, nil
}
func (f domainSources) Discover(context.Context, identity.Envelope, string) (sources.Discovery, error) {
	return f.discovery, nil
}

type domainProfiles struct {
	upload  engineering.UploadStatus
	profile engineering.ProfileRun
	found   bool
}

func (f domainProfiles) InspectUpload(context.Context, identity.Envelope, string) (engineering.UploadStatus, error) {
	return f.upload, nil
}
func (f domainProfiles) InspectProfile(context.Context, identity.Envelope, string) (engineering.ProfileStatus, error) {
	if !f.found {
		return engineering.ProfileStatus{}, store.ErrNotFound
	}
	return f.profile.Profile, nil
}
func (f domainProfiles) Build(context.Context, identity.Envelope, engineering.ProfileSpec, string, bool) (engineering.ProfileRun, error) {
	return f.profile, nil
}

type domainDrafts struct{ version drafts.Version }

func (f domainDrafts) Read(context.Context, identity.Envelope, string, int64) (drafts.Version, error) {
	return f.version, nil
}
func (f domainDrafts) OnboardProfile(context.Context, identity.Envelope, drafts.OnboardRequest) (drafts.Version, error) {
	return f.version, nil
}

type domainTopics struct {
	published topics.Published
	failOnce  bool
}

func (f *domainTopics) Publish(context.Context, identity.Envelope, string, topics.PublishRequest) (topics.Published, error) {
	if f.failOnce {
		f.failOnce = false
		return topics.Published{}, store.ErrConflict
	}
	return f.published, nil
}
func (f *domainTopics) Read(context.Context, identity.Envelope, string, string) (topics.Published, error) {
	return f.published, nil
}

type domainAutopilot struct{ proposal engineering.AutopilotProposal }

func (f domainAutopilot) Get(context.Context, identity.Envelope, string) (engineering.AutopilotProposal, error) {
	return f.proposal, nil
}

func domainFixture(t *testing.T, applied bool) (*Domains, Run, identity.Envelope) {
	t.Helper()
	column := readexec.Column{Name: "amount", NativeType: "numeric", Category: "numeric", Nullable: true, Safe: true}
	source := sources.Source{ID: "source", ContextID: "context", Revision: 2, Status: "registered"}
	profile := engineering.Profile{Version: "profile", Source: "source", Context: "context", Dataset: "dataset", SourceRevision: 2, Schema: []readexec.Column{column}, Columns: []engineering.ColumnProfile{{Name: "amount", NativeType: "numeric", Category: "numeric", Nullable: true, Observed: 2, Nulls: 1}}, Sampling: engineering.Sampling{Rows: 2}}
	pack := semantics.TopicPack{
		SchemaVersion: 1,
		Topic:         "topic",
		Version:       "topic-v1",
		Name:          "Topic",
		Datasets: []semantics.Dataset{{
			ID:      "dataset",
			Name:    "Dataset",
			Source:  semantics.SourceReference{Source: "source", Context: "context", SourceRevision: 2, ProfileVersion: "profile", ProfileDigest: strings.Repeat("b", 64)},
			Columns: []semantics.Column{{ID: "amount", Name: "Amount", SourceName: "amount", NativeType: "numeric", Category: "numeric", Nullable: true}},
		}},
	}
	digest := strings.Repeat("a", 64)
	state := "draft"
	if applied {
		state = "applied"
	}
	topicService := &domainTopics{published: topics.Published{State: topics.State{Topic: "topic", Revision: 1, Version: "topic-v1", Active: true}, Digest: digest}, failOnce: true}
	sourceService := domainSources{
		source: source,
		discovery: sources.Discovery{SourceID: "source", ContextID: "context", Revision: 2,
			Relations: []readexec.Relation{{ID: "dataset", Columns: []readexec.Column{column}}}},
	}
	profileService := domainProfiles{
		upload:  engineering.UploadStatus{ID: "upload", State: "active", Source: &source},
		profile: engineering.ProfileRun{Profile: engineering.ProfileStatus{Version: "profile", State: "complete", Profile: &profile}},
	}
	draftService := domainDrafts{version: drafts.Version{Metadata: drafts.Revision{Topic: "topic", Revision: 1, Version: "topic-v1", Digest: digest}, Pack: pack}}
	autopilotService := domainAutopilot{proposal: engineering.AutopilotProposal{ID: "transform", Revision: 1, Digest: strings.Repeat("c", 64), State: state}}
	domains, err := NewDomains(sourceService, profileService, draftService, topicService, autopilotService)
	if err != nil {
		t.Fatal(err)
	}
	e := serviceEnvelope(t, "run")
	run := Run{ID: "run", Locale: "en", Input: StartRequest{Mode: ModeConnect, Source: "source", Context: "context", Dataset: "dataset", Profile: "profile", Topic: "topic", TopicVersion: "topic-v1", Block: "block", Report: "report"}, Answers: []Answer{}}
	return domains, run, e
}

func TestDomainsComposeExistingServices(t *testing.T) {
	domains, run, envelope := domainFixture(t, true)
	ctx := t.Context()
	connected, err := domains.Connect(ctx, envelope, run.Input, "connect")
	if err != nil || connected.References[0].Revision != 2 {
		t.Fatal(connected, err)
	}
	inspected, err := domains.Inspect(ctx, envelope, run, "inspect")
	if err != nil || len(inspected.Evidence) != 1 || inspected.Evidence[0].Sensitive {
		t.Fatal(inspected, err)
	}
	profiled, err := domains.Profile(ctx, envelope, run, "profile")
	if err != nil || len(profiled.Questions) != 1 || profiled.Questions[0].ID == "" {
		t.Fatal(profiled, err)
	}
	run.Answers = []Answer{{ID: profiled.Questions[0].ID, Value: "missing"}}
	profiled, err = domains.Profile(ctx, envelope, run, "profile")
	if err != nil || len(profiled.Questions) != 0 {
		t.Fatal("profile answer did not resolve", profiled, err)
	}
	semantic, err := domains.DraftSemantics(ctx, envelope, run, "semantic")
	if err != nil || len(semantic.Evidence) != 1 || len(semantic.Questions) < 4 {
		t.Fatal(semantic, err)
	}
	for _, question := range semantic.Questions {
		run.Answers = append(run.Answers, Answer{ID: question.ID, Value: "reviewed"})
	}
	semantic, err = domains.DraftSemantics(ctx, envelope, run, "semantic")
	if err != nil || len(semantic.Questions) != 0 {
		t.Fatal("semantic answers did not resolve", semantic, err)
	}
	published, err := domains.PublishReviewed(ctx, envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish")
	if err != nil || published.References[0].Kind != "topic" {
		t.Fatal("publish reconciliation", published, err)
	}
	proposals, err := domains.ProposeQueriesBlocksReports(ctx, envelope, run, "proposals")
	if err != nil || len(proposals.References) != 3 || proposals.Evidence[0].Confidence != "unresolved" {
		t.Fatal(proposals, err)
	}
	amendment, err := domains.ProposeDriftAmendment(ctx, envelope, Run{ID: "run", Input: run.Input, References: proposals.References}, DriftRequest{SourceRevision: 3, Observation: "catalog-v3"}, "drift")
	if err != nil || !amendment.ExistingIntact || !amendment.Proposal.Private {
		t.Fatal(amendment, err)
	}
}

func TestDomainsTransformationAndNegativeBoundaries(t *testing.T) {
	domains, run, envelope := domainFixture(t, false)
	run.Input.Transformation, run.Input.TransformationProposal = true, "transform"
	result, err := domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil || len(result.Questions) < 2 || result.Questions[len(result.Questions)-1].ID != "approve_transformation" {
		t.Fatal(result, err)
	}
	domains, run, envelope = domainFixture(t, true)
	run.Input.Transformation, run.Input.TransformationProposal = true, "transform"
	result, err = domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil || len(result.References) != 2 || result.References[1].Kind != "engineering_proposal" {
		t.Fatal(result, err)
	}
	domains.sources = domainSources{source: sources.Source{ID: "source", ContextID: "other", Revision: 1, Status: "registered"}}
	if _, err = domains.Connect(t.Context(), envelope, run.Input, "connect"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("context mismatch accepted", err)
	}
	domains, run, envelope = domainFixture(t, true)
	run.Input.Mode, run.Input.Upload = ModeUpload, "upload"
	if _, err = domains.Connect(t.Context(), envelope, run.Input, "connect"); err != nil {
		t.Fatal("active upload rejected", err)
	}
	unsafe := domains.sources.(domainSources)
	unsafe.discovery.Relations[0].Columns[0].Safe = false
	domains.sources = unsafe
	observed, err := domains.Inspect(t.Context(), envelope, run, "inspect")
	if err != nil || !observed.Evidence[0].Sensitive || observed.Evidence[0].Confidence != "unresolved" {
		t.Fatal("unsafe discovery was promoted", observed, err)
	}
	run.Locale = "es"
	profiled, err := domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil || len(profiled.Questions) == 0 || !strings.Contains(profiled.Questions[0].Prompt, "Cómo") {
		t.Fatal("Spanish question missing", profiled, err)
	}
	domains.topics.(*domainTopics).failOnce = false
	if _, err = domains.PublishReviewed(t.Context(), envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish"); err != nil {
		t.Fatal("direct reviewed publication", err)
	}
}
