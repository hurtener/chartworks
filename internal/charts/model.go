// Package charts owns deterministic, provider-neutral output specifications.
// It has no source, store, model, renderer or authentication dependencies. Its
// input is already obtained data; a specification never grants data authority.
package charts

import (
	"errors"
)

// Version pins the data, mapping and output wire contracts.
const Version = 1

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
	Percent        string `json:"percent"`
	FractionDigits int    `json:"fraction_digits"`
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
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Role        string     `json:"role"`
	Grain       string     `json:"grain"`
	Aggregation string     `json:"aggregation"`
	Format      Format     `json:"format"`
	Provenance  Provenance `json:"provenance"`
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
// Series is required for grouped/stacked plots. Parent adds one treemap level.
type Bindings struct {
	Category string   `json:"category,omitempty"`
	Value    string   `json:"value,omitempty"`
	Series   string   `json:"series,omitempty"`
	X        string   `json:"x,omitempty"`
	Y        string   `json:"y,omitempty"`
	Parent   string   `json:"parent,omitempty"`
	Columns  []string `json:"columns,omitempty"`
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
	Version  int      `json:"version"`
	Kind     Kind     `json:"kind"`
	Columns  []Column `json:"columns"`
	Bindings Bindings `json:"bindings"`
	Order    []Order  `json:"order"`
	Options  Options  `json:"options"`
}

// CatalogEntry describes real slot requirements and data semantics for one kind.
type CatalogEntry struct {
	Kind          Kind     `json:"kind"`
	RequiredSlots []string `json:"required_slots"`
	OptionalSlots []string `json:"optional_slots"`
	Negative      string   `json:"negative"`
	Nulls         string   `json:"nulls"`
}

// Candidate retains deterministic rules evidence even after optional ranking.
type Candidate struct {
	Mapping Mapping `json:"mapping"`
	Score   int     `json:"score"`
	Reason  string  `json:"reason"`
}

// Selection contains only validated candidates; table fallback is visibly labeled.
type Selection struct {
	Selected     Candidate   `json:"selected"`
	Alternatives []Candidate `json:"alternatives"`
	Fallback     bool        `json:"fallback"`
	Reason       string      `json:"reason"`
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
	Row      int   `json:"row"`
	Category Cell  `json:"category"`
	Series   Cell  `json:"series"`
	Parent   Cell  `json:"parent"`
	X        Value `json:"x"`
	Y        Value `json:"y"`
	Value    Value `json:"value"`
}

// Total is an exact additive total over returned rows, not a guessed warehouse total.
type Total struct {
	Column string `json:"column"`
	Value  Cell   `json:"value"`
	Scope  string `json:"scope"`
}

// Output is sealed typed input for later renderers. Kind is never replaced with
// table behind the caller's back. Empty states remain specifications of that kind.
type Output struct {
	Version      int          `json:"version"`
	Kind         Kind         `json:"kind"`
	Mapping      Mapping      `json:"mapping"`
	Columns      []Column     `json:"columns"`
	Rows         [][]Cell     `json:"rows"`
	Points       []Point      `json:"points"`
	Totals       []Total      `json:"totals"`
	State        string       `json:"state"`
	InputRows    int          `json:"input_rows"`
	OmittedRows  int          `json:"omitted_rows"`
	Completeness Completeness `json:"completeness"`
	Warnings     []string     `json:"warnings"`
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
