package semantics

import "time"

// ClarificationTimeBoundaryPolicy is part of every canonical time resolution.
// Dates mean local calendar midnights. Gaps and folds are rejected rather than
// delegated to time.Date's unspecified choice of an offset during transitions.
const ClarificationTimeBoundaryPolicy = "reject_missing_or_ambiguous_midnight"

func uniqueClarificationMidnight(candidate time.Time) bool {
	if candidate.Hour() != 0 || candidate.Minute() != 0 || candidate.Second() != 0 || candidate.Nanosecond() != 0 {
		return false
	}
	zone := candidate.Location()
	year, month, day := candidate.Date()
	wall := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	// Adjacent zone periods cover every possible civil offset at this boundary.
	// ZoneBounds makes the work bounded by transitions, not by elapsed seconds.
	// An unusually dense/unsupported transition table fails closed.
	cursor, limit := candidate.Add(-48*time.Hour), candidate.Add(48*time.Hour)
	offsets := map[int]bool{}
	complete := false
	for transitions := 0; transitions < 16; transitions++ {
		_, offset := cursor.In(zone).Zone()
		if offset < -24*60*60 || offset > 24*60*60 {
			return false
		}
		offsets[offset] = true
		_, end := cursor.In(zone).ZoneBounds()
		if end.IsZero() || end.After(limit) {
			complete = true
			break
		}
		if !end.After(cursor) {
			return false
		}
		cursor = end
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
