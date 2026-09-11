package reporting

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// CompositionVersion versions an accepted manifest independently of definitions.
const CompositionVersion = "report-composition-v1"

// WidgetOverride is accepted only for parameter names permitted by the widget.
type WidgetOverride struct {
	Widget    string     `json:"widget"`
	Arguments []Argument `json:"arguments"`
}

// PageInput binds business filters to one declared page, not row-security reach.
// A direct report uses the reserved page ID "main".
type PageInput struct {
	Page      string           `json:"page"`
	Filters   []Argument       `json:"filters"`
	Overrides []WidgetOverride `json:"overrides"`
}

// CompositionRequest is reserved before floating references are resolved.
// Preview must name an exact revision and independently requires signed preview.
type CompositionRequest struct {
	Key            string            `json:"key"`
	Reference      DocumentReference `json:"reference"`
	Preview        bool              `json:"preview"`
	PartialFailure string            `json:"partial_failure,omitempty"`
	Resolution     Resolution        `json:"resolution"`
	Pages          []PageInput       `json:"pages"`
}

// CompositionWidget keeps its own output selection, binding provenance and safe
// presentation even when its query is shared with another widget.
type CompositionWidget struct {
	Definition Widget       `json:"definition"`
	Parameters []BoundValue `json:"parameters"`
	Group      string       `json:"group,omitempty"`
	Code       string       `json:"code,omitempty"`
}

// CompositionPage is an exact report revision, never a floating dashboard page.
type CompositionPage struct {
	ID       string              `json:"id"`
	Report   string              `json:"report"`
	Revision int64               `json:"revision"`
	Digest   string              `json:"digest"`
	Title    string              `json:"title"`
	Private  bool                `json:"private"`
	Locale   string              `json:"locale"`
	Timezone string              `json:"timezone"`
	Widgets  []CompositionWidget `json:"widgets"`
}

// CompositionGroup seals one exact query/context/value identity and the union
// of saved outputs. It does not expose SQL through metadata surfaces.
type CompositionGroup struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	Block          string              `json:"block,omitempty"`
	Revision       int64               `json:"revision,omitempty"`
	Definition     string              `json:"definition_digest,omitempty"`
	Execution      string              `json:"execution_digest,omitempty"`
	Outputs        []string            `json:"outputs"`
	Arguments      []Argument          `json:"arguments"`
	Resolved       Resolved            `json:"resolved"`
	Resolution     Resolution          `json:"resolution"`
	Binding        exec.Binding        `json:"binding"`
	Locale         string              `json:"locale"`
	Policy         string              `json:"policy"`
	Private        bool                `json:"private"`
	Narrative      bool                `json:"narrative"`
	Query          *QueryWidget        `json:"query,omitempty"`
	Origin         *QueryOrigin        `json:"origin,omitempty"`
	References     []ResourceReference `json:"references"`
	Trust          *Trust              `json:"trust,omitempty"`
	ReservedCalls  int                 `json:"reserved_calls"`
	ReservedTokens int                 `json:"reserved_tokens"`
}

// CompositionManifest is service-issued, immutable and bound to the existing
// request operation. Private preview status cannot change with publication.
type CompositionManifest struct {
	Version        string                      `json:"version"`
	ID             string                      `json:"id"`
	Tenant         string                      `json:"tenant"`
	Actor          string                      `json:"actor"`
	Session        string                      `json:"session"`
	Kind           string                      `json:"kind"`
	Document       string                      `json:"document"`
	Revision       int64                       `json:"revision"`
	Digest         string                      `json:"definition_digest"`
	RequestHash    string                      `json:"request_hash"`
	TaskHash       string                      `json:"task_hash"`
	Private        bool                        `json:"private"`
	Policy         string                      `json:"policy"`
	Redacted       bool                        `json:"redacted"`
	Created        time.Time                   `json:"created_at"`
	Expires        time.Time                   `json:"expires_at"`
	Limits         config.ReportingComposition `json:"limits"`
	ArtifactLimits config.ReportingExecution   `json:"artifact_limits"`
	Pages          []CompositionPage           `json:"pages"`
	Groups         []CompositionGroup          `json:"groups"`
}

// ManifestDigest is independent of mutable attempt and result state.
func (m CompositionManifest) ManifestDigest() string { return digest(m) }

