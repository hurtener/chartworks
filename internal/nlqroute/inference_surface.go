package nlqroute

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

// The sentinel is deliberately not a word and can never match a catalog label.
// It occupies exactly one token, so persisted markers do not change polarity.
const opaqueInferenceToken = "\ufffc"

type inferenceToken struct {
	text       string
	start, end int
}

type inferenceSurface struct {
	text            string
	words, original []string
	origins         []int
	// covered distinguishes a wholly private token from a public token damaged
	// by a substring mask. Partial overlap cannot silently discard a constraint.
	covered, touched []bool
}

// Sensitive spellings are masking data, never applicability or source proof.
// Canonicalization is pure and bounded against the current admitted slot.
func inferenceRedactions(in RouteRequest, admitted []admittedTopic) ([]semantics.ClarificationAnswer, []semantics.ClarificationResolution) {
	answers := semantics.CloneClarificationAnswers(in.Answers)
	var redactions []semantics.ClarificationResolution
	for _, item := range admitted {
		for _, pattern := range item.rules.Definition.Patterns {
			for _, slot := range pattern.Slots {
				if slot.Sensitivity != semantics.LiteralSensitive {
					continue
				}
				redactions = append(redactions, semantics.ClarificationResolution{Topic: item.id, Pattern: pattern.ID, Slot: slot.ID, Sensitivity: slot.Sensitivity})
				if slot.Default != nil {
					answers = append(answers, semantics.ClarificationAnswer{Topic: item.id, Pattern: pattern.ID, Slot: slot.ID, Value: slot.Default})
				}
				for _, answer := range answers {
					if answer.Topic != item.id || answer.Pattern != pattern.ID || answer.Slot != slot.ID || answer.Value == nil {
						continue
					}
					if canonical, err := semantics.ResolveClarificationValue(slot, *answer.Value, string(in.Locale)); err == nil {
						canonical.Topic, canonical.Pattern, canonical.Slot = item.id, pattern.ID, slot.ID
						redactions = append(redactions, canonical)
					}
				}
				if slot.Effect != nil {
					for _, value := range slot.Effect.Values {
						redactions = append(redactions, semantics.ClarificationResolution{Value: value.Canonical, Sensitivity: semantics.LiteralSensitive})
						for _, alias := range value.Aliases {
							redactions = append(redactions, semantics.ClarificationResolution{Value: alias, Sensitivity: semantics.LiteralSensitive})
						}
					}
				}
			}
		}
	}
	return answers, redactions
}

