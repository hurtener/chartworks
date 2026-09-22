package chartworks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTypedReportingErrorsSuppressCodesWithoutRegisteredInventory(t *testing.T) {
	var body atomic.Value
	body.Store(`{"error":"conflict"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/reporting/run" {
			t.Errorf("unexpected typed route %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, body.Load().(string))
	}))
	defer server.Close()
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-current", nil })
	if err != nil {
		t.Fatal(err)
	}
	request := ReportingDeliveryRunRequest{Target: ReportingTarget{Kind: "report", ID: "report-safe", Revision: 1}, Key: "run-safe"}
	for _, response := range []string{
		`{"error":"conflict"}`,
		`{"error":"stale_validation"}`,
		`{"error":"invented","detail":"PRIVATE_SQL_AND_ROW"}`,
	} {
		body.Store(response)
		_, callErr := client.RunReporting(t.Context(), request)
		var rejected *StatusError
		if !errors.As(callErr, &rejected) || rejected.Status != http.StatusConflict || rejected.Code != "" {
			t.Fatal("typed error exposed an unvalidated code")
		}
		visible := callErr.Error() + fmt.Sprintf("%v %#v", rejected, rejected)
		if strings.Contains(visible, "conflict") || strings.Contains(visible, "stale") || strings.Contains(visible, "invented") || strings.Contains(visible, "PRIVATE") {
			t.Fatal("typed error exposed response content", callErr)
		}
	}
}
