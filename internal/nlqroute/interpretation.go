package nlqroute

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

const interpretationVersion = "semantic-interpretation-v1"

// InterpretationEdit corrects or removes one previously inferable stable
// target. Replacement values must be reviewed governed-value IDs; arbitrary
// literals cannot become executable constraints through this seam.
type InterpretationEdit struct {
	Target string `json:"target"`
	Action string `json:"action"`
	Value  string `json:"value,omitempty"`
}

func (e InterpretationEdit) valid() bool {
	if e.Target == "" || len(e.Target) > 384 || strings.TrimSpace(e.Target) != e.Target || strings.ContainsAny(e.Target, "\x00\r\n") {
		return false
	}
	switch e.Action {
	case "remove":
		return e.Value == ""
	case "replace":
		return identity.Identifier(e.Value)
	default:
		return false
	}
}

// InterpretationPin binds one persisted interpretation to reviewed semantic coordinates.
type InterpretationPin struct {
	Topic          string `json:"topic"`
	Version        string `json:"version"`
	Digest         string `json:"digest"`
	Source         string `json:"source"`
	Dataset        string `json:"dataset"`
	Context        string `json:"context"`
	SourceRevision int64  `json:"source_revision"`
}

// ValueInterpretation records one deterministic governed-value resolution.
type ValueInterpretation struct {
	ID             string `json:"id"`
	Topic          string `json:"topic"`
	Dimension      string `json:"dimension"`
	Dataset        string `json:"dataset"`
	Column         string `json:"column"`
	GovernedValue  string `json:"governed_value"`
	CanonicalValue string `json:"canonical_value"`
	Operator       string `json:"operator"`
	Geography      bool   `json:"geography"`
	Provenance     string `json:"provenance"`
}

