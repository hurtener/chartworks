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

// ReportingSearchRequest searches a bounded published metadata page. Kind is
// explicit so a continuation cannot be confused with another resource catalog.
type ReportingSearchRequest struct {
	Kind   string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	Query  string `json:"query"`
	Locale string `json:"locale"`
	After  string `json:"after"`
	Limit  int    `json:"limit"`
}

// ReportingResource is an intentionally SQL/definition/value-free search result.
type ReportingResource struct {
	Target      DeliveryTarget `json:"target"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Locale      string         `json:"locale"`
}

// ReportingSearchResult carries a bounded permission-filtered continuation.
type ReportingSearchResult struct {
	Version string              `json:"version"`
	Items   []ReportingResource `json:"items"`
	Next    string              `json:"next"`
}

// ReportingDescribeRequest selects published metadata and exact output IDs.
type ReportingDescribeRequest struct {
	Target  DeliveryTarget `json:"target"`
	Locale  string         `json:"locale"`
	Outputs []string       `json:"outputs"`
}

// ViewerOutputChoice retains stable IDs/order without exposing narrative prompts.
type ViewerOutputChoice struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// ViewerFilter describes a business parameter, not an authorization predicate.
type ViewerFilter struct {
	Page      string    `json:"page"`
	Label     string    `json:"label"`
	Parameter Parameter `json:"parameter"`
}

// ReportingDescription exposes presentation and typed business inputs only.
type ReportingDescription struct {
	Version  string                   `json:"version"`
	Resource ReportingResource        `json:"resource"`
	Outputs  []ViewerOutputChoice     `json:"outputs"`
	Filters  []ViewerFilter           `json:"filters"`
	Pages    []CompositionPageSummary `json:"pages"`
	Trust    *Trust                   `json:"trust,omitempty"`
	Dynamic  bool                     `json:"dynamic"`
	Timezone string                   `json:"timezone"`
}

// ReportingRunRequest is deliberately side-effecting. It cannot contain SQL,
// arbitrary query text, a bearer, a binding, a source URL or a replacement chart.
// A changed filter needs a new key and fresh signed target/dependency authority.
type ReportingRunRequest struct {
	Target         DeliveryTarget `json:"target"`
	Key            string         `json:"key"`
	Arguments      []Argument     `json:"arguments"`
	Pages          []PageInput    `json:"pages"`
	Outputs        []string       `json:"outputs"`
	Policy         string         `json:"policy"`
	Locale         string         `json:"locale"`
	Timezone       string         `json:"timezone"`
	Narrative      bool           `json:"narrative"`
	Dynamic        bool           `json:"dynamic"`
	PartialFailure string         `json:"partial_failure"`
}

// ReportingRunResult supplies an ordinary catalog coordinate, not inline values.
type ReportingRunResult struct {
	Version string         `json:"version"`
	Kind    string         `json:"kind"`
	Run     string         `json:"run"`
	State   string         `json:"state"`
	Code    string         `json:"code"`
	Target  DeliveryTarget `json:"target"`
}

// ReportingRunsRequest never triggers an occurrence or grants recipient access.
type ReportingRunsRequest struct {
	Kind     string `json:"kind" jsonschema:"enum=block,enum=report,enum=dashboard"`
	Resource string `json:"resource"`
	After    string `json:"after"`
	Limit    int    `json:"limit"`
}

// ReportingRunSummary separates artifact publication/retention from execution.
// Scheduled provenance, when present, is metadata, not a viewer scheduler.
type ReportingRunSummary struct {
	Kind    string         `json:"kind"`
	Run     string         `json:"run"`
	Target  DeliveryTarget `json:"target"`
	State   string         `json:"state"`
	Code    string         `json:"code"`
	Private bool           `json:"private"`
	Created time.Time      `json:"created_at"`
	Expires time.Time      `json:"expires_at"`
}

// ReportingRunsResult is a metadata page independent of model availability.
type ReportingRunsResult struct {
	Version string                `json:"version"`
	Items   []ReportingRunSummary `json:"items"`
	Next    string                `json:"next"`
}

// CompositionCatalog lists only currently authorized retained metadata.
// It deliberately has no query/model/worker method.
type CompositionCatalog interface {
	ListCompositionArtifacts(context.Context, identity.Envelope, string, string, string, int) (ReportingRunsResult, error)
}

// ReportingViewRequest addresses one selected artifact output. Redraw and
// pagination cannot change filters or cause a data/model execution.
type ReportingViewRequest struct {
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

// ReportingViewResult is the single portable selected-result contract. The
// manifest itself, source credentials, SQL and run-authority tokens never occur.
// A page of a table preserves its exact labels, units and retained row order.
type ReportingViewResult struct {
	Version        string                   `json:"version"`
	Summary        ReportingRunSummary      `json:"summary"`
	Selection      ReportingViewRequest     `json:"selection"`
	Locale         string                   `json:"locale"`
	Timezone       string                   `json:"timezone"`
	Outputs        []ViewerOutputChoice     `json:"outputs"`
	Pages          []CompositionPageSummary `json:"pages"`
	Filters        []ViewerFilter           `json:"filters"`
	Trust          *Trust                   `json:"trust,omitempty"`
	Observed       *time.Time               `json:"observed_at,omitempty"`
	MixedFreshness bool                     `json:"mixed_freshness"`
	Redacted       bool                     `json:"redacted"`
	Output         *ViewerOutput            `json:"output,omitempty"`
	Text           *TextWidget              `json:"text,omitempty"`
	PageBounds     ViewerPage               `json:"page_bounds"`
}
