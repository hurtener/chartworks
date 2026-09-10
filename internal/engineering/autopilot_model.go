package engineering

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// AutopilotVersion identifies immutable reviewed engineering material.
const AutopilotVersion = "reviewed-engineering-v1"

// AutopilotPromptVersion pins the bounded planning and matching instructions.
const AutopilotPromptVersion = "scope-plan-confirm-v1"

// ErrProposalReview means that the exact material has not received an independent
// review. Being its creator, a model, or a named administrator is not approval.
var ErrProposalReview = errors.New("engineering: independent proposal review required")

// ErrProposalDrift rejects changed source or amendment evidence.
var ErrProposalDrift = errors.New("engineering: proposal source evidence changed")

// ErrProposalConflict rejects effects conflicting with current managed state.
var ErrProposalConflict = errors.New("engineering: reviewed proposal effect conflicts with current state")

// ErrCompensationBlocked preserves referenced or unowned managed effects.
var ErrCompensationBlocked = errors.New("engineering: compensation blocked by dependencies or non-owned effects")

// AutopilotGoal fixes source and managed destination coordinates before a model
// sees any catalog metadata. It contains no identity, credentials or approval flag.
type AutopilotGoal struct {
	ID                      string `json:"id"`
	Pipeline                string `json:"pipeline"`
	Name                    string `json:"name"`
	Connection              string `json:"connection"`
	Source                  string `json:"source"`
	Context                 string `json:"context"`
	Goal                    string `json:"goal"`
	ExpectedPipelineVersion int64  `json:"expected_pipeline_version"`
	MaxStalenessSeconds     int    `json:"max_staleness_seconds"`
}

// AutopilotPlan is the retained blind-planning output, before catalog retrieval.
type AutopilotPlan struct {
	Requirements []string `json:"requirements"`
	Rationale    string   `json:"rationale"`
}

// ProposalEvidence identifies an actually observed binding/relation. It contains
// neither result values nor a caller assertion that an unrelated source is safe.
type ProposalEvidence struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Context string `json:"context"`
	Dataset string `json:"dataset,omitempty"`
	Digest  string `json:"digest"`
}

// ProposalAlternative names an authorized dataset considered for direct reuse.
type ProposalAlternative struct {
	Dataset   string `json:"dataset"`
	Rationale string `json:"rationale"`
}

// ProposalObject retains the decision and provenance for each intended object.
// Dataset identities are derived from the managed pipeline contract, not invented
// by model output. Topic meaning is never published by this execution lane.
type ProposalObject struct {
	Kind         string                `json:"kind"`
	ID           string                `json:"id"`
	Decision     string                `json:"decision"`
	Rationale    string                `json:"rationale"`
	Evidence     []string              `json:"evidence"`
	Alternatives []ProposalAlternative `json:"alternatives"`
	Author       string                `json:"author"`
	ModelVersion string                `json:"model_version"`
}

// ProposalReference records required source-partition reach.
type ProposalReference struct {
	Kind       string `json:"kind"`
	Permission string `json:"permission"`
	ID         string `json:"id"`
}

// ProposalOrigin binds a new independently reviewed amendment to exact drift evidence.
type ProposalOrigin struct {
	Proposal       string `json:"proposal"`
	Revision       int64  `json:"revision"`
	Digest         string `json:"digest"`
	Drift          string `json:"drift"`
	EvidenceDigest string `json:"evidence_digest"`
}

// AutopilotAmendRequest selects exact drift evidence for independent review.
type AutopilotAmendRequest struct {
	Drift string `json:"drift"`
}

