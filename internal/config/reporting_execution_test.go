package config

import (
	"strings"
	"testing"
	"time"
)

func TestReportingExecutionBounds(t *testing.T) {
	base := DefaultReportingExecution()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	if time.Duration(base.Retention) != 7*24*time.Hour || time.Duration(base.PreviewRetention) != 24*time.Hour {
		t.Fatal("retention defaults changed")
	}
	for _, change := range []func(*ReportingExecution){
		func(c *ReportingExecution) { c.Retention = 0 },
		func(c *ReportingExecution) { c.Retention = Duration(91 * 24 * time.Hour) },
		func(c *ReportingExecution) { c.PreviewRetention = 0 },
		func(c *ReportingExecution) { c.PreviewRetention = Duration(8 * 24 * time.Hour) },
		func(c *ReportingExecution) { c.Retention = Duration(time.Hour) },
		func(c *ReportingExecution) { c.Timeout = 0 },
		func(c *ReportingExecution) { c.Timeout = Duration(2 * time.Minute) },
		func(c *ReportingExecution) { c.MaxRows = 0 },
		func(c *ReportingExecution) { c.MaxRows = 10001 },
		func(c *ReportingExecution) { c.MaxResultBytes = 1023 },
		func(c *ReportingExecution) { c.MaxResultBytes = 5 << 20 },
		func(c *ReportingExecution) { c.MaxArtifactBytes = 1024 },
		func(c *ReportingExecution) { c.MaxArtifactBytes = 65 << 20 },
		func(c *ReportingExecution) { c.MaxTenantBytes = 0 },
		func(c *ReportingExecution) { c.MaxTenantBytes = 1 << 41 },
		func(c *ReportingExecution) { c.MaxRequests = 0 },
		func(c *ReportingExecution) { c.MaxRequests = 100001 },
		func(c *ReportingExecution) { c.PageRows = 0 },
		func(c *ReportingExecution) { c.PageRows = 1001 },
		func(c *ReportingExecution) { c.MaxReuseAge = 0 },
		func(c *ReportingExecution) { c.MaxReuseAge = Duration(25 * time.Hour) },
		func(c *ReportingExecution) { c.NarrativeCalls = 0 },
		func(c *ReportingExecution) { c.NarrativeCalls = 9 },
		func(c *ReportingExecution) { c.NarrativeTokens = 63 },
		func(c *ReportingExecution) { c.NarrativeTokens = 1 << 20 },
		func(c *ReportingExecution) { c.NarrativeTimeout = 0 },
		func(c *ReportingExecution) { c.NarrativeTimeout = Duration(2 * time.Minute) },
	} {
		c := base
		change(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("accepted invalid limits: %+v", c)
		}
	}
}

func TestReportingNarrativeModelPolicyVersion(t *testing.T) {
	for _, version := range []string{"", "narrative-policy-v1", strings.Repeat("v", 256)} {
		limits := DefaultReportingExecution()
		limits.ModelVersion = version
		if err := limits.Validate(); err != nil {
			t.Fatal(version, err)
		}
	}
	for _, version := range []string{"with space", "line\nbreak", "\x00", "\x7f", strings.Repeat("v", 257)} {
		limits := DefaultReportingExecution()
		limits.ModelVersion = version
		if err := limits.Validate(); err == nil {
			t.Fatal("invalid policy version accepted", version)
		}
	}
}
