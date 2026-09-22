package semantics

import (
	"testing"
	"time"
)

func TestClarificationCalendarTransitions(t *testing.T) {
	for _, tc := range []struct {
		name, zone, start, end string
		accepted               bool
	}{
		{"ordinary-day", "America/Argentina/Buenos_Aires", "2026-01-02", "2026-01-03", true},
		{"leap-day", "UTC", "2024-02-29", "2024-03-01", true},
		{"missing-civil-day", "Pacific/Apia", "2011-12-30", "2012-01-01", false},
		{"ambiguous-midnight", "America/Havana", "2025-11-02", "2025-11-03", false},
		{"short-day-with-unambiguous-boundaries", "America/New_York", "2025-03-09", "2025-03-10", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot := cw01TimeSlot()
			slot.Effect.TimeZone = tc.zone
			slot.Effect.Grains = []string{"day"}
			resolution, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Start: tc.start, End: tc.end, Grain: "day", Calendar: "gregorian", TimeZone: tc.zone}}, "en")
			if (err == nil) != tc.accepted {
				t.Fatalf("calendar acceptance: %v", err)
			}
			if !tc.accepted {
				if resolution.Time != nil {
					t.Fatal("invalid boundary produced partial resolution")
				}
				return
			}
			if resolution.Time.BoundaryPolicy != ClarificationTimeBoundaryPolicy || resolution.Time.Bounds != "[)" {
				t.Fatal("boundary policy missing")
			}
			if tc.name == "short-day-with-unambiguous-boundaries" {
				start, e1 := time.Parse(time.RFC3339, resolution.Time.StartUTC)
				end, e2 := time.Parse(time.RFC3339, resolution.Time.EndUTC)
				if e1 != nil || e2 != nil || end.Sub(start) != 23*time.Hour {
					t.Fatal("calendar day coerced to 24 elapsed hours")
				}
			}
		})
	}
}

func FuzzClarificationCalendar(f *testing.F) {
	for _, seed := range []string{"2026-01-01", "2024-02-29", "2025-11-02", "2011-12-30", "2026-02-30", "", "9999-12-31"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, start string) {
		slot := cw01TimeSlot()
		slot.Effect.Grains = []string{"day"}
		slot.Effect.TimeZone = "UTC"
		resolution, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Start: start, End: "2027-01-01", Grain: "day", Calendar: "gregorian", TimeZone: "UTC"}}, "en")
		if err != nil {
			if resolution.Time != nil {
				t.Fatal("failed parse returned a time constraint")
			}
			return
		}
		if resolution.Time == nil || resolution.Time.LocalStart != start || resolution.Time.BoundaryPolicy != ClarificationTimeBoundaryPolicy {
			t.Fatal("calendar parser lost explicit input")
		}
		roundtrip, problem := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Start: resolution.Time.LocalStart, End: resolution.Time.LocalEnd, Grain: "day", Calendar: resolution.Time.Calendar, TimeZone: resolution.Time.TimeZone}}, "es")
		if problem != nil || *roundtrip.Time != *resolution.Time {
			t.Fatal("canonical time cannot roundtrip independently of locale")
		}
	})
}
