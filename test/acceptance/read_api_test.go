package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/sourceapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestReadAPIAndSDK(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	x := readExecutor(t, f, nil)
	ctx := context.Background()
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
	h := sourceapi.ExecutionHandler(f.token.verifier, f.validator, x, sourceapi.Handler(f.token.verifier, f.s, f.validator, http.NotFoundHandler()))
	registry, registrationErr := sourceapi.ExecutionAPIRegistry()
	if registrationErr != nil {
		t.Fatal(registrationErr)
	}
	h = assertRegisteredWireSchemas(t, registry, h)
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	input := cw.ReadExecutionRequest{Context: source.ContextID, SQL: `SELECT amount,active FROM analytics.sales ORDER BY id`, Execution: cw.ReadExecutionOptions{Operation: "sdk-read", Number: 1}}
	report, err := client.ExecuteRead(ctx, source.ID, input)
	if err != nil || report.Result == nil || report.Attempt.Status != "succeeded" || string(report.Result.Rows[0][0]) != `"9007199254740993.125"` || string(report.Result.Rows[0][1]) != "true" {
		t.Fatal("exact SDK result", err, report.Attempt)
	}
	if _, err = client.ExecuteRead(ctx, source.ID, input); err == nil {
		t.Fatal("SDK silently replayed an attempt")
	} else {
		var status *cw.StatusError
		if !errors.As(err, &status) || status.Status != 409 {
			t.Fatal("replay status", err)
		}
	}
	before := f.lookups.Load()
	found, err := client.ReadOperation(ctx, "sdk-read")
	if err != nil || found.ID != report.Attempt.ID {
		t.Fatal("lost-response lookup", err)
	}
	receipt, err := client.ReadExecution(ctx, found.ID)
	if err != nil || receipt.Status != "succeeded" {
		t.Fatal("attempt receipt", err)
	}
	cancel, err := client.CancelRead(ctx, found.ID)
	if err != nil || cancel.Attempt.Status != "succeeded" {
		t.Fatal("terminal cancellation rewrote history", err)
	}
	reconciled, err := client.ReconcileRead(ctx, found.ID)
	if err != nil || reconciled.Attempt.Status != "succeeded" {
		t.Fatal("terminal reconciliation", err)
	}
	if f.lookups.Load() != before {
		t.Fatal("terminal metadata operation accessed warehouse")
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-read-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory []sourceapi.Operation
	if json.Unmarshal(raw, &inventory) != nil || !reflect.DeepEqual(inventory, sourceapi.ExecutionRegistry()) {
		t.Fatal("read route manifest drift")
	}
	bare := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), nil), nil)
	for _, op := range inventory {
		path := strings.ReplaceAll(op.Path, "{id}", found.ID)
		if strings.Contains(path, "/sources/") {
			path = strings.ReplaceAll(op.Path, "{id}", source.ID)
		}
		for _, c := range []struct {
			token  string
			status int
		}{{"", 401}, {bare, 403}} {
			r := callProtected(t, h, op.Method, path, c.token, "{}", nil)
			if r.Code != c.status {
				t.Fatal("unprotected execution route", op, r.Code)
			}
		}
	}
	if f.lookups.Load() != before {
		t.Fatal("denied route resolved credentials")
	}
	input.Parameters = []cw.ReadParameter{}
	encoded, _ := json.Marshal(input)
	for _, body := range []string{`{}`, `null`, strings.Replace(string(encoded), `"execution":{`, `"execution":{"tenant":"foreign",`, 1), strings.Replace(string(encoded), `"attempt":1`, `"attempt":1,"attempt":2`, 1), strings.Replace(string(encoded), `"rows":0`, `"ROWS":0`, 1), strings.Replace(string(encoded), `"preview":false`, `"preview":null`, 1)} {
		r := callProtected(t, h, "POST", "/v1/sources/sales/execute", token, body, nil)
		if r.Code != 400 {
			t.Fatal("ambiguous execution body", r.Code)
		}
	}
	for _, request := range []struct {
		method, path, body string
		headers            map[string]string
	}{
		{"POST", "/v1/sources/sales/execute?skip_validation=true", string(encoded), nil},
		{"POST", "/v1/sources/sales/execute", string(encoded), map[string]string{"Content-Encoding": "gzip"}},
		{"POST", "/v1/sources/sales/execute", string(encoded), map[string]string{"Idempotency-Key": "different-key"}},
		{"GET", "/v1/read-executions/" + found.ID, "{}", nil},
	} {
		if r := callProtected(t, h, request.method, request.path, token, request.body, request.headers); r.Code != 400 {
			t.Fatal("ambiguous transport", r.Code)
		}
	}
	if r := callProtected(t, h, "DELETE", "/v1/read-executions/"+found.ID, token, "", nil); r.Code != 405 {
		t.Fatal("unknown method")
	}
	if r := callProtected(t, h, "GET", "/unregistered", token, "", nil); r.Code != 404 {
		t.Fatal("route fallthrough")
	}
	for _, v := range []any{report.Attempt, receipt, found} {
		data, _ := json.Marshal(v)
		for _, secret := range []string{sourcePassword, f.role, "CHARTWORKS_SOURCE_READ", "SELECT amount", "9007199254740993.125"} {
			if strings.Contains(string(data), secret) {
				t.Fatal("private content in attempt metadata")
			}
		}
	}
	before = f.lookups.Load()
	invalidBounds := input
	invalidBounds.Execution.Operation = "oversized-http"
	invalidBounds.Execution.Rows = 100001
	_, err = client.ExecuteRead(ctx, source.ID, invalidBounds)
	var rejected *cw.StatusError
	if !errors.As(err, &rejected) || rejected.Status != 413 || f.lookups.Load() != before {
		t.Fatal("HTTP ceilings did not reject before native planning", err)
	}
	// The execution endpoint has a larger explicit SDK bound than ordinary metadata.
	if _, err = f.admin.Exec(ctx, `UPDATE analytics.sales SET name=repeat('z',700000)`); err != nil {
		t.Fatal(err)
	}
	input.SQL = `SELECT name FROM analytics.sales ORDER BY id`
	input.Execution.Operation = "sdk-large"
	large, err := client.ExecuteRead(ctx, source.ID, input)
	if err != nil || large.Result == nil || large.Result.Bytes <= 1<<20 || len(large.Result.Rows) != 2 {
		t.Fatal("bounded large SDK response", err, large.Attempt)
	}
	for _, call := range []func() error{
		func() error { _, e := client.ReadOperation(ctx, "../bad"); return e }, func() error { _, e := client.ReadExecution(ctx, "../bad"); return e }, func() error { _, e := client.CancelRead(ctx, "../bad"); return e }, func() error { _, e := client.ReconcileRead(ctx, "../bad"); return e }, func() error { _, e := client.ExecuteRead(ctx, "../bad", input); return e },
	} {
		if call() == nil {
			t.Fatal("invalid SDK coordinates")
		}
	}
	if r := callProtected(t, sourceapi.ExecutionHandler(nil, nil, nil, http.NotFoundHandler()), "GET", "/v1/read-operations/a", token, "", nil); r.Code != 404 {
		t.Fatal("missing verifier exposed routes")
	}
	if r := callProtected(t, sourceapi.ExecutionHandler(f.token.verifier, nil, nil, http.NotFoundHandler()), "GET", "/v1/read-operations/a", token, "", nil); r.Code != 404 {
		t.Fatal("unavailable execution advertised")
	}
}
