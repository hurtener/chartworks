package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClarificationHTTPErrorRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-authority" {
			t.Error("missing current authority")
		}
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_request","clarification":{"outcome":"invalid","reason":"invalid_date","fields":[{"field":"time.start","code":"invalid_date","message":"Ingresá una fecha válida."}]}}`))
	}))
	defer server.Close()
	calls := 0
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { calls++; return "test-authority", nil })
	if err != nil {
		t.Fatal(err)
	}
	var out any
	err = client.call(context.Background(), "POST", "/v1/nlq/plans", "", struct{}{}, &out)
	var status *StatusError
	if !errors.As(err, &status) || status.Status != 400 || status.Clarification == nil || calls != 1 {
		t.Fatal("typed failure lost or silently replayed")
	}
	if status.Clarification.Fields[0].Message != "Ingresá una fecha válida." || strings.Contains(err.Error(), "Ingresá") {
		t.Fatal("diagnostic boundary violated")
	}
	raw, _ := json.Marshal(status.Clarification)
	if DecodeClarificationProblem(raw) == nil {
		t.Fatal("MCP isolated repair payload cannot roundtrip")
	}
	for _, raw := range []string{`{}`, `{"outcome":"satisfied"}`, `{"outcome":"invalid","outcome":"missing"}`, strings.Repeat("x", 65537)} {
		if DecodeClarificationProblem([]byte(raw)) != nil {
			t.Fatal("accepted invalid remote diagnostic")
		}
	}
}
