package chartworks

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// ErrBlockRequest rejects a malformed coordinate before any network call.
var ErrBlockRequest = errors.New("chartworks: invalid block coordinate")

// BlockParameterizeRequest mirrors the common governed block wire contract.
type BlockParameterizeRequest = reporting.ParameterizeRequest

// BlockRename mirrors the common governed block wire contract.
type BlockRename = reporting.Rename

// BlockImpactRequest mirrors the common governed block wire contract.
type BlockImpactRequest = reporting.ImpactRequest

// BlockImpact mirrors the common governed block wire contract.
type BlockImpact = reporting.Impact

// BlockApplyImpactRequest mirrors the common governed block wire contract.
type BlockApplyImpactRequest = reporting.ApplyImpactRequest

// BlockLocalized mirrors the common governed block wire contract.
type BlockLocalized = reporting.Localized

// BlockTopicPin mirrors the common governed block wire contract.
type BlockTopicPin = reporting.TopicPin

// BlockTemplatePin mirrors the common governed block wire contract.
type BlockTemplatePin = reporting.TemplatePin

// BlockDimensionReference mirrors the common governed block wire contract.
type BlockDimensionReference = reporting.DimensionReference

// BlockParameter mirrors the common governed block wire contract.
type BlockParameter = reporting.Parameter

// BlockValue mirrors the common governed block wire contract.
type BlockValue = reporting.Value

// BlockArgument mirrors the common governed block wire contract.
type BlockArgument = reporting.Argument

// BlockPeriod mirrors the common governed block wire contract.
type BlockPeriod = reporting.Period

// BlockWindow mirrors the common governed block wire contract.
type BlockWindow = reporting.Window

// BlockResolution mirrors the common governed block wire contract.
type BlockResolution = reporting.Resolution

// BlockBoundValue mirrors the common governed block wire contract.
type BlockBoundValue = reporting.BoundValue

// BlockResolved mirrors the common governed block wire contract.
type BlockResolved = reporting.Resolved

// BlockNarrative mirrors the common governed block wire contract.
type BlockNarrative = reporting.Narrative

// BlockOutput mirrors the common governed block wire contract.
type BlockOutput = reporting.Output

// BlockDefinition mirrors the common governed block wire contract.
type BlockDefinition = reporting.Definition

// BlockReference mirrors the common governed block wire contract.
type BlockReference = reporting.Reference

// BlockState mirrors the common governed block wire contract.
type BlockState = reporting.State

// BlockEvidence mirrors the common governed block wire contract.
type BlockEvidence = reporting.Evidence

// BlockAttestation mirrors the common governed block wire contract.
type BlockAttestation = reporting.Attestation

// BlockWithdrawal mirrors the common governed block wire contract.
type BlockWithdrawal = reporting.Withdrawal

// BlockHealth mirrors the common governed block wire contract.
type BlockHealth = reporting.Health

// BlockTrust mirrors the common governed block wire contract.
type BlockTrust = reporting.Trust

// BlockView mirrors the common governed block wire contract.
type BlockView = reporting.View

// BlockSQLView mirrors the common governed block wire contract.
type BlockSQLView = reporting.SQLView

// BlockCreateRequest mirrors the common governed block wire contract.
type BlockCreateRequest = reporting.CreateRequest

// BlockEditRequest mirrors the common governed block wire contract.
type BlockEditRequest = reporting.EditRequest

// BlockTransitionRequest mirrors the common governed block wire contract.
type BlockTransitionRequest = reporting.TransitionRequest

// BlockRestoreRequest mirrors the common governed block wire contract.
type BlockRestoreRequest = reporting.RestoreRequest

// BlockPublishRequest mirrors the common governed block wire contract.
type BlockPublishRequest = reporting.PublishRequest

// BlockCertifyRequest mirrors the common governed block wire contract.
type BlockCertifyRequest = reporting.CertifyRequest

// BlockWithdrawRequest mirrors the common governed block wire contract.
type BlockWithdrawRequest = reporting.WithdrawRequest

// BlockValidateRequest mirrors the common governed block wire contract.
type BlockValidateRequest = reporting.ValidateRequest

