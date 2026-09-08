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

// TopicPack is the complete private semantic authoring definition.
type TopicPack = semantics.TopicPack

// TopicDataset is one source-bound semantic dataset.
type TopicDataset = semantics.Dataset

// TopicColumn is one stable semantic column.
type TopicColumn = semantics.Column

// TopicSourceReference pins profile and source evidence.
type TopicSourceReference = semantics.SourceReference

// TopicReference addresses an exact semantic entity.
type TopicReference = semantics.Reference

// TopicMeasure is an aggregate semantic entity.
type TopicMeasure = semantics.Measure

// TopicDimension is a grouping semantic entity.
type TopicDimension = semantics.Dimension

// TopicKPI is a derived business expression.
type TopicKPI = semantics.KPI

// TopicJoin is an equality relationship between datasets.
type TopicJoin = semantics.Join

// TopicCanonicalEntity binds global meaning to topic-local keys.
type TopicCanonicalEntity = semantics.CanonicalEntity

// PortableTopicPack is a neutral logical-slot topic bundle.
type PortableTopicPack = semantics.PortablePack

// TopicExportDatasetSlots maps one dataset for export.
type TopicExportDatasetSlots = semantics.ExportDatasetSlots

// TopicExportColumnSlot maps one column for export.
type TopicExportColumnSlot = semantics.ExportColumnSlot

// TopicDraftBindings binds a portable pack to one installation.
type TopicDraftBindings = semantics.DraftBindings

// TopicImportDatasetBinding supplies one destination dataset binding.
type TopicImportDatasetBinding = semantics.ImportDatasetBinding

// TopicImportColumnBinding supplies one destination column binding.
type TopicImportColumnBinding = semantics.ImportColumnBinding

// TopicVersionDiff compares two retained semantic revisions.
type TopicVersionDiff = semantics.VersionDiff

// TopicReviewRequest decides one exact draft digest.
type TopicReviewRequest = semantictopics.ReviewRequest

// TopicReview is an immutable review receipt.
type TopicReview = semantictopics.Review

// PublishTopicRequest activates an approved draft.
type PublishTopicRequest = semantictopics.PublishRequest

// PublishedTopic is one retained public semantic definition.
type PublishedTopic = semantictopics.Published

// PublishedTopicState is the current lifecycle pointer.
type PublishedTopicState = semantictopics.State

// TopicContract is a current-source-confirmed publication.
type TopicContract = semantictopics.Contract

// TopicHealth is the retained public source-health observation.
type TopicHealth = semantictopics.Health

// TopicTransitionRequest CAS-fences a rollback.
type TopicTransitionRequest = semantictopics.TransitionRequest

// RuleSetDefinition is a version-pinned topic ruleset.
type RuleSetDefinition = semantics.RuleSetDefinition

// RuleDefinition is one semantic or execution rule.
type RuleDefinition = semantics.RuleDefinition

// RuleScope identifies the semantic objects governed by a rule.
type RuleScope = semantics.RuleScope

// RuleProvenance records rule authoring provenance.
type RuleProvenance = semantics.RuleProvenance

// RuleConstraint is a closed non-executable hard constraint.
type RuleConstraint = semantics.Constraint

// RuleDraft is one immutable private rules revision.
type RuleDraft = rulesets.Draft

// SaveRuleDraftRequest creates a private rules revision.
type SaveRuleDraftRequest = rulesets.SaveRequest

// RuleReviewRequest decides an exact rules digest.
type RuleReviewRequest = rulesets.ReviewRequest

// RuleReview is an immutable rules review receipt.
type RuleReview = rulesets.Review

// PublishRulesRequest activates approved rules.
type PublishRulesRequest = rulesets.PublishRequest

// PublishedRules is one retained public rules version.
type PublishedRules = rulesets.Published

// RetireRulesRequest retires the active rules pointer.
type RetireRulesRequest = rulesets.RetireRequest

// RuleState is the current rules lifecycle pointer.
type RuleState = rulesets.State

// RuleEvaluationRequest supplies references for deterministic evaluation.
type RuleEvaluationRequest = rulesets.EvaluateRequest

// RuleEvaluation contains deterministic hard-constraint results.
type RuleEvaluation = rulesets.Evaluation

// ArchiveTopicRequest CAS-fences a topic archive transition.
type ArchiveTopicRequest struct {
	Expected int64  `json:"expected_revision"`
	Note     string `json:"note"`
}

// TopicDraftRevision is retained private revision metadata.
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

// TopicDraft contains one private immutable pack and metadata.
type TopicDraft struct {
	Metadata TopicDraftRevision `json:"metadata"`
	Pack     TopicPack          `json:"pack"`
}

// SaveTopicDraftRequest creates one CAS-fenced private revision.
type SaveTopicDraftRequest struct {
	Expected int64     `json:"expected_revision"`
	Pack     TopicPack `json:"pack"`
	Change   string    `json:"change"`
}

// ImportTopicDraftRequest binds a neutral pack into a private draft.
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

// EnhanceTopicRequest advances one bounded generation checkpoint.
type EnhanceTopicRequest = drafts.EnhanceRequest

// EnhanceTopicResult includes the committed draft and next stable cursor.
type EnhanceTopicResult = drafts.EnhanceResult

// TopicEntityMutation is one closed entity put or delete operation.
type TopicEntityMutation = semantics.EntityMutation

// TopicColumnRebinding maps a stable semantic column ID to a physical source name.
type TopicColumnRebinding = drafts.ColumnRebinding

