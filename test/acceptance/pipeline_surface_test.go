package acceptance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/sourceapi"
)

func TestPipelineOperationManifestParity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/contracts/chartworks-pipeline-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var published []sourceapi.Operation
	if json.Unmarshal(raw, &published) != nil || !reflect.DeepEqual(published, sourceapi.PipelineRegistry(true)) {
		t.Fatal("published pipeline operation manifest drifted from executable registry")
	}
	seen := map[string]bool{}
	for _, operation := range published {
		key := operation.Method + " " + operation.Path
		if seen[key] || operation.Action == "" || operation.Effect == "" {
			t.Fatal("duplicate or unclassified pipeline operation", operation)
		}
		seen[key] = true
	}
}

func TestPipelineRegisteredSurfacesDenyBeforeBodyOrSource(t *testing.T) {
	f := newPipelineFixture(t, nil, nil)
	handler := sourceapi.PipelineHandler(f.token.verifier, f.pipelines, http.NotFoundHandler())
	for _, operation := range sourceapi.PipelineRegistry(true) {
		path := strings.ReplaceAll(operation.Path, "{id}", "hidden")
		request := httptest.NewRequest(operation.Method, path, strings.NewReader("PRIVATE_BODY_CANARY"))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "PRIVATE_BODY_CANARY") {
			t.Fatal("pipeline operation bypassed bearer verification", operation, response.Code)
		}
		request = httptest.NewRequest(operation.Method, path, strings.NewReader("PRIVATE_BODY_CANARY"))
		request.Header.Set("Authorization", "Bearer "+f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), []string{"ops.read", "cw.tenant.read:" + f.e.Tenant()}), nil))
		response = httptest.NewRecorder()
		before := f.lookups.Load()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || f.lookups.Load() != before || strings.Contains(response.Body.String(), "PRIVATE_BODY_CANARY") {
			t.Fatal("pipeline action denied after body/source access", operation, response.Code)
		}
	}
}
