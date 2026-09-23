package nlqexec

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"time"
)

func calendarGrainWord(s string) string {
	switch s {
	case "day", "día", "dia":
		return "day"
	case "month", "mes":
		return "month"
	case "quarter", "trimestre":
		return "quarter"
	case "year", "año", "ano":
		return "year"
	default:
		return ""
	}
}
func compileCalendarBucket(d grainDimension, col semantics.Column, grain string) (exec.AnalyticalBucket, error) {
	bad := func() (exec.AnalyticalBucket, error) {
		return exec.AnalyticalBucket{}, analyticalUnsupported("analytical_grain_unsupported")
	}
	if d.role != semantics.DimensionTemporal || d.temporal == nil || d.temporal.Calendar != "gregorian" || len(d.filters) != 0 {
		return bad()
	}
	allowed := false
	for _, g := range d.temporal.Grains {
		if string(g) == grain {
			allowed = true
		}
	}
	if !allowed {
		return bad()
	}
	b := exec.AnalyticalBucket{Column: col.SourceName, Grain: grain, Calendar: "gregorian"}
	switch exec.AnalyticalCalendarKind(col.NativeType, col.Category) {
	case "date", "civil":
	case "instant":
		b.Timezone = d.temporal.Timezone
		if b.Timezone == "" || b.Timezone == "Local" || len(b.Timezone) > 128 {
			return bad()
		}
		if _, err := time.LoadLocation(b.Timezone); err != nil {
			return bad()
		}
	default:
		return bad()
	}
	return b, nil
}
