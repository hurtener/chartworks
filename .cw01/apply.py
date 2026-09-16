from pathlib import Path

def edit(path, before, after, count=1):
    p = Path(path)
    text = p.read_text()
    if text.count(before) != count:
        raise SystemExit(f'{path}: mismatched edit anchor')
    p.write_text(text.replace(before, after))

def create(path, text):
    p = Path(path)
    if p.exists():
        raise SystemExit(f'{path}: refusing overwrite')
    p.write_text(text)

create('internal/nlqapi/clarification.go', '''package nlqapi

import (
 "errors"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
)

// clarificationProblem exposes only the reviewed, bounded repair projection.
// Error messages and submitted scalar values never become transport diagnostics.
func clarificationProblem(err error) *semantics.ClarificationProblem {
 var failure *nlqroute.Clarification
 if !errors.As(err, &failure) { return nil }
 return semantics.PublicClarificationProblem(semantics.ClarificationProblem{
  Outcome: failure.Outcome, Reason: failure.Reason,
  Questions: failure.Questions, Fields: failure.Errors,
 })
}
''')
edit('internal/nlqapi/http.go', '''	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})''', '''	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
		Clarification *semantics.ClarificationProblem `json:"clarification,omitempty"`
	}{code, clarificationProblem(err)})''')
