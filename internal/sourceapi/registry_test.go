package sourceapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
)

func TestSourceRegistrySchemasAndManifest(t *testing.T) {
	r, err := SourceRegistry(true, true)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile("../../docs/contracts/chartworks-source-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []Operation
	if json.Unmarshal(wire, &manifest) != nil || !reflect.DeepEqual(manifest, r.Operations()) || !reflect.DeepEqual(manifest, Registry(true, true)) {
		t.Fatal("registered operations drifted from existing manifest")
	}
	for _, settings := range []struct {
		warehouse, validation bool
		count                 int
	}{{false, false, 2}, {false, true, 2}, {true, false, 6}, {true, true, 7}} {
		registry, err := SourceRegistry(settings.warehouse, settings.validation)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := registry.OpenAPI("Chartworks source operations", "1")
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Paths map[string]map[string]json.RawMessage `json:"paths"`
		}
		if json.Unmarshal(doc, &parsed) != nil {
			t.Fatal("invalid generated document")
		}
		count := 0
		for _, path := range parsed.Paths {
			count += len(path)
		}
		if count != settings.count || len(registry.Definitions()) != settings.count {
			t.Fatal("disabled capability advertised an operation")
		}
	}
	samples := map[string]string{
		"createSource": `{"id":"source1","name":"Synthetic source","connection":"approved_alias"}`,
		"testSource":   `{}`,
		"rotateSource": `{"expected_revision":1}`,
		"validateRead": `{"context":"source1:v1","sql":"SELECT 1","parameters":[]}`,
	}
	for _, d := range r.Definitions() {
		if d.Request == nil {
			continue
		}
		if err := d.Request.Validate([]byte(samples[d.ID]), 65536); err != nil {
			t.Fatalf("%s rejected its existing wire shape: %v", d.ID, err)
		}
		var object map[string]any
		_ = json.Unmarshal([]byte(samples[d.ID]), &object)
		object["tenant"] = "foreign"
		bad, _ := json.Marshal(object)
		if d.Request.Validate(bad, 65536) == nil {
			t.Fatalf("%s accepted extra identity field", d.ID)
		}
	}
}

func TestSourceRegistryDescribesExistingErrorCodes(t *testing.T) {
	registered := sourceErrors()
	for _, err := range []error{access.ErrUnauthenticated, access.ErrForbidden, access.ErrNotFound, store.ErrInvalid, store.ErrConflict, readexec.ErrBinding, readexec.ErrUnsafe, readexec.ErrUnsupported, readexec.ErrLimit, context.Canceled, errors.New("backend unavailable")} {
		w := httptest.NewRecorder()
		failure(w, err)
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(w.Body.Bytes(), &payload) != nil {
			t.Fatal("invalid error response")
		}
		found := false
		for _, definition := range registered {
			if definition.Status == w.Code && definition.Code == payload.Error {
				found = true
			}
		}
		if !found {
			t.Fatalf("unregistered actual error %d/%s", w.Code, payload.Error)
		}
	}
}
