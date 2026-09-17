package semantics

// ClarificationEffectsConflict compares reviewed constraints known by the
// caller to address the same physical field and aggregation level. It does not
// infer mappings or grant authority. Unknown or oversized input fails closed.
func ClarificationEffectsConflict(effects []ClarificationEffect) bool {
	if len(effects) > 64 {
		return true
	}
	for i, a := range effects {
		if a.Kind == "" || a.Operator == "" || a.Nulls == "" {
			return true
		}
		for _, b := range effects[i+1:] {
			if clarificationScalarKind(a.Kind) != clarificationScalarKind(b.Kind) || a.Unit != b.Unit || a.Calendar != b.Calendar || a.TimeZone != b.TimeZone || a.TemporalType != b.TemporalType || a.Nulls == "only" && b.Nulls == "exclude" || b.Nulls == "only" && a.Nulls == "exclude" {
				return true
			}
		}
	}
	return false
}

// ClarificationResolutionGroupConflict applies exact interval/null semantics
// after physical field equivalence has been established by the binding owner.
func ClarificationResolutionGroupConflict(group []ClarificationResolution) bool {
	effects := make([]ClarificationEffect, 0, len(group))
	for _, r := range group {
		if r.Effect == nil {
			return true
		}
		effects = append(effects, *r.Effect)
	}
	return ClarificationEffectsConflict(effects) || clarificationValueGroupConflict(group)
}
