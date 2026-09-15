package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestReportingViewerConfigurationBounds(t *testing.T) {
	valid := []ReportingViewer{DefaultReportingViewer(), {MaxMessageBytes: 16384, MaxRows: 1, PageRows: 1, MaxOutputs: 1, MaxPoints: 1}, {MaxMessageBytes: 4 << 20, MaxRows: 1000, PageRows: 1000, MaxOutputs: 64, MaxPoints: 10000}}
	for _, v := range valid {
		if err := v.Validate(); err != nil {
			t.Fatal("valid closed bounds rejected", v, err)
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*ReportingViewer)
	}{
		{"message-low", func(v *ReportingViewer) { v.MaxMessageBytes = 16383 }},
		{"message-high", func(v *ReportingViewer) { v.MaxMessageBytes = 4<<20 + 1 }},
		{"rows-low", func(v *ReportingViewer) { v.MaxRows = 0 }},
		{"rows-high", func(v *ReportingViewer) { v.MaxRows = 1001 }},
		{"page-low", func(v *ReportingViewer) { v.PageRows = 0 }},
		{"page-high", func(v *ReportingViewer) { v.PageRows = v.MaxRows + 1 }},
		{"outputs-low", func(v *ReportingViewer) { v.MaxOutputs = 0 }},
		{"outputs-high", func(v *ReportingViewer) { v.MaxOutputs = 65 }},
		{"points-low", func(v *ReportingViewer) { v.MaxPoints = 0 }},
		{"points-high", func(v *ReportingViewer) { v.MaxPoints = 10001 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := DefaultReportingViewer()
			tc.edit(&v)
			if v.Validate() == nil {
				t.Fatal("unbounded setting accepted", v)
			}
		})
	}
}

func TestReportingDeliveryOperatorExcerpt(t *testing.T) {
	data, err := os.ReadFile("../../examples/chartworks.reporting-delivery.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		MCP       MCP `json:"mcp"`
		Reporting struct {
			Viewer ReportingViewer `json:"viewer"`
		} `json:"reporting"`
		Jobs Jobs `json:"jobs"`
	}
	v.MCP, v.Jobs = DefaultMCP(), DefaultJobs()
	v.Reporting.Viewer = DefaultReportingViewer()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		t.Fatal("unknown or invalid example setting", err)
	}
	if err := ValidateMCP(v.MCP); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJobs(v.Jobs, Auth{}); err != nil {
		t.Fatal(err)
	}
	if err := v.Reporting.Viewer.Validate(); err != nil {
		t.Fatal(err)
	}
	if v.Jobs.Enabled || len(v.Jobs.Credentials) != 0 {
		t.Fatal("excerpt bootstraps authority")
	}
}
