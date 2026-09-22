package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

type domainSources struct {
	source          sources.Source
	discovery       sources.Discovery
	output          sources.Source
	outputDiscovery sources.Discovery
}

func (f domainSources) Get(_ context.Context, _ identity.Envelope, id string) (sources.Source, error) {
	if id == "pipeline.dataset" {
		return f.output, nil
	}
	return f.source, nil
}
func (f domainSources) Discover(_ context.Context, _ identity.Envelope, id string) (sources.Discovery, error) {
	if id == "pipeline.dataset" {
		return f.outputDiscovery, nil
	}
	return f.discovery, nil
}

type domainProfiles struct {
	upload  engineering.UploadStatus
	profile engineering.ProfileRun
	found   bool
}

func (f *domainProfiles) InspectUpload(context.Context, identity.Envelope, string) (engineering.UploadStatus, error) {
	return f.upload, nil
}
func (f *domainProfiles) InspectProfile(context.Context, identity.Envelope, string) (engineering.ProfileStatus, error) {
	if !f.found {
		return engineering.ProfileStatus{}, store.ErrNotFound
	}
	return f.profile.Profile, nil
}
func (f *domainProfiles) Build(_ context.Context, _ identity.Envelope, spec engineering.ProfileSpec, _ string, _ bool) (engineering.ProfileRun, error) {
	out := f.profile
	copy := *out.Profile.Profile
	copy.Source, copy.Context, copy.Dataset = spec.Source, spec.Context, spec.Dataset
	if spec.Source == "pipeline.dataset" {
		copy.SourceRevision = 3
	}
	out.Profile.Profile = &copy
	return out, nil
}

type domainDrafts struct{ version drafts.Version }

func (f domainDrafts) Read(context.Context, identity.Envelope, string, int64) (drafts.Version, error) {
	return f.version, nil
}
func (f domainDrafts) OnboardProfile(context.Context, identity.Envelope, drafts.OnboardRequest) (drafts.Version, error) {
	return f.version, nil
}

type domainTopics struct {
	published    topics.Published
	active       topics.Published
	failOnce     bool
	exactMissing bool
	publishes    int
	expected     int64
	limits       gateway.Limits
}

func (f *domainTopics) PublishBounded(_ context.Context, _ identity.Envelope, _ string, in topics.PublishRequest, limits gateway.Limits) (topics.Published, error) {
	f.publishes++
	f.expected = in.Expected
	f.limits = limits
	if f.failOnce {
		f.failOnce = false
		return topics.Published{}, store.ErrConflict
	}
	return f.published, nil
}

func TestDomainsPublicationReceivesCompleteReservedAllowance(t *testing.T) {
	domains, run, envelope := domainFixture(t, true)
	topic := domains.topics.(*domainTopics)
	topic.exactMissing = true
	topic.failOnce = false
	topic.active = topics.Published{State: topics.State{Topic: "topic", Revision: 7, Version: "prior-v1", Active: true}, Digest: strings.Repeat("b", 64)}
	run.Lease = &Lease{Stage: StageReview, ReservedCalls: 3, ReservedTokens: 2300}
	run.References = []Reference{{Kind: "topic_draft", ID: "topic", Source: "source", Context: "context", Dataset: "dataset", Columns: []string{"amount"}}}
	if _, err := domains.PublishReviewed(t.Context(), envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish"); err != nil {
		t.Fatal(err)
	}
	if topic.publishes != 1 || topic.expected != 7 || topic.limits.Calls != 3 || topic.limits.Tokens != 2300 {
		t.Fatal("publication lost multi-call reservation", topic.publishes, topic.expected, topic.limits)
	}
}

