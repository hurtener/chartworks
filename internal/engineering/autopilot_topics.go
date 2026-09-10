package engineering

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

// AutopilotTopicGoal addresses profile-backed private topic authoring. Applying
// this change does not publish or approve its business meaning.
type AutopilotTopicGoal struct {
	Topic            string `json:"topic"`
	Profile          string `json:"profile"`
	Version          string `json:"version"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	ExpectedRevision int64  `json:"expected_revision"`
}

// ProposalTopicResult identifies the ordinary saved private draft.
type ProposalTopicResult struct {
	Topic    string
	Revision int64
	Digest   string
}

// AutopilotTopics is implemented by the existing topic draft service. Planning
// and checking perform no writes; applying repeats its normal admission gates.
type AutopilotTopics interface {
	PlanAutopilotTopic(context.Context, identity.Envelope, AutopilotTopicGoal) (semantics.TopicPack, error)
	CheckAutopilotTopic(context.Context, identity.Envelope, AutopilotTopicGoal, semantics.TopicPack) error
	ApplyAutopilotTopic(context.Context, identity.Envelope, AutopilotProposal) (ProposalTopicResult, error)
}

func validTopicGoal(g *AutopilotTopicGoal) bool {
	return g == nil || identity.Identifier(g.Topic) && identity.Identifier(g.Profile) && identity.Identifier(g.Version) && proposalText(g.Name, 128) && proposalText(g.Description, 4096) && g.ExpectedRevision >= 0 && g.ExpectedRevision < 128
}

func validateProposalTopic(m ProposalMaterial) error {
	if (m.Request.Topic == nil) != (m.Topic == nil) {
		return ErrInvalid
	}
	if m.Topic == nil {
		return nil
	}
	g := m.Request.Topic
	if m.Topic.Topic != g.Topic || m.Topic.Version != g.Version || len(m.Topic.Datasets) != 1 {
		return ErrInvalid
	}
	if _, err := semantics.Compile(*m.Topic); err != nil {
		return ErrInvalid
	}
	d := m.Topic.Datasets[0]
	if d.Source.Source != m.Binding.Source || d.Source.Context != m.Binding.Context || d.Source.SourceRevision != m.Binding.Revision || d.Source.ProfileVersion != g.Profile {
		return ErrProposalDrift
	}
	found := false
	for _, r := range m.Binding.Relations {
		if r.ID == d.ID {
			found = true
		}
	}
	if !found {
		return ErrProposalDrift
	}
	return nil
}

func (s *Autopilot) applyTopic(ctx context.Context, e identity.Envelope, p AutopilotProposal) (AutopilotProposal, error) {
	if p.Material.Topic == nil || p.State == "applied" {
		return p, nil
	}
	if s.topics == nil {
		return p, ErrInvalid
	}
	result, err := s.topics.ApplyAutopilotTopic(ctx, e, p)
	if err != nil {
		return p, err
	}
	proof, err := prepareProposalApply(e, p, "engineering.autopilot.apply", "", nil)
	if err != nil {
		return p, err
	}
	return s.repo.RecordAutopilotTopic(ctx, e, proof, result)
}

func topicObjectCount(m ProposalMaterial) int {
	if m.Topic != nil {
		return 1
	}
	return 0
}
