// Package onboarding coordinates the bounded, human-gated setup journey over
// existing Chartworks services. It owns progress only, never domain objects.
package onboarding

import (
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

var (
	ErrInvalid   = errors.New("onboarding: invalid request")
	ErrBudget    = errors.New("onboarding: budget exhausted")
	ErrCancelled = errors.New("onboarding: cancelled")
	ErrAttention = errors.New("onboarding: human attention required")
)

type Stage string

const (
	StageConnect   Stage = "connect"
	StageInspect   Stage = "inspect"
	StageProfile   Stage = "profile"
	StageSemantic  Stage = "semantic_draft"
	StageReview    Stage = "review_publish"
	StageProposals Stage = "query_block_report_proposals"
	StageComplete  Stage = "complete"
)

type Status string

const (
	StatusReady     Status = "ready"
	StatusRunning   Status = "running"
	StatusAttention Status = "attention"
	StatusCancelled Status = "cancelled"
	StatusComplete  Status = "complete"
	StatusFailed    Status = "failed"
)

type Mode string

const (
	ModeConnect Mode = "connect"
	ModeUpload  Mode = "upload"
)

// Limits are enforced before and after every domain/model operation.
type Limits struct {
	MaxStages     int           `json:"max_stages"`
	MaxModelCalls int           `json:"max_model_calls"`
	MaxTokens     int           `json:"max_tokens"`
	MaxEntities   int           `json:"max_entities"`
	MaxDuration   time.Duration `json:"max_duration_ns"`
}

func DefaultLimits() Limits {
	return Limits{MaxStages: 12, MaxModelCalls: 4, MaxTokens: 24000, MaxEntities: 256, MaxDuration: 20 * time.Minute}
}

func (l Limits) valid() bool {
	return l.MaxStages >= 6 && l.MaxStages <= 32 && l.MaxModelCalls >= 0 && l.MaxModelCalls <= 16 && l.MaxTokens >= 0 && l.MaxTokens <= 200000 && l.MaxEntities >= 1 && l.MaxEntities <= 2048 && l.MaxDuration >= time.Minute && l.MaxDuration <= 2*time.Hour
}

type StartRequest struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Mode           Mode   `json:"mode"`
	Locale         string `json:"locale"`
	Source         string `json:"source"`
	Context        string `json:"context"`
	Dataset        string `json:"dataset"`
	Profile        string `json:"profile"`
	Topic          string `json:"topic"`
	TopicVersion   string `json:"topic_version"`
	Block          string `json:"block"`
	Report         string `json:"report"`
	Upload         string `json:"upload,omitempty"`
	Transformation bool   `json:"transformation_required"`
	// TransformationProposal is an existing reviewed-engineering proposal. The
	// onboarding ledger never accepts SQL or managed-write instructions itself.
	TransformationProposal string `json:"transformation_proposal,omitempty"`
}

type AnswerRequest struct {
	ID              string           `json:"id"`
	ExpectedVersion int64            `json:"expected_version"`
	Answers         []Answer         `json:"answers"`
	Review          *ReviewReference `json:"review,omitempty"`
}

type Answer struct {
	ID        string `json:"id"`
	Decision  string `json:"decision" jsonschema:"enum=confirmed_external,enum=unresolved,enum=not_applicable"`
	Reference string `json:"reference,omitempty"`
}

type ResumeRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type CancelRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason" jsonschema:"enum=user_requested,enum=superseded,enum=incorrect_source,enum=budget"`
}

type DriftRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type ReviewReference struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}

