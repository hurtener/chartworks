// Package charts owns deterministic, provider-neutral output specifications.
// It has no source, store, model, renderer or authentication dependencies. Its
// input is already obtained data; a specification never grants data authority.
package charts

import (
	"errors"
)

// Version pins the original data, provenance, scalar mapping and output contracts.
const Version = 1

// RichVersion pins repeated bindings and their retained drawing representation.
// Data and column provenance remain version one; scalar mappings stay readable.
const RichVersion = 2

// DisplayVersion pins reviewed KPI/table behavior and authored display metadata.
// Earlier mappings remain byte-compatible and retain their original defaults.
const DisplayVersion = 3

// BuildVersion invalidates frozen-output reuse after a transformation contract
// change without rewriting immutable scalar definitions or old retained views.
const BuildVersion = 3

var (
	// ErrInvalid rejects malformed data, definitions and configuration.
	ErrInvalid = errors.New("charts: invalid input")
	// ErrLimit rejects work before a configured resource bound is exceeded.
	ErrLimit = errors.New("charts: limit exceeded")
	// ErrUnsuitable distinguishes a valid dataset from an incompatible chart.
	ErrUnsuitable = errors.New("charts: unsuitable binding")
	// ErrMappingChanged requires explicit review after a saved column changes.
	ErrMappingChanged = errors.New("charts: saved mapping incompatible")
)

// Kind is one entry in the closed presentation catalog.
type Kind string

// The fourteen output kinds are specifications, not a claim of rendered pixels.
const (
	Area          Kind = "area"
	Bar           Kind = "bar"
	ColumnChart   Kind = "column"
	Donut         Kind = "donut"
	GroupedBar    Kind = "grouped_bar"
	Heatmap       Kind = "heatmap"
	KPI           Kind = "kpi"
	Line          Kind = "line"
	Pie           Kind = "pie"
	Scatter       Kind = "scatter"
	StackedBar    Kind = "stacked_bar"
	StackedColumn Kind = "stacked_column"
	Table         Kind = "table"
	Treemap       Kind = "treemap"
)

// Limits bounds one request's input, category cardinality and output work.
// Options have a closed two-level shape, additionally bounded by bytes/depth.
type Limits struct {
	MaxRows         int `json:"max_rows"`
	MaxColumns      int `json:"max_columns"`
	MaxBytes        int `json:"max_bytes"`
	MaxCellBytes    int `json:"max_cell_bytes"`
	MaxCategories   int `json:"max_categories"`
	MaxSeries       int `json:"max_series"`
	MaxAlternatives int `json:"max_alternatives"`
	SelectionFloor  int `json:"selection_floor"`
	MaxOptionsBytes int `json:"max_options_bytes"`
	MaxOptionsDepth int `json:"max_options_depth"`
}

// Defaults are independent of a live warehouse or model configuration.
func Defaults() Limits {
	return Limits{MaxRows: 5000, MaxColumns: 64, MaxBytes: 2 << 20, MaxCellBytes: 4096,
		MaxCategories: 100, MaxSeries: 12, MaxAlternatives: 3, SelectionFloor: 50,
		MaxOptionsBytes: 4096, MaxOptionsDepth: 2}
}

// Validate rejects unbounded or contradictory limits; zero is not unlimited.
func (l Limits) Validate() error {
	if l.MaxRows < 1 || l.MaxRows > 100000 || l.MaxColumns < 1 || l.MaxColumns > 256 ||
		l.MaxBytes < 1024 || l.MaxBytes > 8<<20 || l.MaxCellBytes < 16 || l.MaxCellBytes > 16384 || l.MaxCellBytes > l.MaxBytes ||
		l.MaxCategories < 1 || l.MaxCategories > 10000 || l.MaxSeries < 1 || l.MaxSeries > 128 ||
		l.MaxAlternatives < 0 || l.MaxAlternatives > 13 || l.SelectionFloor < 1 || l.SelectionFloor > 100 ||
		l.MaxOptionsBytes < 128 || l.MaxOptionsBytes > 16384 || l.MaxOptionsDepth < 1 || l.MaxOptionsDepth > 8 {
		return ErrInvalid
	}
	return nil
}

// Format contains declarative hints only. Exact labels are never rounded by them.
// Percent is empty, fraction (0.1 means 10%), or whole (10 means 10%).
type Format struct {
	Unit           string `json:"unit"`
	Currency       string `json:"currency"`
	CurrencySymbol string `json:"currency_symbol,omitempty"`
	Percent        string `json:"percent"`
	FractionDigits int    `json:"fraction_digits"`
	Locale         string `json:"locale,omitempty"`
	DatePattern    string `json:"date_pattern,omitempty" jsonschema:"enum=,enum=date_short,enum=date_medium,enum=date_long,enum=datetime_short,enum=year_month"`
}

