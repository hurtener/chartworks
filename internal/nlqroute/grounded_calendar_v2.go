package nlqroute

import (
	"strings"
	"unicode"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

// GroundedCalendarPolicy opts into a version-bound, deterministic phrase producer.
// Named zones assert current reviewed policy; they never override it.
const GroundedCalendarPolicy = "grounded-calendar-v2"

type calendarPhrase struct {
	text  string
	zone  string
	grain semantics.TimeGrain
}

type calendarProjection struct {
	consumed map[int]bool
	locale   nlq.Language
	text     string
	zone     string
	grouping semantics.TimeGrain
}

func calendarZonePhrases(locale nlq.Language) []calendarPhrase {
	zones := []calendarPhrase{{text: "america new york", zone: "America/New_York"}, {text: "utc", zone: "UTC"}}
	if locale == nlq.LanguageEnglish {
		return append(zones, calendarPhrase{text: "new york", zone: "America/New_York"})
	}
	return append(zones, calendarPhrase{text: "nueva york", zone: "America/New_York"})
}

func calendarYearPhrasesV2(locale nlq.Language) []calendarPhrase {
	var out []calendarPhrase
	for _, connector := range []string{"in", "during"} {
		if locale == nlq.LanguageEnglish {
			out = append(out, calendarPhrase{text: connector})
		}
	}
	for _, connector := range []string{"en", "durante"} {
		if locale == nlq.LanguageSpanish {
			out = append(out, calendarPhrase{text: connector})
		}
	}
	if locale == nlq.LanguageEnglish {
		for _, connector := range []string{"in", "during", "for", "for the", "of"} {
			for _, qualifier := range []string{"local", "calendar", "local calendar", "calendar year", "local calendar year"} {
				out = append(out, calendarPhrase{text: connector + " " + qualifier})
			}
			for _, z := range calendarZonePhrases(locale) {
				out = append(out, calendarPhrase{text: connector + " " + z.text + " calendar year", zone: z.zone})
			}
		}
	} else {
		for _, connector := range []string{"en", "en el", "durante", "durante el", "para", "para el", "del"} {
			for _, qualifier := range []string{"año", "ano", "año local", "ano local", "año calendario", "ano calendario", "año calendario local", "ano calendario local"} {
				out = append(out, calendarPhrase{text: connector + " " + qualifier})
			}
		}
	}
	return out
}

func calendarGroupingPhrasesV2(locale nlq.Language) []calendarPhrase {
	var out []calendarPhrase
	for grain, phrases := range explicitCalendarGroupingForms(locale) {
		for _, phrase := range phrases {
			out = append(out, calendarPhrase{text: phrase, grain: grain})
		}
	}
	for _, z := range calendarZonePhrases(locale) {
		if locale == nlq.LanguageEnglish {
			for _, connector := range []string{"by", "per"} {
				for _, grain := range []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear} {
					for _, qualifier := range []string{"", "calendar ", "booking "} {
						out = append(out, calendarPhrase{text: connector + " " + z.text + " " + qualifier + string(grain), zone: z.zone, grain: grain})
						out = append(out, calendarPhrase{text: connector + " " + qualifier + string(grain) + " in " + z.text, zone: z.zone, grain: grain})
					}
				}
			}
		} else {
			for _, g := range []struct {
				word  string
				grain semantics.TimeGrain
			}{{"mes", semantics.GrainMonth}, {"trimestre", semantics.GrainQuarter}, {"año", semantics.GrainYear}, {"ano", semantics.GrainYear}} {
				for _, qualifier := range []string{"", " de registro", " calendario"} {
					for _, connector := range []string{" en ", " de "} {
						out = append(out, calendarPhrase{text: "por " + g.word + qualifier + connector + z.text, zone: z.zone, grain: g.grain})
					}
				}
			}
		}
	}
	return out
}

func phraseAt(words []string, at int, phrase string) bool {
	parts := strings.Fields(phrase)
	return at >= 0 && at+len(parts) <= len(words) && strings.Join(words[at:at+len(parts)], " ") == phrase
}

func calendarMeaningError(locale nlq.Language, reason string) error {
	prompt := "Clarify one supported calendar, reviewed timezone and grouping for this request."
	if locale == nlq.LanguageSpanish {
		prompt = "Aclara un calendario compatible, una zona horaria revisada y una agrupación para esta consulta."
	}
	return &Clarification{Reason: reason, Outcome: semantics.ClarificationInvalid, Prompt: prompt}
}

