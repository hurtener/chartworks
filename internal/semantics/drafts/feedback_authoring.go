package drafts

import (
	"context"
	"encoding/json"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"reflect"
	"time"
)

// PrepareFeedbackAuthoring reuses the complete authoring disclosure and live
// profile admission. The private pack is never the model input: only the second
// return value has passed the authoring disclosure projection.
func (s *Service) PrepareFeedbackAuthoring(ctx context.Context, e identity.Envelope, topic string, expected int64) (Version, json.RawMessage, error) {
	return s.PrepareFeedbackVocabulary(ctx, e, topic, expected, nil)
}

// PrepareFeedbackVocabulary admits explicit caller vocabulary through the same live disclosure fence.
func (s *Service) PrepareFeedbackVocabulary(ctx context.Context, e identity.Envelope, topic string, expected int64, vocabulary []AuthoringValue) (Version, json.RawMessage, error) {
	if ctx == nil || expected < 1 || expected >= MaxRevisions {
		return Version{}, nil, store.ErrInvalid
	}
	current, err := s.repo.ReadTopicDraft(ctx, e, topic, 0, Write)
	if err != nil {
		return Version{}, nil, err
	}
	if current.Metadata.Revision != expected {
		return Version{}, nil, store.ErrConflict
	}
	model, err := semantics.Compile(current.Pack)
	if err != nil {
		return Version{}, nil, err
	}
	input, err := s.authoringContext(ctx, e, model, vocabulary)
	if err != nil {
		return Version{}, nil, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return Version{}, nil, store.ErrInvalid
	}
	return current, raw, nil
}

// FeedbackApplication is a content-bound reference to a persisted proposal.
// It acquires no authority until the repository checks its exact stored candidate.
type FeedbackApplication struct {
	ID     string
	Digest string
}

// FeedbackApplication returns the optional application fence only to its owner.
func (p Prepared) FeedbackApplication(e identity.Envelope) (FeedbackApplication, bool, error) {
	if _, err := p.Pack(e); err != nil {
		return FeedbackApplication{}, false, err
	}
	if p.feedback == nil {
		return FeedbackApplication{}, false, nil
	}
	return *p.feedback, true, nil
}

// PrepareFeedbackRevision checks live profile/source evidence and returns an
// opaque ordinary-draft admission. Publication and quality approval are absent.
func (s *Service) PrepareFeedbackRevision(ctx context.Context, e identity.Envelope, pack semantics.TopicPack, expected int64, application FeedbackApplication) (Prepared, error) {
	if ctx == nil || !identity.Identifier(application.ID) || len(application.Digest) != 64 {
		return Prepared{}, store.ErrInvalid
	}
	current, _, err := s.PrepareFeedbackAuthoring(ctx, e, pack.Topic, expected)
	if err != nil {
		return Prepared{}, err
	}
	// Proposals may change semantic entities, never physical source provenance.
	if !reflect.DeepEqual(current.Pack.Datasets, pack.Datasets) {
		return Prepared{}, readexec.ErrBinding
	}
	model, err := semantics.Compile(pack)
	if err != nil {
		return Prepared{}, err
	}
	if _, err = s.repo.CheckCanonicalMeanings(ctx, e, pack.Topic, Write, model.CanonicalMeanings()); err != nil {
		return Prepared{}, err
	}
	if _, err = s.authoringContext(ctx, e, model); err != nil {
		return Prepared{}, err
	}
	deadline := time.Now().Add(10 * time.Second)
	if e.Deadline().Before(deadline) {
		deadline = e.Deadline()
	}
	return Prepared{model: model, tenant: e.Tenant(), actor: e.User(), session: e.Session(), deadline: deadline, feedback: &application}, nil
}
