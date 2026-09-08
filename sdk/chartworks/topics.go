package chartworks

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	semantictopics "github.com/hurtener/chartworks/internal/semantics/topics"
)

// These public authoring DTO aliases preserve the compiler's exact reference
// contract. They carry no verified authority, publication or execution proof.
type TopicPack = semantics.TopicPack
type TopicDataset = semantics.Dataset
type TopicColumn = semantics.Column
type TopicSourceReference = semantics.SourceReference
type TopicReference = semantics.Reference
type TopicMeasure = semantics.Measure
type TopicDimension = semantics.Dimension
type TopicKPI = semantics.KPI
type TopicJoin = semantics.Join
type TopicCanonicalEntity = semantics.CanonicalEntity
type PortableTopicPack = semantics.PortablePack
type TopicExportDatasetSlots = semantics.ExportDatasetSlots
type TopicExportColumnSlot = semantics.ExportColumnSlot
type TopicDraftBindings = semantics.DraftBindings
type TopicImportDatasetBinding = semantics.ImportDatasetBinding
type TopicImportColumnBinding = semantics.ImportColumnBinding
type TopicVersionDiff = semantics.VersionDiff
type TopicReviewRequest = semantictopics.ReviewRequest
type TopicReview = semantictopics.Review
type PublishTopicRequest = semantictopics.PublishRequest
type PublishedTopic = semantictopics.Published
type PublishedTopicState = semantictopics.State
type TopicContract = semantictopics.Contract
type TopicTransitionRequest = semantictopics.TransitionRequest
type RuleSetDefinition = semantics.RuleSetDefinition
type RuleDefinition = semantics.RuleDefinition
type RuleScope = semantics.RuleScope
type RuleProvenance = semantics.RuleProvenance
type RuleConstraint = semantics.Constraint
type RuleDraft = rulesets.Draft
type SaveRuleDraftRequest = rulesets.SaveRequest
type RuleReviewRequest = rulesets.ReviewRequest
type RuleReview = rulesets.Review
type PublishRulesRequest = rulesets.PublishRequest
type PublishedRules = rulesets.Published
type RetireRulesRequest = rulesets.RetireRequest
type RuleState = rulesets.State
type RuleEvaluationRequest = rulesets.EvaluateRequest
type RuleEvaluation = rulesets.Evaluation

type ArchiveTopicRequest struct {
	Expected int64  `json:"expected_revision"`
	Note     string `json:"note"`
}

type TopicDraftRevision struct {
	Topic    string    `json:"topic"`
	Revision int64     `json:"revision"`
	Version  string    `json:"version"`
	Digest   string    `json:"digest"`
	Actor    string    `json:"actor"`
	Session  string    `json:"session"`
	Created  time.Time `json:"created_at"`
	Change   string    `json:"change"`
}
type TopicDraft struct {
	Metadata TopicDraftRevision `json:"metadata"`
	Pack     TopicPack          `json:"pack"`
}
type SaveTopicDraftRequest struct {
	Expected int64     `json:"expected_revision"`
	Pack     TopicPack `json:"pack"`
	Change   string    `json:"change"`
}
type ImportTopicDraftRequest struct {
	Expected int64              `json:"expected_revision"`
	Portable PortableTopicPack  `json:"portable"`
	Bindings TopicDraftBindings `json:"bindings"`
	Change   string             `json:"change"`
}

// OnboardTopicProfileRequest creates an unresolved topic draft from a private profile.
type OnboardTopicProfileRequest = drafts.OnboardRequest

// MutateTopicEntitiesRequest applies atomic CRUD to an exact draft revision.
type MutateTopicEntitiesRequest = drafts.EntityMutationRequest

// RebindTopicDatasetRequest moves a draft dataset to active profile evidence.
type RebindTopicDatasetRequest = drafts.RebindRequest

// TopicEntityMutation is one closed entity put or delete operation.
type TopicEntityMutation = semantics.EntityMutation

// TopicColumnRebinding maps a stable semantic column ID to a physical source name.
type TopicColumnRebinding = drafts.ColumnRebinding

func (c *Client) SaveTopicDraft(ctx context.Context, in SaveTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-drafts", "", in, &out, 2<<20)
	return
}
func (c *Client) ImportTopicDraft(ctx context.Context, in ImportTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-draft-imports", "", in, &out, 2<<20)
	return
}