func inferenceLex(text string) []inferenceToken {
	var out []inferenceToken
	for i := 0; i < len(text); {
		if len(text)-i >= len("[redacted answer]") && strings.EqualFold(text[i:i+len("[redacted answer]")], "[redacted answer]") {
			out = append(out, inferenceToken{opaqueInferenceToken, i, i + len("[redacted answer]")})
			i += len("[redacted answer]")
			continue
		}
		r, n := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			i += n
			continue
		}
		start := i
		for i < len(text) {
			r, n = utf8.DecodeRuneInString(text[i:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			i += n
		}
		out = append(out, inferenceToken{strings.ToLower(text[start:i]), start, i})
	}
	return out
}

func questionInferenceSurface(in RouteRequest, admitted []admittedTopic) inferenceSurface {
	answers, redactions := inferenceRedactions(in, admitted)
	ranges := semantics.ClarificationTextRedactions(in.Question, answers, redactions)
	original := inferenceLex(in.Question)
	covered, touched := make([]bool, len(original)), make([]bool, len(original))
	for i, token := range original {
		for _, span := range ranges {
			if token.start < span[1] && token.end > span[0] {
				touched[i] = true
			}
			if span[0] <= token.start && span[1] >= token.end {
				covered[i] = true
			}
		}
	}
	// Expand only to touching lexical tokens. Unicode whitespace and punctuation
	// delimit independent public words, even beside a persisted marker.
	for i := range ranges {
		for _, token := range original {
			if token.start < ranges[i][1] && token.end > ranges[i][0] {
				ranges[i][0] = min(ranges[i][0], token.start)
				ranges[i][1] = max(ranges[i][1], token.end)
			}
		}
		for ranges[i][0] > 0 {
			r, n := utf8.DecodeLastRuneInString(in.Question[:ranges[i][0]])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			ranges[i][0] -= n
		}
		for ranges[i][1] < len(in.Question) {
			r, n := utf8.DecodeRuneInString(in.Question[ranges[i][1]:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			ranges[i][1] += n
		}
	}
	var merged [][2]int
	for _, span := range ranges {
		if len(merged) > 0 && span[0] <= merged[len(merged)-1][1] {
			merged[len(merged)-1][1] = max(merged[len(merged)-1][1], span[1])
		} else {
			merged = append(merged, span)
		}
	}
	out := inferenceSurface{covered: covered, touched: touched}
	for _, token := range original {
		out.original = append(out.original, token.text)
	}
	at, emitted := 0, false
	emit := func() { out.words = append(out.words, opaqueInferenceToken); out.origins = append(out.origins, -1) }
	for i, token := range original {
		for at < len(merged) && merged[at][1] <= token.start {
			if !emitted {
				emit()
			}
			at++
			emitted = false
		}
		if at < len(merged) && token.start < merged[at][1] && token.end > merged[at][0] {
			if !emitted {
				emit()
				emitted = true
			}
			continue
		}
		out.words = append(out.words, token.text)
		out.origins = append(out.origins, i)
	}
	for at < len(merged) {
		if !emitted {
			emit()
		}
		at++
		emitted = false
	}
	out.text = strings.Join(out.words, " ")
	return out
}

func (s inferenceSurface) negatedAt(start, window int, original bool) bool {
	words := s.words
	if original {
		start = s.origins[start]
		words = s.original
	}
	for i := max(0, start-window); i < start; i++ {
		if temporalNegator(words[i]) {
			return true
		}
	}
	return false
}

func (s inferenceSurface) stablePhrasePolarity(phrase string) bool {
	words := strings.Fields(phrase)
	for start := 0; start+len(words) <= len(s.words); start++ {
		if strings.Join(s.words[start:start+len(words)], " ") != phrase {
			continue
		}
		if phraseNegatedAt(s.words, start) != phraseNegatedAt(s.original, s.origins[start]) {
			return false
		}
	}
	return true
}

func protectedMeaningFailure(locale nlq.Language) error {
	return clarificationFailure(locale, "question", "ambiguous_protected_meaning")
}

// Only surviving public temporal/grouping words can make a changed polarity
// material. A wholly private period disappearing does not require a refusal.
func (s inferenceSurface) stableTemporalPolarity() bool {
	for i, word := range s.words {
		if s.origins[i] < 0 {
			continue
		}
		switch word {
		case "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december",
			"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
			"last", "this", "este", "ultimo", "último", "mes", "año", "ano", "trimestre", "by", "per", "por", "monthly", "quarterly", "yearly", "mensual", "trimestral", "anual", "agrupar":
		default:
			if !numericYear(word) {
				continue
			}
		}
		if s.negatedAt(i, 3, false) != s.negatedAt(i, 3, true) {
			return false
		}
	}
	return true
}

// A reviewed phrase may disappear only when every word was wholly private.
// Partial-token or mixed-public/private phrases need bounded clarification.
func (s inferenceSurface) damagedPhrase(phrase string) bool {
	if phrase == "" || strings.Contains(phrase, opaqueInferenceToken) {
		return false
	}
	width := len(strings.Fields(phrase))
	for start := 0; start+width <= len(s.original); start++ {
		if strings.Join(s.original[start:start+width], " ") != phrase {
			continue
		}
		touched, private := false, true
		for i := start; i < start+width; i++ {
			touched = touched || s.touched[i]
			private = private && s.covered[i]
		}
		if touched && !private {
			return true
		}
	}
	return false
}

// This pre-provider check applies equally to learned and deterministic selection.
// Catalog labels remain data; a private spelling cannot negate or mutilate one
// surviving public concept and then ask a model to repair its meaning.
func protectedCatalogMeaning(in RouteRequest, admitted []admittedTopic) error {
	surface := questionInferenceSurface(in, admitted)
	check := func(name string, aliases []string) bool {
		for _, label := range append([]string{name}, aliases...) {
			phrase := normalizedPhrase(label)
			if surface.damagedPhrase(phrase) || !surface.stableCatalogPhrasePolarity(phrase) {
				return false
			}
		}
		return true
	}
	for _, item := range admitted {
		for _, m := range item.publication.Definition.Measures {
			if !check(m.Name, m.Aliases) {
				return protectedMeaningFailure(in.Locale)
			}
		}
		for _, k := range item.publication.Definition.KPIs {
			if !check(k.Name, k.Aliases) {
				return protectedMeaningFailure(in.Locale)
			}
		}
		for _, d := range item.publication.Definition.Dimensions {
			if !check(d.Name, d.Aliases) {
				return protectedMeaningFailure(in.Locale)
			}
		}
	}
	return nil
}

// temporalComparisonQuestion preserves the original grammar of a MIXED calendar
// expression but hides wholly private expressions. Its output is used only for
// deterministic comparison, never inference authority, persistence, or a model.
// The same temporal parser consumes both projections; proximity alone is not a
// substitute for its prefix, range, relative-period and year grammar.
func (s inferenceSurface) temporalComparisonQuestion(locale nlq.Language) string {
	words := s.original
	restore := make([]bool, len(words))
	month := func(w string) bool {
		switch w {
		case "january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december",
			"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre":
			return true
		}
		return false
	}
	// A prefix is included only if the parser assigns it temporal polarity.
	prefix := func(start int) int {
		if start > 0 && temporalNegator(words[start-1]) {
			return start - 1
		}
		if start > 0 {
			switch words[start-1] {
			case "in", "during", "en", "durante", "from", "de":
				if start > 1 && temporalNegator(words[start-2]) {
					return start - 2
				}
				return start - 1
			}
		}
		return start
	}
	endOfMonth := func(start int) int {
		end := start + 1
		if end < len(words) && namedMonthConnector(words[end]) {
			end++
		}
		if end < len(words) && numericYear(words[end]) {
			end++
		}
		return end
	}
	retain := func(coreStart, coreEnd, start, end int) {
		public := false
		for i := coreStart; i < coreEnd; i++ {
			public = public || !s.covered[i]
		}
		if !public {
			return
		}
		for i := start; i < end; i++ {
			restore[i] = true
		}
	}
	for i, word := range words {
		if month(word) && !(locale == nlq.LanguageEnglish && i == 0 && word == "may" && len(words) > 1 && words[1] == "i") {
			end := endOfMonth(i)
			retain(i, end, prefix(i), end)
		}
		if rangeStartForLocale(word, locale) && i+3 < len(words) && month(words[i+1]) && rangeJoinForLocale(words[i+2], locale) && month(words[i+3]) {
			start, end := i, endOfMonth(i+3)
			if negatedTemporalRange(words, i) {
				for j := i - 1; j >= 0 && j >= i-4; j-- {
					if temporalNegator(words[j]) {
						start = j
						break
					}
				}
			}
			retain(i+1, end, start, end)
		}
		if numericYear(word) && i > 0 && yearConnectorForLocale(words[i-1], locale) {
			retain(i, i+1, prefix(i), i+1)
		}
	}
	for _, phrase := range []string{"last month", "this month", "mes pasado", "ultimo mes", "último mes", "este mes", "this year", "este año", "este ano", "last year", "año pasado", "ano pasado", "this quarter", "este trimestre", "last quarter", "trimestre pasado", "último trimestre", "ultimo trimestre"} {
		width := len(strings.Fields(phrase))
		for i := 0; i+width <= len(words); i++ {
			if strings.Join(words[i:i+width], " ") != phrase {
				continue
			}
			retain(i, i+width, prefix(i), i+width)
			// Continuation reads a bounded negation window, not arbitrary preceding
			// calendar operands. Restore only its negators; never resurrect a wholly
			// private earlier month/year merely because it is nearby.
			public := false
			for j := i; j < i+width; j++ {
				public = public || !s.covered[j]
			}
			if public {
				for j := max(0, i-3); j < i; j++ {
					if temporalNegator(words[j]) {
						restore[j] = true
					}
				}
			}
		}
	}
	changed := false
	for i := range words {
		changed = changed || restore[i] && s.covered[i] || s.touched[i] && !s.covered[i]
	}
	if !changed {
		return s.text
	}
	var projected []string
	hidden := false
	for i, word := range words {
		if word == opaqueInferenceToken {
			projected = append(projected, word)
			hidden = false
			continue
		}
		if s.covered[i] && !restore[i] {
			if !hidden {
				projected = append(projected, opaqueInferenceToken)
			}
			hidden = true
			continue
		}
		projected = append(projected, word)
		hidden = false
	}
	return strings.Join(projected, " ")
}

func catalogNegatedAt(words []string, start int) bool {
	negated := start > 0 && temporalNegator(words[start-1])
	if start > 1 && (words[start-1] == "by" || words[start-1] == "por") {
		negated = negated || temporalNegator(words[start-2])
	}
	return negated
}

// Match the catalog consumer's immediate-prefix grammar at each surviving
// occurrence. A private earlier occurrence is never used as this one's prefix.
func (s inferenceSurface) stableCatalogPhrasePolarity(phrase string) bool {
	if phrase == "" || strings.Contains(phrase, opaqueInferenceToken) {
		return true
	}
	width := len(strings.Fields(phrase))
	for start := 0; start+width <= len(s.words); start++ {
		if strings.Join(s.words[start:start+width], " ") != phrase {
			continue
		}
		if catalogNegatedAt(s.words, start) != catalogNegatedAt(s.original, s.origins[start]) {
			return false
		}
	}
	return true
}

func protectedCalendarGroupingMeaning(in RouteRequest, admitted []admittedTopic) error {
	surface := questionInferenceSurface(in, admitted)
	// requestedGroupingGrain uses phraseNegatedAt's three-token grammar, unlike
	// catalog terms' immediate-prefix grammar. Keep their contracts separate.
	for _, phrase := range requestedGroupingPhrases() {
		if surface.damagedPhrase(phrase) || !surface.stablePhrasePolarity(phrase) {
			return protectedMeaningFailure(in.Locale)
		}
	}
	for _, phrases := range explicitCalendarGroupingForms(in.Locale) {
		for _, phrase := range phrases {
			if surface.damagedPhrase(phrase) || !surface.stableCatalogPhrasePolarity(phrase) {
				return protectedMeaningFailure(in.Locale)
			}
		}
	}
	return nil
}

// The closure owns detached, read-only masking inputs. It is never serialized or
// printed as data, and cannot establish applicability or source permissions.
func protectedTextRedactor(answers []semantics.ClarificationAnswer, resolutions []semantics.ClarificationResolution) func(string, []semantics.ClarificationAnswer) string {
	base := semantics.CloneClarificationAnswers(answers)
	masks := semantics.CloneClarificationResolutions(resolutions)
	return func(text string, extra []semantics.ClarificationAnswer) string {
		all := append(semantics.CloneClarificationAnswers(base), semantics.CloneClarificationAnswers(extra)...)
		return semantics.RedactClarificationText(text, all, masks)
	}
}

// RedactProtectedText shares the admitted inference mask with downstream input
// storage. A deserialized route has only its canonical resolved-value mask.
func (r RouteResult) RedactProtectedText(text string, answers []semantics.ClarificationAnswer) string {
	if r.protectedRedact != nil {
		return r.protectedRedact(text, answers)
	}
	return semantics.RedactClarificationText(text, answers, r.Resolutions)
}
