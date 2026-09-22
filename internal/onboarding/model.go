// Package onboarding coordinates the bounded, human-gated setup journey over
// existing Chartworks services. It owns progress only, never domain objects.
package onboarding

import (
	"errors"
	"time"
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
	ID    string `json:"id"`
	Value string `json:"value"`
}

type ResumeRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type CancelRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type DriftRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
	SourceRevision  int64  `json:"source_revision"`
	Observation     string `json:"observation"`
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
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision int64  `json:"revision,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Private  bool   `json:"private"`
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

type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Percent   int `json:"percent"`
}

// Run is actor/session private. It contains references and bounded evidence only.
type Run struct {
	ID                 string       `json:"id"`
	Key                string       `json:"key"`
	Version            int64        `json:"version"`
	Stage              Stage        `json:"stage"`
	Status             Status       `json:"status"`
	Locale             string       `json:"locale"`
	RequiredAction     string       `json:"required_action,omitempty"`
	CancellationReason string       `json:"cancellation_reason,omitempty"`
	Message            string       `json:"message"`
	Input              StartRequest `json:"input"`
	References         []Reference  `json:"references"`
	Evidence           []Evidence   `json:"evidence"`
	Questions          []Question   `json:"questions"`
	Answers            []Answer     `json:"answers"`
	Usage              Usage        `json:"usage"`
	Limits             Limits       `json:"limits"`
	Progress           Progress     `json:"progress"`
	Amendments         []Amendment  `json:"amendments"`
	SourceRevision     int64        `json:"source_revision"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	Deadline           time.Time    `json:"deadline"`
}

type StepResult struct {
	References []Reference
	Evidence   []Evidence
	Questions  []Question
	Usage      Usage
}

type Amendment struct {
	Run            string      `json:"run"`
	RunVersion     int64       `json:"run_version"`
	Observation    string      `json:"observation"`
	SourceRevision int64       `json:"source_revision"`
	Affected       []Reference `json:"affected"`
	Proposal       Reference   `json:"proposal"`
	RequiredAction string      `json:"required_action"`
	ExistingIntact bool        `json:"existing_intact"`
	CreatedAt      time.Time   `json:"created_at"`
}
