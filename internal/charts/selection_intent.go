package charts

import (
	"context"
	"strings"
	"unicode"
)

// RulesVersion identifies deterministic selection separately from wire versions.
const RulesVersion = 2

// Cardinality records exact counts over retained rows, not source estimates.
type Cardinality struct {
	Column   string `json:"column"`
	Distinct int    `json:"distinct"`
	Nulls    int    `json:"nulls"`
}

// CandidateEvaluation explains admission before the suitable set is sealed.
// No cells, labels, query text, or source coordinates are ranking metadata.
type CandidateEvaluation struct {
	Kind    Kind     `json:"kind"`
	Variant string   `json:"variant"`
	Outcome string   `json:"outcome"`
	Score   int      `json:"score"`
	Signals []string `json:"signals"`
}

// SelectionEvidence is retained with an exploratory choice. Rules never claim
// that bounded lexical cues establish a live model's quality or business meaning.
type SelectionEvidence struct {
	Version     int                   `json:"version"`
	Intent      string                `json:"intent"`
	Rows        int                   `json:"rows"`
	Measures    int                   `json:"measures"`
	Dimensions  int                   `json:"dimensions"`
	Times       int                   `json:"times"`
	Cardinality []Cardinality         `json:"cardinality"`
	Evaluations []CandidateEvaluation `json:"evaluations"`
	Suitable    int                   `json:"suitable"`
	AboveFloor  int                   `json:"above_floor"`
	TieBreak    string                `json:"tie_break"`
}

// questionIntent consumes only bounded author text; it is not an NLQ parser or
// a new clarification policy. Conflicting cues conservatively preserve defaults.
func questionIntent(intent string) string {
	words := strings.FieldsFunc(strings.ToLower(intent), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	cues := map[string]string{
		"compare": "comparison", "comparison": "comparison", "comparar": "comparison", "comparación": "comparison", "comparacion": "comparison", "ranking": "comparison",
		"trend": "trend", "trends": "trend", "evolution": "trend", "tendencia": "trend", "evolución": "trend", "evolucion": "trend", "line": "trend", "area": "trend", "área": "trend",
		"composition": "composition", "share": "composition", "shares": "composition", "proportion": "composition", "pie": "composition", "donut": "composition", "composición": "composition", "participación": "composition", "participacion": "composition", "proporción": "composition", //nolint:misspell // Intentional unaccented Spanish query cue.
		"correlation": "relationship", "relationship": "relationship", "scatter": "relationship", "correlación": "relationship", "relación": "relationship",
		"composicion": "composition",  //nolint:misspell // Intentional unaccented Spanish query cue.
		"correlacion": "relationship", //nolint:misspell // Intentional unaccented Spanish query cue.
		"bubble":      "bubble", "bubbles": "bubble", "burbuja": "bubble", "burbujas": "bubble",
		"hierarchy": "hierarchy", "hierarchical": "hierarchy", "treemap": "hierarchy", "jerarquía": "hierarchy", "jerarquia": "hierarchy",
		"heatmap": "intensity", "intensity": "intensity", "intensidad": "intensity",
		"table": "table", "tabular": "table", "tabla": "table",
	}
	seen := map[string]bool{}
	for i, word := range words {
		cue := cues[word]
		if cue == "" {
			continue
		}
		negated := false
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			// A preceding chart cue or contrast ends this local negation scope.
			if cues[words[j]] != "" || oneOf(words[j], "but", "pero", "instead") {
				break
			}
			if oneOf(words[j], "no", "not", "without", "sin") {
				negated = true
			}
		}
		if !negated {
			seen[cue] = true
		}
	}
	if seen["bubble"] {
		delete(seen, "relationship")
	}
	if len(seen) > 1 {
		return "mixed"
	}
	for cue := range seen {
		return cue
	}
	return "unspecified"
}

type shapeProfile struct {
	dimensions, times, measures []string
	cardinality                 map[string]int
	evidence                    *SelectionEvidence
}

func profile(ctx context.Context, d Data, intent string) (shapeProfile, error) {
	p := shapeProfile{cardinality: map[string]int{}, evidence: &SelectionEvidence{Version: RulesVersion, Intent: questionIntent(intent), Rows: len(d.Rows), Cardinality: []Cardinality{}, Evaluations: []CandidateEvaluation{}, TieBreak: "catalog_order"}}
	for _, c := range d.Columns {
		if measure(c) {
			p.measures = append(p.measures, c.ID)
		}
		if c.Type == "temporal" {
			p.times = append(p.times, c.ID)
		}
		if category(c) && c.Type != "temporal" {
			p.dimensions = append(p.dimensions, c.ID)
		}
		if !category(c) {
			continue
		}
		seen, nulls := map[string]bool{}, 0
		for _, row := range d.Rows {
			if err := ctx.Err(); err != nil {
				return shapeProfile{}, err
			}
			if at(d, row, c.ID).Null {
				nulls++
				continue
			}
			seen[identityKey(d, row, []string{c.ID})] = true
		}
		p.cardinality[c.ID] = len(seen)
		p.evidence.Cardinality = append(p.evidence.Cardinality, Cardinality{Column: c.ID, Distinct: len(seen), Nulls: nulls})
	}
	p.evidence.Measures, p.evidence.Dimensions, p.evidence.Times = len(p.measures), len(p.dimensions), len(p.times)
	return p, nil
}

func measureBindings(p shapeProfile) Bindings {
	if len(p.measures) == 1 {
		return Bindings{Value: p.measures[0]}
	}
	return Bindings{Values: append([]string{}, p.measures...)}
}

