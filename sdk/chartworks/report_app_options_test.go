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
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppOptionTypedClientPreservesCoordinatesAndState(t *testing.T) {
	const operation = "option:1791093600:0123456789abcdef0123456789abcdef"
	dataset := ReportAppOptionTarget{Dataset: &ReportAppDatasetOptionTarget{NewBlock: "chart", Topic: BlockTopicPin{Topic: "topic", Version: "v1", Digest: strings.Repeat("a", 64)}, Dataset: "sales", Dimension: "region"}}
	report := ReportAppOptionTarget{Report: &ReportAppReportOptionTarget{Policy: "published", Report: "report", Revision: 2, Digest: strings.Repeat("b", 64), Page: "page", Filter: "region"}}
	requests := map[string]any{
		"dataset_options": ReportAppOptionRequest{Target: dataset, Operation: operation, Search: "East_%", Cursor: "", Limit: 20, Locale: "en-US"},
		"report_options":  ReportAppOptionRequest{Target: report, Operation: operation, Search: "East_%", Cursor: "opaque-cursor", Limit: 20, Locale: "en-US"},
		"option_status":   ReportAppOptionReference{Target: report, Operation: operation},
		"option_control":  ReportAppOptionControlRequest{Target: report, Operation: operation, Action: "reconcile"},
	}
	responses := map[string]string{
		"dataset_options": `{"operation":"` + operation + `","input_digest":"digest","status":"completed","values_available":true,"new_operation_allowed":true,"source_revision":1,"options":[],"complete":true}`,
		"report_options":  `{"operation":"` + operation + `","input_digest":"digest","status":"completed","values_available":true,"new_operation_allowed":true,"source_revision":1,"options":[{"value":"East_%","label":"East_%"}],"next":"next-cursor","complete":false}`,
		"option_status":   `{"operation":"` + operation + `","input_digest":"digest","status":"completed","code":"result_not_retained","values_available":false,"new_operation_allowed":true,"source_revision":1,"options":[],"complete":false}`,
		"option_control":  `{"operation":"` + operation + `","input_digest":"digest","status":"uncertain","code":"execution_outcome_unknown","remote_state":"unknown","values_available":false,"new_operation_allowed":false,"source_revision":1,"options":[],"complete":false}`,
	}
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	contracts := map[string]api.Definition{}
	for _, definition := range registry.Definitions() {
		contracts[strings.TrimPrefix(definition.Path, reportingapi.AuthoringPath)] = definition
	}
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact-results", true: "unknown-no-retry"}[failure], func(t *testing.T) {
			seen := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer synthetic-current" || r.Header.Get("Idempotency-Key") != "" || r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, reportingapi.AuthoringPath) {
					t.Error("wrong authority/operation transport")
				}
				suffix := strings.TrimPrefix(r.URL.Path, reportingapi.AuthoringPath)
				seen[suffix]++
				body, err := io.ReadAll(r.Body)
				definition, known := contracts[suffix]
				if err != nil || !known || definition.Request.Validate(body, reportingapi.MaxBodyBytes) != nil {
					t.Error("SDK request differs from shared closed schema", suffix, err)
				}
				want, _ := json.Marshal(requests[suffix])
				var gotObject, wantObject any
				if json.Unmarshal(body, &gotObject) != nil || json.Unmarshal(want, &wantObject) != nil || !reflect.DeepEqual(gotObject, wantObject) {
					t.Error("operation/policy/cursor/target changed", suffix)
				}
				w.Header().Set("Content-Type", "application/json")
				if failure {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"error":"unavailable","detail":"private-source-canary"}`)
					return
				}
				_, _ = io.WriteString(w, responses[suffix])
			}))
			defer server.Close()
			tokens := 0
			client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { tokens++; return "synthetic-current", nil })
			if err != nil {
				t.Fatal(err)
			}
			calls := []struct {
				name string
				call func() (ReportAppOptionView, error)
			}{
				{"dataset_options", func() (ReportAppOptionView, error) {
					return client.SearchManualDatasetOptions(t.Context(), requests["dataset_options"].(ReportAppOptionRequest))
				}},
				{"report_options", func() (ReportAppOptionView, error) {
					return client.SearchReportFilterOptions(t.Context(), requests["report_options"].(ReportAppOptionRequest))
				}},
				{"option_status", func() (ReportAppOptionView, error) {
					return client.ReadReportAppOptionStatus(t.Context(), requests["option_status"].(ReportAppOptionReference))
				}},
				{"option_control", func() (ReportAppOptionView, error) {
					return client.ControlReportAppOptions(t.Context(), requests["option_control"].(ReportAppOptionControlRequest))
				}},
			}
			for _, call := range calls {
				got, err := call.call()
				if failure {
					var status *StatusError
					if !errors.As(err, &status) || status.Status != http.StatusServiceUnavailable || strings.Contains(err.Error(), "canary") {
						t.Fatal("unknown transport outcome not preserved safely", call.name, err)
					}
					continue
				}
				var want ReportAppOptionView
				if err != nil || json.Unmarshal([]byte(responses[call.name]), &want) != nil || !reflect.DeepEqual(got, want) {
					t.Fatal("empty/lost/unknown option state collapsed", call.name, got, err)
				}
			}
			if tokens != 4 || len(seen) != 4 {
				t.Fatal("missing route, stale bearer or automatic follow-up", tokens, seen)
			}
			for name, count := range seen {
				if count != 1 {
					t.Fatal("automatic option retry", name, count)
				}
			}
		})
	}
}
