// Package nlqexec owns the governed question-to-read lifecycle. It composes
// the phase-17 router, the single gateway, and the existing opaque read core;
// it never exposes a second SQL executor or a local authority model.
package nlqexec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

const (
	maxInstructionText = 16 << 10
	maxSQLBytes        = 32 << 10
	maxFeedbackNote    = 4096
	// maxExampleResults lets the durable review endpoint return one more
	// candidate than the seven-item model-context cap. The extra row is
	// inspectable learning state and is never admitted into generation by
	// default.
	maxExampleResults     = nlq.MaxExamples + 1
	maxLearningCandidates = 64
)

var (
	// ErrInvalid identifies a malformed question, session, or lifecycle request.
	ErrInvalid = errors.New("nlqexec: invalid request")
	// ErrGeneration identifies an unusable or unavailable SQL generation response.
	ErrGeneration = errors.New("nlqexec: SQL generation failed")
	// ErrValidationBudget identifies an exhausted validation correction budget.
	ErrValidationBudget = errors.New("nlqexec: validation correction budget exhausted")
	// ErrExecutionBudget identifies an exhausted execution correction budget.
	ErrExecutionBudget = errors.New("nlqexec: execution correction budget exhausted")
	// ErrExecutionFailed identifies a terminal execution failure.
	ErrExecutionFailed = errors.New("nlqexec: execution failed")
	// ErrForeignSession identifies a query or example from another signed session.
	ErrForeignSession = errors.New("nlqexec: session is not accessible")
	// ErrNoPlan identifies a query that has not reached a planned state.
	ErrNoPlan = errors.New("nlqexec: query is not planned")
	// ErrUnsafeCorrection identifies a correction that changes governed semantics.
	ErrUnsafeCorrection = errors.New("nlqexec: correction cannot change governed semantics")
	// ErrInspectionRequired identifies SQL hidden by the inspection boundary.
	ErrInspectionRequired = errors.New("nlqexec: SQL inspection is not authorized")
)

// Router is the existing routed-context consumer. Its result carries a sealed
// in-process context that cannot be reconstructed from the public response.
type Router interface {
	Route(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error)
}

type originVerifier interface {
	VerifyOrigin(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error)
}

// TopicReader is deliberately narrower than the topic service. The phase-18
// service reads current publications again before generation and execution.
type TopicReader interface {
	Contract(context.Context, identity.Envelope, string) (topics.Contract, error)
	RetainedContract(context.Context, identity.Envelope, string, string) (topics.Contract, error)
}

type reviewTopicReader interface {
	ReviewContract(context.Context, identity.Envelope, string) (topics.Contract, error)
}

// SourceReader resolves the actual connector binding used by the validator.
// It is not a raw query or credential seam.
type SourceReader interface {
	Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error)
}

type reviewSourceReader interface {
	ReviewBinding(context.Context, identity.Envelope, string, string) (exec.Binding, error)
}

// PlanValidator and PlanExecutor keep the orchestration logic on the existing
// opaque read seams while allowing deterministic failure-path tests. Production
// wiring supplies *exec.Validator and *exec.Executor; neither interface can
// construct or deserialize a plan.
type PlanValidator interface {
	Validate(context.Context, identity.Envelope, exec.Request) (exec.Plan, error)
}

// PlanExecutor executes only opaque plans issued by the existing read core.
type PlanExecutor interface {
	Execute(context.Context, identity.Envelope, exec.Plan, exec.Options) (exec.ExecutionReport, error)
}

// Repository is the durable phase-18 metadata contract. Implementations must
// scope every operation by tenant and actor and preserve query SQL/evidence as
// protected internal metadata.
type Repository interface {
	CreateSession(context.Context, store.Scope, SessionRecord) error
	ReadSession(context.Context, store.Scope, string) (SessionRecord, error)
	CreateQuery(context.Context, store.Scope, QueryRecord) error
	ReadQuery(context.Context, store.Scope, string) (QueryRecord, error)
	ReadOperation(context.Context, store.Scope, string) (QueryRecord, error)
	ReadExample(context.Context, store.Scope, string) (ExampleRecord, error)
	UpdateQuery(context.Context, store.Scope, QueryRecord, int64) error
	RecordFeedback(context.Context, store.Scope, FeedbackRecord) error
	ApplyFeedback(context.Context, store.Scope, FeedbackRecord, ExampleRecord) (ExampleRecord, bool, error)
	UpsertExample(context.Context, store.Scope, ExampleRecord) (ExampleRecord, error)
	ImportExample(context.Context, store.Scope, ExampleRecord) (ExampleRecord, bool, error)
	ListExamples(context.Context, store.Scope, string, int) ([]ExampleRecord, error)
	SetExampleState(context.Context, store.Scope, ExampleStateRequest, string) (ExampleRecord, error)
}