// TemporalInterpretation records one deterministic reviewed-calendar resolution.
type TemporalInterpretation struct {
	ID           string `json:"id"`
	Topic        string `json:"topic"`
	Dimension    string `json:"dimension"`
	Dataset      string `json:"dataset"`
	Column       string `json:"column"`
	Grain        string `json:"grain"`
	Calendar     string `json:"calendar"`
	TimeZone     string `json:"time_zone"`
	TemporalType string `json:"temporal_type"`
	LocalStart   string `json:"local_start"`
	LocalEnd     string `json:"local_end"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Provenance   string `json:"provenance"`
}

// Interpretation is a detached view of a sealed deterministic interpretation.
// Its digest binds locale/parser, anchor, current topic/source pins and typed
// values. It grants no authority and cannot issue an executable plan.
type Interpretation struct {
	Version       string                   `json:"version"`
	Parser        string                   `json:"parser"`
	Locale        nlq.Language             `json:"locale"`
	Anchor        string                   `json:"anchor"`
	Pins          []InterpretationPin      `json:"pins"`
	Values        []ValueInterpretation    `json:"values,omitempty"`
	Temporal      []TemporalInterpretation `json:"temporal,omitempty"`
	BindingDigest string                   `json:"binding_digest,omitempty"`
	Digest        string                   `json:"digest"`
}

type valueCandidate struct {
	item    *admittedTopic
	dim     semantics.Dimension
	value   semantics.GovernedValue
	dataset topicsDataset
	column  semantics.Column
	phrase  string
}

// topicsDataset avoids leaking a second semantic model while keeping the
// concrete fields used by interpretation local.
type topicsDataset struct {
	ID, Source, Context string
	Revision            int64
}

func (s *Service) interpret(ctx context.Context, e identity.Envelope, in *RouteRequest, admitted []admittedTopic) (*Interpretation, []readexec.BusinessConstraint, error) {
	if in.InterpretationAnchor == "" {
		in.InterpretationAnchor = time.Now().UTC().Format("2006-01-02")
	}
	anchor, err := time.Parse("2006-01-02", in.InterpretationAnchor)
	if err != nil {
		return nil, nil, ErrInvalid
	}
	out := &Interpretation{Version: interpretationVersion, Parser: "deterministic-span-v2", Locale: in.Locale, Anchor: in.InterpretationAnchor}
	question := normalizedPhrase(in.Question)
	knownTargets := map[string]string{}
	var values []valueCandidate
	var temporalCandidates []struct {
		item    *admittedTopic
		dim     semantics.Dimension
		dataset topicsDataset
		column  semantics.Column
	}
	for i := range admitted {
		item := &admitted[i]
		for _, dataset := range item.publication.Definition.Datasets {
			pin := InterpretationPin{Topic: item.id, Version: item.publication.State.Version, Digest: item.publication.Digest, Source: dataset.Source.Source, Dataset: dataset.ID, Context: dataset.Source.Context, SourceRevision: dataset.Source.SourceRevision}
			out.Pins = append(out.Pins, pin)
			for _, dim := range item.publication.Definition.Dimensions {
				if dim.Field.Dataset != dataset.ID {
					continue
				}
				column, ok := semanticColumn(dataset.Columns, dim.Field.ID)
				if !ok {
					return nil, nil, readexec.ErrBinding
				}
				ds := topicsDataset{ID: dataset.ID, Source: dataset.Source.Source, Context: dataset.Source.Context, Revision: dataset.Source.SourceRevision}
				if dim.Temporal != nil {
					knownTargets[interpretationTarget(item.id, dim.ID, "time")] = "time"
					temporalCandidates = append(temporalCandidates, struct {
						item    *admittedTopic
						dim     semantics.Dimension
						dataset topicsDataset
						column  semantics.Column
					}{item, dim, ds, column})
				}
				for _, governed := range dim.Values {
					if governed.Sensitivity != semantics.LiteralNonSensitive {
						continue
					}
					knownTargets[interpretationTarget(item.id, dim.ID, governed.ID)] = "value"
					for _, phrase := range append([]string{governed.Value}, governed.Aliases...) {
						n := normalizedPhrase(phrase)
						if n != "" && containsPhrase(question, n) {
							values = append(values, valueCandidate{item, dim, governed, ds, column, n})
						}
					}
				}
			}
		}
	}
	for _, edit := range in.InterpretationEdits {
		kind, ok := knownTargets[edit.Target]
		if !ok || kind == "time" && edit.Action == "replace" {
			return nil, nil, ErrInvalid
		}
	}
	sort.Slice(out.Pins, func(i, j int) bool {
		a, b := out.Pins[i], out.Pins[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		return a.Dataset < b.Dataset
	})
	values = dedupeValueCandidates(values)
	byPhrase := map[string][]valueCandidate{}
	for _, candidate := range values {
		byPhrase[candidate.phrase] = append(byPhrase[candidate.phrase], candidate)
	}
	phrases := make([]string, 0, len(byPhrase))
	for phrase := range byPhrase {
		phrases = append(phrases, phrase)
	}
	sort.Strings(phrases)
	emittedTargets := map[string]string{}
	for _, phrase := range phrases {
		candidates := byPhrase[phrase]
		if len(candidates) > 1 {
			return nil, nil, &Clarification{Reason: "ambiguous_governed_value", Outcome: semantics.ClarificationConflicting, Prompt: fmt.Sprintf("Choose which reviewed dimension owns %q.", phrase)}
		}
		candidate := candidates[0]
		target := interpretationTarget(candidate.item.id, candidate.dim.ID, candidate.value.ID)
		edited, replacement, remove := applyInterpretationEdit(in.InterpretationEdits, target)
		if remove {
			continue
		}
		if edited {
			found := false
			for _, governed := range candidate.dim.Values {
				if governed.ID == replacement && governed.Sensitivity == semantics.LiteralNonSensitive {
					candidate.value, found = governed, true
					break
				}
			}
			if !found {
				return nil, nil, ErrInvalid
			}
		}
		op := "eq"
		if negatedPhrase(question, phrase, in.Locale) {
			op = "ne"
		}
		if previous, exists := emittedTargets[target]; exists {
			if previous != op {
				return nil, nil, &Clarification{Reason: "conflicting_governed_value", Outcome: semantics.ClarificationConflicting, Prompt: fmt.Sprintf("Clarify whether reviewed value %q is included or excluded.", candidate.value.ID)}
			}
			continue
		}
		emittedTargets[target] = op
		geography := candidate.dim.Geography
		id := readexec.Hash([]any{interpretationVersion, target, candidate.value.ID, op, out.Pins})
		out.Values = append(out.Values, ValueInterpretation{ID: id, Topic: candidate.item.id, Dimension: candidate.dim.ID, Dataset: candidate.dataset.ID, Column: candidate.column.SourceName, GovernedValue: candidate.value.ID, CanonicalValue: candidate.value.Value, Operator: op, Geography: geography, Provenance: "reviewed_governed_value"})
	}
	span, hasSpan, spanErr := temporalSpan(question, in.Locale, anchor)
	if spanErr != nil {
		return nil, nil, spanErr
	}
	if hasSpan {
		groupGrain, groupErr := requestedGroupingGrain(question, in.Locale)
		if groupErr != nil {
			return nil, nil, groupErr
		}
		eligible := temporalCandidates[:0]
		for _, candidate := range temporalCandidates {
			if supportsTemporalRequest(candidate.dim.Temporal.Grains, span, groupGrain) {
				eligible = append(eligible, candidate)
			}
		}
		if len(eligible) == 0 {
			return nil, nil, &Clarification{Reason: "unsupported_temporal_grain", Outcome: semantics.ClarificationInvalid, Prompt: "No reviewed temporal dimension supports this period grain."}
		}
		if len(eligible) > 1 {
			var named []struct {
				item    *admittedTopic
				dim     semantics.Dimension
				dataset topicsDataset
				column  semantics.Column
			}
			for _, candidate := range eligible {
				for _, label := range append([]string{candidate.dim.ID, candidate.dim.Name}, candidate.dim.Aliases...) {
					if containsPhrase(question, normalizedPhrase(label)) {
						named = append(named, candidate)
						break
					}
				}
			}
			eligible = named
		}
		if len(eligible) != 1 {
			return nil, nil, &Clarification{Reason: "ambiguous_temporal_dimension", Outcome: semantics.ClarificationConflicting, Prompt: "Choose the reviewed temporal dimension for this period."}
		}
		candidate := eligible[0]
		target := interpretationTarget(candidate.item.id, candidate.dim.ID, "time")
		_, _, remove := applyInterpretationEdit(in.InterpretationEdits, target)
		if !remove {
			tz := candidate.dim.Temporal.Timezone
			kind := temporalColumnType(candidate.column)
			if tz == "" && kind == "date" {
				tz = "UTC"
			}
			if tz == "" {
				return nil, nil, &Clarification{Reason: "unsupported_temporal_timezone", Outcome: semantics.ClarificationInvalid, Prompt: "This temporal dimension needs a reviewed timezone before interpretation."}
			}
			calendar := candidate.dim.Temporal.Calendar
			if calendar != "gregorian" {
				return nil, nil, &Clarification{Reason: "unsupported_calendar", Outcome: semantics.ClarificationInvalid, Prompt: "This temporal calendar is not supported by deterministic interpretation."}
			}
			temporalType := temporalColumnType(candidate.column)
			start, end, boundaryErr := temporalBoundaryValues(span.start, span.end, tz, temporalType)
			if boundaryErr != nil {
				return nil, nil, boundaryErr
			}
			id := readexec.Hash([]any{interpretationVersion, target, start, end, span.grain, tz, temporalType, out.Pins})
			out.Temporal = append(out.Temporal, TemporalInterpretation{ID: id, Topic: candidate.item.id, Dimension: candidate.dim.ID, Dataset: candidate.dataset.ID, Column: candidate.column.SourceName, Grain: span.grain, Calendar: calendar, TimeZone: tz, TemporalType: temporalType, LocalStart: span.start, LocalEnd: span.end, Start: start, End: end, Provenance: span.provenance})
		}
	}
	sort.Slice(out.Values, func(i, j int) bool { return out.Values[i].ID < out.Values[j].ID })
	constraints := make([]readexec.BusinessConstraint, 0, len(out.Values)+len(out.Temporal))
	reader, needsBinding := s.topics.(clarificationBindingReader)
	if (len(out.Values) > 0 || len(out.Temporal) > 0) && !needsBinding {
		return nil, nil, &readexec.BusinessConstraintError{Code: "unsupported_constraint_binding", Field: "source"}
	}
	neededDatasets := map[string]bool{}
	for _, value := range out.Values {
		neededDatasets[value.Topic+"\x00"+value.Dataset] = true
	}
	for _, temporal := range out.Temporal {
		neededDatasets[temporal.Topic+"\x00"+temporal.Dataset] = true
	}
	var binding *readexec.Binding
	for i := range admitted {
		for _, dataset := range admitted[i].publication.Definition.Datasets {
			if !neededDatasets[admitted[i].id+"\x00"+dataset.ID] {
				continue
			}
			candidate, err := reader.ClarificationBinding(ctx, e, dataset.Source.Source, in.Context)
			if err != nil {
				return nil, nil, err
			}
			if !candidate.Valid() || candidate.Tenant != e.Tenant() || candidate.Context != in.Context || candidate.Source != dataset.Source.Source || candidate.Revision != dataset.Source.SourceRevision {
				return nil, nil, readexec.ErrBinding
			}
			if binding != nil && readexec.Hash(*binding) != readexec.Hash(candidate) {
				return nil, nil, readexec.ErrBinding
			}
			copy := candidate.Clone()
			binding = &copy
			admitted[i].binding = &copy
		}
	}
	if binding != nil {
		out.BindingDigest = readexec.Hash(*binding)
	}
	for _, value := range out.Values {
		constraints = append(constraints, readexec.BusinessConstraint{Resolution: value.ID, Dataset: value.Dataset, Column: value.Column, SourceRevision: binding.Revision, Kind: "text", Operator: value.Operator, Nulls: "exclude", Value: value.CanonicalValue})
	}
	for _, temporal := range out.Temporal {
		constraints = append(constraints, readexec.BusinessConstraint{Resolution: temporal.ID, Dataset: temporal.Dataset, Column: temporal.Column, SourceRevision: binding.Revision, Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: temporal.TemporalType, Calendar: temporal.Calendar, TimeZone: temporal.TimeZone, Grain: temporal.Grain, Value: temporal.Start, Upper: temporal.End})
	}
	if binding != nil && len(constraints) > 0 {
		if err := readexec.ValidateBusinessConstraints(*binding, constraints); err != nil {
			return nil, nil, err
		}
	}
	out.Digest = readexec.Hash(struct {
		Version, Parser string
		Locale          nlq.Language
		Anchor          string
		Pins            []InterpretationPin
		Values          []ValueInterpretation
		Temporal        []TemporalInterpretation
	}{out.Version, out.Parser, out.Locale, out.Anchor, out.Pins, out.Values, out.Temporal})
	return out, constraints, nil
}

func semanticColumn(columns []semantics.Column, id string) (semantics.Column, bool) {
	for _, c := range columns {
		if c.ID == id {
			return c, true
		}
	}
	return semantics.Column{}, false
}

func supportsGrain(grains []semantics.TimeGrain, grain semantics.TimeGrain) bool {
	for _, candidate := range grains {
		if candidate == grain {
			return true
		}
	}
	return false
}

func supportsTemporalRequest(grains []semantics.TimeGrain, span parsedSpan, grouping semantics.TimeGrain) bool {
	// A calendar-year filter does not imply year aggregation. Its reviewed
	// calendar and timezone are checked after selecting the dimension.
	intervalReviewed := (span.grain == "year" && len(grains) > 0) || supportsGrain(grains, semantics.TimeGrain(span.grain))
	if !intervalReviewed {
		return false
	}
	return grouping == "" || supportsGrain(grains, grouping) && temporalGroupingAligned(span, grouping)
}

func temporalGroupingAligned(span parsedSpan, grouping semantics.TimeGrain) bool {
	if span.grain == "year" || grouping == semantics.GrainMonth {
		return true
	}
	start, startErr := time.Parse("2006-01-02", span.start)
	end, endErr := time.Parse("2006-01-02", span.end)
	if startErr != nil || endErr != nil {
		return false
	}
	switch grouping {
	case semantics.GrainQuarter:
		return (int(start.Month())-1)%3 == 0 && (int(end.Month())-1)%3 == 0
	case semantics.GrainYear:
		return start.Month() == time.January && end.Month() == time.January
	default:
		return true
	}
}

func requestedGroupingGrain(question string, locale nlq.Language) (semantics.TimeGrain, error) {
	month := locale == nlq.LanguageEnglish && (containsPhrase(question, "by month") || containsPhrase(question, "per month") || containsPhrase(question, "monthly")) ||
		locale == nlq.LanguageSpanish && (containsPhrase(question, "por mes") || containsPhrase(question, "mensual"))
	quarter := locale == nlq.LanguageEnglish && (containsPhrase(question, "by quarter") || containsPhrase(question, "per quarter") || containsPhrase(question, "quarterly")) ||
		locale == nlq.LanguageSpanish && (containsPhrase(question, "por trimestre") || containsPhrase(question, "trimestral"))
	year := locale == nlq.LanguageEnglish && (containsPhrase(question, "by year") || containsPhrase(question, "per year") || containsPhrase(question, "yearly")) ||
		locale == nlq.LanguageSpanish && (containsPhrase(question, "por año") || containsPhrase(question, "por ano") || containsPhrase(question, "anual"))
	if (month && quarter) || (month && year) || (quarter && year) {
		return "", &Clarification{Reason: "ambiguous_temporal_grain", Outcome: semantics.ClarificationConflicting, Prompt: "Choose one reviewed grouping grain for this route."}
	}
	if month {
		return semantics.GrainMonth, nil
	}
	if quarter {
		return semantics.GrainQuarter, nil
	}
	if year {
		return semantics.GrainYear, nil
	}
	return "", nil
}

func temporalColumnType(column semantics.Column) string {
	native := strings.ToLower(column.NativeType)
	if native == "date" {
		return "date"
	}
	if strings.Contains(native, "with time zone") || strings.Contains(native, "timestamptz") || strings.Contains(native, "timestamp_tz") {
		return "timestamptz"
	}
	return "timestamp"
}
func normalizedPhrase(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
func containsPhrase(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+phrase+" ")
}
func dedupeValueCandidates(in []valueCandidate) []valueCandidate {
	seen := map[string]bool{}
	out := make([]valueCandidate, 0, len(in))
	for _, v := range in {
		k := v.item.id + "\x00" + v.dim.ID + "\x00" + v.value.ID + "\x00" + v.phrase
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}
func interpretationTarget(topic, dimension, value string) string {
	return topic + ":" + dimension + ":" + value
}
func applyInterpretationEdit(edits []InterpretationEdit, target string) (bool, string, bool) {
	for _, e := range edits {
		if e.Target == target {
			return true, e.Value, e.Action == "remove"
		}
	}
	return false, "", false
}
func negatedPhrase(question, phrase string, locale nlq.Language) bool {
	idx := strings.Index(" "+question+" ", " "+phrase+" ")
	if idx < 0 {
		return false
	}
	prefix := strings.Fields((" " + question + " ")[:idx])
	if len(prefix) > 3 {
		prefix = prefix[len(prefix)-3:]
	}
	neg := map[string]bool{"not": true, "except": true, "excluding": true, "without": true, "no": true, "sin": true, "excepto": true, "excluyendo": true}
	for _, p := range prefix {
		if neg[p] {
			return true
		}
	}
	return false
}

type parsedSpan struct{ start, end, grain, provenance string }

func temporalSpan(question string, locale nlq.Language, anchor time.Time) (parsedSpan, bool, error) {
	words := strings.Fields(question)
	yearTokens := 0
	consumedYearTokens := 0
	for _, word := range words {
		if numericYear(word) {
			yearTokens++
		}
	}
	months := map[string]time.Month{"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6, "july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12, "enero": 1, "febrero": 2, "marzo": 3, "abril": 4, "mayo": 5, "junio": 6, "julio": 7, "agosto": 8, "septiembre": 9, "octubre": 10, "noviembre": 11, "diciembre": 12}
	if negatedTemporalRequest(words, months) {
		return parsedSpan{}, false, &Clarification{Reason: "unsupported_temporal_negation", Outcome: semantics.ClarificationInvalid, Prompt: "Use a positive reviewed period or clarify the excluded dates."}
	}
	lastMonth := containsPhrase(question, "last month") || containsPhrase(question, "mes pasado") || containsPhrase(question, "ultimo mes") || containsPhrase(question, "último mes")
	thisMonth := containsPhrase(question, "this month") || containsPhrase(question, "este mes")
	monthPositions := make([]int, 0, 2)
	for i, word := range words {
		if _, ok := months[word]; ok {
			monthPositions = append(monthPositions, i)
		}
	}
	if len(monthPositions) == 0 && yearTokens > 0 {
		if lastMonth || thisMonth {
			return parsedSpan{}, false, ambiguousTemporalSpanError()
		}
		if yearTokens == 1 {
			for i, word := range words {
				if !numericYear(word) || i == 0 || !yearConnectorForLocale(words[i-1], locale) {
					continue
				}
				year, err := strconv.Atoi(word)
				if err == nil && year > 0 && year < 9999 {
					start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
					return parsedSpan{start.Format("2006-01-02"), start.AddDate(1, 0, 0).Format("2006-01-02"), "year", "explicit_calendar_year"}, true, nil
				}
			}
		}
		return parsedSpan{}, false, invalidTemporalSpanError()
	}
	if len(monthPositions) == 2 && yearTokens == 1 {
		if lastMonth || thisMonth {
			return parsedSpan{}, false, ambiguousTemporalSpanError()
		}
		first, second := monthPositions[0], monthPositions[1]
		if first > 0 && second == first+2 && second+1 < len(words) &&
			rangeStartForLocale(words[first-1], locale) && rangeJoinForLocale(words[first+1], locale) {
			yearAt := second + 1
			if namedMonthConnector(words[yearAt]) && connectorForLocale(words[yearAt], locale) {
				yearAt++
			}
			if yearAt < len(words) && numericYear(words[yearAt]) {
				year, err := strconv.Atoi(words[yearAt])
				startMonth, endMonth := months[words[first]], months[words[second]]
				if err == nil && year > 0 && year < 9999 && startMonth <= endMonth {
					start := time.Date(year, startMonth, 1, 0, 0, 0, 0, time.UTC)
					end := time.Date(year, endMonth+1, 1, 0, 0, 0, 0, time.UTC)
					return parsedSpan{start.Format("2006-01-02"), end.Format("2006-01-02"), "month", "explicit_month_range"}, true, nil
				}
			}
		}
	}
	var named []parsedSpan
	seen := map[string]bool{}
	for i, word := range words {
		month, ok := months[word]
		if !ok {
			continue
		}
		year := anchor.Year()
		provenance := "month_anchor_year"
		next := i + 1
		if next < len(words) {
			if namedMonthConnector(words[next]) {
				if !connectorForLocale(words[next], locale) {
					return parsedSpan{}, false, &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Use a supported month connector for the request locale."}
				}
				next++
				if next >= len(words) {
					return parsedSpan{}, false, &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Provide one four-digit year after the month connector."}
				}
			}
			if candidate, parseErr := strconv.Atoi(words[next]); parseErr == nil {
				if !numericYear(words[next]) || candidate < 1 || candidate > 9998 || yearTokens != 1 {
					return parsedSpan{}, false, &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Provide one supported month and four-digit year."}
				}
				year, provenance = candidate, "explicit_month_year"
				consumedYearTokens++
			} else if next != i+1 {
				return parsedSpan{}, false, &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Provide one four-digit year after the month connector."}
			}
		}
		start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		key := start.Format("2006-01-02")
		if !seen[key] {
			seen[key] = true
			named = append(named, parsedSpan{key, start.AddDate(0, 1, 0).Format("2006-01-02"), "month", provenance})
		}
	}
	if consumedYearTokens != yearTokens {
		return parsedSpan{}, false, &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Provide one supported month and four-digit year."}
	}
	expressions := len(named)
	if lastMonth {
		expressions++
	}
	if thisMonth {
		expressions++
	}
	if expressions > 1 {
		return parsedSpan{}, false, &Clarification{Reason: "ambiguous_temporal_span", Outcome: semantics.ClarificationConflicting, Prompt: "Choose one reviewed period for this route."}
	}
	if lastMonth {
		start := time.Date(anchor.Year(), anchor.Month()-1, 1, 0, 0, 0, 0, time.UTC)
		return parsedSpan{start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02"), "month", "relative_anchor"}, true, nil
	}
	if thisMonth {
		start := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
		return parsedSpan{start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02"), "month", "relative_anchor"}, true, nil
	}
	if len(named) == 1 {
		return named[0], true, nil
	}
	return parsedSpan{}, false, nil
}

func negatedTemporalRequest(words []string, months map[string]time.Month) bool {
	for i, word := range words {
		_, namedMonth := months[word]
		relativeMonth := (word == "last" || word == "this") && i+1 < len(words) && words[i+1] == "month" ||
			(word == "mes" && i+1 < len(words) && words[i+1] == "pasado") ||
			(word == "este" || word == "ultimo" || word == "último") && i+1 < len(words) && words[i+1] == "mes"
		year := numericYear(word) && i > 0 && (wordIsYearConnector(words[i-1]))
		if (namedMonth || relativeMonth || year) && negatedTemporalPrefix(words, i) {
			return true
		}
	}
	return false
}

func negatedTemporalPrefix(words []string, i int) bool {
	if i > 0 && temporalNegator(words[i-1]) {
		return true
	}
	return i > 1 && (words[i-1] == "in" || words[i-1] == "during" || words[i-1] == "en" || words[i-1] == "durante" || words[i-1] == "from" || words[i-1] == "de") && temporalNegator(words[i-2])
}

func temporalNegator(word string) bool {
	switch word {
	case "not", "no", "sin", "except", "excepto", "excluding", "excluyendo", "without":
		return true
	}
	return false
}

func wordIsYearConnector(word string) bool {
	return word == "in" || word == "during" || word == "en" || word == "durante"
}

func invalidTemporalSpanError() error {
	return &Clarification{Reason: "invalid_temporal_span", Outcome: semantics.ClarificationInvalid, Prompt: "Provide one supported month or calendar year with a four-digit year."}
}

func ambiguousTemporalSpanError() error {
	return &Clarification{Reason: "ambiguous_temporal_span", Outcome: semantics.ClarificationConflicting, Prompt: "Choose one reviewed period for this route."}
}

func yearConnectorForLocale(word string, locale nlq.Language) bool {
	return locale == nlq.LanguageEnglish && (word == "in" || word == "during") ||
		locale == nlq.LanguageSpanish && (word == "en" || word == "durante")
}

func rangeStartForLocale(word string, locale nlq.Language) bool {
	return locale == nlq.LanguageEnglish && word == "from" || locale == nlq.LanguageSpanish && word == "de"
}

func rangeJoinForLocale(word string, locale nlq.Language) bool {
	return locale == nlq.LanguageEnglish && (word == "through" || word == "to") || locale == nlq.LanguageSpanish && (word == "a" || word == "hasta")
}

func connectorForLocale(word string, locale nlq.Language) bool {
	return locale == nlq.LanguageSpanish && (word == "de" || word == "del") || locale == nlq.LanguageEnglish && word == "of"
}

func namedMonthConnector(word string) bool {
	return word == "de" || word == "del" || word == "of"
}

func numericYear(word string) bool {
	digits := 0
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
		digits++
	}
	return digits == 4
}

func temporalBoundaryValues(start, end, zoneName, temporalType string) (string, string, error) {
	if temporalType != "timestamptz" {
		return start, end, nil
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return "", "", &Clarification{Reason: "unsupported_temporal_timezone", Outcome: semantics.ClarificationInvalid, Prompt: "This temporal dimension needs a supported reviewed timezone."}
	}
	lower, lowerErr := time.ParseInLocation("2006-01-02", start, zone)
	upper, upperErr := time.ParseInLocation("2006-01-02", end, zone)
	if lowerErr != nil || upperErr != nil || lower.Format("2006-01-02") != start || upper.Format("2006-01-02") != end || !uniqueLocalMidnight(lower) || !uniqueLocalMidnight(upper) {
		return "", "", &Clarification{Reason: "ambiguous_temporal_boundary", Outcome: semantics.ClarificationInvalid, Prompt: "This reviewed timezone has a missing or ambiguous calendar boundary."}
	}
	return lower.UTC().Format(time.RFC3339), upper.UTC().Format(time.RFC3339), nil
}

func uniqueLocalMidnight(candidate time.Time) bool {
	if candidate.Hour() != 0 || candidate.Minute() != 0 || candidate.Second() != 0 || candidate.Nanosecond() != 0 {
		return false
	}
	zone := candidate.Location()
	year, month, day := candidate.Date()
	wall := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	cursor, limit := candidate.Add(-48*time.Hour), candidate.Add(48*time.Hour)
	offsets := map[int]bool{}
	complete := false
	for transitions := 0; transitions < 16; transitions++ {
		_, offset := cursor.In(zone).Zone()
		if offset < -24*60*60 || offset > 24*60*60 {
			return false
		}
		offsets[offset] = true
		_, periodEnd := cursor.In(zone).ZoneBounds()
		if periodEnd.IsZero() || periodEnd.After(limit) {
			complete = true
			break
		}
		if !periodEnd.After(cursor) {
			return false
		}
		cursor = periodEnd
	}
	if !complete {
		return false
	}
	matches := 0
	for offset := range offsets {
		actual := wall.Add(-time.Duration(offset) * time.Second).In(zone)
		y, m, d := actual.Date()
		if y == year && m == month && d == day && actual.Hour() == 0 && actual.Minute() == 0 && actual.Second() == 0 && actual.Nanosecond() == 0 {
			matches++
		}
	}
	return matches == 1
}
func mergeInterpretationConstraints(state *nlq.ConstraintState, interpretation *Interpretation) *nlq.ConstraintState {
	if interpretation == nil || len(interpretation.Values)+len(interpretation.Temporal) == 0 {
		return state
	}
	if state == nil {
		state = &nlq.ConstraintState{Allowed: true}
	}
	for _, v := range interpretation.Values {
		state.Required = append(state.Required, nlq.MandatoryConstraint{ID: "interpretation-" + v.ID, Kind: "interpreted_value", Text: "Apply reviewed value " + v.GovernedValue + " to dimension " + v.Dimension + " using " + v.Operator})
	}
	for _, v := range interpretation.Temporal {
		state.Required = append(state.Required, nlq.MandatoryConstraint{ID: "interpretation-" + v.ID, Kind: "interpreted_time", Text: "Apply " + v.Grain + " time window " + v.Start + " through " + v.End + " to dimension " + v.Dimension})
	}
	return state
}

func interpretationConstraintIDs(in *Interpretation) []string {
	if in == nil {
		return nil
	}
	out := make([]string, 0, len(in.Values)+len(in.Temporal))
	for _, value := range in.Values {
		out = append(out, value.ID)
	}
	for _, temporal := range in.Temporal {
		out = append(out, temporal.ID)
	}
	return out
}
