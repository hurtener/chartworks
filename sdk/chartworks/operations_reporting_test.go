package chartworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// Exercise the actual reporting operation registrations, not a shallow stand-in
// for nested output intent, frozen manifests and native definition export.
func TestCW03ReportingOperationCatalog(t *testing.T) {
	factories := []func() (*api.Registry, error){
		func() (*api.Registry, error) { return reportingapi.Registry(true, true, true) },
		func() (*api.Registry, error) { return reportingapi.RuntimeRegistry(true, true) },
		reportingapi.DocumentsRegistry,
		func() (*api.Registry, error) { return reportingapi.DeliveryRegistry(true) },
	}
	registries := make([]*api.Registry, 0, len(factories))
	for _, factory := range factories {
		registry, err := factory()
		if err != nil {
			t.Fatal(err)
		}
		registries = append(registries, registry)
	}
	registry, err := api.Compose(registries...)
	if err != nil {
		t.Fatal(err)
	}
	document, err := registry.OpenAPI("Synthetic reporting inventory", "cw03")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.DecodeJSON(document, operationCatalogLimit); err != nil {
		t.Fatalf("reporting catalog violates shared JSON bounds: bytes=%d, err=%v", len(document), err)
	}
	rows, err := ParseOperations(document)
	if err != nil || len(rows) != len(registry.Definitions()) {
		t.Fatalf("actual reporting inventory rejected: bytes=%d rows=%d err=%v", len(document), len(rows), err)
	}
	byID := make(map[string]OperationInfo, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, definition := range registry.Definitions() {
		row, ok := byID[definition.ID]
		if !ok || row.Action != definition.Action || row.ResourceLoader != definition.ResourceLoader || row.Replay != definition.ReplayPolicy() {
			t.Fatal("registered authority/replay metadata changed", definition.ID)
		}
		pairs := [][2]json.RawMessage{{row.ResponseSchema, definition.Response.Document()}}
		if definition.Request != nil {
			pairs = append(pairs, [2]json.RawMessage{row.RequestSchema, definition.Request.Document()})
		}
		for _, pair := range pairs {
			var expected bytes.Buffer
			if json.Compact(&expected, pair[1]) != nil || !bytes.Equal(pair[0], expected.Bytes()) {
				t.Fatal("schema projection changed declared output policy", definition.ID)
			}
		}
	}
	// Exercise the real client HTTP parsing path without invoking a domain,
	// accessing a warehouse or requiring a model to read registration metadata.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" || r.Method != http.MethodGet {
			t.Error("catalog performed work outside its metadata read")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(document)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-catalog", nil })
	if err != nil {
		t.Fatal(err)
	}
	observed, err := client.Operations(t.Context())
	if err != nil || !reflect.DeepEqual(observed, rows) {
		t.Fatal("HTTP and exported operation catalogs differ", err)
	}
}

func TestCW03CatalogFormattingDoesNotEnlargeSchemaAuthority(t *testing.T) {
	registry := unitOperationRegistry(t)
	document, err := registry.OpenAPI("Synthetic reporting formatting", "cw03")
	if err != nil {
		t.Fatal(err)
	}
	var compact, padded bytes.Buffer
	if json.Compact(&compact, document) != nil {
		t.Fatal("invalid source catalog")
	}
	schema := registry.Definitions()[0].Response.Document()
	if json.Indent(&padded, schema, strings.Repeat(" ", 12<<10), "  ") != nil || padded.Len() <= 64<<10 {
		t.Fatal("invalid bounded formatting fixture", padded.Len())
	}
	formatted := bytes.Replace(compact.Bytes(), schema, padded.Bytes(), 1)
	if len(formatted) <= compact.Len() || len(formatted) > operationCatalogLimit {
		t.Fatal("fixture did not add bounded schema-only whitespace")
	}
	rows, err := ParseOperations(formatted)
	if err != nil || len(rows) != 8 {
		t.Fatal("bounded pretty-printed catalog rejected", err)
	}
	canonical, err := ParseOperations(document)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].ID != canonical[i].ID || !bytes.Equal(rows[i].RequestSchema, canonical[i].RequestSchema) || !bytes.Equal(rows[i].ResponseSchema, canonical[i].ResponseSchema) {
			t.Fatal("formatting changed the compiled operation contract")
		}
	}
	tooLarge := []byte(`{"type":"string","description":"` + strings.Repeat("x", 64<<10) + `"}`)
	_, _, ok := oneContent(operationContent{"application/json": {Schema: tooLarge}})
	if ok {
		t.Fatal("actual schema-content bound was widened")
	}
	_, _, ok = oneContent(operationContent{"application/json": {Schema: json.RawMessage(`{"type":`)}})
	if ok {
		t.Fatal("malformed schema was normalized into acceptance")
	}
	if _, err := ParseOperations(append(document, []byte(strings.Repeat(" ", operationCatalogLimit))...)); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatal("wire catalog ceiling was widened", err)
	}
}
