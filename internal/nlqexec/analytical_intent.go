package nlqexec

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

// compileAnalyticalIntent recognizes a bounded terminal order/limit clause using
// only already selected reviewed labels. It detaches the question used for grain
// compilation; the durable route and original question remain unchanged.
func compileAnalyticalIntent(ctx context.Context, a admission, c exec.AnalyticalContract) (*exec.AnalyticalIntent, admission, error) {
	question := semantics.RedactClarificationText(a.route.Request.Question, a.route.Request.Answers, a.route.Resolutions)
	if len(question) > 16<<10 {
		return nil, a, exec.ErrLimit
	}
	// Preserve signs that the ordinary label tokenizer intentionally discards.
	question = regexp.MustCompile(`[+−-]\s*[0-9]`).ReplaceAllString(question, " \x00 ")
	words := grainWords(question)
	type choice struct {
		order exec.AnalyticalOrder
		label []string
	}
	var choices []choice
	for i, pick := range a.route.Selection.Topics {
		def := a.publications[i].Definition
		compiler := analyticalCompiler{joins: c.Version == exec.AnalyticalIntentVersion || c.Version == exec.AnalyticalGroupedPopulationsVersion || c.Version == exec.AnalyticalGroupedProgramsVersion, ctx: ctx, definition: def, binding: a.binding, dataset: c.Dataset}
		for _, root := range pick.Roots {
			if err := ctx.Err(); err != nil {
				return nil, a, err
			}
			if root.Reason == "required_rule" {
				continue
			}
			var labels []string
			var order exec.AnalyticalOrder
			switch root.Reference.Kind {
			case semantics.KindMeasure:
				for _, m := range def.Measures {
					if m.ID == root.Reference.ID {
						labels = append([]string{m.Name}, m.Aliases...)
						order.Metric = def.Topic + ":measure:" + m.ID
					}
				}
			case semantics.KindKPI:
				for _, m := range def.KPIs {
					if m.ID == root.Reference.ID {
						labels = append([]string{m.Name}, m.Aliases...)
						order.Metric = def.Topic + ":kpi:" + m.ID
					}
				}
			case semantics.KindDimension:
				for _, d := range def.Dimensions {
					if d.ID == root.Reference.ID && len(d.Filters) == 0 && d.Role == semantics.DimensionTemporal && d.Temporal != nil {
						col, err := compiler.column(d.Field)
						if err != nil {
							return nil, a, err
						}
						for _, grain := range d.Temporal.Grains {
							names := map[semantics.TimeGrain][]string{"day": {"day of", "día de", "dia de"}, "month": {"month of", "mes de"}, "quarter": {"quarter of", "trimestre de"}, "year": {"year of", "año de", "ano de"}}[grain]
							bucket := exec.AnalyticalBucket{Column: col.SourceName, Grain: string(grain), Calendar: d.Temporal.Calendar}
							if exec.AnalyticalCalendarKindForDialect(a.binding.Dialect, col.NativeType, col.Category) == "instant" {
								bucket.Timezone = d.Temporal.Timezone
							}
							for _, name := range names {
								for _, label := range append([]string{d.Name}, d.Aliases...) {
									tokens := grainWords(name + " " + label)
									if len(tokens) > 32 {
										return nil, a, exec.ErrLimit
									}
									choices = append(choices, choice{exec.AnalyticalOrder{Bucket: &bucket}, tokens})
								}
							}
						}
					}
					if d.ID == root.Reference.ID && len(d.Filters) == 0 && d.Role != semantics.DimensionTemporal {
						col, err := compiler.column(d.Field)
						if err != nil {
							return nil, a, err
						}
						labels = append([]string{d.Name}, d.Aliases...)
						order.Column = col.SourceName
					}
				}
			}
			for _, label := range labels {
				tokens := grainWords(label)
				if len(tokens) > 32 {
					return nil, a, exec.ErrLimit
				}
				if len(tokens) > 0 {
					choices = append(choices, choice{order, tokens})
				}
			}
		}
	}
	protected := make([]bool, len(words))
	for _, ch := range choices {
		for i := 0; i+len(ch.label) <= len(words); i++ {
			if grainPrefix(words[i:], ch.label) {
				for j := i; j < i+len(ch.label); j++ {
					protected[j] = true
				}
			}
		}
	}
	start, width := -1, 0
	for i := range words {
		if protected[i] {
			continue
		}
		for _, marker := range [][]string{{"ordered", "by"}, {"order", "by"}, {"sorted", "by"}, {"sort", "by"}, {"ordenado", "por"}, {"ordenados", "por"}, {"ordenar", "por"}} {
			if grainPrefix(words[i:], marker) {
				if i > 0 && (words[i-1] == "not" || words[i-1] == "no" || words[i-1] == "sin") {
					continue
				}
				start, width = i, len(marker)
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		if len(words) >= 2 {
			marker := words[len(words)-2]
			if marker == "limit" || marker == "limite" || marker == "límite" {
				n, err := strconv.Atoi(words[len(words)-1])
				if err != nil || n < 1 || n > 100000 {
					return nil, a, analyticalUnsupported("analytical_intent_unsupported")
				}
				if len(words) > 0 && (words[0] == "top" || words[0] == "bottom" || words[0] == "mayores" || words[0] == "menores") {
					return nil, a, analyticalUnsupported("analytical_intent_unsupported")
				}
				a.route.Request.Question = strings.Join(words[:len(words)-2], " ")
				return &exec.AnalyticalIntent{Policy: exec.AnalyticalIntentPolicy, Limit: n}, a, nil
			}
		}
		// A top/bottom prefix has a unique selected metric; never guess among KPIs.
		if len(words) < 3 || (words[0] != "top" && words[0] != "bottom" && words[0] != "mayores" && words[0] != "menores") {
			return nil, a, nil
		}
		n, err := strconv.Atoi(words[1])
		if err != nil || n < 1 || n > 100000 {
			return nil, a, analyticalUnsupported("analytical_intent_unsupported")
		}
		metrics := map[string]bool{}
		for _, ch := range choices {
			if ch.order.Metric != "" {
				metrics[ch.order.Metric] = true
			}
		}
		if len(metrics) != 1 {
			return nil, a, analyticalUnsupported("analytical_intent_unsupported")
		}
		var metric string
		for id := range metrics {
			metric = id
		}
		descending := words[0] == "top" || words[0] == "mayores"
		a.route.Request.Question = strings.Join(words[2:], " ")
		return &exec.AnalyticalIntent{Policy: exec.AnalyticalIntentPolicy, Order: []exec.AnalyticalOrder{{Metric: metric, Descending: descending, Nulls: "last"}}, Limit: n}, a, nil
	}
	if len(words) > 0 && (words[0] == "top" || words[0] == "bottom" || words[0] == "mayores" || words[0] == "menores") {
		return nil, a, analyticalUnsupported("analytical_intent_unsupported")
	}
	p := &exec.AnalyticalIntent{Policy: exec.AnalyticalIntentPolicy}
	fail := func() (*exec.AnalyticalIntent, admission, error) {
		return nil, a, analyticalUnsupported("analytical_intent_unsupported")
	}
	position := start + width
	for position < len(words) {
		matches := map[string]choice{}
		longest := 0
		for _, ch := range choices {
			if len(ch.label) >= longest && grainPrefix(words[position:], ch.label) {
				if len(ch.label) > longest {
					matches = map[string]choice{}
					longest = len(ch.label)
				}
				matches[exec.Hash([]any{ch.order.Metric, ch.order.Column, ch.order.Bucket})] = ch
			}
		}
		if len(matches) != 1 {
			return fail()
		}
		var o exec.AnalyticalOrder
		for _, ch := range matches {
			o = ch.order
		}
		position += longest
		if position < len(words) {
			switch words[position] {
			case "asc", "ascending", "ascendente":
				position++
			case "desc", "descending", "descendente":
				o.Descending = true
				position++
			}
		}
		o.Nulls = "last"
		if o.Descending {
			o.Nulls = "first"
		}
		switch a.binding.Dialect {
		case "mysql", "sqlserver", "bigquery", "databricks":
			o.Nulls = "first"
			if o.Descending {
				o.Nulls = "last"
			}
		}
		if position+1 < len(words) && (words[position] == "nulls" || words[position] == "nulos") {
			switch words[position+1] {
			case "first", "primero", "primeros":
				o.Nulls = "first"
			case "last", "ultimo", "último", "ultimos", "últimos":
				o.Nulls = "last"
			default:
				return fail()
			}
			position += 2
		}
		for _, old := range p.Order {
			if exec.Hash([]any{old.Metric, old.Column, old.Bucket}) == exec.Hash([]any{o.Metric, o.Column, o.Bucket}) {
				return fail()
			}
		}
		p.Order = append(p.Order, o)
		if len(p.Order) > 16 {
			return nil, a, exec.ErrLimit
		}
		if position == len(words) {
			break
		}
		if words[position] == "limit" || words[position] == "limite" || words[position] == "límite" {
			if position+2 != len(words) {
				return fail()
			}
			n, err := strconv.Atoi(words[position+1])
			if err != nil || n < 1 || n > 100000 {
				return fail()
			}
			p.Limit = n
			position += 2
			break
		}
		if words[position] != "and" && words[position] != "y" && words[position] != "," && words[position] != "then" {
			return fail()
		}
		position++
	}
	if len(p.Order) == 0 {
		return fail()
	}
	// RouteResult/Request are values; copying them leaves all shared slices intact.
	a.route.Request.Question = strings.Join(words[:start], " ")
	return p, a, nil
}

func analyticalIntentGuidance(c *exec.AnalyticalContract) string {
	if c == nil || c.Intent == nil {
		return ""
	}
	var b strings.Builder
	if c.Intent.Order != nil {
		b.WriteString(" Required reviewed ordering (in priority order):")
	} else {
		b.WriteString(" Reviewed row limit; ordering is unspecified.")
	}
	for _, o := range c.Intent.Order {
		direction := "ASC"
		if o.Descending {
			direction = "DESC"
		}
		target := o.Metric
		if target == "" {
			target = o.Column
		}
		if o.Bucket != nil {
			target = o.Bucket.Grain + "(" + o.Bucket.Column + ") timezone " + o.Bucket.Timezone
		}
		fmt.Fprintf(&b, " %s %s NULLS %s;", target, direction, strings.ToUpper(o.Nulls))
	}
	if c.Intent.Limit > 0 {
		fmt.Fprintf(&b, " Return at most %d rows, with no offset or ties expansion.", c.Intent.Limit)
	} else {
		b.WriteString(" Do not add LIMIT or OFFSET.")
	}
	return b.String()
}
