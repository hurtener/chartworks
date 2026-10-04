package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppLifecycleTypedClientPreservesCoordinatesAndState(t *testing.T) {
	digest := strings.Repeat("a", 64)
	requests := map[string]any{
		"lifecycle":         ReportAppLifecycleRequest{Report: "report", Revision: 2},
		"block_publish":     ReportAppBlockPublishRequest{Block: "chart", ExpectedVersion: 3, Revision: 2, Digest: digest, Evidence: "evidence"},
		"rebind_published":  ReportAppRebindPublishedRequest{Report: "report", ExpectedVersion: 4, Revision: 2, Digest: digest, Widgets: []ReportAppPublishedWidget{{Widget: "selected", Block: "chart", Revision: 2, Digest: digest}}},
		"report_transition": ReportAppReportTransitionRequest{Report: "report", ExpectedVersion: 4, Revision: 2, Operation: "reject", Note: "Explicit review note"},
	}
	block := BlockState{ID: "chart", Topic: "topic", Version: 4, PublishedRevision: 2}
	report := DocumentState{Kind: "report", ID: "report", Version: 5, LatestRevision: 3, DraftRevision: 3, ReviewRevision: 2, PublishedRevision: 1}
	view := ReportAppLifecycleView{Version: "chartworks-authoring-v1", Stage: "review", Blocks: []ReportAppBlockLifecycle{{Block: reporting.AuthoringBlockMetadata{State: block, Revision: 2, Digest: digest, Actor: "original-author", Private: false, Outputs: []reporting.AuthoringBlockOutput{{ID: "selected", Kind: "chart", Editable: true}, {ID: "unselected", Kind: "table", Editable: true}}}, PublicationScope: "entire_revision", AudienceEffect: "existing_authorized_readers", ValidationFresh: false, CanPublish: false}}}
	responses := map[string]any{"lifecycle": view, "block_publish": block, "rebind_published": report, "report_transition": report}
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	contracts := map[string]api.Definition{}
	for _, definition := range registry.Definitions() {
		contracts[strings.TrimPrefix(definition.Path, reportingapi.AuthoringPath)] = definition
	}
	for _, statusCode := range []int{200, 409, 503} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			seen := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer synthetic-current" || r.Header.Get("Idempotency-Key") != "" || r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, reportingapi.AuthoringPath) {
					t.Error("wrong lifecycle authority/operation transport")
				}
				suffix := strings.TrimPrefix(r.URL.Path, reportingapi.AuthoringPath)
				seen[suffix]++
				body, err := io.ReadAll(r.Body)
				definition, known := contracts[suffix]
				if err != nil || !known || definition.Request.Validate(body, reportingapi.MaxBodyBytes) != nil {
					t.Error("SDK request differs from shared lifecycle schema", suffix, err)
				}
				want, _ := json.Marshal(requests[suffix])
				var gotObject, wantObject any
				if json.Unmarshal(body, &gotObject) != nil || json.Unmarshal(want, &wantObject) != nil || !reflect.DeepEqual(gotObject, wantObject) {
					t.Error("exact revision/evidence/widget/note changed", suffix)
				}
				w.Header().Set("Content-Type", "application/json")
				if statusCode != http.StatusOK {
					w.WriteHeader(statusCode)
					code := "unavailable"
					if statusCode == http.StatusConflict {
						code = "conflict"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"error": code, "detail": "private-lifecycle-canary"})
					return
				}
				_ = json.NewEncoder(w).Encode(responses[suffix])
			}))
			defer server.Close()
			tokens := 0
			client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { tokens++; return "synthetic-current", nil })
			if err != nil {
				t.Fatal(err)
			}
			calls := []struct {
				name string
				call func() (any, error)
			}{
				{"lifecycle", func() (any, error) {
					return client.InspectManualLifecycle(t.Context(), requests["lifecycle"].(ReportAppLifecycleRequest))
				}},
				{"block_publish", func() (any, error) {
					return client.PublishManualChart(t.Context(), requests["block_publish"].(ReportAppBlockPublishRequest))
				}},
				{"rebind_published", func() (any, error) {
					return client.RebindPublishedManualCharts(t.Context(), requests["rebind_published"].(ReportAppRebindPublishedRequest))
				}},
				{"report_transition", func() (any, error) {
					return client.TransitionManualReport(t.Context(), requests["report_transition"].(ReportAppReportTransitionRequest))
				}},
			}
			for _, call := range calls {
				got, err := call.call()
				if statusCode != http.StatusOK {
					var status *StatusError
					if !errors.As(err, &status) || status.Status != statusCode || strings.Contains(err.Error(), "canary") {
						t.Fatal("conflict/unknown not preserved safely", call.name, err)
					}
					continue
				}
				if err != nil || !reflect.DeepEqual(got, responses[call.name]) {
					t.Fatal("entire output disclosure, false hints or independent lifecycle pointers changed", call.name, got, err)
				}
			}
			if tokens != 4 || len(seen) != 4 {
				t.Fatal("missing route, stale bearer or automatic follow-up", tokens, seen)
			}
			for name, count := range seen {
				if count != 1 {
					t.Fatal("automatic lifecycle retry", name, count)
				}
			}
		})
	}
}

func TestReportAppLifecycleGenericCallsRejectAutomaticReplay(t *testing.T) {
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	document, err := registry.OpenAPI("Synthetic manual lifecycle replay contract", "1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" || r.Method != http.MethodGet {
			t.Error("unsafe replay reached mutation transport")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(document)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-current", nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"reporting_authoring_lifecycle_v1", "reporting_authoring_block_publish_v1", "reporting_authoring_rebind_published_v1", "reporting_authoring_report_transition_v1"} {
		if _, err := client.Invoke(t.Context(), id, CallOptions{Attempts: 2}); !errors.Is(err, ErrUnsafeRetry) {
			t.Fatal("automatic lifecycle replay admitted", id, err)
		}
	}
}