// SessionRecord is the durable identity and semantic anchor for one session.
type SessionRecord struct {
	ID      string       `json:"id"`
	Tenant  string       `json:"tenant"`
	Actor   string       `json:"actor"`
	Context string       `json:"context"`
	Topics  []string     `json:"topics"`
	Locale  nlq.Language `json:"locale"`
	Created time.Time    `json:"created_at"`
	Updated time.Time    `json:"updated_at"`
}

// QueryRecord contains protected generation and result metadata. It is never
// returned directly from a public route; Response redacts SQL by default.
type QueryRecord struct {
	Clarification    *ClarificationEvidence       `json:"-"`
	ID               string                       `json:"id"`
	Session          string                       `json:"session"`
	Parent           string                       `json:"parent,omitempty"`
	Operation        string                       `json:"operation,omitempty"`
	Topic            string                       `json:"topic"`
	Topics           []string                     `json:"topics"`
	TopicVersions    []string                     `json:"topic_versions"`
	RuleVersions     []string                     `json:"rule_versions,omitempty"`
	Templates        []rulesets.TemplateSelection `json:"templates,omitempty"`
	ExampleSelection ExampleSelectionEvidence     `json:"example_selection,omitempty"`
	Context          string                       `json:"context"`
	Locale           nlq.Language                 `json:"locale"`
	Question         string                       `json:"question"`
	Route            nlqroute.RouteResult         `json:"route"`
	Generation       nlq.GenerationContext        `json:"generation"`
	SQL              string                       `json:"-"`
	Parameters       []exec.Parameter             `json:"-"`
	Receipt          gateway.Receipt              `json:"receipt"`
	Status           string                       `json:"status"`
	EvidenceStale    bool                         `json:"evidence_stale,omitempty"`
	Result           *exec.Result                 `json:"result,omitempty"`
	Assumptions      []string                     `json:"assumptions,omitempty"`
	Ambiguities      []string                     `json:"ambiguities,omitempty"`
	Errors           []string                     `json:"errors,omitempty"`
	ValidationFixes  int                          `json:"validation_fixes"`
	ExecutionFixes   int                          `json:"execution_fixes"`
	Revision         int64                        `json:"revision"`
	Created          time.Time                    `json:"created_at"`
	Updated          time.Time                    `json:"updated_at"`
}

// FeedbackRecord is a reviewable correction. Recording it never publishes a
// rule or changes a current topic.
type FeedbackRecord struct {
	ID         string    `json:"id"`
	QueryID    string    `json:"query_id"`
	Session    string    `json:"session"`
	Verdict    string    `json:"verdict"`
	Correction string    `json:"-"`
	Note       string    `json:"note,omitempty"`
	Provenance string    `json:"provenance"`
	Created    time.Time `json:"created_at"`
}

// ExampleOrigin pins learned material to the exact governed environment that
// produced it. A matching display name or tenant is never enough to make an
// example applicable to a later generation.
type ExampleOrigin struct {
	SchemaVersion       int                          `json:"schema_version"`
	Locale              nlq.Language                 `json:"locale"`
	TopicVersion        string                       `json:"topic_version"`
	Context             string                       `json:"context"`
	SourceBindingDigest string                       `json:"source_binding_digest"`
	RuleVersions        []string                     `json:"rule_versions,omitempty"`
	Templates           []rulesets.TemplateSelection `json:"templates,omitempty"`
}

// ExampleSelection records one candidate's deterministic applicability and
// optional gateway ranking. It contains no SQL, raw prompt, or result value.
type ExampleSelection struct {
	ExampleID   string   `json:"example_id"`
	Version     int64    `json:"version"`
	Lane        string   `json:"lane"`
	Position    int      `json:"position,omitempty"`
	Decision    string   `json:"decision"`
	Reason      string   `json:"reason,omitempty"`
	Score       float64  `json:"score"`
	Uncertainty float64  `json:"uncertainty"`
	RankScore   *float64 `json:"rank_score,omitempty"`
}

// ExampleSelectionEvidence is frozen with the query. Replaying or running a
// retained plan never reselects examples against mutable learning state.
type ExampleSelectionEvidence struct {
	SchemaVersion  int                `json:"schema_version,omitempty"`
	PolicyVersion  string             `json:"policy_version,omitempty"`
	Selected       []ExampleSelection `json:"selected,omitempty"`
	Excluded       []ExampleSelection `json:"excluded,omitempty"`
	ShadowBaseline []ExampleSelection `json:"shadow_baseline,omitempty"`
	Receipt        gateway.Receipt    `json:"receipt,omitempty"`
}

