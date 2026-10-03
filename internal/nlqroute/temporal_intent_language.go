package nlqroute

import (
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"strings"
	"time"
)

// calendarYearConnector recognizes a bounded calendar-year phrase. Qualifiers
// describe the reviewed calendar, never an arbitrary timezone or fiscal basis.
func calendarYearConnector(words []string, at int, locale nlq.Language) bool {
	if at <= 0 {
		return false
	}
	if yearConnectorForLocale(words[at-1], locale) {
		return true
	}
	var phrases [][]string
	if locale == nlq.LanguageEnglish {
		phrases = [][]string{{"in", "local"}, {"during", "local"}, {"in", "calendar"}, {"during", "calendar"}, {"in", "local", "calendar"}, {"during", "local", "calendar"}, {"in", "calendar", "year"}, {"during", "calendar", "year"}, {"in", "local", "calendar", "year"}, {"during", "local", "calendar", "year"}}
	} else if locale == nlq.LanguageSpanish {
		phrases = [][]string{{"en", "el", "año"}, {"en", "el", "ano"}, {"en", "el", "año", "calendario"}, {"en", "el", "año", "calendario", "local"}, {"durante", "el", "año", "calendario", "local"}, {"por", "mes", "local", "de"}, {"por", "trimestre", "local", "de"}}
	}
	for _, p := range phrases {
		if at < len(p) {
			continue
		}
		match := true
		for j, word := range p {
			if words[at-len(p)+j] != word {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// requestedMetricFacts uses only explicit current catalog roots and aggregate
// inputs. Filter, relationship and period dependencies are not aggregate facts.
func explicitMetricRoots(in RouteRequest, admitted []admittedTopic) map[string]map[semantics.Reference]bool {
	out := map[string]map[semantics.Reference]bool{}
	add := func(topic string, ref semantics.Reference) {
		if out[topic] == nil {
			out[topic] = map[semantics.Reference]bool{}
		}
		out[topic][ref] = true
	}
	for _, ref := range in.References {
		if ref.Kind != semantics.KindMeasure && ref.Kind != semantics.KindKPI {
			continue
		}
		for _, item := range admitted {
			if _, _, ok := catalogReference(item.publication.Definition, ref); ok {
				add(item.id, ref)
			}
		}
	}
	for _, id := range in.MetricIDs {
		count := 0
		topic := ""
		var found semantics.Reference
		for _, item := range admitted {
			for _, kind := range []semantics.Kind{semantics.KindMeasure, semantics.KindKPI} {
				ref := semantics.Reference{Kind: kind, ID: id}
				if _, _, ok := catalogReference(item.publication.Definition, ref); ok {
					count++
					topic = item.id
					found = ref
				}
			}
		}
		if count != 1 {
			return nil
		}
		add(topic, found)
	}
	return out
}
func requestedMetricFacts(in RouteRequest, admitted []admittedTopic) map[string]bool {
	out := map[string]bool{}
	roots := explicitMetricRoots(in, admitted)
	for _, item := range admitted {
		seen := map[semantics.Reference]bool{}
		var visit func(semantics.Reference, int) bool
		visit = func(ref semantics.Reference, depth int) bool {
			if depth > 32 {
				return false
			}
			if seen[ref] {
				return true
			}
			seen[ref] = true
			if ref.Kind == semantics.KindMeasure {
				for _, m := range item.publication.Definition.Measures {
					if m.ID == ref.ID {
						out[item.id+"\x00"+m.Field.Dataset] = true
						return true
					}
				}
			}
			if ref.Kind == semantics.KindKPI {
				for _, k := range item.publication.Definition.KPIs {
					if k.ID == ref.ID {
						for _, r := range k.Inputs {
							if !visit(r, depth+1) {
								return false
							}
						}
						return true
					}
				}
			}
			return false
		}
		for ref := range roots[item.id] {
			if !visit(ref, 0) {
				return nil
			}
		}
	}
	return out
}
func requestedPeriodDimensions(in RouteRequest, admitted []admittedTopic) map[string]bool {
	out := map[string]bool{}
	roots := explicitMetricRoots(in, admitted)
	for _, item := range admitted {
		for _, k := range item.publication.Definition.KPIs {
			if !roots[item.id][semantics.Reference{Kind: semantics.KindKPI, ID: k.ID}] || k.Periods == nil {
				continue
			}
			for _, b := range k.Periods.Bindings {
				out[item.id+"\x00"+b.Dimension.ID] = true
			}
		}
	}
	return out
}

// commonMappedYear permits repeated identical calendar-year clauses only when
// the caller already selected reviewed per-metric period mappings. Distinct
// years or additional named/relative periods cannot collapse into one interval.
func commonMappedYear(question string, locale nlq.Language) (parsedSpan, bool) {
	words := strings.Fields(question)
	year := ""
	count := 0
	for i, w := range words {
		if numericYear(w) {
			if !calendarYearConnector(words, i, locale) || year != "" && year != w {
				return parsedSpan{}, false
			}
			year = w
			count++
			continue
		}
		switch w {
		case "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre", "last", "previous", "pasado", "anterior", "yesterday", "ayer":
			return parsedSpan{}, false
		}
	}
	if count < 2 {
		return parsedSpan{}, false
	}
	connector := "in "
	if locale == nlq.LanguageSpanish {
		connector = "en "
	}
	span, ok, err := temporalSpan(connector+year, locale, time.Time{})
	return span, ok && err == nil
}

func explicitCalendarGroupingForms(locale nlq.Language) map[semantics.TimeGrain][]string {
	var forms map[semantics.TimeGrain][]string
	if locale == nlq.LanguageSpanish {
		forms = map[semantics.TimeGrain][]string{semantics.GrainMonth: {"por mes", "por mes local", "por mes calendario"}, semantics.GrainQuarter: {"por trimestre", "por trimestre local"}, semantics.GrainYear: {"por año", "por ano"}}
	} else {
		forms = map[semantics.TimeGrain][]string{semantics.GrainMonth: {"by month", "per month", "by calendar month", "by local calendar month"}, semantics.GrainQuarter: {"by quarter", "per quarter", "by calendar quarter", "by local calendar quarter"}, semantics.GrainYear: {"by year", "per year", "by calendar year", "by local calendar year"}}
	}
	return forms
}

func explicitCalendarGrouping(question string, locale nlq.Language) semantics.TimeGrain {
	var found semantics.TimeGrain
	for grain, phrases := range explicitCalendarGroupingForms(locale) {
		for _, phrase := range phrases {
			if containsPhrase(question, phrase) {
				if found != "" && found != grain {
					return ""
				}
				found = grain
			}
		}
	}
	return found
}

func reviewedCalendarGrouping(in RouteRequest, admitted []admittedTopic) semantics.TimeGrain {
	// Mask before quote handling: a quote inside a private value is not public
	// grouping syntax. The shared inference lexer reserves marker boundaries.
	in.Question = unquotedIntent(groundedQuestion(in, admitted))
	q := questionInferenceSurface(in, admitted).text
	for _, item := range admitted {
		labels := []string{}
		for _, m := range item.publication.Definition.Measures {
			labels = append(labels, m.Name)
			labels = append(labels, m.Aliases...)
		}
		for _, k := range item.publication.Definition.KPIs {
			labels = append(labels, k.Name)
			labels = append(labels, k.Aliases...)
		}
		for _, label := range labels {
			n := normalizedPhrase(label)
			if explicitCalendarGrouping(n, in.Locale) != "" && containsPhrase(q, n) {
				q = strings.ReplaceAll(q, n, " ")
			}
		}
	}
	for _, neg := range []string{"not by", "not per", "without grouping", "no por", "sin agrupar", "no agrupes"} {
		if containsPhrase(q, neg) {
			return ""
		}
	}
	return explicitCalendarGrouping(q, in.Locale)
}
