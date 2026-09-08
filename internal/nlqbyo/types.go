// Package nlqbyo owns opaque, expiring context references and explicit external
// SQL steps. It composes the existing router and read core, never an agent loop,
// local issuer, result cache or second query executor.
package nlqbyo

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// Version is the closed context/submit contract, independent of the HTTP version.
const Version = 1

var (
	// ErrInvalid identifies malformed/version-incompatible external input.
	ErrInvalid = errors.New("nlqbyo: invalid request")
	// ErrReplan deliberately conflates absent, foreign, expired and stale references.
	ErrReplan = errors.New("nlqbyo: new context required")
	// ErrBudget identifies a retained bundle or explicit step ceiling.
	ErrBudget = errors.New("nlqbyo: budget exhausted")
	// ErrUnavailable means new context construction is not configured.
	ErrUnavailable = errors.New("nlqbyo: context construction unavailable")
)

// Router constructs real sealed semantic context. It is optional for model-free
// lookup/submission after a bundle has already been issued.
type Router interface {
	Route(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error)
}

// TopicReader checks the current exact publication and its source continuity.
type TopicReader interface {
	Contract(context.Context, identity.Envelope, string) (topics.Contract, error)
}

// RuleReader reads current reviewed rules without invoking inference.
type RuleReader interface {
	Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error)
}

// SourceReader resolves metadata under source-read authority, without granting
// query execution. The common validator/reader independently requires query reach.
type SourceReader interface {
	ContextBinding(context.Context, identity.Envelope, string, string) (exec.Binding, error)
}

// Validator is the common read validator, restricted to captured semantic data.
type Validator interface {
	ValidateWithin(context.Context, identity.Envelope, exec.Request, []exec.RelationScope) (exec.Plan, error)
}

// Executor is the ordinary opaque-plan reader. No raw SQL execution is available.
type Executor interface {
	Execute(context.Context, identity.Envelope, exec.Plan, exec.Options) (exec.ExecutionReport, error)
}

// Repository stores references and bounded content-free step evidence. Storage
// coordinates are not authority; Service reauthorizes every operation first.
type Repository interface {
	CreateBYOBundle(context.Context, store.Scope, Record, config.QueryBundles) error
	ReadBYOBundle(context.Context, store.Scope, Reference, string, time.Time) (Record, error)
	ReserveBYOStep(context.Context, store.Scope, Reference, string, Step, time.Time) (Step, bool, error)
	FinishBYOStep(context.Context, store.Scope, Reference, string, Step) error
	ReadBYOSteps(context.Context, store.Scope, Reference, string) ([]Step, error)
}

// CreateRequest asks for one context; it does not submit, generate or execute SQL.
type CreateRequest struct {
	SchemaVersion int                   `json:"schema_version" jsonschema:"enum=1"`
	Route         nlqroute.RouteRequest `json:"route"`
}

// Reference is a data lookup, never a bearer or a signed capability.
type Reference struct {
	SchemaVersion int    `json:"schema_version" jsonschema:"enum=1"`
	ID            string `json:"bundle_id"`
	Context       string `json:"context"`
}

// SubmitRequest describes exactly one explicit, idempotent external step.
// Source, dialect, semantics, limits and identity cannot be substituted here.
type SubmitRequest struct {
	Reference
	Operation  string           `json:"operation"`
	SQL        string           `json:"sql"`
	Parameters []exec.Parameter `json:"parameters"`
}

// SemanticPin records exact topic and rule publications used by every step.
type SemanticPin struct {
	Topic       string `json:"topic"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
	RuleVersion string `json:"rule_version,omitempty"`
	RuleDigest  string `json:"rule_digest,omitempty"`
}

// SQLRequirements makes the native dialect, allowlist and hard reader caps explicit.
type SQLRequirements struct {
	Dialect        string          `json:"dialect"`
	Catalog        string          `json:"catalog,omitempty"`
	ParameterStyle string          `json:"parameter_style"`
	ParameterKinds []string        `json:"parameter_kinds"`
	MaxSQLBytes    int             `json:"max_sql_bytes"`
	MaxParameters  int             `json:"max_parameters"`
	Relations      []exec.Relation `json:"relations"`
	ReadOnly       bool            `json:"read_only"`
	Rows           int             `json:"rows"`
	Bytes          int             `json:"bytes"`
	TimeoutMillis  int64           `json:"timeout_ms"`
}

// Bundle is the exact public context snapshot. Mandatory constraints are explicit
// even when advisory context is pruned by the shared token budget.
type Bundle struct {
	Reference
	CreatedAt    time.Time            `json:"created_at"`
	ExpiresAt    time.Time            `json:"expires_at"`
	Source       string               `json:"source"`
	Semantics    []SemanticPin        `json:"semantics"`
	Context      nlqroute.ContextView `json:"semantic_context"`
	Requirements SQLRequirements      `json:"sql_requirements"`
	MaxSteps     int                  `json:"max_steps"`
	ContextUsage []gateway.Usage      `json:"context_usage"`
	Warnings     []string             `json:"warnings"`
	Provenance   string               `json:"provenance"`
}

// CreateResult preserves no-route/clarification outcomes without issuing a reference.
type CreateResult struct {
	SchemaVersion int                     `json:"schema_version" jsonschema:"enum=1"`
	Outcome       nlq.Strategy            `json:"outcome"`
	Bundle        *Bundle                 `json:"bundle,omitempty"`
	Clarification *nlqroute.Clarification `json:"clarification,omitempty"`
}

// View returns one reauthorized snapshot and content-free step history.
type View struct {
	Bundle Bundle `json:"bundle"`
	Steps  []Step `json:"steps"`
}

// Step is bounded durable per-step evidence. SQL/parameter/result bytes and
// bearer tokens are deliberately absent; hashes bind the exact supplied input.
type Step struct {
	Operation    string        `json:"operation"`
	Number       int           `json:"step"`
	InputDigest  string        `json:"input_digest"`
	BundleDigest string        `json:"bundle_digest"`
	Semantics    []SemanticPin `json:"semantics"`
	Status       string        `json:"status"`
	Code         string        `json:"code"`
	CreatedAt    time.Time     `json:"created_at"`
	Deadline     time.Time     `json:"deadline"`
	FinishedAt   *time.Time    `json:"finished_at,omitempty"`
	Execution    *exec.Attempt `json:"execution,omitempty"`
	ModelCalls   int           `json:"model_calls"`
}

// SubmitResult never implies that lost values were retained or that a retry ran.
// Replayed terminal receipts intentionally have ValuesAvailable=false.
type SubmitResult struct {
	SchemaVersion   int          `json:"schema_version" jsonschema:"enum=1"`
	Step            Step         `json:"step"`
	Result          *exec.Result `json:"result,omitempty"`
	Replayed        bool         `json:"replayed"`
	ValuesAvailable bool         `json:"values_available"`
}

// Record is private persistence input, not a transport shape or permission token.
type Record struct {
	Bundle      Bundle
	Session     string
	Binding     exec.Binding
	DataReach   string
	Digest      string
	RetainUntil time.Time
}