// Protection checks use the shared inference surface. Original private text is
// only a damage/polarity comparator, never a producer input or retained evidence.
func protectedGroundedCalendarMeaning(in RouteRequest, admitted []admittedTopic) error {
	surface := questionInferenceSurface(in, admitted)
	phrases := calendarGroupingPhrasesV2(in.Locale)
	years := calendarYearPhrasesV2(in.Locale)
	for at, word := range surface.original {
		if !numericYear(word) {
			continue
		}
		for _, phrase := range years {
			start := at - len(strings.Fields(phrase.text))
			if phraseAt(surface.original, start, phrase.text) {
				text := phrase.text + " " + word
				// A zone suffix belongs to the same public calendar expression.
				for _, connector := range []string{"in", "en", "de"} {
					for _, z := range calendarZonePhrases(in.Locale) {
						suffix := connector + " " + z.text
						if phraseAt(surface.original, at+1, suffix) {
							text += " " + suffix
						}
					}
				}
				phrases = append(phrases, calendarPhrase{text: text})
			}
		}
	}
	// Zone qualifiers following any supported month/year remain one mixed
	// expression even when the whole zone suffix is a private spelling.
	for at, word := range surface.original {
		if at+1 >= len(surface.original) || !calendarPeriodTailV2(word) {
			continue
		}
		if surface.original[at+1] != "in" && surface.original[at+1] != "en" && surface.original[at+1] != "de" {
			continue
		}
		end := min(len(surface.original), at+3)
		for _, z := range calendarZonePhrases(in.Locale) {
			if phraseAt(surface.original, at+2, z.text) {
				end = at + 2 + len(strings.Fields(z.text))
				break
			}
		}
		phrases = append(phrases, calendarPhrase{text: strings.Join(surface.original[at:end], " ")})
	}
	for _, phrase := range phrases {
		if surface.damagedPhrase(phrase.text) || !surface.stablePhrasePolarity(phrase.text) || !stableCalendarPhrasePolarityV2(surface, phrase.text) {
			return protectedMeaningFailure(in.Locale)
		}
	}
	return nil
}