// SaveTopicDraft creates one private immutable draft revision.
func (c *Client) SaveTopicDraft(ctx context.Context, in SaveTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-drafts", "", in, &out, 2<<20)
	return
}

// ImportTopicDraft maps and admits a neutral portable pack.
func (c *Client) ImportTopicDraft(ctx context.Context, in ImportTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-draft-imports", "", in, &out, 2<<20)
	return
}

// OnboardTopicProfile creates an unresolved draft from active private profile evidence.
func (c *Client) OnboardTopicProfile(ctx context.Context, in OnboardTopicProfileRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-onboarding", "", in, &out, 2<<20)
	return
}

// TopicDraft reads the current scoped private draft.
func (c *Client) TopicDraft(ctx context.Context, id string) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/draft", "", nil, &out, 2<<20)
	return
}

// TopicDraftVersion reads an exact scoped private revision.
func (c *Client) TopicDraftVersion(ctx context.Context, id string, revision int64) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-versions/read", "", struct {
		Revision int64 `json:"revision"`
	}{revision}, &out, 2<<20)
	return
}

// TopicDraftHistory lists bounded descending private revision metadata.
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

// DiffTopicDraft compares two exact private revisions.
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

// ExportTopicDraft projects an exact revision into neutral logical slots.
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

// EnhanceTopicDraft advances one Bifrost-backed resumable authoring step.
func (c *Client) EnhanceTopicDraft(ctx context.Context, id string, in EnhanceTopicRequest) (out EnhanceTopicResult, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-enhancements", "", in, &out, 2<<20)
	return
}

// ReviewTopic records a decision for one exact draft digest.
func (c *Client) ReviewTopic(ctx context.Context, id string, in TopicReviewRequest) (out TopicReview, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/reviews", "", in, &out, 2<<20)
	return
}

// PublishTopic activates an approved draft and matching facets atomically.
func (c *Client) PublishTopic(ctx context.Context, id string, in PublishTopicRequest) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/publications", "", in, &out, 2<<20)
	return
}

// PublishedTopic reads the retained active public definition.
func (c *Client) PublishedTopic(ctx context.Context, id string) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/published", "", nil, &out, 2<<20)
	return
}

// PublishedTopicVersion reads an exact retained public version.
func (c *Client) PublishedTopicVersion(ctx context.Context, id, version string) (out PublishedTopic, err error) {
	if !wireID(id) || !wireID(version) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/published-versions/read", "", struct {
		Version string `json:"version"`
	}{version}, &out, 2<<20)
	return
}

// TopicContract reads a publication after current source validation.
func (c *Client) TopicContract(ctx context.Context, id string) (out TopicContract, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/contract", "", nil, &out, 2<<20)
	return
}

// HealthTopic reads retained current-source health without source or model work.
func (c *Client) HealthTopic(ctx context.Context, id string) (out TopicHealth, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.call(ctx, "GET", "/v1/topics/"+id+"/health", "", nil, &out)
	return
}

// RecheckTopic verifies current public source continuity and commits the observation.
func (c *Client) RecheckTopic(ctx context.Context, id string) (out TopicHealth, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.call(ctx, "POST", "/v1/topics/"+id+"/recheck", "", struct{}{}, &out)
	return
}

// RollbackTopic reactivates one exact retained definition and facet set.
func (c *Client) RollbackTopic(ctx context.Context, id string, in TopicTransitionRequest) (out PublishedTopic, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rollbacks", "", in, &out, 2<<20)
	return
}

// ArchiveTopic retires the active topic and matching facet heads.
func (c *Client) ArchiveTopic(ctx context.Context, id string, in ArchiveTopicRequest) (out PublishedTopicState, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/archive", "", in, &out, 2<<20)
	return
}

// SaveRuleDraft creates one immutable private rules revision.
func (c *Client) SaveRuleDraft(ctx context.Context, id string, in SaveRuleDraftRequest) (out RuleDraft, err error) {
	if !wireID(id) || in.Definition.Topic != id {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-drafts", "", in, &out, 2<<20)
	return
}

// ReviewRules records a decision for one exact rules digest.
func (c *Client) ReviewRules(ctx context.Context, id string, in RuleReviewRequest) (out RuleReview, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-reviews", "", in, &out, 2<<20)
	return
}

// PublishRules activates an approved rules revision.
func (c *Client) PublishRules(ctx context.Context, id string, in PublishRulesRequest) (out PublishedRules, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-publications", "", in, &out, 2<<20)
	return
}

// PublishedRules reads the active retained rules version.
func (c *Client) PublishedRules(ctx context.Context, id string) (out PublishedRules, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/rules", "", nil, &out, 2<<20)
	return
}

// PublishedRuleVersion reads one exact retained rules version.
func (c *Client) PublishedRuleVersion(ctx context.Context, id, version string) (out PublishedRules, err error) {
	if !wireID(id) || !wireID(version) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rule-versions/read", "", struct {
		Version string `json:"version"`
	}{version}, &out, 2<<20)
	return
}

// EvaluateRules applies deterministic non-executable hard constraints.
func (c *Client) EvaluateRules(ctx context.Context, id string, in RuleEvaluationRequest) (out RuleEvaluation, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rules/evaluate", "", in, &out, 2<<20)
	return
}

// RetireRules retires the current active rules pointer.
func (c *Client) RetireRules(ctx context.Context, id string, in RetireRulesRequest) (out RuleState, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/rules/retire", "", in, &out, 2<<20)
	return
}
