package chartworks

import (
	"context"
	"github.com/hurtener/chartworks/internal/chartdata"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
)

// Public aliases mirror the registered closed wire contract without a parallel
// SDK model or business implementation.
type (
	// ChartData is ordered caller-supplied data, not an authority-bearing artifact.
	ChartData = charts.Data
	// ChartColumn carries type, format and reviewed semantic provenance.
	ChartColumn = charts.Column
	// ChartCell preserves exact text and a separate null bit.
	ChartCell = charts.Cell
	// ChartCompleteness describes the result, never full-source coverage.
	ChartCompleteness = charts.Completeness
	// ChartFormat provides literal unit/currency/percentage hints.
	ChartFormat = charts.Format
	// ChartColumnProvenance pins semantic meaning separately from display labels.
	ChartColumnProvenance = charts.Provenance
	// ChartLimits are active server bounds, not client-widenable permissions.
	ChartLimits = charts.Limits
	// ChartKind is a closed catalog kind.
	ChartKind = charts.Kind
	// ChartBindings assigns columns to required and optional slots.
	ChartBindings = charts.Bindings
	// ChartOrder describes a stable explicit sort.
	ChartOrder = charts.Order
	// ChartOptions cannot contain JavaScript or remote resources.
	ChartOptions = charts.Options
	// ChartLegend is declarative display metadata.
	ChartLegend = charts.Legend
	// ChartMapping is an exact portable saved definition.
	ChartMapping = charts.Mapping
	// ChartOutput separates exact labels and optional approximate coordinates.
	ChartOutput = charts.Output
	// ChartProposal is detached and requires explicit author review.
	ChartProposal = charts.Proposal
	// ChartCatalogResult contains all fourteen real specification kinds.
	ChartCatalogResult = chartservice.CatalogResult
	// ChartSelectRequest opts into exploratory ranking explicitly.
	ChartSelectRequest = chartservice.SelectRequest
	// ChartSelectionResult preserves suitability and ranking provenance separately.
	ChartSelectionResult = chartservice.SelectionResult
	// ChartSpecifyRequest authors one explicit kind without a table fallback.
	ChartSpecifyRequest = chartservice.SpecifyRequest
	// ChartBuildRequest applies only an exact saved definition.
	ChartBuildRequest = chartservice.BuildRequest
	// ChartBuildResult is typed drawing input, not rendered pixels.
	ChartBuildResult = chartservice.BuildResult
)

// The catalog has fourteen distinct kinds; table fallback is never chart parity.
const (
	ChartArea          = charts.Area
	ChartBar           = charts.Bar
	ChartColumnKind    = charts.ColumnChart
	ChartDonut         = charts.Donut
	ChartGroupedBar    = charts.GroupedBar
	ChartHeatmap       = charts.Heatmap
	ChartKPI           = charts.KPI
	ChartLine          = charts.Line
	ChartPie           = charts.Pie
	ChartScatter       = charts.Scatter
	ChartStackedBar    = charts.StackedBar
	ChartStackedColumn = charts.StackedColumn
	ChartTable         = charts.Table
	ChartTreemap       = charts.Treemap
)

// DefaultChartOptions returns the same literal options as the common core.
func DefaultChartOptions() ChartOptions { return charts.DefaultOptions() }

// ChartCatalog reads current supported kinds and limits using fresh authority.
func (c *Client) ChartCatalog(ctx context.Context) (ChartCatalogResult, error) {
	var out ChartCatalogResult
	err := c.call(ctx, "GET", "/v1/charts/catalog", "", nil, &out)
	return out, err
}

// SelectChart chooses a suitable definition; it never executes or retrieves data.
func (c *Client) SelectChart(ctx context.Context, in ChartSelectRequest) (ChartSelectionResult, error) {
	var out ChartSelectionResult
	err := c.callLimit(ctx, "POST", "/v1/charts/select", "", in, &out, 40<<20)
	return out, err
}

// SpecifyChart authors one explicit catalog kind without fallback or inference.
func (c *Client) SpecifyChart(ctx context.Context, in ChartSpecifyRequest) (ChartBuildResult, error) {
	var out ChartBuildResult
	err := c.callLimit(ctx, "POST", "/v1/charts/specify", "", in, &out, 40<<20)
	return out, err
}

// BuildChart applies an exact mapping without reselecting, rebinding or ranking.
func (c *Client) BuildChart(ctx context.Context, in ChartBuildRequest) (ChartBuildResult, error) {
	var out ChartBuildResult
	err := c.callLimit(ctx, "POST", "/v1/charts/build", "", in, &out, 40<<20)
	return out, err
}

// RebindChart returns a proposal; it does not update any approved definition.
func (c *Client) RebindChart(ctx context.Context, in ChartBuildRequest) (ChartProposal, error) {
	var out ChartProposal
	err := c.callLimit(ctx, "POST", "/v1/charts/rebind", "", in, &out, 2<<20)
	return out, err
}

// ErrChartResult rejects incompatible or malformed read-result wire data.
var ErrChartResult = chartdata.ErrInvalid

// ChartDataFromReadResult losslessly adapts the existing qualified read result.
// It never infers additive aggregation, units, currency, semantic approval or a
// source partition. Callers may add reviewed metadata before authoring a mapping.
// Limits come from ChartCatalog; oversize data fails rather than truncating again.
func ChartDataFromReadResult(ctx context.Context, result ReadResult, limits ChartLimits) (ChartData, error) {
	return chartdata.FromReadResult(ctx, result, limits)
}