// calendarProjectionV2 canonicalizes only closed grammatical phrases. Every
// numeric year remains visible to the existing bounded period parser, including
// malformed, conflicting or repeated years. No value is inferred from SQL.
func calendarProjectionV2(question string, locale nlq.Language) (calendarProjection, error) {
	out := calendarProjection{text: question, locale: locale, consumed: map[int]bool{}}
	words := strings.Fields(question)
	replaced := append([]string(nil), words...)
	consumeZone := func(start, end int) {
		for _, z := range calendarZonePhrases(locale) {
			for at := start; at+len(strings.Fields(z.text)) <= end; at++ {
				if phraseAt(words, at, z.text) {
					for i := at; i < at+len(strings.Fields(z.text)); i++ {
						out.consumed[i] = true
					}
				}
			}
		}
	}
	mergeZone := func(zone string) error {
		if zone == "" {
			return nil
		}
		if out.zone != "" && out.zone != zone {
			return calendarMeaningError(locale, "ambiguous_temporal_timezone")
		}
		out.zone = zone
		return nil
	}
	for at, word := range words {
		if !calendarPeriodTailV2(word) {
			continue
		}
		if (word == "mes" || word == "trimestre" || word == "año" || word == "ano") && phraseAt(words, at+1, "de registro") {
			continue
		}
		zone, width, zoneErr := calendarZoneSuffixV2(words, at+1, locale, !numericYear(word))
		if zoneErr != nil {
			return out, zoneErr
		}
		if err := mergeZone(zone); err != nil {
			return out, err
		}
		consumeZone(at+1, at+1+width)
		for i := at + 1; i < at+1+width; i++ {
			replaced[i] = ""
		}
	}
	for at, word := range words {
		if !numericYear(word) {
			continue
		}
		start := at
		var best calendarPhrase
		for _, phrase := range calendarYearPhrasesV2(locale) {
			i := at - len(strings.Fields(phrase.text))
			if i < start && phraseAt(words, i, phrase.text) {
				start, best = i, phrase
			}
		}
		if start == at && at > 0 && yearConnectorForLocale(words[at-1], locale) {
			start = at - 1
		}
		if start == at {
			continue
		} // The base parser rejects unanchored years.
		if negatedTemporalRange(words, start) {
			return out, calendarMeaningError(locale, "unsupported_temporal_negation")
		}
		if err := mergeZone(best.zone); err != nil {
			return out, err
		}
		if best.zone != "" {
			consumeZone(start, at)
		}
		connector := "in"
		if locale == nlq.LanguageSpanish {
			connector = "en"
		}
		replaced[start] = connector
		for i := start + 1; i < at; i++ {
			replaced[i] = ""
		}
		// A suffix is a strict zone assertion, never a prefix match that can
		// swallow an offset, an alternative zone, or an unknown named place.
		zone, width, zoneErr := calendarZoneSuffixV2(words, at+1, locale, false)
		if zoneErr != nil {
			return out, zoneErr
		}
		if err := mergeZone(zone); err != nil {
			return out, err
		}
		consumeZone(at+1, at+1+width)
		for i := at + 1; i < at+1+width; i++ {
			replaced[i] = ""
		}
	}
	for at, word := range words {
		if word != "by" && word != "per" && word != "por" {
			continue
		}
		var best calendarPhrase
		for _, phrase := range calendarGroupingPhrasesV2(locale) {
			if len(phrase.text) > len(best.text) && phraseAt(words, at, phrase.text) {
				best = phrase
			}
		}
		if best.text == "" {
			// A named-zone/calendar modifier cannot fall through as scalar intent.
			for i := at + 1; i < len(words) && i <= at+6; i++ {
				if words[i] == "month" || words[i] == "quarter" || words[i] == "year" || words[i] == "mes" || words[i] == "trimestre" || words[i] == "año" || words[i] == "ano" {
					return out, calendarMeaningError(locale, "unsupported_calendar_grouping")
				}
			}
			continue
		}
		if negatedTemporalRange(words, at) {
			return out, calendarMeaningError(locale, "unsupported_grouping_negation")
		}
		if out.grouping != "" && out.grouping != best.grain {
			return out, calendarMeaningError(locale, "ambiguous_temporal_grain")
		}
		end := at + len(strings.Fields(best.text))
		if best.zone != "" && invalidZoneTailV2(words, end) {
			return out, calendarMeaningError(locale, "unsupported_temporal_timezone")
		}
		zone, _, zoneErr := calendarZoneSuffixV2(words, end, locale, true)
		if zoneErr != nil {
			return out, zoneErr
		}
		if err := mergeZone(zone); err != nil {
			return out, err
		}
		if err := mergeZone(best.zone); err != nil {
			return out, err
		}
		if best.zone != "" {
			consumeZone(at, end)
		}
		if zone != "" {
			_, width, _ := calendarZoneSuffixV2(words, end, locale, true)
			consumeZone(end, end+width)
		}
		out.grouping = best.grain
	}
	// Fiscal/offset/DST language is outside this producer, even beside an otherwise
	// recognized Gregorian phrase. The existing early hourly guard still runs.
	for _, word := range words {
		switch word {
		case "fiscal", "dst", "gmt", "est", "edt", "cet", "cest", "timezone", "offset", "horaria":
			return out, calendarMeaningError(locale, "unsupported_calendar")
		}
	}
	out.text = strings.Join(strings.Fields(strings.Join(replaced, " ")), " ")
	return out, nil
}

func (p calendarProjection) validateZone(zone string) error {
	if p.zone != "" && p.zone != zone {
		return calendarMeaningError(p.locale, "temporal_timezone_mismatch")
	}
	return nil
}