// Provenance pins reviewed meaning separately from a mutable display label.
// These coordinates are descriptive; only a domain's verified envelope is authority.
type Provenance struct {
	Version        int    `json:"version"`
	Source         string `json:"source"`
	SourceRevision int64  `json:"source_revision"`
	Topic          string `json:"topic"`
	TopicVersion   string `json:"topic_version"`
	SemanticID     string `json:"semantic_id"`
}

// Column preserves portable result metadata with no native driver or UI type.
// Role is unknown, identifier, dimension, time, measure or kpi. Aggregation is
// explicit; averages, distinct counts and percentages are never summed implicitly.
type Column struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	DisplayLabel string     `json:"display_label,omitempty"`
	Type         string     `json:"type"`
	Role         string     `json:"role"`
	Grain        string     `json:"grain"`
	Aggregation  string     `json:"aggregation"`
	Format       Format     `json:"format"`
	Provenance   Provenance `json:"provenance"`
}

// Cell uses an explicit null bit and lossless text, including exact numeric text.
// The text is literal data, never HTML, a formatter, code or a resource URL.
type Cell struct {
	Null  bool   `json:"null"`
	Value string `json:"value"`
}

// Completeness describes this result only, never the entire underlying source.
type Completeness struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Data is a bounded, ordered result. Complete means complete_result, not a
// full-source aggregate. Native adapters and callers preserve this distinction.
type Data struct {
	Version      int          `json:"version"`
	Columns      []Column     `json:"columns"`
	Rows         [][]Cell     `json:"rows"`
	Completeness Completeness `json:"completeness"`
}

// Bindings uses closed slots rather than arbitrary renderer option objects.
// Category/value cover categorical/time plots. X/Y cover scatter and heatmap.
// Values is an ordered measure list, exclusive with Value. Hierarchy declares
// root-to-leaf columns, exclusive with Category/Parent. Size encodes bubble area.
// These extensions, and category/series line and base-bar variants, require v2.
type Bindings struct {
	Category   string   `json:"category,omitempty"`
	Value      string   `json:"value,omitempty"`
	Series     string   `json:"series,omitempty"`
	X          string   `json:"x,omitempty"`
	Y          string   `json:"y,omitempty"`
	Parent     string   `json:"parent,omitempty"`
	Columns    []string `json:"columns,omitempty"`
	Values     []string `json:"values,omitempty"`
	Hierarchy  []string `json:"hierarchy,omitempty"`
	Size       string   `json:"size,omitempty"`
	Comparison string   `json:"comparison,omitempty"`
	Target     string   `json:"target,omitempty"`
}

// KPIThreshold maps an exact numeric boundary to a reviewed display state.
type KPIThreshold struct {
	Operator string `json:"operator" jsonschema:"enum=lt,enum=lte,enum=gt,enum=gte"`
	Value    string `json:"value"`
	State    string `json:"state"`
	Label    string `json:"label,omitempty"`
}

// KPIOptions describes deterministic retained calculations. ComparisonColumn
// uses the comparison binding; previous_row compares the selected row to the
// immediately preceding ordered row. Sparkline consumes the ordered value rows.
type KPIOptions struct {
	ValueRow             string         `json:"value_row" jsonschema:"enum=first,enum=last"`
	ComparisonMode       string         `json:"comparison_mode" jsonschema:"enum=none,enum=previous_row,enum=comparison_column"`
	ShowDelta            bool           `json:"show_delta"`
	ShowPercentDelta     bool           `json:"show_percent_delta"`
	ShowTargetDifference bool           `json:"show_target_difference"`
	Sparkline            bool           `json:"sparkline"`
	Thresholds           []KPIThreshold `json:"thresholds"`
}

// TableColumnIntent keeps authored visibility independently from query/schema
// presence. Hidden columns may still participate in the immutable saved sort.
type TableColumnIntent struct {
	Column  string `json:"column"`
	Visible bool   `json:"visible"`
}

// TableOptions carries reviewed display controls; page size is presentation
// intent and never widens retained-result or HTTP paging limits.
type TableOptions struct {
	Columns    []TableColumnIntent `json:"columns"`
	PageSize   int                 `json:"page_size"`
	ShowTotals bool                `json:"show_totals"`
}

// Order is an explicit, stable sort over a bound column. Nulls sort last.
type Order struct {
	Column    string `json:"column"`
	Direction string `json:"direction"`
}

// Legend is a bounded declarative legend setting.
type Legend struct {
	Visible  bool   `json:"visible"`
	Position string `json:"position"`
}

// Options intentionally cannot express formatter callbacks, URLs or scripts.
type Options struct {
	Title         string `json:"title"`
	Legend        Legend `json:"legend"`
	LabelMaxRunes int    `json:"label_max_runes"`
}

// DefaultOptions keeps complete labels even when a renderer shortens display text.
func DefaultOptions() Options {
	return Options{Legend: Legend{Visible: true, Position: "bottom"}, LabelMaxRunes: 80}
}

