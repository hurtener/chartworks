package semantics

import (
	"math/big"
	"time"
)

type clarificationEffectOwner struct {
	key    clarificationKey
	effect ClarificationEffect
}

func clarificationField(model RuleModel, target Reference) Reference {
	if target.Kind == KindColumn {
		return target
	}
	if deps := model.graph[target]; len(deps) == 1 && deps[0].Kind == KindColumn {
		return deps[0]
	}
	return target
}

func clarificationConflicts(model RuleModel, patterns []rankedClarificationPattern, resolutions []ClarificationResolution) map[clarificationKey]bool {
	conflicts := map[clarificationKey]bool{}
	owners := map[Reference][]clarificationEffectOwner{}
	resolved := map[clarificationKey]bool{}
	groups := map[Reference][]ClarificationResolution{}
	for _, resolution := range resolutions {
		key := clarificationKey{resolution.Pattern, resolution.Slot}
		resolved[key] = true
		if resolution.Effect != nil {
			field := clarificationField(model, resolution.Effect.Target)
			groups[field] = append(groups[field], resolution)
		}
	}
	for _, ranked := range patterns {
		if !ranked.active {
			continue
		}
		for _, slot := range ranked.pattern.Slots {
			key := clarificationKey{ranked.pattern.ID, slot.ID}
			if slot.Effect == nil || (!slot.Required && slot.Default == nil && !resolved[key]) {
				continue
			}
			field := clarificationField(model, slot.Effect.Target)
			owners[field] = append(owners[field], clarificationEffectOwner{key: key, effect: *slot.Effect})
		}
	}
	for _, list := range owners {
		bad := false
		for i := range list {
			for j := i + 1; j < len(list); j++ {
				a, b := list[i].effect, list[j].effect
				if clarificationScalarKind(a.Kind) != clarificationScalarKind(b.Kind) || a.Unit != b.Unit || a.Calendar != b.Calendar || a.TimeZone != b.TimeZone || a.TemporalType != b.TemporalType || (a.Nulls == "only" && b.Nulls == "exclude") || (b.Nulls == "only" && a.Nulls == "exclude") {
					bad = true
				}
			}
		}
		if bad {
			for _, owner := range list {
				conflicts[owner.key] = true
			}
		}
	}
	for _, group := range groups {
		if !clarificationValueGroupConflict(group) {
			continue
		}
		for _, resolution := range group {
			conflicts[clarificationKey{resolution.Pattern, resolution.Slot}] = true
		}
	}
	return conflicts
}

func clarificationScalarKind(kind string) string {
	if kind == "entity" {
		return "text"
	}
	return kind
}

func clarificationValueGroupConflict(group []ClarificationResolution) bool {
	if len(group) < 2 {
		return false
	}
	nullAllowed, nonNullAllowed := true, true
	for _, resolution := range group {
		nullAllowed = nullAllowed && (resolution.Null || resolution.Effect.Nulls == "include")
		nonNullAllowed = nonNullAllowed && !resolution.Null && resolution.Effect.Nulls != "only"
	}
	if !nonNullAllowed {
		return !nullAllowed
	}
	if nullAllowed {
		// The conjunction can intentionally select NULL only; this is not
		// an empty predicate and never silently widens null semantics.
		return false
	}
	switch clarificationScalarKind(group[0].Effect.Kind) {
	case "time_window":
		var start, end time.Time
		for _, resolution := range group {
			if resolution.Time == nil {
				return true
			}
			lo, loErr := time.Parse(time.RFC3339, resolution.Time.StartUTC)
			hi, hiErr := time.Parse(time.RFC3339, resolution.Time.EndUTC)
			if loErr != nil || hiErr != nil {
				return true
			}
			if start.IsZero() || lo.After(start) {
				start = lo
			}
			if end.IsZero() || hi.Before(end) {
				end = hi
			}
		}
		return !start.Before(end)
	case "number":
		return clarificationNumericConflict(group)
	default:
		var equal *string
		excluded := map[string]bool{}
		for _, resolution := range group {
			switch resolution.Effect.Operator {
			case "eq":
				if equal != nil && *equal != resolution.Value {
					return true
				}
				value := resolution.Value
				equal = &value
			case "ne":
				excluded[resolution.Value] = true
			}
		}
		return equal != nil && excluded[*equal]
	}
}

type clarificationNumericBound struct {
	value     *big.Rat
	inclusive bool
}

func clarificationNumericConflict(group []ClarificationResolution) bool {
	var lower, upper *clarificationNumericBound
	var excluded []*big.Rat
	setLower := func(value *big.Rat, inclusive bool) {
		if lower == nil || value.Cmp(lower.value) > 0 {
			lower = &clarificationNumericBound{value: value, inclusive: inclusive}
		} else if value.Cmp(lower.value) == 0 {
			lower.inclusive = lower.inclusive && inclusive
		}
	}
	setUpper := func(value *big.Rat, inclusive bool) {
		if upper == nil || value.Cmp(upper.value) < 0 {
			upper = &clarificationNumericBound{value: value, inclusive: inclusive}
		} else if value.Cmp(upper.value) == 0 {
			upper.inclusive = upper.inclusive && inclusive
		}
	}
	for _, resolution := range group {
		value, ok := new(big.Rat).SetString(resolution.Value)
		if !ok {
			return true
		}
		switch resolution.Effect.Operator {
		case "eq":
			setLower(value, true)
			setUpper(value, true)
		case "ne":
			excluded = append(excluded, value)
		case "gt", "gte":
			setLower(value, resolution.Effect.Operator == "gte")
		case "lt", "lte":
			setUpper(value, resolution.Effect.Operator == "lte")
		case "range":
			high, ok := new(big.Rat).SetString(resolution.Upper)
			if !ok || len(resolution.Effect.Bounds) != 2 {
				return true
			}
			setLower(value, resolution.Effect.Bounds[0] == '[')
			setUpper(high, resolution.Effect.Bounds[1] == ']')
		default:
			return true
		}
	}
	if lower == nil || upper == nil {
		return false
	}
	comparison := lower.value.Cmp(upper.value)
	if comparison > 0 || (comparison == 0 && (!lower.inclusive || !upper.inclusive)) {
		return true
	}
	if comparison == 0 {
		for _, value := range excluded {
			if value.Cmp(lower.value) == 0 {
				return true
			}
		}
	}
	return false
}
