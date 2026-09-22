package semantics

import "testing"

func TestClarificationRejectsImplicitTimezones(t *testing.T) {
	for _, zone := range []string{"", "Local"} {
		t.Run("policy/"+zone, func(t *testing.T) {
			slot := cw01TimeSlot()
			slot.Effect.TimeZone = zone
			subject, definition := cw01Definition(t, slot)
			if _, err := CompilePublishedRules(subject, definition); err == nil {
				t.Fatal("host-dependent temporal policy compiled")
			}
			for _, locale := range []string{"en", "es"} {
				input := ClarificationValue{Time: &ClarificationTimeInput{
					Start: "2026-01-01", End: "2026-02-01", Grain: "month",
					Calendar: "gregorian", TimeZone: zone,
				}}
				out, err := ResolveClarificationValue(slot, input, locale)
				if err == nil || err.Code != "calendar_mismatch" || err.Message == "" || out.Time != nil || out.Effect != nil {
					t.Fatal("implicit timezone produced a partial resolution or no repair error")
				}
			}
		})
	}
	slot := cw01TimeSlot()
	slot.Effect.TimeZone = "UTC"
	subject, definition := cw01Definition(t, slot)
	if _, err := CompilePublishedRules(subject, definition); err != nil {
		t.Fatal("explicit UTC policy rejected", err)
	}
	out, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{
		Start: "2026-01-01", End: "2026-02-01", Grain: "month", Calendar: "gregorian", TimeZone: "UTC",
	}}, "en")
	if err != nil || out.Time == nil || out.Time.StartUTC != "2026-01-01T00:00:00Z" || out.Time.EndUTC != "2026-02-01T00:00:00Z" {
		t.Fatal("explicit UTC lost exact canonical boundaries", err)
	}
}
