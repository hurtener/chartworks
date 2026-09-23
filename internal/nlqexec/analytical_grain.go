package nlqexec

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

type grainDimension struct {
	id      string
	field   semantics.Reference
	role    semantics.DimensionRole
	filters []semantics.SemanticFilter
}

// compileAnalyticalGrain recognizes only a complete terminal by/por/per clause
// whose dimension labels resolve to already selected reviewed roots. A mentioned
// filter, KPI ingredient or rule-required dimension is NOT a grouping by default.
// This is a bounded, versioned grammar, not a general natural-language parser.
func compileAnalyticalGrain(ctx context.Context, a admission, contract exec.AnalyticalContract) (*exec.AnalyticalGrain, error) {
	question := semantics.RedactClarificationText(a.route.Request.Question, a.route.Request.Answers, a.route.Resolutions)
	if len(question) > 16<<10 {
		return nil, exec.ErrLimit
	}
	words := grainWords(strings.ReplaceAll(question, "[redacted answer]", "\x00"))
	// A reviewed metric can itself contain a grouping-looking word (for example
	// "Revenue per order"). Do not parse words inside that exact label as intent.
	protected := make([]bool, len(words))
	for i, pick := range a.route.Selection.Topics {
		def := a.publications[i].Definition
		for _, root := range pick.Roots {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if root.Reason == "required_rule" {
				continue
			}
			var labels []string
			for _, m := range def.Measures {
				if root.Reference.Kind == semantics.KindMeasure && root.Reference.ID == m.ID {
					labels = append([]string{m.Name}, m.Aliases...)
				}
			}
			for _, k := range def.KPIs {
				if root.Reference.Kind == semantics.KindKPI && root.Reference.ID == k.ID {
					labels = append([]string{k.Name}, k.Aliases...)
				}
			}
			for _, label := range labels {
				tokens := grainWords(label)
				if len(tokens) == 0 || strings.Contains(strings.Join(tokens, " "), "\x00") {
					continue
				}
				if len(tokens) > 32 {
					return nil, exec.ErrLimit
				}
				for start := 0; start+len(tokens) <= len(words); start++ {
					if start%64 == 0 {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
					}
					if grainPrefix(words[start:], tokens) {
						for j := start; j < start+len(tokens); j++ {
							protected[j] = true
						}
					}
				}
			}
		}
	}
	start := -1
	for i, word := range words {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if protected[i] || word != "by" && word != "por" && word != "per" {
			continue
		}
		if unmeasuredGrainMarker(words, i) {
			continue
		}
		start = i + 1
		break
	}
	if start < 0 {
		return nil, nil
	} // Unknown is not a scalar-total assertion.
	if len(a.publications) != 1 {
		return nil, analyticalUnsupported("analytical_grain_unsupported")
	}
	terms := map[string][]grainDimension{}
	maxWords, size := 0, 0
	def := a.publications[0].Definition
	for _, root := range a.route.Selection.Topics[0].Roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if root.Reference.Kind != semantics.KindDimension || root.Reason == "required_rule" {
			continue
		}
		if !root.Reference.Valid() {
			return nil, exec.ErrBinding
		}
		for _, d := range def.Dimensions {
			if d.ID != root.Reference.ID {
				continue
			}
			value := grainDimension{id: def.Topic + ":dimension:" + d.ID, field: d.Field, role: d.Role, filters: d.Filters}
			for _, label := range append([]string{d.Name}, d.Aliases...) {
				tokens := grainWords(label)
				if len(tokens) == 0 || strings.Contains(strings.Join(tokens, " "), "\x00") {
					continue
				}
				if len(tokens) > 32 {
					return nil, exec.ErrLimit
				}
				key := strings.Join(tokens, " ")
				size += len(key)
				if size > 1<<20 {
					return nil, exec.ErrLimit
				}
				duplicate := false
				for _, old := range terms[key] {
					if old.id == value.id {
						duplicate = true
					}
				}
				if !duplicate {
					terms[key] = append(terms[key], value)
				}
				maxWords = max(maxWords, len(tokens))
			}
		}
	}
	result := &exec.AnalyticalGrain{Policy: exec.AnalyticalGrainPolicy}
	columns, dimensions := map[string]bool{}, map[string]bool{}
	compiler := analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, dataset: contract.Dataset}
	for start < len(words) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var choices []grainDimension
		width := min(maxWords, len(words)-start)
		for ; width > 0; width-- {
			if values := terms[strings.Join(words[start:start+width], " ")]; len(values) > 0 {
				choices = values
				break
			}
		}
		if len(choices) == 0 && len(dimensions) == 0 {
			return nil, nil
		} // No recognized grain: retain the explicitly limited metric-only scope.
		if len(choices) != 1 {
			return nil, analyticalUnsupported("analytical_grain_unsupported")
		}
		chosen := choices[0]
		// Temporal buckets and dimension-specific populations need their own
		// expression/predicate proof; a direct field cannot stand in for those.
		if chosen.role == semantics.DimensionTemporal || len(chosen.filters) > 0 {
			return nil, analyticalUnsupported("analytical_grain_unsupported")
		}
		col, err := compiler.column(chosen.field)
		if err != nil {
			return nil, err
		}
		columns[col.SourceName], dimensions[chosen.id] = true, true
		if len(dimensions) > 16 {
			return nil, exec.ErrLimit
		}
		start += width
		if start == len(words) {
			break
		}
		// Consume one explicit conjunction; a comma optionally followed by and/y
		// supports ordinary lists. Never silently ignore the rest of a clause.
		switch words[start] {
		case ",":
			start++
			if start < len(words) && (words[start] == "and" || words[start] == "y") {
				start++
			}
		case "and", "y":
			start++
		default:
			return nil, analyticalUnsupported("analytical_grain_unsupported")
		}
		if start == len(words) {
			return nil, analyticalUnsupported("analytical_grain_unsupported")
		}
	}
	if len(dimensions) == 0 {
		return nil, analyticalUnsupported("analytical_grain_unsupported")
	}
	for column := range columns {
		result.Columns = append(result.Columns, column)
	}
	for id := range dimensions {
		result.Dimensions = append(result.Dimensions, id)
	}
	sort.Strings(result.Columns)
	sort.Strings(result.Dimensions)
	return result, nil
}

