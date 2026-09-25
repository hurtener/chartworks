package nlqapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
)

func TestSQLRecoveryParameterEditPublicSchema(t *testing.T) {
	registry, err := ExecutionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, _, ok := registry.Match(http.MethodPost, "/v1/nlq/refinements")
	if !ok || definition.Request == nil {
		t.Fatal("missing Refine schema")
	}
	body := `{"query_id":"parent","parameter_edits":[{"position":1,"replacement":{"kind":"number","value":"9007199254740993.125"}}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/nlq/refinements", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var decoded nlqexec.RefineRequest
	if err := decodeBody(httptest.NewRecorder(), r, definition.Request, &decoded); err != nil || len(decoded.ParameterEdits) != 1 || decoded.ParameterEdits[0].Replacement.Value != "9007199254740993.125" {
		t.Fatal("typed edit lost wire precision", err)
	}
	for _, bad := range []string{strings.Replace(body, `"position":1`, `"position":1.5`, 1), strings.Replace(body, `"value":"9007199254740993.125"`, `"value":9007199254740993.125`, 1), strings.Replace(body, `"position":1`, `"position":1,"authority":"admin"`, 1)} {
		if err := definition.Request.Validate([]byte(bad), MaxBodyBytes); err == nil {
			t.Fatal("open/untyped edit schema")
		}
	}
	plan, _, ok := registry.Match(http.MethodPost, "/v1/nlq/plans")
	if !ok {
		t.Fatal("missing Plan")
	}
	if err := plan.Request.Validate([]byte(`{"parameter_edits":[]}`), MaxBodyBytes); err == nil {
		t.Fatal("fresh Plan accepted a parent-only edit")
	}
	out, _ := json.Marshal(nlqexec.PlanResult{})
	if strings.Contains(string(out), "parameter_edits") {
		t.Fatal("private edits became response data")
	}
}