// OnboardTopicProfile creates an unresolved draft from active private profile evidence.
func (c *Client) OnboardTopicProfile(ctx context.Context, in OnboardTopicProfileRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-onboarding", "", in, &out, 2<<20)
	return
}
func (c *Client) TopicDraft(ctx context.Context, id string) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/draft", "", nil, &out, 2<<20)
	return
}
func (c *Client) TopicDraftVersion(ctx context.Context, id string, revision int64) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-versions/read", "", struct {
		Revision int64 `json:"revision"`
	}{revision}, &out, 2<<20)
	return
}
func (c *Client) TopicDraftHistory(ctx context.Context, id string, before int64, limit int) (out []TopicDraftRevision, err error) {
	if !wireID(id) {
		return nil, errors.New("chartworks: invalid topic identifier")
	}
	err = c.call(ctx, "POST", "/v1/topics/"+id+"/draft-history", "", struct {
		Before int64 `json:"before"`
		Limit  int   `json:"limit"`
	}{before, limit}, &out)
	return
}
func (c *Client) DiffTopicDraft(ctx context.Context, id string, before, after int64) (out TopicVersionDiff, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-diff", "", struct {
		Before int64 `json:"before"`
		After  int64 `json:"after"`
	}{before, after}, &out, 16<<20)
	return
}
func (c *Client) ExportTopicDraft(ctx context.Context, id string, revision int64, mapping []TopicExportDatasetSlots) (out PortableTopicPack, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-export", "", struct {
		Revision int64                     `json:"revision"`
		Mapping  []TopicExportDatasetSlots `json:"mapping"`
	}{revision, mapping}, &out, 2<<20)
	return
}

// MutateTopicEntities applies one atomic entity CRUD batch to a new draft revision.
func (c *Client) MutateTopicEntities(ctx context.Context, id string, in MutateTopicEntitiesRequest) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-entities", "", in, &out, 2<<20)
	return
}

// RebindTopicDataset maps stable column IDs to active target profile evidence.
func (c *Client) RebindTopicDataset(ctx context.Context, id string, in RebindTopicDatasetRequest) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-rebind", "", in, &out, 2<<20)
	return
}

func (c *Client) ReviewTopic(ctx context.Context, id string, in TopicReviewRequest) (out TopicReview, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/reviews", "", in, &out, 2<<20)
	return
}

func (c *Client) PublishTopic(ctx context.Context, id string, in PublishTopicRequest) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/publications", "", in, &out, 2<<20)
	return
}

func (c *Client) PublishedTopic(ctx context.Context, id string) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/published", "", nil, &out, 2<<20)
	return
}

func (c *Client) PublishedTopicVersion(ctx context.Context, id, version string) (out PublishedTopic, err error) {
	if !wireID(id) || !wireID(version) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/published-versions/read", "", struct {
		Version string `json:"version"`
	}{version}, &out, 2<<20)
	return
}

func (c *Client) TopicContract(ctx context.Context, id string) (out TopicContract, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/contract", "", nil, &out, 2<<20)
	return
}

func (c *Client) RollbackTopic(ctx context.Context, id string, in TopicTransitionRequest) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rollbacks", "", in, &out, 2<<20)
	return
}

func (c *Client) ArchiveTopic(ctx context.Context, id string, in ArchiveTopicRequest) (out PublishedTopicState, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/archive", "", in, &out, 2<<20)
	return
}

func (c *Client) SaveRuleDraft(ctx context.Context, id string, in SaveRuleDraftRequest) (out RuleDraft, err error) {
	if !wireID(id) || in.Definition.Topic != id {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-drafts", "", in, &out, 2<<20)
	return
}

func (c *Client) ReviewRules(ctx context.Context, id string, in RuleReviewRequest) (out RuleReview, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-reviews", "", in, &out, 2<<20)
	return
}

func (c *Client) PublishRules(ctx context.Context, id string, in PublishRulesRequest) (out PublishedRules, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-publications", "", in, &out, 2<<20)
	return
}

func (c *Client) PublishedRules(ctx context.Context, id string) (out PublishedRules, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/rules", "", nil, &out, 2<<20)
	return
}

func (c *Client) PublishedRuleVersion(ctx context.Context, id, version string) (out PublishedRules, err error) {
	if !wireID(id) || !wireID(version) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-versions/read", "", struct {
		Version string `json:"version"`
	}{version}, &out, 2<<20)
	return
}

func (c *Client) EvaluateRules(ctx context.Context, id string, in RuleEvaluationRequest) (out RuleEvaluation, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rules/evaluate", "", in, &out, 2<<20)
	return
}

func (c *Client) RetireRules(ctx context.Context, id string, in RetireRulesRequest) (out RuleState, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rules/retire", "", in, &out, 2<<20)
	return
}
