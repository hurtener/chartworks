// Package reporting owns governed block definitions. It composes the existing
// semantic, source, validator and execution services; it is not a query engine,
// model client, artifact store or identity-policy owner.
package reporting

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

// SchemaVersion is the retained v1 wire version. Existing records keep their hashes.
const SchemaVersion = 1

// CurrentSchemaVersion adds explicit output intent and bounded reporting policies.
// Migration creates a new draft; published v1 bytes are never rewritten.
const CurrentSchemaVersion = 2

// CanonicalizationVersion identifies the canonical definition and evidence hashing contract.
const CanonicalizationVersion = "block-definition-v1"

var (
	// ErrInvalid identifies an invalid definition, parameter or request.
	ErrInvalid = errors.New("reporting: invalid definition or request")
	// ErrStale requires new validation or dependency review before the requested transition.
	ErrStale = errors.New("reporting: current validation or dependency review required")
	// ErrUnavailable identifies unavailable validation dependencies.
	ErrUnavailable = errors.New("reporting: validation unavailable")
	// ErrBusy identifies exhausted bounded validation concurrency.
	ErrBusy = errors.New("reporting: validation concurrency exhausted")
)

// Localized contains plain text, never frontend HTML. Locale is a canonical BCP47
// language tag. Alias order and all stable output IDs survive every projection.
type Localized struct {
	Locale      string          `json:"locale"`
	Title       string          `json:"title"`
	Question    string          `json:"question"`
	Aliases     []string        `json:"aliases"`
	Description string          `json:"description"`
	Intent      *QuestionIntent `json:"intent,omitempty"`
}

// QuestionIntent is reviewed authoring metadata used only for bounded duplicate
// assessment. It is never interpreted as SQL, routing evidence or authority.
type QuestionIntent struct {
	Metrics    []string       `json:"metrics"`
	Grain      string         `json:"grain,omitempty"`
	Population string         `json:"population,omitempty"`
	Filters    []IntentFilter `json:"filters,omitempty"`
	Period     *IntentPeriod  `json:"period,omitempty"`
}