// ProposalMaterial is the immutable unit of review, including amendment provenance.
type ProposalMaterial struct {
	Origin        *ProposalOrigin     `json:"origin,omitempty"`
	Version       string              `json:"version"`
	Request       AutopilotGoal       `json:"request"`
	Plan          AutopilotPlan       `json:"blind_plan"`
	Binding       readexec.Binding    `json:"binding"`
	Pipeline      PipelineDefinition  `json:"pipeline"`
	Evidence      []ProposalEvidence  `json:"evidence"`
	Objects       []ProposalObject    `json:"objects"`
	References    []ProposalReference `json:"references"`
	Author        string              `json:"author"`
	Session       string              `json:"session"`
	Created       time.Time           `json:"created_at"`
	ModelVersion  string              `json:"model_version"`
	PromptVersion string              `json:"prompt_version"`
	Usage         gateway.Receipt     `json:"usage"`
}

// Digest binds the entire immutable proposal material.
func (m ProposalMaterial) Digest() string { return readexec.Hash(m) }

// ProposalReview records the signed independent decision over exact material.
type ProposalReview struct {
	Revision int64     `json:"revision"`
	Digest   string    `json:"digest"`
	Decision string    `json:"decision"`
	Reason   string    `json:"reason"`
	Actor    string    `json:"actor"`
	Session  string    `json:"session"`
	Created  time.Time `json:"created_at"`
}

// ProposalEffect distinguishes intended, committed and reconciled work. The
// operation receipt owns physical attempts; no global atomicity is asserted.
type ProposalEffect struct {
	Kind      string    `json:"kind"`
	Target    string    `json:"target"`
	State     string    `json:"state"`
	Version   int64     `json:"version,omitempty"`
	Digest    string    `json:"digest,omitempty"`
	Operation string    `json:"operation,omitempty"`
	Code      string    `json:"code,omitempty"`
	Observed  time.Time `json:"observed_at"`
}

// AutopilotProposal combines immutable material with review and staged effects.
type AutopilotProposal struct {
	ID           string           `json:"id"`
	Tenant       string           `json:"tenant"`
	Version      int64            `json:"version"`
	Revision     int64            `json:"revision"`
	Digest       string           `json:"digest"`
	State        string           `json:"state"`
	OriginAuthor string           `json:"origin_author"`
	Material     ProposalMaterial `json:"material"`
	Review       *ProposalReview  `json:"review,omitempty"`
	Effects      []ProposalEffect `json:"effects"`
	Operation    string           `json:"operation,omitempty"`
	ApplyActor   string           `json:"apply_actor,omitempty"`
	ApplySession string           `json:"apply_session,omitempty"`
	Created      time.Time        `json:"created_at"`
	Updated      time.Time        `json:"updated_at"`
	Applied      *time.Time       `json:"applied_at,omitempty"`
	AmendmentOf  string           `json:"amendment_of,omitempty"`
}

