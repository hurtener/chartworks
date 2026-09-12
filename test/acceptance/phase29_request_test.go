package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func TestDocumentCompositionRequestWire(t *testing.T) {
	registry, err := reportingapi.DocumentsRegistry()
	if err != nil {
		t.Fatal(err)
	}
	request := reporting.CompositionRequest{Key: "wire-admission"}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range registry.Definitions() {
		if route.ID != "admit_report_run" && route.ID != "admit_dashboard_run" {
			continue
		}
		if err := route.Request.Validate(body, route.MaxBodyBytes); err != nil {
			// These are synthetic fixture values and public wire schemas, never
			// bearer material or database error strings from a user's source.
			compiler := jsonschema.NewCompiler()
			const location = "https://chartworks.invalid/request.json"
			if err := compiler.AddResource(location, bytes.NewReader(route.Request.Document())); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(location)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("default composition request rejected by %s: %v; fixture=%s", route.ID, schema.Validate(value), body)
		}
	}
}

func TestDocumentAdmissionParity(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	scopes := append(phase29DocumentScopes(), "reporting.execute", "cw.report.execute:*", "cw.dashboard.execute:*", "cw.run.read:*")
	author := phase27Actor(t, f, f.f.e.User(), scopes)
	documents, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewCompositions(documents, f.f.db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	state, err := documents.Create(ctx, author, "report", "admission-parity", phase29Text("Admission parity"))
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, documents, author, state)
	input := reporting.CompositionRequest{Key: "parity-direct"}
	if _, err := runs.Admit(ctx, author, "report", state.ID, input); err != nil {
		t.Fatal("direct admission failed", err)
	}
	input.Key = "parity-http"
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	handler := reportingapi.DocumentsHandler(f.f.token.verifier, documents, runs, http.NotFoundHandler())
	bearer := phase27Token(t, f, author.User(), author.Session(), scopes)
	response := callProtected(t, handler, "POST", "/v1/reports/"+state.ID+"/runs", bearer, string(body), map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusOK {
		t.Fatalf("same synthetic request differs over HTTP: status=%d error=%s", response.Code, response.Body.String())
	}
}
