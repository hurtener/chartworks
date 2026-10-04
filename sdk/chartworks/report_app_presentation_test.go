package chartworks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestManualChartPresentationClientCanonicalUnionAndNoRetry(t *testing.T) {
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-current" || r.Header.Get("Idempotency-Key") != "" {
			t.Error("presentation changed auth/method/replay contract")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		definition, _, matched := registry.Match(r.Method, r.URL.Path)
		if !matched || definition.Request.Validate(raw, reportingapi.MaxBodyBytes) != nil {
			t.Errorf("SDK request disagrees with shared union: %s", raw)
		}
		var object map[string]any
		_ = json.Unmarshal(raw, &object)
		if object["block"] != "source" || object["expected_version"] != float64(7) || object["revision"] != float64(3) || object["digest"] != strings.Repeat("a", 64) || object["output"] != "selected" {
			t.Error("exact source/output/CAS coordinates changed", string(raw))
		}
		if _, ok := object["mapping"]; ok {
			t.Error("presentation request serialized a mapping")
		}
		patch := object["presentation"].(map[string]any)
		set := patch["edits"].([]any)[0].(map[string]any)["set"].(map[string]any)
		if patch["version"] != float64(1) || set["display_label"] != "" || set["fraction_digits"] != float64(0) {
			t.Error("explicit empty/zero patch lost", string(raw))
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "block_copy") {
			if object["new_block"] != "copy" {
				t.Error("copy target changed")
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"unavailable"}`))
			return
		}
		if _, ok := object["new_block"]; ok {
			t.Error("amend can serialize copy-only coordinate")
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict"}`))
	}))
	defer server.Close()
	bearers := 0
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) {
		bearers++
		return "synthetic-current", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, zero := "", 0
	patch := ReportAppPresentationPatch{Version: ReportAppPresentationVersion, Edits: []ReportAppColumnPresentationEdit{{Column: "value", Set: &ReportAppColumnPresentationSet{DisplayLabel: &empty, FractionDigits: &zero}}}}
	if _, err := client.AmendManualChartPresentation(t.Context(), ReportAppBlockPresentationRequest{Block: "source", ExpectedVersion: 7, Revision: 3, Digest: strings.Repeat("a", 64), Output: "selected", Presentation: patch}); err == nil {
		t.Fatal("presentation CAS conflict hidden")
	}
	if _, err := client.CopyManualChartPresentation(t.Context(), ReportAppBlockPresentationCopyRequest{Block: "source", ExpectedVersion: 7, Revision: 3, Digest: strings.Repeat("a", 64), Output: "selected", NewBlock: "copy", Presentation: patch}); err == nil {
		t.Fatal("presentation copy uncertainty hidden")
	}
	if bearers != 2 || len(seen) != 2 || seen[reportingapi.AuthoringPath+"block_mapping"] != 1 || seen[reportingapi.AuthoringPath+"block_copy"] != 1 {
		t.Fatal("new route or automatic retry", bearers, seen)
	}
}

func TestManualChartLegacySDKSourceCompatibility(t *testing.T) {
	// These assignments intentionally compile without pointer conversions or
	// changing existing caller method signatures.
	mapping := ReportAppChartMapping{Kind: ChartBar, Options: ChartOptions{}}
	legacy := ReportAppBlockMappingRequest{Mapping: mapping}
	copyRequest := ReportAppBlockCopyRequest{Mapping: mapping}
	var oldMapping reporting.AuthoringBlockMappingRequest = legacy
	var oldCopy reporting.AuthoringBlockCopyRequest = copyRequest
	var amend func(context.Context, ReportAppBlockMappingRequest) (ReportAppBlockView, error) = (&Client{}).AmendManualChart
	var copyMethod func(context.Context, ReportAppBlockCopyRequest) (ReportAppBlockView, error) = (&Client{}).CopyManualChart
	if !reflect.DeepEqual(oldMapping.Mapping, mapping) || !reflect.DeepEqual(oldCopy.Mapping, mapping) || amend == nil || copyMethod == nil {
		t.Fatal("legacy aliases/methods changed")
	}
	reset := ReportAppPresentationPatch{Version: ReportAppPresentationVersion, Edits: []ReportAppColumnPresentationEdit{{Column: "value", Reset: []ReportAppPresentationField{ReportAppPresentationDisplayLabel, ReportAppPresentationFractionDigits}}}}
	raw, err := json.Marshal(reset)
	if err != nil || string(raw) != `{"version":1,"edits":[{"column":"value","reset":["display_label","fraction_digits"]}]}` {
		t.Fatal("SDK reset wire changed", string(raw), err)
	}
}
