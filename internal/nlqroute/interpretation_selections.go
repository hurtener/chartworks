package nlqroute

import (
	"context"
	"sort"
	"strings"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

// InterpretationContinuationPolicy explicitly retains continuation parsing when
// every filter has been removed. An absent policy keeps historical parser
// behavior; this mode selects no values and grants no source authority.
const InterpretationContinuationPolicy = "continuation-v1"

// InterpretationPeriod is a civil, half-open reviewed-calendar interval. It
// contains no SQL, timezone override or source authority. Current catalog policy
// determines the source type, calendar, timezone and executable bound values.
type InterpretationPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Grain string `json:"grain"`
}

// InterpretationSelection is explicit/retained intent, not a previously issued
// binding. Value names a non-sensitive reviewed value ID, never a scalar literal.
// Exactly one of Value or Period is required. Fresh language supersedes retained
// selections for the same dimension; otherwise these selections survive edits.
type InterpretationSelection struct {
	Topic     string                `json:"topic"`
	Dimension string                `json:"dimension"`
	Value     string                `json:"value,omitempty"`
	Operator  string                `json:"operator,omitempty"`
	Period    *InterpretationPeriod `json:"period,omitempty"`
}

func (p InterpretationPeriod) valid() bool {
	start, e1 := time.Parse("2006-01-02", p.Start)
	end, e2 := time.Parse("2006-01-02", p.End)
	if e1 != nil || e2 != nil || start.Year() < 1 || end.Year() > 9999 || !end.After(start) {
		return false
	}
	switch p.Grain {
	case "day":
		return true
	case "month":
		return start.Day() == 1 && end.Day() == 1
	case "quarter":
		return start.Day() == 1 && end.Day() == 1 && (int(start.Month())-1)%3 == 0 && (int(end.Month())-1)%3 == 0
	case "year":
		return start.Day() == 1 && end.Day() == 1 && start.Month() == time.January && end.Month() == time.January
	}
	return false
}
func (s InterpretationSelection) valid() bool {
	if !identity.Identifier(s.Topic) || !identity.Identifier(s.Dimension) {
		return false
	}
	if s.Period != nil {
		return s.Value == "" && s.Operator == "" && s.Period.valid()
	}
	return identity.Identifier(s.Value) && (s.Operator == "eq" || s.Operator == "ne")
}
func selectionTarget(s InterpretationSelection) string {
	value := s.Value
	if s.Period != nil {
		value = "time"
	}
	return interpretationTarget(s.Topic, s.Dimension, value)
}

// CloneInterpretationSelections detaches every nested interval from caller memory.
func CloneInterpretationSelections(in []InterpretationSelection) []InterpretationSelection {
	out := append([]InterpretationSelection(nil), in...)
	for i := range out {
		if out[i].Period != nil {
			v := *out[i].Period
			out[i].Period = &v
		}
	}
	return out
}

// CloneInterpretationEdits detaches optional replacement intervals as well.
func CloneInterpretationEdits(in []InterpretationEdit) []InterpretationEdit {
	out := append([]InterpretationEdit(nil), in...)
	for i := range out {
		if out[i].Period != nil {
			v := *out[i].Period
			out[i].Period = &v
		}
	}
	return out
}

// RetainedInterpretationSelections projects resolved intent only. The caller
// first reauthorizes/replays its parent; Route always resolves these coordinates
// anew, so a saved selection or a forged client object never bypasses admission.
func RetainedInterpretationSelections(in *Interpretation) []InterpretationSelection {
	if in == nil {
		return nil
	}
	out := make([]InterpretationSelection, 0, len(in.Values)+len(in.Temporal))
	for _, v := range in.Values {
		out = append(out, InterpretationSelection{Topic: v.Topic, Dimension: v.Dimension, Value: v.GovernedValue, Operator: v.Operator})
	}
	for _, v := range in.Temporal {
		out = append(out, InterpretationSelection{Topic: v.Topic, Dimension: v.Dimension, Period: &InterpretationPeriod{Start: v.LocalStart, End: v.LocalEnd, Grain: v.Grain}})
	}
	sort.Slice(out, func(i, j int) bool { return selectionTarget(out[i]) < selectionTarget(out[j]) })
	return out
}

