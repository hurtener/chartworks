package reporting

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

func validQuestionIntent(in *QuestionIntent) bool {
	if in == nil {
		return true
	}
	if len(in.Metrics) == 0 || len(in.Metrics) > 16 || len(in.Filters) > 16 || !text(in.Grain, 128) || in.Population != "" && !identity.Identifier(in.Population) {
		return false
	}
	seen := map[string]bool{}
	for _, metric := range in.Metrics {
		key := normalizeQuestion(metric)
		if !identity.Identifier(metric) || key == "" || seen[key] {
			return false
		}
		seen[key] = true
	}
	if in.Grain != "" && !slices.Contains([]string{"second", "minute", "hour", "day", "week", "month", "quarter", "year"}, in.Grain) {
		return false
	}
	for _, filter := range in.Filters {
		if !identity.Identifier(filter.Dimension) || !slices.Contains([]string{"eq", "neq", "lt", "lte", "gt", "gte", "in", "not_in", "is_null", "not_null"}, filter.Operator) || filter.Value != "" && !identity.Identifier(filter.Value) {
			return false
		}
		if (filter.Operator == "is_null" || filter.Operator == "not_null") != (filter.Value == "") {
			return false
		}
		key := normalizeQuestion(filter.Dimension) + "\x00" + strings.ToLower(filter.Operator) + "\x00" + normalizeQuestion(filter.Value)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	if in.Period != nil {
		if !slices.Contains([]string{"previous", "rolling", "explicit", "from_date", "schedule_window"}, in.Period.Mode) {
			return false
		}
		if in.Period.Mode == "explicit" {
			start, startErr := time.Parse(time.DateOnly, in.Period.Start)
			end, endErr := time.Parse(time.DateOnly, in.Period.End)
			if startErr != nil || endErr != nil || !start.Before(end) || in.Period.Unit != "" || in.Period.Count != 0 {
				return false
			}
		} else if !slices.Contains([]string{"hour", "day", "week", "month", "quarter", "year"}, in.Period.Unit) || in.Period.Count < 1 || in.Period.Count > 10000 || in.Period.Start != "" || in.Period.End != "" {
			return false
		}
	}
	return true
}

type normalizedIntent struct {
	Metrics    []string
	Grain      string
	Population string
	Filters    []IntentFilter
	Period     *IntentPeriod
}

func normalizeIntent(in QuestionIntent) normalizedIntent {
	out := normalizedIntent{Metrics: clone(in.Metrics), Grain: strings.ToLower(strings.TrimSpace(in.Grain)), Population: normalizeQuestion(in.Population), Filters: clone(in.Filters), Period: clone(in.Period)}
	for i := range out.Metrics {
		out.Metrics[i] = normalizeQuestion(out.Metrics[i])
	}
	sort.Strings(out.Metrics)
	for i := range out.Filters {
		out.Filters[i].Dimension = normalizeQuestion(out.Filters[i].Dimension)
		out.Filters[i].Value = normalizeQuestion(out.Filters[i].Value)
	}
	sort.Slice(out.Filters, func(i, j int) bool {
		return digest(out.Filters[i]) < digest(out.Filters[j])
	})
	return out
}

func semanticQuestionMatch(a, b QuestionIntent) (kind string, score float64, evidence string) {
	left, right := normalizeIntent(a), normalizeIntent(b)
	metricOverlap := 0
	for _, metric := range left.Metrics {
		if slices.Contains(right.Metrics, metric) {
			metricOverlap++
		}
	}
	components := []bool{
		metricOverlap == len(left.Metrics) && metricOverlap == len(right.Metrics),
		left.Grain == right.Grain,
		left.Population == right.Population,
		digest(left.Filters) == digest(right.Filters),
		digest(left.Period) == digest(right.Period),
	}
	equal := 0
	for _, same := range components {
		if same {
			equal++
		}
	}
	score = float64(equal) / float64(len(components))
	switch {
	case metricOverlap == 0:
		kind = "unique"
		score = 0
	case equal == len(components):
		kind = "duplicate"
	default:
		kind = "overlap"
	}
	evidence = digest(struct {
		Version     string
		Left, Right normalizedIntent
		Kind        string
	}{"question-intent-overlap-v1", left, right, kind})
	return
}