func (f *domainTopics) Read(_ context.Context, _ identity.Envelope, _ string, version string) (topics.Published, error) {
	if version != "" && f.exactMissing {
		return topics.Published{}, store.ErrNotFound
	}
	if version == "" && f.active.State.Revision > 0 {
		return f.active, nil
	}
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
		output: sources.Source{ID: "pipeline.dataset", ContextID: "pipeline.dataset:v3", Revision: 3, Status: "registered"},
		outputDiscovery: sources.Discovery{SourceID: "pipeline.dataset", ContextID: "pipeline.dataset:v3", Revision: 3,
			Relations: []readexec.Relation{{ID: "dataset", Columns: []readexec.Column{column}}}},
	}
	profileService := &domainProfiles{
		upload:  engineering.UploadStatus{ID: "upload", State: "active", Source: &source},
		profile: engineering.ProfileRun{Profile: engineering.ProfileStatus{Version: "profile", State: "complete", Profile: &profile}},
	}
	draftService := domainDrafts{version: drafts.Version{Metadata: drafts.Revision{Topic: "topic", Revision: 1, Version: "topic-v1", Digest: digest}, Pack: pack}}
	material := engineering.ProposalMaterial{
		Binding:  readexec.Binding{Source: "source", Context: "context", Revision: 2},
		Request:  engineering.AutopilotGoal{Source: "source", Context: "context"},
		Pipeline: engineering.PipelineDefinition{ID: "pipeline", Steps: []engineering.PipelineStep{{ID: "dataset", Columns: []engineering.PipelineColumn{{Name: "amount", Type: "numeric"}}}}},
	}
	operation := "pipeline-operation"
	autopilotService := domainAutopilot{proposal: engineering.AutopilotProposal{ID: "transform", Revision: 1, Digest: material.Digest(), State: state, Material: material, Operation: operation, Effects: []engineering.ProposalEffect{{Kind: "pipeline_run", Target: "pipeline", State: "published", Version: 1, Digest: readexec.Hash(material.Pipeline), Operation: operation}, {Kind: "managed_step", Target: "pipeline.dataset", State: "checked", Version: 3, Digest: "output-digest", Operation: operation}}}}
	domains, err := NewDomains(sourceService, profileService, draftService, topicService, autopilotService)
	if err != nil {
		t.Fatal(err)
	}
	e := serviceEnvelope(t, "run")
	run := Run{ID: "run", Locale: "en", Input: StartRequest{Mode: ModeConnect, Source: "source", Context: "context", Dataset: "dataset", Profile: "profile", Topic: "topic", TopicVersion: "topic-v1", Block: "block", Report: "report"}, Answers: []Answer{}, SourceRevision: 2, Deadline: time.Now().Add(time.Minute), Lease: &Lease{ReservedCalls: 1, ReservedTokens: 1000}}
	return domains, run, e
}

