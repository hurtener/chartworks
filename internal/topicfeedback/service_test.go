package topicfeedback

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"testing"
	"time"
)

type testRepository struct {
	record Record
	err    error
}

func (r *testRepository) ReadTopicFeedback(context.Context, identity.Envelope, string) (Record, error) {
	return r.record, r.err
}
func (r *testRepository) PutTopicFeedback(_ context.Context, _ identity.Envelope, x Record) (Record, error) {
	return x, r.err
}
func (r *testRepository) SaveTopicDraft(context.Context, identity.Envelope, drafts.Prepared, int64, string) (drafts.Version, error) {
	return drafts.Version{}, r.err
}

type testEvidence struct{ err error }

func (r testEvidence) SemanticFeedbackEvidence(context.Context, identity.Envelope, string) (nlqexec.SemanticFeedback, error) {
	return nlqexec.SemanticFeedback{}, r.err
}

type testDraft struct{}

func (testDraft) PrepareFeedbackAuthoring(context.Context, identity.Envelope, string, int64) (drafts.Version, json.RawMessage, error) {
	return drafts.Version{}, nil, store.ErrInvalid
}
func (testDraft) PrepareFeedbackRevision(context.Context, identity.Envelope, semantics.TopicPack, int64, drafts.FeedbackApplication) (drafts.Prepared, error) {
	return drafts.Prepared{}, store.ErrInvalid
}
func (testDraft) Read(context.Context, identity.Envelope, string, int64) (drafts.Version, error) {
	return drafts.Version{Metadata: drafts.Revision{Revision: 2}}, nil
}
func proposalEnvelope(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "author", "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestProposalServiceAdmissionAndRetainedPaths(t *testing.T) {
	ctx := context.Background()
	e := proposalEnvelope(t, "topics.write", "feedback.write", "cw.topic.write:sales", "cw.source.read:warehouse", "cw.dataset.query:orders", "cw.execution_context.use:partition")
	repo := &testRepository{err: store.ErrNotFound}
	svc := &Service{repo: repo, evidence: testEvidence{err: store.ErrConflict}, drafts: testDraft{}}
	if _, err := New(nil, nil, nil, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	good := ProposeRequest{ID: "proposal", Topic: "sales", FeedbackID: "feedback", Expected: 1, AuthorIntent: "Review aggregate"}
	bad := good
	bad.AuthorIntent = ""
	if _, err := svc.Propose(ctx, e, bad); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Propose(nil, e, good); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Propose(ctx, proposalEnvelope(t, "topics.write"), good); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := svc.Propose(ctx, e, good); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	repo.err = store.ErrUnavailable
	if _, err := svc.Propose(ctx, e, good); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal(err)
	}
	repo.err = nil
	repo.record = Record{Proposal: Proposal{ID: "proposal", Topic: "sales", RequestDigest: requestDigest(good)}, Candidate: proposalTestPack()}
	if _, err := svc.Propose(ctx, e, good); err != nil {
		t.Fatal("replay", err)
	}
	changed := good
	changed.Expected = 2
	if _, err := svc.Propose(ctx, e, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := svc.Read(ctx, e, ReadRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Read(ctx, proposalEnvelope(t, "topics.write"), ReadRequest{ID: "proposal"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, e, ApplyRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, e, ApplyRequest{ID: "proposal", Digest: string(make([]byte, 64))}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	p := repo.record.Proposal
	p.Digest = proposalDigest(p)
	p.State = "applied"
	p.AppliedRevision = 2
	repo.record.Proposal = p
	if got, err := svc.Apply(ctx, e, ApplyRequest{ID: p.ID, Digest: p.Digest}); err != nil || got.Metadata.Revision != 2 {
		t.Fatal("apply replay", err)
	}
	p.State = "proposed"
	repo.record.Proposal = p
	if _, err := svc.Apply(ctx, e, ApplyRequest{ID: p.ID, Digest: p.Digest}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := svc.Evidence(ctx, e, EvidenceRequest{QueryID: "query"}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal(err)
	}
	svc.evidence = testEvidence{}
	if _, _, err := svc.origin(ctx, e, "feedback"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
}
func TestProposalRecordRejectsTamperedCandidate(t *testing.T) {
	base := proposalTestPack()
	model, err := semantics.Compile(base)
	if err != nil {
		t.Fatal(err)
	}
	agg := semantics.AggregationAverage
	req := ProposeRequest{ID: "proposal", Topic: base.Topic, FeedbackID: "feedback", Expected: 1, AuthorIntent: "Review aggregate"}
	p := Proposal{ID: req.ID, Topic: req.Topic, RequestDigest: requestDigest(req), Origin: nlqexec.SemanticFeedbackOrigin{FeedbackID: req.FeedbackID, QueryID: "query", QueryRevision: 1, Topic: base.Topic, TopicVersion: "v1", PublicationDigest: exec.Hash("publication"), SourceBindingDigest: exec.Hash("source"), EvidenceDigest: exec.Hash("evidence"), Verdict: "negative"}, DraftRevision: 1, DraftDigest: model.Digest(), AuthorIntent: req.AuthorIntent, Edits: []Edit{{Kind: semantics.KindMeasure, ID: "revenue", Aggregation: &agg}}}
	candidate, err := applyEdits(base, "feedback-"+exec.Hash([]any{p.ID, p.RequestDigest})[:24], p.Edits)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := semantics.Compile(candidate)
	p.CandidateDigest = m.Digest()
	p.Digest = proposalDigest(p)
	r := Record{p, candidate}
	if err = ValidateRecord(base, r); err != nil {
		t.Fatal(err)
	}
	r.Candidate.Measures[0].Aggregation = semantics.AggregationCount
	if err = ValidateRecord(base, r); err == nil {
		t.Fatal("tampered candidate admitted")
	}
	r.Candidate = candidate
	r.Proposal.AuthorIntent = "different"
	if err = ValidateRecord(base, r); err == nil {
		t.Fatal("tampered author intent admitted")
	}
}