// AutopilotReviewRequest binds approval or rejection to an exact revision.
type AutopilotReviewRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Revision        int64  `json:"revision"`
	Digest          string `json:"digest"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}

// AutopilotEditRequest appends material without inheriting approval.
type AutopilotEditRequest struct {
	ExpectedVersion int64              `json:"expected_version"`
	Definition      PipelineDefinition `json:"definition"`
	Reason          string             `json:"reason"`
}

// AutopilotApplyRequest identifies reviewed material and explicit retry intent.
type AutopilotApplyRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Revision        int64  `json:"revision"`
	Digest          string `json:"digest"`
	Resume          bool   `json:"resume"`
}

// ProposalImpact identifies a currently authorized affected definition.
type ProposalImpact struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// AutopilotDrift retains observed failure evidence without changing definitions.
type AutopilotDrift struct {
	ID                   string           `json:"id"`
	Proposal             string           `json:"proposal"`
	Revision             int64            `json:"revision"`
	Kind                 string           `json:"kind"`
	EvidenceDigest       string           `json:"evidence_digest"`
	PriorBindingDigest   string           `json:"prior_binding_digest"`
	CurrentContext       string           `json:"current_context,omitempty"`
	CurrentBindingDigest string           `json:"current_binding_digest"`
	Observed             time.Time        `json:"observed_at"`
	State                string           `json:"state"`
	Impacts              []ProposalImpact `json:"impacts"`
}

// AutopilotRepository is the first consumer of proposal metadata, not a second
// job engine. Actual managed effects run in the existing pipeline request ledger.
type AutopilotRepository interface {
	ReadAutopilotAmendment(context.Context, identity.Envelope, string) (AutopilotProposal, error)
	SaveAutopilotProposal(context.Context, identity.Envelope, PreparedProposal, int64, int) (AutopilotProposal, error)
	ReadAutopilotProposal(context.Context, identity.Envelope, string, string) (AutopilotProposal, error)
	ReviewAutopilotProposal(context.Context, identity.Envelope, string, AutopilotReviewRequest) (AutopilotProposal, error)
	StageAutopilotPipeline(context.Context, identity.Envelope, PreparedProposalApply, string) (AutopilotProposal, error)
	RecordAutopilotRun(context.Context, identity.Envelope, PreparedProposalApply, PipelineRun) (AutopilotProposal, error)
	CompensateAutopilot(context.Context, identity.Envelope, PreparedProposalApply, PipelineExecution) (AutopilotProposal, error)
	RecordAutopilotDrift(context.Context, identity.Envelope, PreparedProposalApply, AutopilotDrift, time.Duration) (AutopilotDrift, error)
}

func proposalText(s string, maximum int) bool {
	return len(s) > 0 && len(s) <= maximum && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r")
}

func validateAutopilotGoal(g AutopilotGoal) error {
	if !identity.Identifier(g.ID) || !identity.Identifier(g.Pipeline) || len(g.Pipeline) > 48 || g.Pipeline == g.Source ||
		!identity.Identifier(g.Source) || !identity.Identifier(g.Context) || !identity.Identifier(g.Connection) || !proposalText(g.Name, 128) ||
		!proposalText(g.Goal, 4096) || g.ExpectedPipelineVersion < 0 || g.ExpectedPipelineVersion > 4096 || g.MaxStalenessSeconds < 60 || g.MaxStalenessSeconds > 604800 {
		return ErrInvalid
	}
	return nil
}

// RequireAutopilotMaterial applies every recorded source/dataset/context before
// any material may be shown to a model, reviewer, executor or compensation path.
func RequireAutopilotMaterial(e identity.Envelope, m ProposalMaterial, action string) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if m.Binding.Tenant != e.Tenant() {
		return access.ErrForbidden
	}
	permission := "write"
	if action == "engineering.autopilot.read" {
		permission = "read"
	}
	refs := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: m.Pipeline.ID}}
	for _, r := range m.References {
		refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: r.Kind, Permission: r.Permission, ID: r.ID})
	}
	return access.Require(e, action, refs...)
}

// ValidateAutopilotMaterial also runs at the repository boundary. Native SQL
// safety is additionally guaranteed by the opaque service-issued proof.
func ValidateAutopilotMaterial(m ProposalMaterial, l config.Autopilot) error {
	if l.Validate() != nil || m.Version != AutopilotVersion || m.PromptVersion != AutopilotPromptVersion || validateAutopilotGoal(m.Request) != nil ||
		!m.Binding.Valid() || m.Binding.Source != m.Request.Source || m.Binding.Context != m.Request.Context || m.Binding.Dialect != "postgres" ||
		m.Pipeline.ID != m.Request.Pipeline || m.Pipeline.Name != m.Request.Name || m.Pipeline.Connection != m.Request.Connection ||
		!identity.Identifier(m.Author) || !identity.Identifier(m.Session) || m.Created.IsZero() || !proposalText(m.ModelVersion, 256) ||
		len(m.Plan.Requirements) < 1 || len(m.Plan.Requirements) > l.MaxSteps || !proposalText(m.Plan.Rationale, 4096) || len(m.Objects) != len(m.Pipeline.Steps)+1 {
		return ErrInvalid
	}
	for _, r := range m.Plan.Requirements {
		if !proposalText(r, 1024) {
			return ErrInvalid
		}
	}
	if m.Origin != nil {
		o := m.Origin
		if !identity.Identifier(o.Proposal) || o.Proposal == m.Request.ID || o.Revision < 1 || o.Revision > 256 || !digest64(o.Digest) || !digest64(o.EvidenceDigest) || len(o.Drift) != 32 || !identity.Identifier(o.Drift) {
			return ErrInvalid
		}
	}
	pl := config.DefaultPipelines()
	pl.MaxSteps = l.MaxSteps
	if ValidatePipelineDefinition(m.Pipeline, pl) != nil {
		return ErrInvalid
	}
	relations := map[string]bool{}
	wantEvidence := map[string]string{"binding": readexec.Hash(m.Binding)}
	for _, r := range m.Binding.Relations {
		relations[r.ID] = true
		wantEvidence["relation:"+r.ID] = readexec.Hash(r)
	}
	if len(m.Evidence) != len(wantEvidence) {
		return ErrInvalid
	}
	seenEvidence := map[string]bool{}
	for _, v := range m.Evidence {
		if seenEvidence[v.ID] || wantEvidence[v.ID] != v.Digest || v.Source != m.Binding.Source || v.Context != m.Binding.Context ||
			(v.ID == "binding" && (v.Kind != "source_binding" || v.Dataset != "")) ||
			(v.ID != "binding" && (v.Kind != "relation" || v.ID != "relation:"+v.Dataset || !relations[v.Dataset])) {
			return ErrInvalid
		}
		seenEvidence[v.ID] = true
	}
	wantRefs := []ProposalReference{{Kind: "source", Permission: "query", ID: m.Binding.Source}, {Kind: "execution_context", Permission: "use", ID: m.Binding.Context}}
	for _, r := range m.Binding.Relations {
		wantRefs = append(wantRefs, ProposalReference{Kind: "dataset", Permission: "query", ID: r.ID})
	}
	if readexec.Hash(m.References) != readexec.Hash(wantRefs) {
		return ErrInvalid
	}
	for index, o := range m.Objects {
		kind, id := "pipeline", m.Pipeline.ID
		if index > 0 {
			kind, id = "dataset", m.Pipeline.ID+"."+m.Pipeline.Steps[index-1].ID
		}
		if o.Kind != kind || o.ID != id || o.Decision != "build_managed" || o.Author != m.Author || o.ModelVersion != m.ModelVersion ||
			!proposalText(o.Rationale, 4096) || len(o.Evidence) < 1 || len(o.Evidence) > 33 || len(o.Alternatives) < 1 || len(o.Alternatives) > 16 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, id := range o.Evidence {
			if !seenEvidence[id] || seen[id] {
				return ErrInvalid
			}
			seen[id] = true
		}
		for _, alternative := range o.Alternatives {
			if !relations[alternative.Dataset] || !proposalText(alternative.Rationale, 2048) {
				return ErrInvalid
			}
		}
	}
	for _, step := range m.Pipeline.Steps {
		if len(step.FromSteps) != 0 || step.Source != m.Binding.Source || step.Context != m.Binding.Context || step.Strategy != "replace" {
			return ErrInvalid
		}
		for _, id := range step.Inputs {
			if !relations[id] {
				return ErrInvalid
			}
		}
	}
	if len(m.Usage.Calls) < 1 || len(m.Usage.Calls) > l.MaxCalls {
		return ErrInvalid
	}
	for _, u := range m.Usage.Calls {
		if u.Attempts < 0 || u.Attempts > l.MaxCalls || !slices.Contains([]string{"pipeline_draft"}, u.Role) {
			return ErrInvalid
		}
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > l.MaxProposalBytes {
		return ErrLimit
	}
	return nil
}

func digest64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
