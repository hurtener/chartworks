package reporting

import (
	"fmt"
	"strconv"
	"strings"
)

// PeriodFinding is deterministic certification evidence for a mismatch between
// localized authored wording and an exact relative-period default.
type PeriodFinding struct {
	Locale    string `json:"locale"`
	Parameter string `json:"parameter"`
	Code      string `json:"code"`
	Question  string `json:"question"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Digest    string `json:"digest"`
}

// PeriodReview is an explicit reviewer disposition bound to one finding digest.
type PeriodReview struct {
	Finding     string `json:"finding"`
	Disposition string `json:"disposition" jsonschema:"enum=accepted_exception"`
}

type periodMention struct {
	mode, unit string
	count      int
}

func canonicalPeriod(p Period) string {
	return fmt.Sprintf("%s:%s:%d:%s:%s:%s:%s:%s:%s", p.Mode, p.Unit, p.Count, p.Start, p.End, p.FromDate, p.FirstOccurrence, p.DSTPolicy, p.MonthPolicy)
}

func periodWords(locale, question string) []periodMention {
	words := strings.Fields(normalizeQuestion(question))
	if len(words) > 128 {
		return nil
	}
	unit := func(s string) string {
		switch s {
		case "hour", "hours", "hora", "horas":
			return "hour"
		case "day", "days", "día", "días", "dia", "dias":
			return "day"
		case "week", "weeks", "semana", "semanas":
			return "week"
		case "month", "months", "mes", "meses":
			return "month"
		case "quarter", "quarters", "trimestre", "trimestres":
			return "quarter"
		case "year", "years", "año", "años", "ano", "anos":
			return "year"
		}
		return ""
	}
	mentions := []periodMention{}
	for i, word := range words {
		// Current-period wording is recognized explicitly even though the governed
		// relative-period union has no current mode. Certification must surface it.
		if (word == "this" || word == "current" || word == "este" || word == "esta" || word == "estos" || word == "estas") && i+1 < len(words) {
			if u := unit(words[i+1]); u != "" {
				mentions = append(mentions, periodMention{mode: "current", unit: u, count: 1})
				continue
			}
		}
		if u := unit(word); u != "" && i+1 < len(words) {
			switch words[i+1] {
			case "actual":
				mentions = append(mentions, periodMention{mode: "current", unit: u, count: 1})
				continue
			case "pasado", "pasada":
				// Postposed singular Spanish means the previous completed period.
				mentions = append(mentions, periodMention{mode: "previous", unit: u, count: 1})
				continue
			}
		}
		mode := ""
		switch word {
		case "last", "previous", "último", "última", "últimos", "últimas", "anterior", "anteriores":
			mode = "previous"
		case "past", "rolling", "pasados", "pasadas", "móviles", "moviles":
			mode = "rolling"
		}
		if mode == "" {
			continue
		}
		count, u := 1, ""
		if i+1 < len(words) {
			if n, err := strconv.Atoi(words[i+1]); err == nil && n > 0 && n <= 10000 {
				count = n
				if i+2 < len(words) {
					u = unit(words[i+2])
				}
			} else {
				u = unit(words[i+1])
			}
		}
		if u == "" && i > 0 {
			u = unit(words[i-1])
		}
		if u != "" {
			mentions = append(mentions, periodMention{mode: mode, unit: u, count: count})
		}
	}
	_ = locale // locale is retained in evidence; accepted vocabulary is bilingual.
	return mentions
}

func periodFindings(d Definition) []PeriodFinding {
	defaults := []struct {
		name   string
		period Period
	}{}
	for _, p := range d.Parameters {
		if p.Type == "relative_period" && p.Default != nil && p.Default.Period != nil {
			defaults = append(defaults, struct {
				name   string
				period Period
			}{p.Name, *p.Default.Period})
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	out := []PeriodFinding{}
	for _, localized := range d.Metadata {
		mentions := periodWords(localized.Locale, localized.Question)
		if len(mentions) == 0 {
			continue
		}
		for _, target := range defaults {
			expected := canonicalPeriod(target.period)
			code, observed := "", ""
			if len(mentions) != 1 || len(defaults) != 1 {
				code, observed = "ambiguous_period_wording", digest(mentions)
			} else {
				observed = fmt.Sprintf("%s:%s:%d", mentions[0].mode, mentions[0].unit, mentions[0].count)
				if mentions[0].mode == "current" {
					code = "unsupported_period_wording"
				} else if mentions[0].mode != target.period.Mode || mentions[0].unit != target.period.Unit || mentions[0].count != target.period.Count {
					code = "period_wording_mismatch"
				}
			}
			if code == "" {
				continue
			}
			f := PeriodFinding{Locale: localized.Locale, Parameter: target.name, Code: code, Question: localized.Question, Expected: expected, Observed: observed}
			f.Digest = digest(struct {
				Version string
				Finding PeriodFinding
			}{"period-language-review-v1", f})
			out = append(out, f)
		}
	}
	return out
}

func validatePeriodReviews(findings []PeriodFinding, reviews []PeriodReview) error {
	if len(findings) != len(reviews) {
		return ErrInvalid
	}
	wanted := map[string]bool{}
	for _, f := range findings {
		wanted[f.Digest] = true
	}
	for _, r := range reviews {
		if !wanted[r.Finding] || r.Disposition != "accepted_exception" {
			return ErrInvalid
		}
		delete(wanted, r.Finding)
	}
	if len(wanted) != 0 {
		return ErrInvalid
	}
	return nil
}