// ExampleRecord is the DB-first learning projection. State changes are
// explicit and audited; feedback only creates or strengthens a candidate.
type ExampleRecord struct {
	ID               string        `json:"id"`
	Topic            string        `json:"topic"`
	Question         string        `json:"question"`
	SQL              string        `json:"-"`
	Digest           string        `json:"digest"`
	State            string        `json:"state"`
	Weight           float64       `json:"weight"`
	Uncertainty      float64       `json:"uncertainty"`
	EvidenceCount    int           `json:"evidence_count"`
	PositiveEvidence int           `json:"positive_evidence"`
	NegativeEvidence int           `json:"negative_evidence"`
	EvidenceOutcome  string        `json:"-"`
	Origin           ExampleOrigin `json:"origin"`
	Version          int64         `json:"version"`
	ReviewedBy       string        `json:"reviewed_by,omitempty"`
	ReviewNote       string        `json:"-"`
	ReviewedAt       *time.Time    `json:"reviewed_at,omitempty"`
	Provenance       string        `json:"provenance"`
	Created          time.Time     `json:"created_at"`
	Updated          time.Time     `json:"updated_at"`
}

// QuestionRequest is shared by preflight and plan. The verified envelope
// supplies tenant, actor and session; none of those are accepted from JSON.
type QuestionRequest struct {
	// ClarificationQuery anchors a typed submission to a retained preflight in
	// the current actor/session. The ID grants no authority and is rechecked.
	ClarificationQuery   string                          `json:"clarification_query,omitempty"`
	Answers              []semantics.ClarificationAnswer `json:"answers,omitempty"`
	AnswerContext        string                          `json:"answer_context,omitempty"`
	Topic                string                          `json:"topic,omitempty"`
	Topics               []string                        `json:"topics,omitempty"`
	Context              string                          `json:"context"`
	Locale               nlq.Language                    `json:"locale"`
	Question             string                          `json:"question"`
	Templates            []rulesets.TemplateSelection    `json:"templates,omitempty"`
	Kinds                []string                        `json:"kinds,omitempty"`
	LimitPerKind         int                             `json:"limit_per_kind,omitempty"`
	References           []semantics.Reference           `json:"references,omitempty"`
	Choices              []nlqroute.ChoiceSelection      `json:"choices,omitempty"`
	Joins                []nlqroute.JoinChoice           `json:"joins,omitempty"`
	MetricIDs            []string                        `json:"metric_ids,omitempty"`
	Examples             []nlq.OptionalItem              `json:"examples,omitempty"`
	Rerank               bool                            `json:"rerank,omitempty"`
	InterpretationAnchor string                          `json:"interpretation_anchor,omitempty"`
	InterpretationEdits  []nlqroute.InterpretationEdit   `json:"interpretation_edits,omitempty"`
	EditBase             []nlq.Instruction               `json:"edit_base,omitempty"`
	Hints                []nlq.Instruction               `json:"hints,omitempty"`
	ExampleInput         []nlq.Instruction               `json:"example_instructions,omitempty"`
	Default              []nlq.Instruction               `json:"default_instructions,omitempty"`
}

// SemanticReference is accepted through the semantic package's typed value. This
// alias keeps the public request independent from rule internals.
type SemanticReference = semantics.Reference

// PreflightRequest admits a question without generating or executing SQL.
type PreflightRequest struct{ QuestionRequest }

// PlanRequest generates and validates one opaque read plan. Operation is kept
// for an optional later idempotent run; it never authorizes execution itself.
type PlanRequest struct {
	QuestionRequest
	Operation string `json:"operation,omitempty"`
}

// RunRequest executes a previously planned query after revalidating its stored
// candidate against current source state and current signed reach.
type RunRequest struct {
	QueryID   string `json:"query_id"`
	Operation string `json:"operation"`
	Preview   bool   `json:"preview,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
}

// RefineRequest creates a child query in the same signed session.
type RefineRequest struct {
	QueryID string `json:"query_id"`
	QuestionRequest
}

// FeedbackRequest records a governed review and optional corrected SQL.
type FeedbackRequest struct {
	QueryID    string `json:"query_id"`
	Verdict    string `json:"verdict"`
	Correction string `json:"correction,omitempty"`
	Note       string `json:"note,omitempty"`
}

// ExampleStateRequest advances one candidate through its reviewed lifecycle.
type ExampleStateRequest struct {
	ExampleID       string `json:"example_id"`
	State           string `json:"state"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
	ReviewNote      string `json:"review_note,omitempty"`
}

// PortableExample is the protected, neutral learning interchange row. SQL is
// present only on the explicit export/import operations, never ordinary list
// projections or logs.
type PortableExample struct {
	SchemaVersion    int           `json:"schema_version"`
	Question         string        `json:"question"`
	SQL              string        `json:"sql"`
	Digest           string        `json:"digest"`
	Origin           ExampleOrigin `json:"origin"`
	PositiveEvidence int           `json:"positive_evidence"`
	NegativeEvidence int           `json:"negative_evidence"`
}