func candidateBinding(kind Kind, d Data, p shapeProfile) (Bindings, []Order, int, string, bool) {
	var b Bindings
	var order []Order
	score, reason := 60, "categorical_comparison"
	switch kind {
	case Table:
		if p.evidence.Intent != "table" {
			return b, order, 0, "fallback_only", false
		}
		for _, c := range d.Columns {
			b.Columns = append(b.Columns, c.ID)
		}
		return b, order, 100, "explicit_tabular_intent", true
	case KPI:
		if len(d.Rows) > 1 || len(p.measures) != 1 {
			return b, order, 0, "single_measure_row_required", false
		}
		b.Value, score, reason = p.measures[0], 100, "single_value"
	case Line, Area:
		if len(p.times) == 0 || len(p.measures) == 0 {
			return b, order, 0, "temporal_measure_required", false
		}
		b = measureBindings(p)
		b.Category = p.times[0]
		if len(p.dimensions) > 0 {
			b.Series = p.dimensions[0]
		}
		order = []Order{{Column: b.Category, Direction: "asc"}}
		score, reason = 95, "time_series_all_measures"
		if kind == Area {
			score = 85
		}
	case Scatter:
		if len(p.measures) < 2 {
			return b, order, 0, "numeric_pair_required", false
		}
		b.X, b.Y, score, reason = p.measures[0], p.measures[1], 90, "numeric_relationship"
		if len(p.dimensions) > 0 {
			b.Series = p.dimensions[0]
		}
		if p.evidence.Intent == "bubble" {
			if len(p.measures) < 3 {
				return b, order, 0, "explicit_size_measure_required", false
			}
			b.Size, reason = p.measures[2], "numeric_relationship_and_area_size"
		}
	case Heatmap:
		if len(p.dimensions) < 2 || len(p.measures) != 1 {
			return b, order, 0, "two_dimensions_one_measure_required", false
		}
		b.X, b.Y, b.Value, score, reason = p.dimensions[0], p.dimensions[1], p.measures[0], 85, "two_dimensional_intensity"
	case StackedBar, StackedColumn:
		if len(p.dimensions) < 2 || len(p.measures) != 1 {
			return b, order, 0, "one_measure_and_categorical_series_required", false
		}
		b.Category, b.Series, b.Value, score, reason = p.dimensions[0], p.dimensions[1], p.measures[0], 80, "category_by_series"
	case Bar, ColumnChart, GroupedBar:
		if len(p.dimensions) == 0 || len(p.measures) == 0 {
			return b, order, 0, "categorical_measure_required", false
		}
		b = measureBindings(p)
		b.Category = p.dimensions[0]
		if len(p.dimensions) > 1 {
			b.Series = p.dimensions[1]
		}
		score = 90
		if kind == ColumnChart {
			score = 85
		}
		if kind == GroupedBar {
			if b.Series == "" && len(b.Values) < 2 {
				return b, order, 0, "multiple_measures_or_series_required", false
			}
			score, reason = 93, "all_measures_by_category"
		}
	default:
		if len(p.dimensions) == 0 || len(p.measures) != 1 {
			return b, order, 0, "one_composition_measure_required", false
		}
		b.Category, b.Value = p.dimensions[0], p.measures[0]
		if kind == Treemap && len(p.dimensions) > 1 {
			if p.evidence.Intent == "hierarchy" {
				b.Category = ""
				b.Hierarchy = append([]string{}, p.dimensions...)
			} else {
				b.Parent = p.dimensions[1]
			}
			score, reason = 75, "declared_result_order_hierarchy"
		}
		if kind == Pie || kind == Donut {
			score, reason = 65, "nonnegative_parts"
		}
	}
	return b, order, score, reason, true
}

func candidateScore(kind Kind, d Data, b Bindings, p shapeProfile, score int) (int, []string) {
	signals := []string{"typed_roles"}
	intent := p.evidence.Intent
	if len(b.Values) > 1 {
		signals = append(signals, "all_measures_retained")
	}
	if b.Series != "" {
		signals = append(signals, "categorical_series")
	}
	if kind == Pie || kind == Donut {
		cardinality := p.cardinality[b.Category]
		if cardinality > 6 {
			score -= 25
			signals = append(signals, "composition_cardinality_high")
		} else {
			signals = append(signals, "composition_cardinality_low")
		}
		c := d.Columns[columnIndex(d.Columns, b.Value)]
		if oneOf(c.Aggregation, "average", "minimum", "maximum", "distinct_count") {
			score -= 40
			signals = append(signals, "nonadditive_measure")
		}
	}
	if kind == ColumnChart && p.cardinality[b.Category] > 30 {
		score -= 15
		signals = append(signals, "many_category_labels")
	}
	boost := 0
	switch intent {
	case "trend":
		switch kind {
		case Line:
			boost = 5
		case Area:
			boost = 10
		default:
			boost = -20
		}
	case "comparison":
		switch kind {
		case GroupedBar:
			boost = 5
		case Bar:
			boost = 7
		case ColumnChart:
			boost = 9
		default:
			boost = -15
		}
	case "composition":
		if (kind == Pie || kind == Donut) && p.cardinality[b.Category] <= 6 && score >= 50 {
			boost = 35
			if kind == Donut {
				boost = 33
			}
		} else if kind == Treemap {
			boost = 10
		}
	case "relationship", "bubble":
		if kind == Scatter {
			boost = 10
		} else {
			boost = -20
		}
	case "hierarchy":
		if kind == Treemap {
			boost = 25
		} else {
			boost = -20
		}
	case "intensity":
		if kind == Heatmap {
			boost = 15
		} else {
			boost = -20
		}
	case "table":
		if kind != Table {
			boost = -30
		}
	}
	if boost != 0 {
		signals = append(signals, "intent_"+intent)
		score += boost
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score, signals
}