// IntentFilter is a closed, inert question-intent component.
type IntentFilter struct {
	Dimension string `json:"dimension"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
}

// IntentPeriod describes reviewed question meaning, not executable resolution.
type IntentPeriod struct {
	Mode  string `json:"mode"`
	Unit  string `json:"unit,omitempty"`
	Count int    `json:"count,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// TopicPin binds a reviewed semantic topic version to its exact content digest.
type TopicPin struct {
	Topic   string `json:"topic"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// TemplatePin identifies a server-verified capture template, not an arbitrary
// caller claim that a manual definition came from a reviewed template.
type TemplatePin struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// TemplateSelection binds a server-reviewed template to the complete immutable
// topic and ruleset publication coordinates that selected it. New captures use
// this per-topic representation; TemplatePin remains only for retained legacy
// records whose sole rule pin can supply the missing topic coordinates.
type TemplateSelection struct {
	ID           string `json:"id"`
	Topic        string `json:"topic"`
	TopicVersion string `json:"topic_version"`
	PackDigest   string `json:"pack_digest"`
	RuleVersion  string `json:"rule_version"`
	RuleDigest   string `json:"rule_digest"`
}

// DimensionReference pins the semantic dimension governing a parameter value.
type DimensionReference struct {
	Topic     string `json:"topic"`
	Version   string `json:"version"`
	Dimension string `json:"dimension"`
}

// Parameter uses exact text for scalar values. Only the domain binder converts
// them to the common reader's typed bind parameters; SQL fragments are absent.
type Parameter struct {
	Name       string              `json:"name"`
	Type       string              `json:"type" jsonschema:"enum=date,enum=datetime,enum=relative_period,enum=dimension_value,enum=number,enum=integer,enum=boolean,enum=grain,enum=top_n,enum=dimension_list,enum=number_list,enum=integer_list"`
	Required   bool                `json:"required"`
	Default    *Value              `json:"default,omitempty"`
	Min        string              `json:"min,omitempty"`
	Max        string              `json:"max,omitempty"`
	Enum       []string            `json:"enum,omitempty"`
	Dimension  *DimensionReference `json:"dimension,omitempty"`
	ListLength int                 `json:"list_length,omitempty" jsonschema:"minimum=0,maximum=32"`
}

// Value carries exactly one scalar literal or a structured period.
type Value struct {
	Literal string   `json:"literal,omitempty"`
	Period  *Period  `json:"period,omitempty"`
	Items   []string `json:"items,omitempty"`
}

// Argument assigns a typed value to a declared parameter by name.
type Argument struct {
	Name  string `json:"name"`
	Value Value  `json:"value"`
}

// Period is resolved to a half-open interval at one explicit logical instant.
// Calendar operations use named-zone civil time, never an assumed 24-hour day.
type Period struct {
	Mode            string `json:"mode" jsonschema:"enum=explicit,enum=from_date,enum=previous,enum=rolling,enum=schedule_window"`
	Unit            string `json:"unit,omitempty" jsonschema:"enum=hour,enum=day,enum=week,enum=month,enum=quarter,enum=year"`
	Count           int    `json:"count,omitempty"`
	Start           string `json:"start,omitempty"`
	End             string `json:"end,omitempty"`
	FromDate        string `json:"from_date,omitempty"`
	FirstOccurrence string `json:"first_occurrence,omitempty" jsonschema:"enum=reject,enum=from_date,enum=previous"`
	DSTPolicy       string `json:"dst_policy" jsonschema:"enum=reject,enum=earlier"`
	MonthPolicy     string `json:"month_policy" jsonschema:"enum=clamp,enum=reject"`
}

// Window is a half-open interval with explicit instants.
type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Resolution is authoring input, not a claim that a scheduled occurrence ran.
// A later scheduler supplies the accepted occurrence's sealed values here.
type Resolution struct {
	At             time.Time `json:"at"`
	Timezone       string    `json:"timezone"`
	ScheduleWindow *Window   `json:"schedule_window,omitempty"`
}

// BoundValue records the resolved parameter identity, origin and digest without SQL fragments.
type BoundValue struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Provenance string  `json:"provenance"`
	Window     *Window `json:"window,omitempty"`
	Digest     string  `json:"digest"`
}

// Resolved contains exact typed execution binds and their resolution evidence.
type Resolved struct {
	Values     []BoundValue     `json:"values"`
	Parameters []exec.Parameter `json:"parameters"`
	At         time.Time        `json:"at"`
	Timezone   string           `json:"timezone"`
}

// Narrative is a bounded saved definition only. Phase 28 owns actual generation
// through Bifrost. No tools, executable templates, URLs or HTML can be supplied.
type Narrative struct {
	PolicyVersion   string   `json:"policy_version,omitempty" jsonschema:"enum=bounded-narrative-v2"`
	MaxClaims       int      `json:"max_claims,omitempty" jsonschema:"minimum=1,maximum=32"`
	Type            string   `json:"type" jsonschema:"enum=summary,enum=comparison,enum=explanation"`
	Instructions    string   `json:"instructions"`
	Fields          []string `json:"fields"`
	RedactedFields  []string `json:"redacted_fields"`
	Reduction       string   `json:"reduction" jsonschema:"enum=first_rows,enum=aggregate_evidence"`
	MaxRows         int      `json:"max_rows"`
	MaxBytes        int      `json:"max_bytes"`
	MaxCharacters   int      `json:"max_characters"`
	MaxCalls        int      `json:"max_calls"`
	MaxTokens       int      `json:"max_tokens"`
	TimeoutMillis   int      `json:"timeout_ms"`
	PromptVersion   string   `json:"prompt_version"`
	ModelVersion    string   `json:"model_version"`
	SchemaVersion   string   `json:"schema_version"`
	Locale          string   `json:"locale"`
	Tone            string   `json:"tone" jsonschema:"enum=neutral,enum=concise,enum=technical"`
	RequireEvidence bool     `json:"require_evidence"`
	RequireCaveats  bool     `json:"require_caveats"`
}

// Output is a closed tagged union. A chart/KPI/table has exactly one saved phase
// 20 mapping; a narrative has exactly one bounded narrative specification.
type Output struct {
	Intent    *OutputIntent   `json:"intent,omitempty"`
	ID        string          `json:"id"`
	Kind      string          `json:"kind" jsonschema:"enum=chart,enum=kpi,enum=table,enum=narrative"`
	Mapping   *charts.Mapping `json:"mapping,omitempty"`
	Narrative *Narrative      `json:"narrative,omitempty"`
}

// Definition is private authoring/persistence input. Never return this type from
// a normal block read: SQL has its own separately authorized projection.
type Definition struct {
	QueryLimits    *QueryLimits        `json:"query_limits,omitempty"`
	ResultPolicy   []ResultFieldPolicy `json:"result_policy,omitempty"`
	SchemaVersion  int                 `json:"schema_version" jsonschema:"enum=1,enum=2"`
	Metadata       []Localized         `json:"metadata"`
	Source         string              `json:"source"`
	Context        string              `json:"context"`
	Topics         []TopicPin          `json:"topics"`
	Rules          []RulePin           `json:"rules,omitempty"`
	Template       *TemplatePin        `json:"template,omitempty"`
	Templates      []TemplateSelection `json:"templates,omitempty"`
	SQL            string              `json:"sql"`
	Parameters     []Parameter         `json:"parameters"`
	ExpectedSchema []exec.Field        `json:"expected_schema"`
	Outputs        []Output            `json:"outputs"`
}

// RulePin binds one immutable reviewed ruleset to the exact semantic topic
// publication used by a block. It carries no rule text and grants no authority.
type RulePin struct {
	Topic        string `json:"topic"`
	TopicVersion string `json:"topic_version"`
	PackDigest   string `json:"pack_digest"`
	RuleVersion  string `json:"rule_version"`
	RuleDigest   string `json:"rule_digest"`
}

// Provenance records the server-derived origin of an authored revision.
type Provenance struct {
	Kind             string                    `json:"kind"`
	ParentRevision   int64                     `json:"parent_revision,omitempty"`
	Query            string                    `json:"query,omitempty"`
	Template         *TemplatePin              `json:"template,omitempty"`
	Templates        []TemplateSelection       `json:"templates,omitempty"`
	OriginalQuestion string                    `json:"original_question,omitempty"`
	ChangeDigest     string                    `json:"change_digest,omitempty"`
	Parameterization *ParameterizationEvidence `json:"parameterization,omitempty"`
}

// ParameterizationEvidence retains reviewed authoring intent without making it
// execution authority. The transformed SQL remains the immutable definition.
type ParameterizationEvidence struct {
	ProposalDigest        string `json:"proposal_digest"`
	Dialect               string `json:"dialect"`
	Source                string `json:"source"`
	Context               string `json:"context"`
	SourceRevision        int64  `json:"source_revision"`
	BindingDigest         string `json:"binding_digest"`
	TopicsDigest          string `json:"topics_digest"`
	OriginalQuestion      string `json:"original_question,omitempty"`
	QuestionDisposition   string `json:"question_disposition"`
	TemplateDisposition   string `json:"template_disposition"`
	ParaphraseDisposition string `json:"paraphrase_disposition"`
}

// Dependency is derived by the service from validator-issued relation IDs and
// current server-owned source/semantic contracts. There is no client manifest.
type Dependency struct {
	Source         string        `json:"source"`
	Context        string        `json:"context"`
	SourceRevision int64         `json:"source_revision"`
	Dataset        string        `json:"dataset"`
	Schema         string        `json:"schema"`
	Name           string        `json:"name"`
	Columns        []exec.Column `json:"columns"`
}

// Reference is a coordinate, never a bearer. The default selects a published
// revision. An exact private revision still requires the original actor/reach.
type Reference struct {
	Revision int64 `json:"revision,omitempty"`
	Draft    bool  `json:"draft"`
}

// State contains the CAS version and current lifecycle pointers for a stable block identity.
type State struct {
	ID                string    `json:"id"`
	Topic             string    `json:"topic"`
	Version           int64     `json:"version"`
	DraftRevision     int64     `json:"draft_revision"`
	PublishedRevision int64     `json:"published_revision"`
	DraftState        string    `json:"draft_state"`
	Archived          bool      `json:"archived"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Revision is an immutable definition snapshot with stable content and execution digests.
type Revision struct {
	Number          int64      `json:"number"`
	ID              string     `json:"id"`
	Definition      Definition `json:"definition"`
	Digest          string     `json:"digest"`
	ExecutionDigest string     `json:"execution_digest"`
	Actor           string     `json:"actor"`
	CreatedAt       time.Time  `json:"created_at"`
	Provenance      Provenance `json:"provenance"`
}

// Evidence is content-free validation evidence. Observed schema and the real
// query attempt are retained, but SQL, parameter values and result rows are not.
type Evidence struct {
	ResultPolicy            []EffectiveFieldPolicy `json:"result_policy,omitempty"`
	ID                      string                 `json:"id"`
	Revision                int64                  `json:"revision"`
	RevisionID              string                 `json:"revision_id"`
	DefinitionDigest        string                 `json:"definition_digest"`
	ExecutionDigest         string                 `json:"execution_digest"`
	ParameterDigest         string                 `json:"parameter_digest"`
	DependencyDigest        string                 `json:"dependency_digest"`
	SchemaDigest            string                 `json:"schema_digest"`
	CanonicalizationVersion string                 `json:"canonicalization_version"`
	ValidatorVersion        string                 `json:"validator_version"`
	ValidationManifest      string                 `json:"validation_manifest"`
	Schema                  []exec.Field           `json:"schema"`
	Attempt                 exec.Attempt           `json:"attempt"`
	Actor                   string                 `json:"actor"`
	CreatedAt               time.Time              `json:"created_at"`
	ExpiresAt               time.Time              `json:"expires_at"`
	ResolvedAt              time.Time              `json:"resolved_at"`
	Timezone                string                 `json:"timezone"`
	Parameters              []BoundValue           `json:"parameters"`
}

// ValidationRecord is private server-derived dependency evidence for persistence.
// It is intentionally not an API response schema.
type ValidationRecord struct {
	Catalog       sources.CatalogIdentity `json:"catalog_identity"`
	Evidence      Evidence                `json:"evidence"`
	Dependencies  []Dependency            `json:"dependencies"`
	BindingDigest string                  `json:"binding_digest"`
	Topics        []TopicPin              `json:"topics"`
	Binding       exec.Binding            `json:"binding"`
	Definitions   []topics.Definition     `json:"definitions"`
	Rules         []RulePin               `json:"rules,omitempty"`
}

// Attestation records a separately authorized certification of exact revision evidence.
type Attestation struct {
	ID                string          `json:"id"`
	Revision          int64           `json:"revision"`
	Evidence          string          `json:"evidence"`
	Actor             string          `json:"actor"`
	Note              string          `json:"note"`
	CreatedAt         time.Time       `json:"created_at"`
	EvidenceExpiresAt time.Time       `json:"evidence_expires_at"`
	DependencyDigest  string          `json:"dependency_digest"`
	PeriodFindings    []PeriodFinding `json:"period_findings,omitempty"`
	PeriodReviews     []PeriodReview  `json:"period_reviews,omitempty"`
}

// Withdrawal records the explicit withdrawal of an existing certification.
type Withdrawal struct {
	Attestation string    `json:"attestation"`
	Actor       string    `json:"actor"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// Health records current dependency status independently of historical approval.
type Health struct {
	Status           string     `json:"status"`
	ObservedAt       *time.Time `json:"observed_at,omitempty"`
	DependencyDigest string     `json:"dependency_digest,omitempty"`
	Reason           string     `json:"reason"`
}

// Trust projects publication, certification, current health and historical attestation separately.
type Trust struct {
	Publication           string       `json:"publication"`
	Certification         string       `json:"certification"`
	HistoricalAttestation *Attestation `json:"historical_attestation,omitempty"`
	Withdrawal            *Withdrawal  `json:"withdrawal,omitempty"`
	Health                Health       `json:"health"`
}

// View deliberately has no Definition, SQL or private capture provenance field.
// JSON reflection cannot accidentally expose them when a new field is added.
type View struct {
	SchemaVersion   int                    `json:"schema_version"`
	QueryLimits     *QueryLimits           `json:"query_limits,omitempty"`
	ResultPolicy    []EffectiveFieldPolicy `json:"result_policy"`
	State           State                  `json:"state"`
	Revision        int64                  `json:"revision"`
	RevisionID      string                 `json:"revision_id"`
	Digest          string                 `json:"digest"`
	ExecutionDigest string                 `json:"execution_digest"`
	Metadata        []Localized            `json:"metadata"`
	Source          string                 `json:"source"`
	Context         string                 `json:"context"`
	Topics          []TopicPin             `json:"topics"`
	Rules           []RulePin              `json:"rules,omitempty"`
	Parameters      []Parameter            `json:"parameters"`
	ExpectedSchema  []exec.Field           `json:"expected_schema"`
	Outputs         []Output               `json:"outputs"`
	Actor           string                 `json:"actor"`
	CreatedAt       time.Time              `json:"created_at"`
	Private         bool                   `json:"private"`
	Trust           Trust                  `json:"trust"`
	Evidence        *Evidence              `json:"validation,omitempty"`
}

// SQLView is the separately authorized SQL-bearing revision projection.
// Definition is the native, versioned authoring export, never an ordinary read
// field. It carries authored (not resolved) policies without inventing approval.
type SQLView struct {
	Definition *Definition `json:"definition,omitempty"`
	ID         string      `json:"id"`
	Revision   int64       `json:"revision"`
	Digest     string      `json:"digest"`
	SQL        string      `json:"sql"`
	Provenance Provenance  `json:"provenance"`
}

// Snapshot is an internal repository result. Publication and current metadata
// continuity are independent of the immutable definition/attestation history.
type Snapshot struct {
	State       State
	Revision    Revision
	Validation  *ValidationRecord
	Attestation *Attestation
	Withdrawal  *Withdrawal
	PublishedAt *time.Time
	Current     bool
	Health      Health
	References  []ResourceReference
}

// CreateRequest supplies a new stable block identity and its private definition.
type CreateRequest struct {
	ID         string     `json:"id"`
	Definition Definition `json:"definition"`
}

// EditRequest supplies a complete amendment under an expected CAS version.
type EditRequest struct {
	ExpectedVersion int64      `json:"expected_version"`
	Definition      Definition `json:"definition"`
}

// TransitionRequest identifies a version-fenced lifecycle transition and its audit note.
type TransitionRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Note            string `json:"note"`
}

// RestoreRequest selects an existing revision to restore as a new private amendment.
type RestoreRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Revision        int64  `json:"revision"`
	Note            string `json:"note"`
}

// PublishRequest binds publication to an expected state version and validation receipt.
type PublishRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Evidence        string `json:"evidence"`
}

// CertifyRequest identifies the exact revision and evidence to certify under separate authority.
type CertifyRequest struct {
	ExpectedVersion int64          `json:"expected_version"`
	Revision        int64          `json:"revision"`
	Evidence        string         `json:"evidence"`
	Note            string         `json:"note"`
	PeriodReviews   []PeriodReview `json:"period_reviews,omitempty"`
}

// PeriodReviewRequest selects one immutable published revision.
type PeriodReviewRequest struct {
	Revision int64 `json:"revision"`
}

// PeriodReviewResult exposes exact certification findings before attestation.
type PeriodReviewResult struct {
	Revision         int64           `json:"revision"`
	DefinitionDigest string          `json:"definition_digest"`
	Findings         []PeriodFinding `json:"findings"`
}

// WithdrawRequest identifies the attestation to withdraw under an expected state version.
type WithdrawRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Attestation     string `json:"attestation"`
	Note            string `json:"note"`
	Revision        int64  `json:"revision,omitempty"`
}

// ValidateRequest supplies the version-fenced draft and typed execution arguments.
type ValidateRequest struct {
	ExpectedVersion int64      `json:"expected_version"`
	Revision        int64      `json:"revision,omitempty"`
	Arguments       []Argument `json:"arguments"`
	Resolution      Resolution `json:"resolution"`
}

// PreviewRequest adds an optional saved-output subset to a private validation request.
type PreviewRequest struct {
	ValidateRequest
	Outputs []string `json:"outputs" wire:"optional"`
}

// ValidationResult returns persisted validation evidence and the resulting block state.
type ValidationResult struct {
	State    State    `json:"state"`
	Evidence Evidence `json:"evidence"`
}

// PreviewResult is an ephemeral private result, never a retained artifact or a
// published run. Its source-call receipt is real; saved narratives are not run.
type PreviewResult struct {
	ID                  string       `json:"id"`
	Revision            int64        `json:"revision"`
	Digest              string       `json:"digest"`
	Private             bool         `json:"private"`
	Outputs             []Output     `json:"outputs"`
	Result              exec.Result  `json:"result"`
	Attempt             exec.Attempt `json:"attempt"`
	Resolved            []BoundValue `json:"resolved"`
	NarrativesGenerated bool         `json:"narratives_generated"`
}

// CaptureRequest references an existing private query handoff without accepting caller-supplied provenance.
type CaptureRequest struct {
	ID         string      `json:"id"`
	Query      string      `json:"query"`
	Metadata   []Localized `json:"metadata"`
	Outputs    []Output    `json:"outputs"`
	Parameters []Parameter `json:"parameters"`
	Resolution Resolution  `json:"resolution"`
}

// Capture is supplied by the existing query service after its own session and
// current-reach checks. A capture always creates an unvalidated private draft.
type Capture struct {
	SQL        string
	Parameters []exec.Parameter
	Schema     []exec.Field
	Source     string
	Context    string
	Topics     []TopicPin
	Rules      []RulePin
	Template   *TemplatePin
	Templates  []TemplateSelection
	Question   string
}

// ListRequest bounds a permission-filtered scan and controls private-draft inclusion.
type ListRequest struct {
	After         string `json:"after,omitempty"`
	Limit         int    `json:"limit"`
	IncludeDrafts bool   `json:"include_drafts"`
}

// Summary is a SQL-free lifecycle and localized-metadata projection.
type Summary struct {
	State    State       `json:"state"`
	Revision int64       `json:"revision"`
	Metadata []Localized `json:"metadata"`
	Private  bool        `json:"private"`
}

// Page contains only eligible block summaries and a bounded continuation cursor.
type Page struct {
	Items []Summary `json:"items"`
	Next  string    `json:"next,omitempty"`
}

// QuestionRequest supplies localized text for advisory, permission-filtered question assessment.
type QuestionRequest struct {
	Locale        string          `json:"locale"`
	Question      string          `json:"question"`
	IncludeDrafts bool            `json:"include_drafts"`
	Intent        *QuestionIntent `json:"intent,omitempty"`
}

// QuestionMatch identifies an eligible localized question or alias match.
type QuestionMatch struct {
	ID       string  `json:"id"`
	Revision int64   `json:"revision"`
	Question string  `json:"question"`
	Kind     string  `json:"kind"`
	Score    float64 `json:"score"`
	Method   string  `json:"method"`
	Evidence string  `json:"evidence"`
}

// Assessment reports bounded advisory matches without claiming global uniqueness.
type Assessment struct {
	ID                   string          `json:"id"`
	Matches              []QuestionMatch `json:"matches"`
	Complete             bool            `json:"complete"`
	Threshold            float64         `json:"threshold"`
	AuthorityDigest      string          `json:"authority_digest"`
	Method               string          `json:"method"`
	CandidateScopeDigest string          `json:"candidate_scope_digest"`
	EvidenceDigest       string          `json:"evidence_digest"`
}

// QuestionAssessmentRecord is protected observation evidence. It contains no
// SQL, result values, token, prompt or hidden candidate name.
type QuestionAssessmentRecord struct {
	ID                   string          `json:"id"`
	RequestDigest        string          `json:"request_digest"`
	CandidateScopeDigest string          `json:"candidate_scope_digest"`
	EvidenceDigest       string          `json:"evidence_digest"`
	AuthorityDigest      string          `json:"authority_digest"`
	Threshold            float64         `json:"threshold"`
	Method               string          `json:"method"`
	Matches              []QuestionMatch `json:"matches"`
	Complete             bool            `json:"complete"`
	CreatedAt            time.Time       `json:"created_at"`
}

// Event records an immutable lifecycle event and its actor coordinates.
type Event struct {
	Version   int64     `json:"version"`
	Kind      string    `json:"kind"`
	Revision  int64     `json:"revision"`
	Actor     string    `json:"actor"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// History is the lifecycle and trust history eligible under current signed reach and persisted privacy.
type History struct {
	State  State   `json:"state"`
	Events []Event `json:"events"`
}

// TopicReader and SourceReader expose the already-governed metadata services.
type TopicReader interface {
	Read(context.Context, identity.Envelope, string, string) (topics.Published, error)
}

// RuleReader resolves retained immutable rule publications. Exact reads still
// require current signed rule/topic reach from the verified envelope.
type RuleReader interface {
	Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error)
}

// SourceReader supplies current registered source bindings to the common reporting service.
type SourceReader interface {
	ContextBinding(context.Context, identity.Envelope, string, string) (exec.Binding, error)
}

// Validator issues the existing opaque source-safe execution plan.
type Validator interface {
	ValidateWithin(context.Context, identity.Envelope, exec.Request, []exec.RelationScope) (exec.Plan, error)
}

// Executor executes an opaque validated plan through the existing read core.
type Executor interface {
	Execute(context.Context, identity.Envelope, exec.Plan, exec.Options) (exec.ExecutionReport, error)
}

// QueryCapture supplies a private source-backed query definition from the owning query service.
type QueryCapture interface {
	Capture(context.Context, identity.Envelope, string) (Capture, error)
}

// Repository consumes only service-issued mutation proofs. Metadata reads retain
// the verified envelope and enforce tenant/resource/private eligibility in SQL.
type Repository interface {
	ReadBlock(context.Context, identity.Envelope, string, Reference, Access) (Snapshot, error)
	CommitBlock(context.Context, identity.Envelope, Prepared) (State, error)
	ListBlocks(context.Context, identity.Envelope, ListRequest) (Page, error)
	BlockHistory(context.Context, identity.Envelope, string) (History, error)
}

// QuestionAssessmentRepository is an optional persistence capability used by
// the production store. Lightweight test repositories retain read-only behavior.
type QuestionAssessmentRepository interface {
	RecordQuestionAssessment(context.Context, identity.Envelope, QuestionAssessmentRecord) error
}
