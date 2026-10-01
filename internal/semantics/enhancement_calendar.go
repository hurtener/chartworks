package semantics

// EnhancementCalendarNames is the closed producer vocabulary for the calendar
// currently understood by governed grouping. Recognized display spellings are
// accepted only at this authoring boundary and saved as the canonical token.
// The returned slice is detached; it grants no new calendar interpretation.
func EnhancementCalendarNames() []string {
	return []string{"gregorian", "Gregorian", "GREGORIAN"}
}

func canonicalEnhancementTemporal(value *TemporalPolicy) (*TemporalPolicy, bool) {
	if value == nil {
		return nil, true
	}
	known := false
	for _, name := range EnhancementCalendarNames() {
		known = known || value.Calendar == name
	}
	if !known {
		return nil, false
	}
	copy := *value
	copy.Calendar = "gregorian"
	copy.Grains = append([]TimeGrain(nil), value.Grains...)
	return &copy, true
}
