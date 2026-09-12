package reporting

import (
	"github.com/hurtener/chartworks/internal/calendars"
	"slices"
	"strings"
	"time"
)

func namedZone(name string) (*time.Location, error) {
	if len(name) == 0 || len(name) > 128 || name == "Local" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00\r\n") {
		return nil, ErrInvalid
	}
	zone, err := calendars.Location(name)
	if err != nil {
		return nil, ErrInvalid
	}
	return zone, nil
}

func windowValid(w Window) bool {
	return !w.Start.IsZero() && !w.End.IsZero() && w.Start.Year() >= 1 && w.End.Year() <= 9999 && w.Start.Before(w.End) && w.End.Sub(w.Start) <= 36625*24*time.Hour
}

func validatePeriod(p Period) error {
	if !slices.Contains([]string{"reject", "earlier"}, p.DSTPolicy) || !slices.Contains([]string{"clamp", "reject"}, p.MonthPolicy) {
		return ErrInvalid
	}
	unitCount := func() bool {
		return slices.Contains([]string{"hour", "day", "week", "month", "quarter", "year"}, p.Unit) && p.Count > 0 && p.Count <= 3660
	}
	switch p.Mode {
	case "explicit":
		if p.Start == "" || p.End == "" || p.FromDate != "" || p.Unit != "" || p.Count != 0 || p.FirstOccurrence != "" {
			return ErrInvalid
		}
	case "from_date":
		if p.FromDate == "" || p.Start != "" || p.End != "" || p.Unit != "" || p.Count != 0 || p.FirstOccurrence != "" {
			return ErrInvalid
		}
	case "previous", "rolling":
		if !unitCount() || p.Start != "" || p.End != "" || p.FromDate != "" || p.FirstOccurrence != "" {
			return ErrInvalid
		}
	case "schedule_window":
		if p.Start != "" || p.End != "" || !slices.Contains([]string{"reject", "from_date", "previous"}, p.FirstOccurrence) {
			return ErrInvalid
		}
		if p.FirstOccurrence == "previous" {
			if !unitCount() || p.FromDate != "" {
				return ErrInvalid
			}
		} else if p.Unit != "" || p.Count != 0 || p.FirstOccurrence == "from_date" && p.FromDate == "" || p.FirstOccurrence == "reject" && p.FromDate != "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	for _, value := range []string{p.Start, p.End, p.FromDate} {
		if len(value) > 64 {
			return ErrInvalid
		}
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

// civil resolves a civil timestamp explicitly. Gaps are rejected, while folds
// require the declared earlier policy. We test candidate offsets around the
// transition, including non-hour DST changes and whole-day date-line changes.
func civil(y int, m time.Month, d, h, minute, second, nanos int, zone *time.Location, policy string) (time.Time, error) {
	if y < 1 || y > 9999 {
		return time.Time{}, ErrInvalid
	}
	wall := time.Date(y, m, d, h, minute, second, nanos, time.UTC)
	if wall.Year() != y || wall.Month() != m || wall.Day() != d {
		return time.Time{}, ErrInvalid
	}
	guess := time.Date(y, m, d, h, minute, second, nanos, zone)
	offsets := map[int]bool{}
	for _, delta := range []time.Duration{-72 * time.Hour, -36 * time.Hour, 0, 36 * time.Hour, 72 * time.Hour} {
		_, offset := guess.Add(delta).Zone()
		offsets[offset] = true
	}
	candidates := []time.Time{}
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second).In(zone)
		if candidate.Year() == y && candidate.Month() == m && candidate.Day() == d && candidate.Hour() == h && candidate.Minute() == minute && candidate.Second() == second && candidate.Nanosecond() == nanos {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 || len(candidates) > 1 && policy != "earlier" {
		return time.Time{}, ErrInvalid
	}
	slices.SortFunc(candidates, func(a, b time.Time) int { return a.Compare(b) })
	return candidates[0], nil
}

func endpoint(value string, zone *time.Location, policy string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, ErrInvalid
	}
	return civil(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone, policy)
}

func periodFloor(t time.Time, unit, policy string, zone *time.Location) (time.Time, error) {
	t = t.In(zone)
	y, m, d := t.Date()
	hour := 0
	switch unit {
	case "hour":
		hour = t.Hour()
	case "day":
	case "week":
		delta := (int(t.Weekday()) + 6) % 7
		date := time.Date(y, m, d-delta, 0, 0, 0, 0, time.UTC)
		y, m, d = date.Date()
	case "month":
		d = 1
	case "quarter":
		m, d = time.Month((int(m)-1)/3*3+1), 1
	case "year":
		m, d = time.January, 1
	default:
		return time.Time{}, ErrInvalid
	}
	return civil(y, m, d, hour, 0, 0, 0, zone, policy)
}

func shiftCalendar(t time.Time, unit string, count int, zone *time.Location, dst, monthPolicy string) (time.Time, error) {
	if unit == "hour" {
		return t.Add(time.Duration(count) * time.Hour), nil
	}
	t = t.In(zone)
	y, m, d := t.Date()
	switch unit {
	case "day", "week":
		if unit == "week" {
			count *= 7
		}
		date := time.Date(y, m, d+count, 0, 0, 0, 0, time.UTC)
		y, m, d = date.Date()
	case "month", "quarter", "year":
		if unit == "quarter" {
			count *= 3
		}
		if unit == "year" {
			count *= 12
		}
		first := time.Date(y, m+time.Month(count), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		if d > last {
			if monthPolicy != "clamp" {
				return time.Time{}, ErrInvalid
			}
			d = last
		}
		y, m = first.Year(), first.Month()
	default:
		return time.Time{}, ErrInvalid
	}
	return civil(y, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), zone, dst)
}

func resolvePeriod(p Period, resolution Resolution, zone *time.Location) (Window, error) {
	var w Window
	if validatePeriod(p) != nil {
		return w, ErrInvalid
	}
	if p.Mode == "schedule_window" {
		if resolution.ScheduleWindow != nil {
			w = *resolution.ScheduleWindow
			if !windowValid(w) {
				return Window{}, ErrInvalid
			}
			return Window{Start: w.Start.UTC(), End: w.End.UTC()}, nil
		}
		switch p.FirstOccurrence {
		case "reject":
			return w, ErrInvalid
		case "from_date":
			p.Mode = "from_date"
		case "previous":
			p.Mode = "previous"
		}
		p.FirstOccurrence = ""
	}
	var err error
	switch p.Mode {
	case "explicit":
		w.Start, err = endpoint(p.Start, zone, p.DSTPolicy)
		if err == nil {
			w.End, err = endpoint(p.End, zone, p.DSTPolicy)
		}
	case "from_date":
		w.Start, err = endpoint(p.FromDate, zone, p.DSTPolicy)
		w.End = resolution.At
	case "previous":
		w.End, err = periodFloor(resolution.At, p.Unit, p.DSTPolicy, zone)
		if err == nil {
			w.Start, err = shiftCalendar(w.End, p.Unit, -p.Count, zone, p.DSTPolicy, p.MonthPolicy)
		}
	case "rolling":
		w.End = resolution.At
		w.Start, err = shiftCalendar(w.End, p.Unit, -p.Count, zone, p.DSTPolicy, p.MonthPolicy)
	default:
		return w, ErrInvalid
	}
	if err != nil || !windowValid(w) {
		return Window{}, ErrInvalid
	}
	return Window{Start: w.Start.UTC(), End: w.End.UTC()}, nil
}
