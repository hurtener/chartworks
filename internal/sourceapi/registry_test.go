package sourceapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
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
	}{{false, false, 4}, {false, true, 4}, {true, false, 8}, {true, true, 9}} {
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
		"listDatasets":    `{"source":"source1","context":"source1:v1","after":"","limit":32}`,
		"describeDataset": `{"source":"source1","context":"source1:v1","dataset":"sales"}`,
		"createSource":    `{"id":"source1","name":"Synthetic source","connection":"approved_alias"}`,
		"testSource":      `{}`,
		"rotateSource":    `{"expected_revision":1}`,
		"validateRead":    `{"context":"source1:v1","sql":"SELECT 1","parameters":[]}`,
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

func TestFamilyRegistriesGenerateExistingManifests(t *testing.T) {
	for _, family := range []struct {
		name, manifest string
		build          func() (*api.Registry, error)
		projection     func() []Operation
		count          int
	}{
		{"execution", "chartworks-read-operations.json", ExecutionAPIRegistry, ExecutionRegistry, 5},
		{"engineering", "chartworks-engineering-operations.json", func() (*api.Registry, error) { return EngineeringAPIRegistry(true, true, 100<<20) }, func() []Operation { return EngineeringRegistry(true, true) }, 14},
		{"pipeline", "chartworks-pipeline-operations.json", func() (*api.Registry, error) { return PipelineAPIRegistry(true) }, func() []Operation { return PipelineRegistry(true) }, 7},
	} {
		t.Run(family.name, func(t *testing.T) {
			registry, err := family.build()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile("../../docs/contracts/" + family.manifest)
			if err != nil {
				t.Fatal(err)
			}
			var want []Operation
			if json.Unmarshal(raw, &want) != nil || !reflect.DeepEqual(want, registry.Operations()) || !reflect.DeepEqual(want, family.projection()) {
				t.Fatal("legacy/registered manifest drift")
			}
			doc, err := registry.OpenAPI("Source families", "1")
			if err != nil {
				t.Fatal(err)
			}
			var parsed struct {
				Paths map[string]map[string]struct {
					ID      string `json:"operationId"`
					Action  string `json:"x-chartworks-action"`
					Request struct {
						Content map[string]json.RawMessage `json:"content"`
					} `json:"requestBody"`
				} `json:"paths"`
			}
			if json.Unmarshal(doc, &parsed) != nil {
				t.Fatal("invalid OpenAPI")
			}
			count := 0
			for _, path := range parsed.Paths {
				count += len(path)
			}
			if count != family.count {
				t.Fatal("OpenAPI operation drift", count)
			}
			for _, d := range registry.Definitions() {
				generated := parsed.Paths[d.Path][strings.ToLower(d.Method)]
				if generated.ID != d.ID || generated.Action != d.Action {
					t.Fatal("OpenAPI metadata drift", d.ID)
				}
				if d.RequestContentType == "application/octet-stream" {
					if d.MaxBodyBytes != 100<<20 || len(generated.Request.Content["application/octet-stream"]) == 0 || len(generated.Request.Content["application/json"]) != 0 {
						t.Fatal("binary upload described as JSON or wrong cap")
					}
				}
			}
		})
	}
	for _, setting := range []struct {
		uploads, profiles bool
		count             int
	}{{false, false, 7}, {true, false, 12}, {false, true, 9}, {true, true, 14}} {
		r, err := EngineeringAPIRegistry(setting.uploads, setting.profiles, 4096)
		if err != nil || len(r.Operations()) != setting.count {
			t.Fatal("engineering capability registration", err)
		}
		for _, d := range r.Definitions() {
			if d.ID == "stageUpload" && d.MaxBodyBytes != 4096 {
				t.Fatal("configured upload cap lost")
			}
		}
	}
	for _, limit := range []int64{0, -1, 100<<20 + 1} {
		if _, err := EngineeringAPIRegistry(true, false, limit); err == nil {
			t.Fatal("invalid upload cap accepted")
		}
	}
	off, err := PipelineAPIRegistry(false)
	if err != nil || len(off.Operations()) != 3 {
		t.Fatal("retained pipeline registry", err)
	}
}

func TestPipelineSchemaPreservesExistingOptionalDecoderFields(t *testing.T) {
	registry, err := PipelineAPIRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := registry.Match("POST", "/v1/pipelines")
	for _, body := range []string{`{}`, `{"definition":{"steps":null},"expected_revision":null}`, `{"definition":{"steps":[{"depends_on":null,"checks":null}]}}`} {
		var input pipelineDraftRequest
		request := httptest.NewRequest("POST", "/v1/pipelines", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if err := pipelineBody(httptest.NewRecorder(), request, &input); err != nil {
			t.Fatal("existing decoder rejected fixture", err)
		}
		if err := d.Request.Validate([]byte(body), d.MaxBodyBytes); err != nil {
			t.Fatal("schema narrowed existing decoder", err)
		}
	}
	for _, body := range []string{`null`, `{"tenant":"foreign"}`, `{"definition":{"steps":[{"secret":"x"}]}}`} {
		if d.Request.Validate([]byte(body), d.MaxBodyBytes) == nil {
			t.Fatal("schema lost closed shape", body)
		}
	}
}
