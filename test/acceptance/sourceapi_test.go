package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSourceAPIAndSDK(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := context.Background()
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
	h := sourceapi.Handler(f.token.verifier, f.s, f.validator, http.NotFoundHandler())
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateSource(ctx, cw.SourceRequest{ID: "sales", Name: "Synthetic source", Connection: "warehouse"})
	if err != nil || created.ContextID != "sales:v1" {
		t.Fatal("SDK create", err)
	}
	list, err := client.Sources(ctx)
	if err != nil || len(list) != 1 || list[0] != created {
		t.Fatal("SDK list", err)
	}
	read, err := client.Source(ctx, created.ID)
	if err != nil || read != created {
		t.Fatal("SDK read", err)
	}
	status, err := client.TestSource(ctx, created.ID)
	if err != nil || !status.Available || status.Revision != 1 {
		t.Fatal("SDK probe", err)
	}
	schema, err := client.DiscoverSource(ctx, created.ID)
	if err != nil || len(schema.Relations) != 2 || schema.ContextID != created.ContextID {
		t.Fatal("SDK discovery", err)
	}
	receipt, err := client.ValidateRead(ctx, created.ID, cw.ReadValidationRequest{Context: created.ContextID, SQL: `SELECT id FROM analytics.sales WHERE id=$1`, Parameters: []cw.ReadParameter{{Kind: "integer", Value: "1"}}})
	if err != nil || !receipt.Validated || len(receipt.Dependencies) != 1 {
		t.Fatal("SDK validate", err)
	}
	if _, err = client.ValidateRead(ctx, created.ID, cw.ReadValidationRequest{Context: created.ContextID, SQL: `DELETE FROM analytics.sales`}); err == nil {
		t.Fatal("unsafe HTTP validation")
	}
	rotated, err := client.RotateSource(ctx, created.ID, 1)
	if err != nil || rotated.ContextID != "sales:v2" {
		t.Fatal("SDK rotation", err)
	}
	if _, err = client.RotateSource(ctx, created.ID, 1); err == nil {
		t.Fatal("SDK stale rotation")
	}
	if _, err = client.ValidateRead(ctx, created.ID, cw.ReadValidationRequest{Context: created.ContextID, SQL: `SELECT id FROM analytics.sales`}); err == nil {
		t.Fatal("SDK old context still valid")
	}
	if _, err = client.ValidateRead(ctx, created.ID, cw.ReadValidationRequest{Context: rotated.ContextID, SQL: `SELECT id FROM analytics.sales`}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../docs/contracts/chartworks-source-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory []sourceapi.Operation
	if json.Unmarshal(data, &inventory) != nil || !reflect.DeepEqual(inventory, sourceapi.Registry(true, true)) {
		t.Fatal("operation manifest differs from executable router")
	}
	bare := f.token.envelope(t, f.e.Tenant(), "reader")
	bareToken := f.token.sign(t, f.token.claims(bare.Tenant(), bare.User(), bare.Scopes()), nil)
	before := f.lookups.Load()
	for _, op := range inventory {
		path := strings.ReplaceAll(op.Path, "{id}", created.ID)
		for _, request := range []struct {
			bearer string
			want   int
		}{{"", 401}, {bareToken, 403}} {
			result := callProtected(t, h, op.Method, path, request.bearer, "{}", nil)
			if result.Code != request.want {
				t.Fatal("denied registry operation", op, result.Code)
			}
		}
	}
	if f.lookups.Load() != before {
		t.Fatal("denied HTTP operation consulted credentials")
	}
	for _, body := range []string{`null`, `{}`, `{"id":"new","name":"New","connection":"warehouse","tenant":"foreign"}`, `{"id":"new","name":"New","connection":"warehouse","connection":"other"}`, `{"ID":"new","name":"New","connection":"warehouse"}`, `{"id":null,"name":"New","connection":"warehouse"}`, strings.Repeat("x", 65537)} {
		if result := callProtected(t, h, "POST", "/v1/sources", token, body, nil); result.Code != 400 {
			t.Fatal("ambiguous source JSON", result.Code)
		}
	}
	for _, body := range []string{`{"context":"sales:v2","sql":"SELECT 1","parameters":null}`, `{"context":"sales:v2","sql":"SELECT $1","parameters":[{"kind":"integer","value":"1","tenant":"other"}]}`, `{"context":"sales:v2","sql":"SELECT $1","parameters":[{"kind":"integer","VALUE":"1"}]}`} {
		if result := callProtected(t, h, "POST", "/v1/sources/sales/validate", token, body, nil); result.Code != 400 {
			t.Fatal("ambiguous nested parameter", result.Code)
		}
	}
	for _, r := range []struct {
		method, path, body string
		headers            map[string]string
	}{
		{"GET", "/v1/sources?tenant=other", "", nil},
		{"GET", "/v1/sources", "{}", nil},
		{"POST", "/v1/sources/sales/test", "{}", map[string]string{"Content-Encoding": "gzip"}},
		{"POST", "/v1/sources/sales/test", "{}", map[string]string{"Content-Type": "text/plain"}},
	} {
		if got := callProtected(t, h, r.method, r.path, token, r.body, r.headers); got.Code != 400 {
			t.Fatal("transport ambiguity", r.path, got.Code)
		}
	}
	if got := callProtected(t, h, "DELETE", "/v1/sources", token, "", nil); got.Code != 405 {
		t.Fatal("unsupported method")
	}
	if got := callProtected(t, h, "GET", "/unregistered", token, "", nil); got.Code != 404 {
		t.Fatal("router fallthrough")
	}
	for _, value := range []any{created, list, status, schema, receipt} {
		wire, _ := json.Marshal(value)
		for _, secret := range []string{sourcePassword, f.role, "CHARTWORKS_SOURCE_READ", "PRIVATE_COLUMN_CANARY", "connection_alias"} {
			if strings.Contains(string(wire), secret) {
				t.Fatal("public DTO includes protected fields", secret)
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := client.Source(ctx, created.ID); err != nil || got != rotated {
				t.Error("concurrent SDK metadata read", err)
			}
		}()
	}
	wg.Wait()
	cfg := f.cfg
	cfg.Enabled = false
	cfg.Connections = nil
	metadata, err := sources.New(f.db, cfg, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer metadata.Close()
	offline := sourceapi.Handler(f.token.verifier, metadata, nil, http.NotFoundHandler())
	before = f.lookups.Load()
	for _, path := range []string{"/v1/sources", "/v1/sources/sales"} {
		if got := callProtected(t, offline, "GET", path, token, "", nil); got.Code != 200 || got.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("retained metadata lost", got.Code)
		}
	}
	if got := callProtected(t, offline, "POST", "/v1/sources", token, "{}", nil); got.Code != 405 {
		t.Fatal("metadata admitted source creation")
	}
	if got := callProtected(t, offline, "POST", "/v1/sources/sales/validate", token, "{}", nil); got.Code != 404 {
		t.Fatal("metadata advertised native planning")
	}
	if f.lookups.Load() != before {
		t.Fatal("offline metadata consulted warehouse")
	}
	for _, id := range []string{"", "bad/path"} {
		if _, err := client.Source(ctx, id); err == nil {
			t.Fatal("unsafe SDK path")
		}
		if _, err := client.TestSource(ctx, id); err == nil {
			t.Fatal("unsafe SDK probe path")
		}
		if _, err := client.DiscoverSource(ctx, id); err == nil {
			t.Fatal("unsafe SDK schema path")
		}
		if _, err := client.RotateSource(ctx, id, 1); err == nil {
			t.Fatal("unsafe SDK rotate path")
		}
		if _, err := client.ValidateRead(ctx, id, cw.ReadValidationRequest{}); err == nil {
			t.Fatal("unsafe SDK validation path")
		}
		if _, err := client.CreateSource(ctx, cw.SourceRequest{ID: id, Connection: "warehouse"}); err == nil {
			t.Fatal("unsafe SDK create path")
		}
	}
}
