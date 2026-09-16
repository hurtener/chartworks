package reporting

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

// DeliveryVersion is shared by HTTP, MCP, SDK and the bundled read viewer.
const DeliveryVersion = "reporting-view-v1"

// DeliveryTarget is a resource coordinate, never an execution binding or token.
type DeliveryTarget struct {
	Kind     string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

// DeliverySearchRequest searches a bounded published metadata page. Kind is
// explicit so a continuation cannot be confused with another resource catalog.
type DeliverySearchRequest struct {
	Kind   string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	Query  string `json:"query"`
	Locale string `json:"locale"`
	After  string `json:"after"`
	Limit  int    `json:"limit"`
}

// DeliveryResource is an intentionally SQL/definition/value-free search result.
type DeliveryResource struct {
	Target      DeliveryTarget `json:"target"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Locale      string         `json:"locale"`
}

// DeliverySearchResult carries a bounded permission-filtered continuation.
type DeliverySearchResult struct {
	Version string             `json:"version"`
	Items   []DeliveryResource `json:"items"`
	Next    string             `json:"next"`
}

// DeliveryDescribeRequest selects published metadata and exact output IDs.
type DeliveryDescribeRequest struct {
	Target  DeliveryTarget `json:"target"`
	Locale  string         `json:"locale"`
	Outputs []string       `json:"outputs" wire:"optional"`
}

// ViewerOutputChoice retains stable IDs/order without exposing narrative prompts.
type ViewerOutputChoice struct {
	Metadata        []OutputMetadata `json:"metadata,omitempty"`
	Description     string           `json:"description"`
	Locale          string           `json:"locale"`
	DisplayOrder    int              `json:"display_order"`
	Enabled         bool             `json:"enabled"`
	DefaultSelected bool             `json:"default_selected"`
	Selected        bool             `json:"selected"`
	State           string           `json:"state"`
	Code            string           `json:"code,omitempty"`
	ID              string           `json:"id"`
	Kind            string           `json:"kind"`
	Title           string           `json:"title"`
}

// ViewerFilter describes a business parameter, not an authorization predicate.
type ViewerFilter struct {
	Page      string    `json:"page"`
	Label     string    `json:"label"`
	Parameter Parameter `json:"parameter"`
}

// DeliveryDescription exposes presentation and typed business inputs only.
type DeliveryDescription struct {
	Selection     *OutputSelection         `json:"selection,omitempty"`
	SelectionCode string                   `json:"selection_code,omitempty"`
	QueryLimits   *QueryLimits             `json:"query_limits,omitempty"`
	ResultPolicy  []EffectiveFieldPolicy   `json:"result_policy,omitempty"`
	Version       string                   `json:"version"`
	Resource      DeliveryResource         `json:"resource"`
	Outputs       []ViewerOutputChoice     `json:"outputs"`
	Filters       []ViewerFilter           `json:"filters"`
	Pages         []CompositionPageSummary `json:"pages"`
	Trust         *Trust                   `json:"trust,omitempty"`
	Dynamic       bool                     `json:"dynamic"`
	Timezone      string                   `json:"timezone"`
}

// DeliveryRunRequest is deliberately side-effecting. It cannot contain SQL,
// arbitrary query text, a bearer, a binding, a source URL or a replacement chart.
// A changed filter needs a new key and fresh signed target/dependency authority.
type DeliveryRunRequest struct {
	Limits         *QueryLimits   `json:"limits,omitempty"`
	Target         DeliveryTarget `json:"target"`
	Key            string         `json:"key"`
	Arguments      []Argument     `json:"arguments"`
	Pages          []PageInput    `json:"pages"`
	Outputs        []string       `json:"outputs" wire:"optional"`
	Policy         string         `json:"policy"`
	Locale         string         `json:"locale"`
	Timezone       string         `json:"timezone"`
	Narrative      bool           `json:"narrative"`
	Dynamic        bool           `json:"dynamic"`
	PartialFailure string         `json:"partial_failure"`
}

// DeliveryRunResult supplies an ordinary catalog coordinate, not inline values.
type DeliveryRunResult struct {
	Version string         `json:"version"`
	Kind    string         `json:"kind"`
	Run     string         `json:"run"`
	State   string         `json:"state"`
	Code    string         `json:"code"`
	Target  DeliveryTarget `json:"target"`
}

// DeliveryRunsRequest never triggers an occurrence or grants recipient access.
type DeliveryRunsRequest struct {
	Kind     string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	Resource string `json:"resource"`
	After    string `json:"after"`
	Limit    int    `json:"limit"`
}

// DeliveryRunSummary separates artifact publication/retention from execution.
// Scheduled provenance, when present, is metadata, not a viewer scheduler.
type DeliveryRunSummary struct {
	Scheduled *ScheduledProvenance `json:"scheduled,omitempty"`
	Kind      string               `json:"kind"`
	Run       string               `json:"run"`
	Target    DeliveryTarget       `json:"target"`
	State     string               `json:"state"`
	Code      string               `json:"code"`
	Private   bool                 `json:"private"`
	Created   time.Time            `json:"created_at"`
	Expires   time.Time            `json:"expires_at"`
}

// DeliveryRunsResult is a metadata page independent of model availability.
type DeliveryRunsResult struct {
	Version string               `json:"version"`
	Items   []DeliveryRunSummary `json:"items"`
	Next    string               `json:"next"`
}

// CompositionCatalog lists only currently authorized retained metadata.
// It deliberately has no query/model/worker method.
type CompositionCatalog interface {
	ListCompositionArtifacts(context.Context, identity.Envelope, string, string, string, int) (DeliveryRunsResult, error)
}

// DeliveryViewRequest addresses one selected artifact output. Redraw and
// pagination cannot change filters or cause a data/model execution.
type DeliveryViewRequest struct {
	Kind   string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	Run    string `json:"run"`
	Page   string `json:"page"`
	Widget string `json:"widget"`
	Output string `json:"output"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// ViewerPage is an exact retained-table window, never a chart approximation.
type ViewerPage struct {
	Offset int  `json:"offset"`
	Limit  int  `json:"limit"`
	Total  int  `json:"total"`
	Next   *int `json:"next,omitempty"`
}

// DeliveryViewResult is the single portable selected-result contract. The
// manifest itself, source credentials, SQL and run-authority tokens never occur.
// A page of a table preserves its exact labels, units and retained row order.
type DeliveryViewResult struct {
	AcceptedSelection *OutputSelection         `json:"accepted_selection,omitempty"`
	QueryLimits       *QueryLimits             `json:"query_limits,omitempty"`
	ResultPolicy      []EffectiveFieldPolicy   `json:"result_policy,omitempty"`
	Policy            string                   `json:"policy,omitempty"`
	Version           string                   `json:"version"`
	Summary           DeliveryRunSummary       `json:"summary"`
	Selection         DeliveryViewRequest      `json:"selection"`
	Locale            string                   `json:"locale"`
	Timezone          string                   `json:"timezone"`
	Outputs           []ViewerOutputChoice     `json:"outputs"`
	Pages             []CompositionPageSummary `json:"pages"`
	Filters           []ViewerFilter           `json:"filters"`
	Trust             *Trust                   `json:"trust,omitempty"`
	Observed          *time.Time               `json:"observed_at,omitempty"`
	MixedFreshness    bool                     `json:"mixed_freshness"`
	Redacted          bool                     `json:"redacted"`
	Output            *ViewerOutput            `json:"output,omitempty"`
	Text              *TextWidget              `json:"text,omitempty"`
	PageBounds        ViewerPage               `json:"page_bounds"`
}
