package chartworks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportOptionDependenciesTypedClient(t *testing.T) {
	in := ReportOptionDependencyRequest{Mode: "status", Operation: "option:1791250000:" + strings.Repeat("a", 32), Target: ReportAppOptionTarget{Report: &ReportAppReportOptionTarget{Policy: "private_preview", Report: "report", Revision: 1, Digest: strings.Repeat("b", 64), Page: "analysis", Filter: "region"}}}
	want := ReportOptionDependencyManifest{Version: "report-option-dependencies-v1", Target: in.Target, Operation: in.Operation, Original: true, Actions: []string{"reporting.read"}, References: []reporting.ResourceReference{}}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var got ReportOptionDependencyRequest
		if r.Method != "POST" || r.URL.Path != reportingapi.OptionDependencyDiscoveryPath || r.Header.Get("Authorization") != "Bearer synthetic-current" || json.NewDecoder(r.Body).Decode(&got) != nil || !reflect.DeepEqual(got, in) {
			t.Error("changed metadata request")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer server.Close()
	c, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-current", nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.ReportOptionDependencies(t.Context(), in)
	if err != nil || !reflect.DeepEqual(out, want) || calls != 1 {
		t.Fatal(out, err, calls)
	}
}