// Mapping is a portable saved definition. Columns pins every bound column's
// type and semantic metadata; approved mappings are never rebound in place.
type Mapping struct {
	Version  int           `json:"version"`
	Kind     Kind          `json:"kind"`
	Columns  []Column      `json:"columns"`
	Bindings Bindings      `json:"bindings"`
	Order    []Order       `json:"order"`
	Options  Options       `json:"options"`
	KPI      *KPIOptions   `json:"kpi,omitempty"`
	Table    *TableOptions `json:"table,omitempty"`
}

// CatalogEntry describes real slot requirements and data semantics for one kind.
type CatalogEntry struct {
	Kind          Kind             `json:"kind"`
	RequiredSlots []string         `json:"required_slots"`
	OptionalSlots []string         `json:"optional_slots"`
	Negative      string           `json:"negative"`
	Nulls         string           `json:"nulls"`
	Variants      []BindingVariant `json:"variants"`
}

// Candidate retains deterministic rules evidence even after optional ranking.
type Candidate struct {
	Mapping       Mapping  `json:"mapping"`
	Score         int      `json:"score"`
	Reason        string   `json:"reason"`
	Variant       string   `json:"variant,omitempty"`
	Signals       []string `json:"signals,omitempty"`
	UnusedColumns []string `json:"unused_columns,omitempty"`
}

// Selection contains only validated candidates; table fallback is visibly labeled.
type Selection struct {
	Selected     Candidate          `json:"selected"`
	Alternatives []Candidate        `json:"alternatives"`
	Fallback     bool               `json:"fallback"`
	Reason       string             `json:"reason"`
	Evidence     *SelectionEvidence `json:"evidence,omitempty"`
}

// Value separates exact labels from optional approximate geometric coordinates.
type Value struct {
	Null        bool     `json:"null"`
	Exact       string   `json:"exact"`
	Coordinate  *float64 `json:"coordinate"`
	Approximate bool     `json:"approximate"`
}

// Point contains exact category/series/hierarchy labels and numeric coordinates.
// Missing values remain missing: charts never substitute zero or connect gaps.
type Point struct {
	Row         int    `json:"row"`
	Category    Cell   `json:"category"`
	Series      Cell   `json:"series"`
	Parent      Cell   `json:"parent"`
	X           Value  `json:"x"`
	Y           Value  `json:"y"`
	Value       Value  `json:"value"`
	Measure     string `json:"measure,omitempty"`
	SeriesID    string `json:"series_id,omitempty"`
	CategoryKey string `json:"category_key,omitempty"`
	Size        *Value `json:"size,omitempty"`
	Path        []Cell `json:"path,omitempty"`
}

// Total is an exact additive total over returned rows, not a guessed warehouse total.
type Total struct {
	Column string `json:"column"`
	Value  Cell   `json:"value"`
	Scope  string `json:"scope"`
}

// KPIResult is exact retained display evidence. Coordinates are supplied only
// for drawing the sparkline; all displayed labels use Exact.
type KPIResult struct {
	Value            Value   `json:"value"`
	Comparison       *Value  `json:"comparison,omitempty"`
	Delta            *Value  `json:"delta,omitempty"`
	PercentDelta     *Value  `json:"percent_delta,omitempty"`
	Target           *Value  `json:"target,omitempty"`
	TargetDifference *Value  `json:"target_difference,omitempty"`
	ThresholdState   string  `json:"threshold_state,omitempty"`
	ThresholdLabel   string  `json:"threshold_label,omitempty"`
	Sparkline        []Value `json:"sparkline"`
}

// Output is sealed typed input for later renderers. Kind is never replaced with
// table behind the caller's back. Empty states remain specifications of that kind.
type Output struct {
	Version        int                `json:"version"`
	Kind           Kind               `json:"kind"`
	Mapping        Mapping            `json:"mapping"`
	Columns        []Column           `json:"columns"`
	Rows           [][]Cell           `json:"rows"`
	Points         []Point            `json:"points"`
	Totals         []Total            `json:"totals"`
	State          string             `json:"state"`
	InputRows      int                `json:"input_rows"`
	OmittedRows    int                `json:"omitted_rows"`
	Completeness   Completeness       `json:"completeness"`
	Warnings       []string           `json:"warnings"`
	Series         []SeriesDefinition `json:"series,omitempty"`
	Hierarchy      []HierarchyNode    `json:"hierarchy,omitempty"`
	RowIndices     []int              `json:"row_indices,omitempty"`
	Transformation *Transformation    `json:"transformation,omitempty"`
	KPIResult      *KPIResult         `json:"kpi_result,omitempty"`
	TablePageSize  int                `json:"table_page_size,omitempty"`
	ShowTotals     bool               `json:"show_totals,omitempty"`
}

// Change describes one proposed column replacement, never an applied publication.
type Change struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Proposal is explicitly review-required and carries a detached replacement.
type Proposal struct {
	Status  string   `json:"status"`
	Mapping Mapping  `json:"mapping"`
	Changes []Change `json:"changes"`
}