// Reference is a content-free pointer to an existing domain object or a stable
// private proposal coordinate retained by this onboarding run. Proposal
// coordinates grant no block/report authority and contain no authored content.
type Reference struct {
	Kind           string   `json:"kind"`
	ID             string   `json:"id"`
	Revision       int64    `json:"revision,omitempty"`
	SourceRevision int64    `json:"source_revision,omitempty"`
	Digest         string   `json:"digest,omitempty"`
	Private        bool     `json:"private"`
	Source         string   `json:"source,omitempty"`
	Context        string   `json:"context,omitempty"`
	Dataset        string   `json:"dataset,omitempty"`
	Columns        []string `json:"columns,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
}

// Evidence records why a proposed semantic entity exists without retaining rows,
// SQL, prompts or credentials.
type Evidence struct {
	Entity      string   `json:"entity"`
	Kind        string   `json:"kind"`
	Basis       []string `json:"basis"`
	Confidence  string   `json:"confidence"`
	Uncertainty string   `json:"uncertainty,omitempty"`
	Sensitive   bool     `json:"sensitive"`
}

type Question struct {
	ID       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Evidence []string `json:"evidence"`
	Required bool     `json:"required"`
}

type Usage struct {
	Stages     int `json:"stages"`
	ModelCalls int `json:"model_calls"`
	Tokens     int `json:"tokens"`
	Entities   int `json:"entities"`
}

// Lease is the durable pre-effect fence. A cancellation request cannot erase it;
// the same operation must reconcile and attach its receipt before cancellation
// becomes final.
type Lease struct {
	Stage          Stage  `json:"stage"`
	Operation      string `json:"operation"`
	Fence          string `json:"fence"`
	InputDigest    string `json:"input_digest,omitempty"`
	ReservedCalls  int    `json:"reserved_calls"`
	ReservedTokens int    `json:"reserved_tokens"`
	Charged        bool   `json:"charged"`
}

// DelegatedReceipt is a bounded content-free accounting projection. Provider
// prompts, model names, SQL, rows and native errors are never retained here.
type DelegatedReceipt struct {
	Operation     string `json:"operation"`
	Calls         int    `json:"calls"`
	Tokens        int    `json:"tokens"`
	UnknownTokens bool   `json:"unknown_tokens"`
	Reconciled    bool   `json:"reconciled"`
}

type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Percent   int `json:"percent"`
}

// RunAuthority is a server-resolved current source binding needed before any
// persisted run projection can be returned.
type RunAuthority struct {
	Source   string
	Context  string
	Dataset  string
	Revision int64
	Exact    bool
}

// Run is actor/session private. It contains references and bounded evidence only.
type Run struct {
	ID                 string             `json:"id"`
	Key                string             `json:"key"`
	Version            int64              `json:"version"`
	Stage              Stage              `json:"stage"`
	Status             Status             `json:"status"`
	Locale             string             `json:"locale"`
	RequiredAction     string             `json:"required_action,omitempty"`
	CancellationReason string             `json:"cancellation_reason,omitempty"`
	CancelRequested    bool               `json:"cancel_requested"`
	Lease              *Lease             `json:"lease,omitempty"`
	Message            string             `json:"message"`
	Input              StartRequest       `json:"input"`
	References         []Reference        `json:"references"`
	Evidence           []Evidence         `json:"evidence"`
	Questions          []Question         `json:"questions"`
	Answers            []Answer           `json:"answers"`
	Usage              Usage              `json:"usage"`
	Receipts           []DelegatedReceipt `json:"receipts"`
	Limits             Limits             `json:"limits"`
	Progress           Progress           `json:"progress"`
	Amendments         []Amendment        `json:"amendments"`
	SourceRevision     int64              `json:"source_revision"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	Deadline           time.Time          `json:"deadline"`
}

type StepResult struct {
	References []Reference
	Evidence   []Evidence
	Questions  []Question
	Receipt    gateway.Receipt
}

type Amendment struct {
	Run            string           `json:"run"`
	RunVersion     int64            `json:"run_version"`
	Observation    string           `json:"observation"`
	Source         string           `json:"source"`
	Context        string           `json:"context"`
	Dataset        string           `json:"dataset"`
	SourceRevision int64            `json:"source_revision"`
	Changes        []string         `json:"changes"`
	Affected       []Reference      `json:"affected"`
	ImpactEvidence []ImpactEvidence `json:"impact_evidence"`
	Proposal       Reference        `json:"proposal"`
	RequiredAction string           `json:"required_action"`
	ExistingIntact bool             `json:"existing_intact"`
	CreatedAt      time.Time        `json:"created_at"`
}

// ImpactEvidence explains the stable dependency edge that caused one private
// coordinate to enter a drift amendment. Conservative closure is explicit for
// proposal intents that have no executable definition yet.
type ImpactEvidence struct {
	Kind         string   `json:"kind"`
	ID           string   `json:"id"`
	Basis        []string `json:"basis"`
	Conservative bool     `json:"conservative"`
}