// BlockPreviewRequest mirrors the common governed block wire contract.
type BlockPreviewRequest = reporting.PreviewRequest

// BlockValidationResult mirrors the common governed block wire contract.
type BlockValidationResult = reporting.ValidationResult

// BlockPreviewResult mirrors the common governed block wire contract.
type BlockPreviewResult = reporting.PreviewResult

// BlockCaptureRequest mirrors the common governed block wire contract.
type BlockCaptureRequest = reporting.CaptureRequest

// BlockListRequest mirrors the common governed block wire contract.
type BlockListRequest = reporting.ListRequest

// BlockSummary mirrors the common governed block wire contract.
type BlockSummary = reporting.Summary

// BlockPage mirrors the common governed block wire contract.
type BlockPage = reporting.Page

// BlockQuestionRequest mirrors the common governed block wire contract.
type BlockQuestionRequest = reporting.QuestionRequest

// BlockQuestionMatch mirrors the common governed block wire contract.
type BlockQuestionMatch = reporting.QuestionMatch

// BlockAssessment mirrors the common governed block wire contract.
type BlockAssessment = reporting.Assessment

// BlockEvent mirrors the common governed block wire contract.
type BlockEvent = reporting.Event

// BlockHistory mirrors the common governed block wire contract.
type BlockHistory = reporting.History

// BlockResolveRequest mirrors the common governed block wire contract.
type BlockResolveRequest = reporting.ResolveRequest

// BlockResolutionResult mirrors the common governed block wire contract.
type BlockResolutionResult = reporting.ResolutionResult

// BlockProvenance mirrors the common governed block wire contract.
type BlockProvenance = reporting.Provenance

