package chartworks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppDocumentationTypedClient(t *testing.T) {
	for _, failure := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var in ReportAppDocumentationRequest
			if r.Method != "POST" || r.URL.Path != reportingapi.AuthoringPath+"documentation" || r.Header.Get("Authorization") != "Bearer synthetic" || json.NewDecoder(r.Body).Decode(&in) != nil || in.URI != "chartworks://report_app/docs/workflows/v1" {
				t.Error("documentation request changed")
			}
			if failure {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"error":"unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{"reference":{"uri":"chartworks://report_app/docs/workflows/v1","name":"Workflow","description":"Public contract fixture","mime_type":"text/markdown","version":1,"sha256":"digest","bytes":8,"document_ref":"docs/contracts/fixture.md"},"text":"# Public"}`))
		}))
		client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic", nil })
		if err != nil {
			t.Fatal(err)
		}
		out, err := client.ReadReportAppDocumentation(t.Context(), ReportAppDocumentationRequest{URI: "chartworks://report_app/docs/workflows/v1"})
		server.Close()
		if calls != 1 || failure != (err != nil) || !failure && (out.Text != "# Public" || out.Reference.SHA256 != "digest") {
			t.Fatal("typed read or retry contract", calls, out, err)
		}
	}
}