// GroupResult is an immutable checkpoint of a completed or explicitly failed
// query group. Query evidence has no block certificate field.
type GroupResult struct {
	Group     string               `json:"group"`
	Kind      string               `json:"kind"`
	State     string               `json:"state"`
	Code      string               `json:"code,omitempty"`
	ChildRun  string               `json:"child_run,omitempty"`
	Block     *RunView             `json:"block,omitempty"`
	Outputs   []RetainedOutput     `json:"outputs"`
	Query     *nlqexec.SavedResult `json:"query,omitempty"`
	QueryPlan *nlqexec.SavedPlan   `json:"query_plan,omitempty"`
	Observed  *time.Time           `json:"observed_at,omitempty"`
	Digest    string               `json:"digest"`
}

// CompositionWidgetSummary has no raw normalized values, SQL or hidden payload.
type CompositionWidgetSummary struct {
	ID             string       `json:"id"`
	Kind           string       `json:"kind"`
	State          string       `json:"state"`
	Code           string       `json:"code,omitempty"`
	Grid           GridCell     `json:"grid"`
	Presentation   Presentation `json:"presentation"`
	Outputs        []string     `json:"outputs"`
	Parameters     []BoundValue `json:"parameters"`
	Durability     string       `json:"durability,omitempty"`
	Trust          *Trust       `json:"trust,omitempty"`
	QueryDigest    string       `json:"query_digest,omitempty"`
	SemanticDigest string       `json:"semantic_digest,omitempty"`
	Observed       *time.Time   `json:"observed_at,omitempty"`
}

// CompositionPageSummary is independently redacted under current report reach.
type CompositionPageSummary struct {
	ID       string                     `json:"id"`
	Report   string                     `json:"report"`
	Revision int64                      `json:"revision"`
	Title    string                     `json:"title"`
	Widgets  []CompositionWidgetSummary `json:"widgets"`
}

// CompositionView is metadata only. Opening it never executes a source or model.
type CompositionView struct {
	ID             string                   `json:"id"`
	Kind           string                   `json:"kind"`
	Document       string                   `json:"document"`
	Revision       int64                    `json:"revision"`
	Manifest       string                   `json:"manifest_digest"`
	State          string                   `json:"state"`
	Code           string                   `json:"code,omitempty"`
	Private        bool                     `json:"private"`
	Complete       bool                     `json:"complete"`
	MixedFreshness bool                     `json:"mixed_freshness"`
	Redacted       bool                     `json:"redacted"`
	Created        time.Time                `json:"created_at"`
	Expires        time.Time                `json:"expires_at"`
	Finished       *time.Time               `json:"finished_at,omitempty"`
	Pages          []CompositionPageSummary `json:"pages"`
	QueryGroups    int                      `json:"query_groups"`
	RetainedBytes  int64                    `json:"retained_bytes"`
}

// CompositionPayload returns only the addressed visible widget's saved subset.
type CompositionPayload struct {
	Page    string               `json:"page"`
	Widget  string               `json:"widget"`
	State   string               `json:"state"`
	Code    string               `json:"code,omitempty"`
	Text    *TextWidget          `json:"text,omitempty"`
	Outputs []RetainedOutput     `json:"outputs"`
	Query   *nlqexec.SavedResult `json:"query,omitempty"`
}

// CompositionRecord is private execution state, not a metadata response shape.
type CompositionRecord struct {
	Manifest CompositionManifest
	State    string
	Code     string
	Results  []GroupResult
	Plans    map[string]nlqexec.SavedPlan
}

// CompositionRepository adds retained composition state to the existing leased
// operation ledger. It does not introduce a scheduler or independent queue.
type CompositionRepository interface {
	ReadComposition(context.Context, identity.Envelope, string) (CompositionRecord, error)
	SealComposition(context.Context, identity.Envelope, jobs.RequestTask, PreparedComposition) (CompositionRecord, error)
	CheckpointComposition(context.Context, jobs.Invocation, PreparedCompositionWrite) (CompositionRecord, error)
	ViewComposition(context.Context, identity.Envelope, string) (CompositionView, error)
	CompositionWidget(context.Context, identity.Envelope, string, string, string) (CompositionPayload, error)
}