// ExampleExportRequest selects a bounded protected learning export.
type ExampleExportRequest struct {
	Topic string `json:"topic"`
	Limit int    `json:"limit"`
}

// ExampleBundle is a versioned neutral learning interchange payload.
type ExampleBundle struct {
	SchemaVersion int               `json:"schema_version"`
	Topic         string            `json:"topic"`
	Examples      []PortableExample `json:"examples"`
}

// ExampleImportRequest routes the anchor through the current governed context
// before validating and retaining one portable example as a candidate.
type ExampleImportRequest struct {
	Anchor  QuestionRequest `json:"anchor"`
	Example PortableExample `json:"example"`
}

// PreflightResult exposes routing evidence and typed clarification data only.
type PreflightResult struct {
	QueryID     string               `json:"query_id"`
	SessionID   string               `json:"session_id"`
	Route       nlqroute.RouteResult `json:"route"`
	Confidence  float64              `json:"confidence"`
	Assumptions []string             `json:"assumptions,omitempty"`
	Ambiguities []string             `json:"ambiguities,omitempty"`
	Errors      []string             `json:"errors,omitempty"`
}

// PlanResult contains a validated plan receipt. SQL is included only when the
// caller has the separate reporting.sql.read authority.
type PlanResult struct {
	Bindings        *exec.BusinessBindingReceipt `json:"bindings,omitempty"`
	AnswerChanges   []ClarificationChange        `json:"answer_changes,omitempty"`
	QueryID         string                       `json:"query_id"`
	SessionID       string                       `json:"session_id"`
	Status          string                       `json:"status"`
	Route           nlqroute.RouteResult         `json:"route"`
	Confidence      float64                      `json:"confidence"`
	Generation      string                       `json:"generation"`
	ValidationFixes int                          `json:"validation_fixes"`
	Assumptions     []string                     `json:"assumptions,omitempty"`
	Ambiguities     []string                     `json:"ambiguities,omitempty"`
	SQL             string                       `json:"sql,omitempty"`
	Receipt         gateway.Receipt              `json:"receipt"`
	validated       exec.Plan
}

// RunResult contains the opaque executor receipt and normalized rows. The
// underlying attempt journal remains the source of cancellation/reconciliation.
type RunResult struct {
	Bindings        *exec.BusinessBindingReceipt `json:"bindings,omitempty"`
	AnswerChanges   []ClarificationChange        `json:"answer_changes,omitempty"`
	QueryID         string                       `json:"query_id"`
	SessionID       string                       `json:"session_id"`
	Status          string                       `json:"status"`
	EvidenceStale   bool                         `json:"evidence_stale,omitempty"`
	Route           nlqroute.RouteResult         `json:"route"`
	Confidence      float64                      `json:"confidence"`
	Assumptions     []string                     `json:"assumptions,omitempty"`
	Ambiguities     []string                     `json:"ambiguities,omitempty"`
	Execution       exec.ExecutionReport         `json:"execution"`
	ValidationFixes int                          `json:"validation_fixes"`
	ExecutionFixes  int                          `json:"execution_fixes"`
	SQL             string                       `json:"sql,omitempty"`
}

// Service composes only existing authority, routing, gateway and read seams.
type Service struct {
	router    Router
	topics    TopicReader
	sources   SourceReader
	validator PlanValidator
	executor  PlanExecutor
	engine    gateway.Engine
	repo      Repository
}

// New composes the routed context, topic/source readers, gateway, read core, and durable repository.
func New(router Router, topicsReader TopicReader, sourcesReader SourceReader, validator PlanValidator, executor PlanExecutor, engine gateway.Engine, repo Repository) (*Service, error) {
	if router == nil || topicsReader == nil || sourcesReader == nil || validator == nil || executor == nil || engine == nil || repo == nil {
		return nil, store.ErrInvalid
	}
	return &Service{router: router, topics: topicsReader, sources: sourcesReader, validator: validator, executor: executor, engine: engine, repo: repo}, nil
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", store.ErrUnavailable
	}
	return hex.EncodeToString(raw[:]), nil
}

func scope(e identity.Envelope) (store.Scope, error) { return store.NewScope(e.Tenant(), e.User()) }

func canInspect(e identity.Envelope) bool {
	return e.Has("reporting.sql.read")
}

func instructionValid(items []nlq.Instruction) bool {
	if len(items) > nlq.MaxInstructions {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !identity.Identifier(item.Key) || len(item.Text) == 0 || len(item.Text) > maxInstructionText || !utf8.ValidString(item.Text) || strings.ContainsRune(item.Text, 0) || seen[item.Key] {
			return false
		}
		seen[item.Key] = true
	}
	return true
}