// Explicit ordering/filtering and negative clauses are not affirmative grain.
// Unknown wording remains outside this versioned recognizer's quality claim.
func unmeasuredGrainMarker(words []string, i int) bool {
	if i == 0 {
		return false
	}
	switch words[i-1] {
	case "order", "ordered", "sort", "sorted", "ordenar", "ordenado",
		"filter", "filtered", "filtering", "filtrar", "filtrado", "filtrada", "filtrados", "filtradas",
		"not", "without", "no", "sin":
		return true
	case "group", "grouped", "grouping", "agrupar", "agrupado", "agrupada", "agrupados", "agrupadas", "agrupando", "agrupes", "agrupe":
		if i > 1 {
			switch words[i-2] {
			case "not", "without", "no", "sin":
				return true
			}
		}
	}
	return false
}

// grainWords preserves commas as list delimiters. Quoted literals are masked
// before recognition; a value named "by Region" cannot create a grouping.
func grainWords(s string) []string {
	var b strings.Builder
	var quote rune
	for _, r := range strings.ToLower(s) {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
		case '“':
			quote = '”'
		case '‘':
			quote = '’'
		case '«':
			quote = '»'
		}
		if quote != 0 {
			b.WriteString(" \x00 ")
			continue
		}
		switch {
		case r == 0:
			b.WriteString(" \x00 ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ',':
			b.WriteString(" , ")
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}
func grainPrefix(words, prefix []string) bool {
	if len(words) < len(prefix) {
		return false
	}
	for i, w := range prefix {
		if words[i] != w {
			return false
		}
	}
	return true
}

func analyticalGrainGuidance(contract *exec.AnalyticalContract) string {
	if contract == nil || contract.Grain == nil {
		return ""
	}
	// All content is already validated reviewed physical/semantic coordinates,
	// never private scalar answers. This is counted by full-envelope fitting.
	raw, err := json.Marshal(contract.Grain)
	if err != nil {
		return ""
	}
	return " The exact selected grouping is enforced: project and GROUP BY every listed physical column; do not add invisible grouping keys or replace the grouping with a scalar total. Grouping contract: " + string(raw)
}