// ParameterizeBlock append an AST-verified typed period amendment without publication. Mutations are never automatically replayed.
func (c *Client) ParameterizeBlock(ctx context.Context, id string, in BlockParameterizeRequest) (out BlockView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/parameters/assist"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// RecheckBlockImpact explicitly observe dependency impact without altering definitions. Mutations are never automatically replayed.
func (c *Client) RecheckBlockImpact(ctx context.Context, id string, in BlockImpactRequest) (out BlockImpact, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/impact"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// ApplyBlockImpact create a private draft for an exact current dependency proposal. Mutations are never automatically replayed.
func (c *Client) ApplyBlockImpact(ctx context.Context, id string, in BlockApplyImpactRequest) (out BlockView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/impact/apply"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// CreateBlock create an unvalidated private block draft. Mutations are never automatically replayed.
func (c *Client) CreateBlock(ctx context.Context, in BlockCreateRequest) (out BlockView, err error) {
	path := "/v1/blocks"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// ListBlocks list only currently authorized block definitions. Mutations are never automatically replayed.
func (c *Client) ListBlocks(ctx context.Context, in BlockListRequest) (out BlockPage, err error) {
	path := "/v1/blocks"
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return out, ErrBlockRequest
	}
	q := url.Values{"limit": []string{strconv.Itoa(in.Limit)}}
	if in.After != "" {
		q.Set("after", in.After)
	}
	if in.IncludeDrafts {
		q.Set("include_drafts", "true")
	}
	path += "?" + q.Encode()
	err = c.callLimit(ctx, "GET", path, "", nil, &out, 16<<20)
	return
}

// CaptureBlock capture a completed authorized query as an unvalidated draft. Mutations are never automatically replayed.
func (c *Client) CaptureBlock(ctx context.Context, in BlockCaptureRequest) (out BlockView, err error) {
	path := "/v1/blocks/capture"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// AssessBlockQuestions assess authorized localized questions with bounded lexical matching. Mutations are never automatically replayed.
func (c *Client) AssessBlockQuestions(ctx context.Context, in BlockQuestionRequest) (out BlockAssessment, err error) {
	path := "/v1/blocks/questions/assess"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// ReadBlock read a SQL-private published or authorized exact revision. Mutations are never automatically replayed.
func (c *Client) ReadBlock(ctx context.Context, id string, in BlockReference) (out BlockView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + ""
	if in.Revision < 0 || in.Revision > 256 || in.Draft && in.Revision != 0 {
		return out, ErrBlockRequest
	}
	q := url.Values{}
	if in.Revision > 0 {
		q.Set("revision", strconv.FormatInt(in.Revision, 10))
	}
	if in.Draft {
		q.Set("draft", "true")
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err = c.callLimit(ctx, "GET", path, "", nil, &out, 16<<20)
	return
}

// ReadBlockSQL read SQL through separately scoped inspection authority. Mutations are never automatically replayed.
func (c *Client) ReadBlockSQL(ctx context.Context, id string, in BlockReference) (out BlockSQLView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/sql"
	if in.Revision < 0 || in.Revision > 256 || in.Draft && in.Revision != 0 {
		return out, ErrBlockRequest
	}
	q := url.Values{}
	if in.Revision > 0 {
		q.Set("revision", strconv.FormatInt(in.Revision, 10))
	}
	if in.Draft {
		q.Set("draft", "true")
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err = c.callLimit(ctx, "GET", path, "", nil, &out, 16<<20)
	return
}

// BlockHistory read permission-filtered immutable lifecycle history. Mutations are never automatically replayed.
func (c *Client) BlockHistory(ctx context.Context, id string) (out BlockHistory, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/history"
	err = c.callLimit(ctx, "GET", path, "", nil, &out, 16<<20)
	return
}

// EditBlock append a CAS-fenced private amendment without publication. Mutations are never automatically replayed.
func (c *Client) EditBlock(ctx context.Context, id string, in BlockEditRequest) (out BlockView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + ""
	err = c.callLimit(ctx, "PUT", path, "", in, &out, 16<<20)
	return
}

// ValidateBlock validate an exact revision through the existing bounded read core. Mutations are never automatically replayed.
func (c *Client) ValidateBlock(ctx context.Context, id string, in BlockValidateRequest) (out BlockValidationResult, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/validate"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// PreviewBlock preview exact outputs privately without retaining artifacts. Mutations are never automatically replayed.
func (c *Client) PreviewBlock(ctx context.Context, id string, in BlockPreviewRequest) (out BlockPreviewResult, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/preview"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// PublishBlock publish one exact revision against fresh validation evidence. Mutations are never automatically replayed.
func (c *Client) PublishBlock(ctx context.Context, id string, in BlockPublishRequest) (out BlockState, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/publish"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// CertifyBlock attest to one exact published revision and evidence receipt. Mutations are never automatically replayed.
func (c *Client) CertifyBlock(ctx context.Context, id string, in BlockCertifyRequest) (out BlockAttestation, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/certify"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// WithdrawBlockCertification withdraw an attestation while preserving its history. Mutations are never automatically replayed.
func (c *Client) WithdrawBlockCertification(ctx context.Context, id string, in BlockWithdrawRequest) (out BlockWithdrawal, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/withdraw"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// RejectBlock reject the current private draft without removing history. Mutations are never automatically replayed.
func (c *Client) RejectBlock(ctx context.Context, id string, in BlockTransitionRequest) (out BlockState, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/reject"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// RestoreBlock copy an authorized revision into a new unvalidated draft. Mutations are never automatically replayed.
func (c *Client) RestoreBlock(ctx context.Context, id string, in BlockRestoreRequest) (out BlockView, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/restore"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// ArchiveBlock archive the default publication while retaining exact revisions. Mutations are never automatically replayed.
func (c *Client) ArchiveBlock(ctx context.Context, id string, in BlockTransitionRequest) (out BlockState, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/archive"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}

// ResolveBlockParameters resolve typed defaults and logical period windows without execution. Mutations are never automatically replayed.
func (c *Client) ResolveBlockParameters(ctx context.Context, id string, in BlockResolveRequest) (out BlockResolutionResult, err error) {
	if !identity.Identifier(id) {
		return out, ErrBlockRequest
	}
	path := "/v1/blocks/" + id + "/parameters/resolve"
	err = c.callLimit(ctx, "POST", path, "", in, &out, 16<<20)
	return
}