func (s *Service) mergeRetainedInterpretation(ctx context.Context, in RouteRequest, admitted []admittedTopic, out *Interpretation, fresh map[string]bool) error {
	if len(in.InterpretationSelections) == 0 {
		return nil
	}
	out.Parser = "deterministic-continuation-v1"
	for _, selected := range in.InterpretationSelections {
		if err := ctx.Err(); err != nil {
			return err
		}
		var dim semantics.Dimension
		var column semantics.Column
		var dataset string
		found := 0
		for _, item := range admitted {
			if item.id != selected.Topic {
				continue
			}
			for _, d := range item.publication.Definition.Dimensions {
				if d.ID != selected.Dimension {
					continue
				}
				for _, ds := range item.publication.Definition.Datasets {
					if ds.ID != d.Field.Dataset {
						continue
					}
					if c, ok := semanticColumn(ds.Columns, d.Field.ID); ok {
						dim, column, dataset = d, c, ds.ID
						found++
					}
				}
			}
		}
		if found != 1 {
			return readexec.ErrBinding
		}
		target := selectionTarget(selected)
		var governed *semantics.GovernedValue
		if selected.Period == nil {
			for _, v := range dim.Values {
				if v.ID == selected.Value && v.Sensitivity == semantics.LiteralNonSensitive {
					copy := v
					governed = &copy
				}
			}
			if governed == nil {
				return ErrInvalid
			}
		} else if dim.Temporal == nil {
			return ErrInvalid
		}
		// Validate the old coordinate even when fresh language supersedes it: a
		// stale or foreign selection cannot be smuggled through as unused context.
		if fresh[selected.Topic+"\x00"+selected.Dimension] {
			continue
		}
		edited, replacement, remove := applyInterpretationEdit(in.InterpretationEdits, target)
		if remove {
			continue
		}
		if selected.Period == nil {
			if edited {
				governed = nil
				for _, v := range dim.Values {
					if v.ID == replacement && v.Sensitivity == semantics.LiteralNonSensitive {
						copy := v
						governed = &copy
					}
				}
				if governed == nil {
					return ErrInvalid
				}
			}
			id := readexec.Hash([]any{interpretationVersion, target, governed.ID, selected.Operator, out.Pins})
			out.Values = append(out.Values, ValueInterpretation{ID: id, Topic: selected.Topic, Dimension: dim.ID, Dataset: dataset, Column: column.SourceName, GovernedValue: governed.ID, CanonicalValue: governed.Value, Operator: selected.Operator, Geography: dim.Geography, Provenance: "retained_reviewed_value"})
		} else {
			period := *selected.Period
			for _, edit := range in.InterpretationEdits {
				if edit.Target == target && edit.Period != nil {
					period = *edit.Period
				}
			}
			grouping, err := requestedGroupingGrain(normalizedPhrase(in.Question), in.Locale)
			if err != nil {
				return err
			}
			span := parsedSpan{start: period.Start, end: period.End, grain: period.Grain, provenance: "retained_reviewed_interval"}
			if !supportsTemporalRequest(dim.Temporal.Grains, span, grouping) {
				return ErrInvalid
			}
			temporal, err := interpretedPeriod(selected.Topic, dim, column, dataset, span, out.Pins)
			if err != nil {
				return err
			}
			out.Temporal = append(out.Temporal, temporal)
		}
	}
	return nil
}

func interpretedPeriod(topic string, dim semantics.Dimension, column semantics.Column, dataset string, span parsedSpan, pins []InterpretationPin) (TemporalInterpretation, error) {
	if dim.Temporal == nil || dim.Temporal.Calendar != "gregorian" {
		return TemporalInterpretation{}, ErrInvalid
	}
	kind := temporalColumnType(column)
	zone := dim.Temporal.Timezone
	if zone == "" && kind == "date" {
		zone = "UTC"
	}
	if zone == "" {
		return TemporalInterpretation{}, ErrInvalid
	}
	start, end, err := temporalBoundaryValues(span.start, span.end, zone, kind)
	if err != nil {
		return TemporalInterpretation{}, err
	}
	id := readexec.Hash([]any{interpretationVersion, interpretationTarget(topic, dim.ID, "time"), start, end, span.grain, zone, kind, pins})
	return TemporalInterpretation{ID: id, Topic: topic, Dimension: dim.ID, Dataset: dataset, Column: column.SourceName, Grain: span.grain, Calendar: "gregorian", TimeZone: zone, TemporalType: kind, LocalStart: span.start, LocalEnd: span.end, Start: start, End: end, Provenance: span.provenance}, nil
}

// The expanded relative language applies only to the new continuation packet;
// retained requests without selections keep the original parser unchanged.
func continuationSpan(question string, anchor time.Time, base parsedSpan, has bool, err error) (parsedSpan, bool, error) {
	if err != nil {
		return base, has, err
	}
	var expressions []parsedSpan
	for _, item := range []struct {
		phrases []string
		grain   string
		offset  int
	}{
		{[]string{"this year", "este año", "este ano"}, "year", 0}, {[]string{"last year", "año pasado", "ano pasado"}, "year", -1},
		{[]string{"this quarter", "este trimestre"}, "quarter", 0}, {[]string{"last quarter", "trimestre pasado", "último trimestre", "ultimo trimestre"}, "quarter", -1},
	} {
		for _, phrase := range item.phrases {
			if containsPhrase(question, phrase) {
				if negatedPhrase(question, phrase, "en") || negatedPhrase(question, phrase, "es") {
					return parsedSpan{}, false, invalidTemporalSpanError()
				}
				year, month := anchor.Year(), time.January
				months := 12
				if item.grain == "quarter" {
					month = time.Month((int(anchor.Month())-1)/3*3 + 1 + item.offset*3)
					months = 3
				} else {
					year += item.offset
				}
				start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
				end := start.AddDate(0, months, 0)
				expressions = append(expressions, parsedSpan{start.Format("2006-01-02"), end.Format("2006-01-02"), item.grain, "relative_continuation_anchor"})
				break
			}
		}
	}
	if len(expressions) > 1 || has && len(expressions) > 0 {
		return parsedSpan{}, false, ambiguousTemporalSpanError()
	}
	if len(expressions) == 1 {
		span := expressions[0]
		if !(InterpretationPeriod{Start: span.start, End: span.end, Grain: span.grain}).valid() {
			return parsedSpan{}, false, invalidTemporalSpanError()
		}
		return span, true, nil
	}
	return base, has, nil
}

func validateInterpretationSelections(in []InterpretationSelection) bool {
	if len(in) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range in {
		key := selectionTarget(s)
		if !s.valid() || seen[key] || strings.ContainsAny(key, "\x00\r\n") {
			return false
		}
		seen[key] = true
	}
	return true
}
