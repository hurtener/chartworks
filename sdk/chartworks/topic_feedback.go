package chartworks

import (
	"context"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/topicfeedback"
)

// TopicFeedbackFilterProposal selects only explicitly admitted vocabulary IDs.
type TopicFeedbackFilterProposal = drafts.VocabularyFilterProposal

// TopicFeedbackEvidence identifies owned feedback without historical content.
type TopicFeedbackEvidence = nlqexec.SemanticFeedbackReference

// TopicFeedbackEdit is a closed proposed change to an existing semantic entity.
type TopicFeedbackEdit = topicfeedback.Edit

// TopicFeedbackProposal is a private, explicitly reviewable semantic proposal.
type TopicFeedbackProposal = topicfeedback.Proposal

// TopicFeedbackProposeRequest supplies new author intent and exact owned evidence.
type TopicFeedbackProposeRequest = topicfeedback.ProposeRequest

// TopicFeedbackApplyRequest accepts an exact proposal digest into a private draft.
type TopicFeedbackApplyRequest = topicfeedback.ApplyRequest

// ProposeTopicFeedback never applies or publishes its generated edits.
func (c *Client) ProposeTopicFeedback(ctx context.Context, in TopicFeedbackProposeRequest) (out TopicFeedbackProposal, err error) {
	err = c.call(ctx, "POST", "/v1/topic-feedback/proposals", "", in, &out)
	return
}

// ReadTopicFeedback retrieves owned retained proposal evidence.
func (c *Client) ReadTopicFeedback(ctx context.Context, id string) (out TopicFeedbackProposal, err error) {
	err = c.call(ctx, "POST", "/v1/topic-feedback/read", "", topicfeedback.ReadRequest{ID: id}, &out)
	return
}

// ApplyTopicFeedback creates a private draft; normal review/publication is separate.
func (c *Client) ApplyTopicFeedback(ctx context.Context, in TopicFeedbackApplyRequest) (out TopicDraft, err error) {
	err = c.call(ctx, "POST", "/v1/topic-feedback/apply", "", in, &out)
	return
}

// ListTopicFeedbackEvidence discovers exact content-free owned correction IDs.
func (c *Client) ListTopicFeedbackEvidence(ctx context.Context, queryID string) (out []TopicFeedbackEvidence, err error) {
	err = c.call(ctx, "POST", "/v1/topic-feedback/evidence", "", topicfeedback.EvidenceRequest{QueryID: queryID}, &out)
	return
}