func invalidZoneTailV2(words []string, at int) bool {
	if at >= len(words) {
		return false
	}
	if words[at] == "or" || words[at] == "o" || words[at] == "and" || words[at] == "y" {
		return true
	}
	for _, r := range words[at] {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func calendarZoneSuffixV2(words []string, at int, locale nlq.Language, allowYear bool) (string, int, error) {
	if at >= len(words) || words[at] != "in" && words[at] != "en" && words[at] != "de" {
		return "", 0, nil
	}
	if allowYear {
		for _, phrase := range calendarYearPhrasesV2(locale) {
			end := at + len(strings.Fields(phrase.text))
			if end < len(words) && phraseAt(words, at, phrase.text) && numericYear(words[end]) {
				return "", 0, nil
			}
		}
	}
	if allowYear && at+1 < len(words) && (numericYear(words[at+1]) || calendarPeriodTailV2(words[at+1]) || words[at+1] == "last" || words[at+1] == "this" || words[at+1] == "este") {
		return "", 0, nil
	}
	for _, z := range calendarZonePhrases(locale) {
		if phraseAt(words, at+1, z.text) {
			width := 1 + len(strings.Fields(z.text))
			if invalidZoneTailV2(words, at+width) {
				return "", 0, calendarMeaningError(locale, "unsupported_temporal_timezone")
			}
			return z.zone, width, nil
		}
	}
	return "", 0, calendarMeaningError(locale, "unsupported_temporal_timezone")
}

// Catalog names containing grouping grammar are data, as in the original
// reviewed grouping producer. Do not promote a measure title into intent.
func calendarInferenceQuestionV2(question string, locale nlq.Language, admitted []admittedTopic) string {
	words := strings.Fields(question)
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
			for _, phrase := range calendarGroupingPhrasesV2(locale) {
				if !containsPhrase(n, phrase.text) {
					continue
				}
				for at := range words {
					if phraseAt(words, at, n) {
						for i := at; i < at+len(strings.Fields(n)); i++ {
							words[i] = opaqueInferenceToken
						}
					}
				}
				break
			}
		}
	}
	return strings.Join(words, " ")
}

// Quote masking preserves token coordinates. Privacy comes exclusively from the
// shared grounded surface; a private quote cannot open or close public syntax.
func calendarUnquotedQuestionV2(in RouteRequest, admitted []admittedTopic, surface inferenceSurface) (string, error) {
	safe := groundedQuestion(in, admitted)
	tokens := inferenceLex(safe)
	if len(tokens) != len(surface.words) {
		return "", protectedMeaningFailure(in.Locale)
	}
	var masked strings.Builder
	var end rune
	runes := []rune(safe)
	for i, r := range runes {
		if end != 0 {
			masked.WriteString(strings.Repeat(" ", len(string(r))))
			if r == end && (i == 0 || runes[i-1] != '\\') {
				end = 0
			}
			continue
		}
		switch r {
		case '\'':
			if i == 0 || !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) {
				end = '\''
			}
		case '"':
			end = '"'
		case '“':
			end = '”'
		case '‘':
			end = '’'
		}
		if end != 0 {
			masked.WriteString(strings.Repeat(" ", len(string(r))))
		} else {
			masked.WriteRune(r)
		}
	}
	if end != 0 {
		return "", protectedMeaningFailure(in.Locale)
	}
	projected := append([]string(nil), surface.words...)
	raw := masked.String()
	for i, token := range tokens {
		if token.text != surface.words[i] {
			return "", protectedMeaningFailure(in.Locale)
		}
		if strings.TrimSpace(raw[token.start:token.end]) == "" {
			projected[i] = opaqueInferenceToken
		}
	}
	return strings.Join(projected, " "), nil
}

func calendarPeriodTailV2(word string) bool {
	if numericYear(word) {
		return true
	}
	switch word {
	case "month", "quarter", "year", "mes", "trimestre", "año", "ano", "pasado", "anterior",
		"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december",
		"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre":
		return true
	}
	return false
}

func stableCalendarPhrasePolarityV2(surface inferenceSurface, phrase string) bool {
	for at := range surface.words {
		if phraseAt(surface.words, at, phrase) && surface.origins[at] >= 0 && negatedTemporalRange(surface.words, at) != negatedTemporalRange(surface.original, surface.origins[at]) {
			return false
		}
	}
	return true
}

// Zone assertions must have a concrete current target even when manual scalar
// intent suppressed inferred grouping or only retained periods remain.
func (p calendarProjection) validateResolvedTargets(in RouteRequest, admitted []admittedTopic, out *Interpretation) error {
	if p.zone == "" {
		return nil
	}
	checked := false
	for _, temporal := range out.Temporal {
		if err := p.validateZone(temporal.TimeZone); err != nil {
			return err
		}
		checked = true
	}
	if in.Grouping != nil {
		for _, key := range in.Grouping.Keys {
			for _, item := range admitted {
				if item.id != key.Topic {
					continue
				}
				for _, dim := range item.publication.Definition.Dimensions {
					if dim.ID == key.Dimension && dim.Temporal != nil {
						if err := p.validateZone(dim.Temporal.Timezone); err != nil {
							return err
						}
						checked = true
					}
				}
			}
		}
	}
	if !checked {
		return calendarMeaningError(in.Locale, "unbound_temporal_timezone")
	}
	return nil
}