func TestDomainsComposeExistingServices(t *testing.T) {
	domains, run, envelope := domainFixture(t, true)
	ctx := t.Context()
	connected, err := domains.Connect(ctx, envelope, run.Input, "connect")
	if err != nil || connected.References[0].Revision != 2 {
		t.Fatal(connected, err)
	}
	if authority, authorityErr := domains.ResolveRunAuthority(ctx, envelope, run); authorityErr != nil || len(authority) != 1 || authority[0].Context != run.Input.Context {
		t.Fatal("root run authority resolution", authority, authorityErr)
	}
	inspected, err := domains.Inspect(ctx, envelope, run, "inspect")
	if err != nil || len(inspected.Evidence) != 1 || inspected.Evidence[0].Sensitive {
		t.Fatal(inspected, err)
	}
	profiled, err := domains.Profile(ctx, envelope, run, "profile")
	if err != nil || len(profiled.Questions) != 1 || profiled.Questions[0].ID == "" {
		t.Fatal(profiled, err)
	}
	run.Answers = []Answer{{ID: profiled.Questions[0].ID, Decision: "confirmed_external"}}
	profiled, err = domains.Profile(ctx, envelope, run, "profile")
	if err != nil || len(profiled.Questions) != 0 {
		t.Fatal("profile answer did not resolve", profiled, err)
	}
	semantic, err := domains.DraftSemantics(ctx, envelope, run, "semantic")
	if err != nil || len(semantic.Evidence) != 1 || len(semantic.Questions) < 4 {
		t.Fatal(semantic, err)
	}
	for _, question := range semantic.Questions {
		run.Answers = append(run.Answers, Answer{ID: question.ID, Decision: "confirmed_external"})
	}
	semantic, err = domains.DraftSemantics(ctx, envelope, run, "semantic")
	if err != nil || len(semantic.Questions) != 0 {
		t.Fatal("semantic answers did not resolve", semantic, err)
	}
	run.References = append(run.References, inspected.References...)
	run.References = append(run.References, profiled.References...)
	run.References = append(run.References, semantic.References...)
	published, err := domains.PublishReviewed(ctx, envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish")
	if err != nil || published.References[0].Kind != "topic" {
		t.Fatal("publish reconciliation", published, err)
	}
	run.References = append(run.References, published.References...)
	proposals, err := domains.ProposeQueriesBlocksReports(ctx, envelope, run, "proposals")
	if err != nil || len(proposals.References) != 3 || proposals.Evidence[0].Confidence != "unresolved" {
		t.Fatal(proposals, err)
	}
	oldEvidence := inspected.Evidence
	references := append([]Reference{}, run.References...)
	references = append(references, proposals.References...)
	if _, unchangedErr := domains.ProposeDriftAmendment(ctx, envelope, Run{ID: "run", Input: run.Input, References: references, Evidence: oldEvidence, SourceRevision: 2}, DriftRequest{}, "drift"); !errors.Is(unchangedErr, store.ErrConflict) {
		t.Fatal("unchanged source produced amendment", unchangedErr)
	}
	sourceAdapter := domains.sources.(domainSources)
	sourceAdapter.source.Revision = 3
	sourceAdapter.discovery.Revision = 3
	sourceAdapter.discovery.Relations[0].Columns[0].Nullable = false
	domains.sources = sourceAdapter
	amendment, err := domains.ProposeDriftAmendment(ctx, envelope, Run{ID: "run", Input: run.Input, References: references, Evidence: oldEvidence, SourceRevision: 2}, DriftRequest{}, "drift")
	queryAffected, conservative := false, false
	for i, ref := range amendment.Affected {
		if ref.Kind == "onboarding_query_intent" {
			queryAffected = true
			conservative = amendment.ImpactEvidence[i].Conservative
		}
	}
	if err != nil || !amendment.ExistingIntact || !amendment.Proposal.Private || !queryAffected || !conservative || len(amendment.ImpactEvidence) != len(amendment.Affected) {
		t.Fatal(amendment, err)
	}
}

func TestDomainsTransformationAndNegativeBoundaries(t *testing.T) {
	domains, run, envelope := domainFixture(t, false)
	run.Input.Transformation, run.Input.TransformationProposal = true, "transform"
	result, err := domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil || len(result.Questions) != 1 || result.Questions[0].ID != "approve_transformation" {
		t.Fatal(result, err)
	}
	domains, run, envelope = domainFixture(t, true)
	run.Input.Transformation, run.Input.TransformationProposal = true, "transform"
	result, err = domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil || len(result.References) != 2 || result.References[0].Kind != "engineering_proposal" || result.References[1].Revision != 3 || result.References[1].Kind != "profile" || !strings.Contains(strings.Join(result.Evidence[len(result.Evidence)-1].Basis, " "), "output_source:pipeline.dataset") {
		t.Fatal(result, err)
	}
	run.References = result.References
	if authority, authorityErr := domains.ResolveRunAuthority(t.Context(), envelope, run); authorityErr != nil || len(authority) != 2 || authority[0].Source != "pipeline.dataset" || authority[0].Context != "pipeline.dataset:v3" || authority[0].Dataset != "dataset" || !authority[0].Exact {
		t.Fatal("managed output authority resolution", authority, authorityErr)
	}
	unrelated := domains.autopilot.(domainAutopilot)
	unrelated.proposal.Material.Binding.Source = "other-source"
	unrelated.proposal.Digest = unrelated.proposal.Material.Digest()
	domains.autopilot = unrelated
	if _, err = domains.Profile(t.Context(), envelope, run, "profile"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("unrelated applied proposal satisfied transformation gate", err)
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
	run.References = []Reference{{Kind: "topic_draft", ID: "topic", Source: "source", Context: "context", Dataset: "dataset", Columns: []string{"amount"}}}
	if _, err = domains.PublishReviewed(t.Context(), envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish"); err != nil {
		t.Fatal("direct reviewed publication", err)
	}
}

func TestDomainsTransformationDriftTracksEffectiveOutput(t *testing.T) {
	domains, run, envelope := domainFixture(t, true)
	run.Input.Transformation, run.Input.TransformationProposal = true, "transform"
	profiled, err := domains.Profile(t.Context(), envelope, run, "profile")
	if err != nil {
		t.Fatal(err)
	}
	output := profiled.References[len(profiled.References)-1]
	topicDraft := Reference{Kind: "topic_draft", ID: run.Input.Topic, Revision: 1, SourceRevision: output.SourceRevision, Private: true, Source: output.Source, Context: output.Context, Dataset: output.Dataset, Columns: []string{"amount"}, DependsOn: []string{"profile:" + run.Input.Profile}}
	topic := Reference{Kind: "topic", ID: run.Input.Topic, Revision: 1, SourceRevision: output.SourceRevision, Source: output.Source, Context: output.Context, Dataset: output.Dataset, Columns: []string{"amount"}, DependsOn: []string{"topic_draft:" + run.Input.Topic}}
	query := Reference{Kind: "onboarding_query_intent", ID: run.ID + "-query", SourceRevision: output.SourceRevision, Private: true, Source: output.Source, Context: output.Context, Dataset: output.Dataset, DependsOn: []string{"topic:" + run.Input.Topic}}
	run.References = profiled.References
	run.References = append(run.References, topicDraft, topic, query)
	run.Evidence = profiled.Evidence
	if _, unchangedErr := domains.ProposeDriftAmendment(t.Context(), envelope, run, DriftRequest{}, "drift"); !errors.Is(unchangedErr, store.ErrConflict) {
		t.Fatal("unchanged managed output produced amendment", unchangedErr)
	}
	sourceAdapter := domains.sources.(domainSources)
	sourceAdapter.output.Revision = 4
	sourceAdapter.output.ContextID = "pipeline.dataset:v4"
	sourceAdapter.outputDiscovery.Revision = 4
	sourceAdapter.outputDiscovery.ContextID = "pipeline.dataset:v4"
	sourceAdapter.outputDiscovery.Relations[0].Columns[0].Nullable = false
	domains.sources = sourceAdapter
	amendment, err := domains.ProposeDriftAmendment(t.Context(), envelope, run, DriftRequest{}, "drift")
	if err != nil || amendment.Source != output.Source || amendment.Dataset != output.Dataset || amendment.Context != "pipeline.dataset:v4" || amendment.SourceRevision != 4 || amendment.Observation != "binding_changed" {
		t.Fatal("managed output drift did not bind effective source", amendment, err)
	}
	if strings.Join(amendment.Changes, ",") != "amount,execution_context" {
		t.Fatal("managed output schema and context changes were not both retained", amendment.Changes)
	}
	if sourceAdapter.source.Revision != 2 || sourceAdapter.source.ContextID != "context" {
		t.Fatal("test accidentally drifted original input source", sourceAdapter.source)
	}
	affected := map[string]bool{}
	for _, ref := range amendment.Affected {
		affected[ref.Kind+":"+ref.ID] = true
		if ref.Source != output.Source || ref.Dataset != output.Dataset {
			t.Fatal("amendment crossed effective output coordinate", ref)
		}
	}
	if !affected["topic:"+run.Input.Topic] || !affected["onboarding_query_intent:"+run.ID+"-query"] {
		t.Fatal("dependent semantic closure missing", amendment.Affected)
	}
	authority, authorityErr := domains.ResolveRunAuthority(t.Context(), envelope, run)
	if authorityErr != nil {
		t.Fatal(authorityErr)
	}
	outputDrifted := false
	for _, coordinate := range authority {
		if coordinate.Source == output.Source {
			outputDrifted = !coordinate.Exact && coordinate.Context == "pipeline.dataset:v4" && coordinate.Revision == 4
		}
	}
	if !outputDrifted {
		t.Fatal("current managed output revision was not server resolved", authority)
	}
}

func TestDomainsPublicationUsesCurrentHeadAndExactReplay(t *testing.T) {
	domains, run, envelope := domainFixture(t, true)
	run.References = []Reference{{Kind: "topic_draft", ID: "topic", Source: "source", Context: "context", Dataset: "dataset", Columns: []string{"amount"}}}
	service := domains.topics.(*domainTopics)
	service.exactMissing = true
	service.active = topics.Published{State: topics.State{Topic: "topic", Revision: 7, Version: "topic-v0", Active: true}, Digest: strings.Repeat("b", 64)}
	service.published.State.Revision = 8
	service.failOnce = false
	step, err := domains.PublishReviewed(t.Context(), envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish")
	if err != nil || service.expected != 7 || service.publishes != 1 || step.References[0].Revision != 8 {
		t.Fatal("publication did not bind current head", service.expected, service.publishes, step, err)
	}
	service.exactMissing = false
	service.publishes = 0
	step, err = domains.PublishReviewed(t.Context(), envelope, run, ReviewReference{ID: "review", Revision: 1, Digest: strings.Repeat("a", 64)}, "publish")
	if err != nil || service.publishes != 0 || step.References[0].Digest != strings.Repeat("a", 64) {
		t.Fatal("exact immutable replay repeated publication", service.publishes, step, err)
	}
}
