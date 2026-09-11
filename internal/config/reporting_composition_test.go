package config

import (
	"testing"
	"time"
)

func TestReportingComposition(t *testing.T) {
	defaults := DefaultReportingComposition()
	if defaults.Validate() != nil || defaults.MaxWidgets != 100 || defaults.MaxFilters != 100 || defaults.MaxPages != 100 || defaults.LiveQueries || defaults.SessionBound || defaults.PartialFailure != "fail_closed" {
		t.Fatal("unsafe composition defaults", defaults)
	}
	for _, mutate := range []func(*ReportingComposition){
		func(c *ReportingComposition) { c.MaxWidgets = 101 },
		func(c *ReportingComposition) { c.MaxFilters = 0 },
		func(c *ReportingComposition) { c.MaxPages = 101 },
		func(c *ReportingComposition) { c.MaxTextBytes = 0 },
		func(c *ReportingComposition) { c.MaxDefinitionBytes = 3 << 20 },
		func(c *ReportingComposition) { c.MaxQueries = 0 },
		func(c *ReportingComposition) { c.MaxRetainedBytes = 17 << 20 },
		func(c *ReportingComposition) { c.PartialFailure = "ignore_errors" },
		func(c *ReportingComposition) { c.Timeout = Duration(2 * time.Minute) },
		func(c *ReportingComposition) { c.SessionBound = true },
	} {
		value := defaults
		mutate(&value)
		if value.Validate() == nil {
			t.Fatal("invalid composition settings accepted", value)
		}
	}
	defaults.LiveQueries, defaults.SessionBound = true, true
	if defaults.Validate() != nil {
		t.Fatal("explicit enablement rejected")
	}
}