edit('internal/nlqapi/mcp.go', 'return mcpserver.Fault{Code: code}', 'return mcpserver.Fault{Code: code, Clarification: clarificationProblem(err)}', 2)
edit('internal/mcpserver/registry.go', '"github.com/hurtener/chartworks/internal/identity"', '"github.com/hurtener/chartworks/internal/identity"\n "github.com/hurtener/chartworks/internal/semantics"')
edit('internal/mcpserver/registry.go', 'type Fault struct {', 'type Fault struct {\n Clarification *semantics.ClarificationProblem `json:"clarification,omitempty"`')
edit('internal/mcpserver/dispatch.go', '"github.com/hurtener/chartworks/internal/identity"', '"github.com/hurtener/chartworks/internal/identity"\n "github.com/hurtener/chartworks/internal/semantics"')
edit('internal/mcpserver/dispatch.go', '''		f.Outcome = "unknown"
''', '''		f.Outcome = "unknown"
		if f.Clarification != nil {
			f.Clarification = semantics.PublicClarificationProblem(*f.Clarification)
		}
''')
edit('internal/mcpserver/dispatch.go', '''		if encodeErr != nil || len(wire)+512 > s.settings.MaxResponseBytes {
			f.Receipt = nil
''', '''		if encodeErr != nil || len(wire)+512 > s.settings.MaxResponseBytes {
			f.Clarification = nil
			f.Receipt = nil
''')
edit('sdk/chartworks/client.go', 'type StatusError struct {', 'type StatusError struct {\n Clarification *ClarificationProblem')
edit('sdk/chartworks/client.go', '''		if path == "/v1/charts/select" {
			rejected.Receipt = readFailureReceipt(resp.Body)
		}
''', '''		if path == "/v1/charts/select" {
			rejected.Receipt = readFailureReceipt(resp.Body)
		} else if strings.HasPrefix(path, "/v1/nlq/") {
			rejected.Clarification = readClarificationProblem(resp.Body)
		}
''')
create('sdk/chartworks/clarification.go', '''package chartworks

import (
 "encoding/json"
 "io"
 "github.com/hurtener/chartworks/internal/gateway"
 "github.com/hurtener/chartworks/internal/semantics"
)

// These aliases preserve the same versioned contract for HTTP, MCP and in-process
// clients. Only the service can resolve them; they never confer read authority.
type ClarificationAnswer = semantics.ClarificationAnswer
type ClarificationValue = semantics.ClarificationValue
type ClarificationTimeInput = semantics.ClarificationTimeInput
type ClarificationNumberInput = semantics.ClarificationNumberInput
type ClarificationProblem = semantics.ClarificationProblem
type ClarificationFieldError = semantics.ClarificationFieldError

// DecodeClarificationProblem validates an isolated repair payload from an HTTP
// error or MCP fault. It never copies arbitrary server error text into Error().
func DecodeClarificationProblem(raw []byte) *ClarificationProblem {
 if _, err := gateway.DecodeJSON(raw, 65536); err != nil { return nil }
 var problem ClarificationProblem
 if json.Unmarshal(raw, &problem) != nil || !semantics.ValidClarificationProblem(problem) { return nil }
 return semantics.PublicClarificationProblem(problem)
}

func readClarificationProblem(body io.Reader) *ClarificationProblem {
 raw, err := io.ReadAll(io.LimitReader(body, 131073))
 if err != nil || len(raw) > 131072 { return nil }
 if _, err := gateway.DecodeJSON(raw, 131072); err != nil { return nil }
 var wrapper struct { Clarification json.RawMessage `json:"clarification"` }
 if json.Unmarshal(raw, &wrapper) != nil { return nil }
 return DecodeClarificationProblem(wrapper.Clarification)
}
''')
create('internal/nlqapi/clarification_test.go', '''package nlqapi

import (
 "encoding/json"
 "net/http/httptest"
 "testing"
 "strings"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
)

func TestClarificationRepairProjection(t *testing.T) {
 err := &nlqroute.Clarification{Outcome: semantics.ClarificationInvalid, Reason: "invalid_date", Errors: []semantics.ClarificationFieldError{{Field:"time.start",Code:"invalid_date",Message:"Ingresá una fecha válida."}}, Questions: []semantics.ClarificationSlotOutcome{{Effect:&semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical:"private-scalar"}}}}}}
 recorder := httptest.NewRecorder()
 failure(recorder, err)
 if recorder.Code != 400 || strings.Contains(recorder.Body.String(), "private-scalar") { t.Fatal("unsafe or misclassified repair response") }
 var out struct { Error string; Clarification *semantics.ClarificationProblem }
 if json.Unmarshal(recorder.Body.Bytes(), &out) != nil || out.Error != "invalid_request" || out.Clarification == nil || out.Clarification.Fields[0].Message != "Ingresá una fecha válida." { t.Fatal("localized field contract lost") }
 if len(err.Questions[0].Effect.Values) != 1 { t.Fatal("projection mutated caller") }
}
''')
create('sdk/chartworks/clarification_test.go', '''package chartworks

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
  if r.Header.Get("Authorization") != "Bearer test-authority" { t.Error("missing current authority") }
  w.WriteHeader(400)
  _, _ = w.Write([]byte(`{"error":"invalid_request","clarification":{"outcome":"invalid","reason":"invalid_date","fields":[{"field":"time.start","code":"invalid_date","message":"Ingresá una fecha válida."}]}}`))
 }))
 defer server.Close()
 calls := 0
 client, err := New(server.URL, server.Client(), func(context.Context)(string,error){calls++; return "test-authority", nil})
 if err != nil {t.Fatal(err)}
 var out any
 err = client.call(context.Background(), "POST", "/v1/nlq/plans", "", struct{}{}, &out)
 var status *StatusError
 if !errors.As(err, &status) || status.Status != 400 || status.Clarification == nil || calls != 1 {t.Fatal("typed failure lost or silently replayed")}
 if status.Clarification.Fields[0].Message != "Ingresá una fecha válida." || strings.Contains(err.Error(), "Ingresá") {t.Fatal("diagnostic boundary violated")}
 raw, _ := json.Marshal(status.Clarification)
 if DecodeClarificationProblem(raw) == nil {t.Fatal("MCP isolated repair payload cannot roundtrip")}
 for _, raw := range []string{`{}`, `{"outcome":"satisfied"}`, `{"outcome":"invalid","outcome":"missing"}`, strings.Repeat("x", 65537)} {
  if DecodeClarificationProblem([]byte(raw)) != nil {t.Fatal("accepted invalid remote diagnostic")}
 }
}
''')
