package chartworks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppTypedClientUsesCanonicalRoutesAndCAS(t *testing.T) {
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-current" {
			t.Error("wrong transport")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid body")
		}
		suffix := r.URL.Path[len(reportingapi.AuthoringPath):]
		seen[suffix]++
		if _, ok := body["tenant"]; ok {
			t.Error("client supplied identity")
		}
		w.Header().Set("Content-Type", "application/json")
		if suffix == "save" || suffix == "widget" || suffix == "block_mapping" || suffix == "block_copy" || suffix == "block_validate" {
			if body["expected_version"] != float64(7) {
				t.Error("CAS changed")
			}
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"error":{"code":"conflict"}}`))
			return
		}
		switch suffix {
		case "capabilities":
			_, _ = w.Write([]byte(`{"version":"report-authoring-v1","builder":false,"consumer":true,"can_create":false,"can_open":false,"can_save":false,"can_preview":false,"can_execute":false}`))
		case "drafts":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	calls := 0
	c, err := New(server.URL, server.Client(), func(context.Context) (string, error) { calls++; return "synthetic-current", nil })
	if err != nil {
		t.Fatal(err)
	}
	if out, err := c.ReportCapabilities(t.Context(), ReportAppCapabilitiesRequest{Report: "report"}); err != nil || !out.Consumer {
		t.Fatal(out, err)
	}
	if _, err := c.ReportDrafts(t.Context(), ReportAppDraftListRequest{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.OpenReportDraft(t.Context(), ReportAppReadRequest{Report: "report", Revision: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateManualReport(t.Context(), ReportAppCreateRequest{ID: "report"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveManualReport(t.Context(), ReportAppSaveRequest{Report: "report", ExpectedVersion: 7, Revision: 3}); err == nil {
		t.Fatal("conflict hidden")
	}
	if _, err := c.PatchReportWidget(t.Context(), ReportWidgetPatchRequest{Report: "report", Widget: "heading", ExpectedVersion: 7, Revision: 3}); err == nil {
		t.Fatal("conflict hidden")
	}
	if _, err := c.PreviewManualReport(t.Context(), ReportAppPreviewRequest{Report: "report", Revision: 3, Key: "preview"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecuteManualPreview(t.Context(), ReportAppExecuteRequest{Run: "run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BootstrapReportApp(t.Context(), ReportAppBootstrapRequest{Version: 1, Mode: "plan", Targets: []string{"report"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadReportAppGuide(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadManualChart(t.Context(), ReportAppBlockReadRequest{Block: "block", Revision: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AmendManualChart(t.Context(), ReportAppBlockMappingRequest{Block: "block", Revision: 3, ExpectedVersion: 7}); err == nil {
		t.Fatal("mapping conflict hidden")
	}
	if _, err := c.CopyManualChart(t.Context(), ReportAppBlockCopyRequest{Block: "block", NewBlock: "private-copy", Revision: 3, ExpectedVersion: 7}); err == nil {
		t.Fatal("copy conflict hidden")
	}
	if _, err := c.ValidateManualChart(t.Context(), ReportAppBlockValidateRequest{Block: "private-copy", Revision: 3, ExpectedVersion: 7}); err == nil {
		t.Fatal("validation conflict hidden")
	}
	if calls != 14 || len(seen) != 14 {
		t.Fatal("missing route or stale bearer", calls, seen)
	}
	for name, n := range seen {
		if n != 1 {
			t.Fatal("automatic retry", name, n)
		}
	}
}
