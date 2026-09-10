package drafts

import (
	"context"
	"errors"
	"fmt"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

var _ engineering.AutopilotTopics = (*Service)(nil)

// PlanAutopilotTopic prepares a new profile scaffold or an exact existing draft
// change. Existing business meaning is preserved until an explicit reviewed edit.
func (s *Service) PlanAutopilotTopic(ctx context.Context, e identity.Envelope, g engineering.AutopilotTopicGoal) (semantics.TopicPack, error) {
	base, err := s.PlanProfile(ctx, e, OnboardRequest{Topic: g.Topic, Profile: g.Profile, Version: g.Version, Name: g.Name, Description: g.Description, Change: "Prepare reviewed engineering topic"})
	if err != nil {
		return semantics.TopicPack{}, err
	}
	current, err := s.Read(ctx, e, g.Topic, 0)
	if g.ExpectedRevision == 0 {
		if err == nil {
			return semantics.TopicPack{}, store.ErrConflict
		}
		if !errors.Is(err, store.ErrNotFound) {
			return semantics.TopicPack{}, err
		}
		return base, nil
	}
	if err != nil {
		return semantics.TopicPack{}, err
	}
	if current.Metadata.Revision != g.ExpectedRevision {
		return semantics.TopicPack{}, store.ErrConflict
	}
	pack := current.Pack
	pack.Version, pack.Name, pack.Description = g.Version, g.Name, g.Description
	if err = s.CheckAutopilotTopic(ctx, e, g, pack); err != nil {
		return semantics.TopicPack{}, err
	}
	return pack, nil
}

// CheckAutopilotTopic verifies exact profile evidence and compilable semantic
// references without persisting a draft or approving business meaning.
func (s *Service) CheckAutopilotTopic(ctx context.Context, e identity.Envelope, g engineering.AutopilotTopicGoal, pack semantics.TopicPack) error {
	base, err := s.PlanProfile(ctx, e, OnboardRequest{Topic: g.Topic, Profile: g.Profile, Version: g.Version, Name: g.Name, Description: g.Description, Change: "Check reviewed engineering topic"})
	if err != nil {
		return err
	}
	if pack.Topic != g.Topic || pack.Version != g.Version || len(pack.Datasets) != 1 || len(base.Datasets) != 1 {
		return store.ErrInvalid
	}
	got, want := pack.Datasets[0], base.Datasets[0]
	if got.ID != want.ID || got.Source != want.Source {
		return store.ErrConflict
	}
	for _, column := range got.Columns {
		found := false
		for _, actual := range want.Columns {
			if column.SourceName == actual.SourceName && column.NativeType == actual.NativeType && column.Category == actual.Category && column.Nullable == actual.Nullable {
				found = true
			}
		}
		if !found {
			return store.ErrInvalid
		}
	}
	model, err := semantics.Compile(pack)
	if err != nil {
		return err
	}
	if err = RequirePack(e, model.Pack(), Write); err != nil {
		return err
	}
	_, err = s.repo.CheckCanonicalMeanings(ctx, e, pack.Topic, Write, model.CanonicalMeanings())
	return err
}

// ApplyAutopilotTopic uses ordinary CAS-fenced private authoring. Lost replies
// reconcile only the same actor/session, exact pack and proposal change note.
func (s *Service) ApplyAutopilotTopic(ctx context.Context, e identity.Envelope, p engineering.AutopilotProposal) (engineering.ProposalTopicResult, error) {
	if p.Material.Topic == nil || p.Material.Request.Topic == nil {
		return engineering.ProposalTopicResult{}, store.ErrInvalid
	}
	g, pack := p.Material.Request.Topic, *p.Material.Topic
	model, err := semantics.Compile(pack)
	if err != nil {
		return engineering.ProposalTopicResult{}, err
	}
	note := fmt.Sprintf("Engineering proposal %s revision %d digest %s", p.ID, p.Revision, p.Digest)
	previous, err := s.Read(ctx, e, g.Topic, g.ExpectedRevision+1)
	if err == nil {
		if previous.Metadata.Change != note || previous.Metadata.Digest != model.Digest() {
			return engineering.ProposalTopicResult{}, store.ErrConflict
		}
		return engineering.ProposalTopicResult{Topic: g.Topic, Revision: previous.Metadata.Revision, Digest: previous.Metadata.Digest}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return engineering.ProposalTopicResult{}, err
	}
	if err = s.CheckAutopilotTopic(ctx, e, *g, pack); err != nil {
		return engineering.ProposalTopicResult{}, err
	}
	saved, err := s.Save(ctx, e, SaveRequest{Expected: g.ExpectedRevision, Pack: pack, Change: note})
	if err != nil {
		return engineering.ProposalTopicResult{}, err
	}
	return engineering.ProposalTopicResult{Topic: g.Topic, Revision: saved.Metadata.Revision, Digest: saved.Metadata.Digest}, nil
}
