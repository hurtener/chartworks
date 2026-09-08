package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/sourceapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
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

// This is the positive consumer counterpart to the registry denial test. It
// crosses the SDK, bearer verifier, HTTP handler, pipeline service, PostgreSQL,
// governed source validator and the pinned Bruin process.
func TestPipelineSDKLifecycle(t *testing.T) {
	model := newGatewayFixture(t, nil)
	f := newPipelineFixture(t, model.engine, func(v *config.Values) {
		v.Features.Gateway = true
		v.Gateway = model.cfg
	})
	source := f.create(t, "sdk-pipeline-source")
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
	var tokenCalls atomic.Int64
	handler := sourceapi.PipelineHandler(f.token.verifier, f.pipelines, http.NotFoundHandler())
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) {
		tokenCalls.Add(1)
		return token, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	proposal, err := client.ProposePipeline(context.Background(), cw.PipelineProposalRequest{ID: "sdk-proposal", Name: "SDK proposal", Connection: "workspace", Source: source.ID, Context: source.ContextID, Instruction: "Select the synthetic identifier"})
	if err != nil || proposal.State != "draft" || proposal.Version != 1 || proposal.Published != nil || model.requests.Load() != 1 {
		t.Fatal("SDK proposal", err, proposal)
	}
	definition := f.definition(t, source, "sdk-pipeline")
	sdkDefinition := cw.PipelineDefinition{ID: definition.ID, Name: definition.Name, Connection: definition.Connection, Steps: []cw.PipelineStep{{
		ID: definition.Steps[0].ID, Source: definition.Steps[0].Source, Context: definition.Steps[0].Context,
		SQL: definition.Steps[0].SQL, Inputs: append([]string{}, definition.Steps[0].Inputs...), DependsOn: []string{}, FromSteps: []string{}, Strategy: "replace",
		Columns: []cw.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []cw.PipelineCheck{},
	}}}
	draft, err := client.DraftPipeline(context.Background(), sdkDefinition, 0)
	if err != nil || draft.State != "draft" || draft.Version != 1 {
		t.Fatal("SDK draft", err, draft)
	}
	if _, err = client.DraftPipeline(context.Background(), sdkDefinition, 0); err == nil {
		t.Fatal("stale SDK draft CAS accepted")
	}
	published, err := client.PublishPipeline(context.Background(), sdkDefinition.ID, draft.Version)
	if err != nil || published.State != "published" || published.Published == nil {
		t.Fatal("SDK publish", err, published)
	}
	read, err := client.Pipeline(context.Background(), sdkDefinition.ID, draft.Version)
	if err != nil || read.Digest != published.Digest {
		t.Fatal("SDK immutable read", err, read)
	}
	admitted, err := client.AdmitPipelineRun(context.Background(), sdkDefinition.ID, draft.Version, "sdk-cancel")
	if err != nil || admitted.Operation.ID == "" || admitted.State != "staged" {
		t.Fatal("SDK admit", err, admitted)
	}
	inspected, err := client.PipelineRun(context.Background(), admitted.Operation.ID)
	if err != nil || inspected.Operation.ID != admitted.Operation.ID {
		t.Fatal("SDK inspect", err, inspected)
	}
	cancelled, err := client.CancelPipelineRun(context.Background(), admitted.Operation.ID)
	if err != nil || cancelled.ID != admitted.Operation.ID || cancelled.State != "cancelled" {
		t.Fatal("SDK cancel", err, cancelled)
	}
	run, err := client.RunPipeline(context.Background(), sdkDefinition.ID, draft.Version, "sdk-run", false)
	if err != nil || run.State != "published" || run.Operation.ID == "" || len(run.Effects) != 1 || run.Effects[0].Source == "" {
		t.Fatal("SDK real run", err, run)
	}
	if got, readErr := client.PipelineRun(context.Background(), run.Operation.ID); readErr != nil || got.State != "published" {
		t.Fatal("SDK completed run inspection", readErr, got)
	}
	if tokenCalls.Load() != 10 {
		t.Fatalf("SDK did not obtain a current bearer for every request: %d", tokenCalls.Load())
	}

	for name, call := range map[string]func() error{
		"proposal": func() error {
			_, e := client.ProposePipeline(context.Background(), cw.PipelineProposalRequest{})
			return e
		},
		"draft":   func() error { _, e := client.DraftPipeline(context.Background(), cw.PipelineDefinition{}, 0); return e },
		"publish": func() error { _, e := client.PublishPipeline(context.Background(), "../bad", 1); return e },
		"read":    func() error { _, e := client.Pipeline(context.Background(), sdkDefinition.ID, 0); return e },
		"run": func() error {
			_, e := client.RunPipeline(context.Background(), sdkDefinition.ID, 1, "", false)
			return e
		},
		"admit":   func() error { _, e := client.AdmitPipelineRun(context.Background(), sdkDefinition.ID, 1, ""); return e },
		"inspect": func() error { _, e := client.PipelineRun(context.Background(), "../bad"); return e },
		"cancel":  func() error { _, e := client.CancelPipelineRun(context.Background(), "../bad"); return e },
	} {
		if call() == nil {
			t.Fatal("SDK accepted malformed", name)
		}
	}
	bad := httptest.NewRequest(http.MethodPost, "/v1/pipelines", strings.NewReader(`{"definition":{},"expected_revision":0,"extra":true}`))
	bad.Header.Set("Authorization", "Bearer "+token)
	bad.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, bad)
	if response.Code != http.StatusBadRequest {
		t.Fatal("closed pipeline body accepted", response.Code)
	}

	disabledValues := f.values
	disabledValues.Pipelines.Enabled = false
	disabled, err := engineering.NewPipelineService(f.db, f.s, f.validator, nil, disabledValues, f.lookup)
	if err != nil {
		t.Fatal("disabled retained service", err)
	}
	defer disabled.Close()
	disabledServer := httptest.NewServer(sourceapi.PipelineHandler(f.token.verifier, disabled, http.NotFoundHandler()))
	defer disabledServer.Close()
	disabledClient, err := cw.New(disabledServer.URL, disabledServer.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, readErr := disabledClient.Pipeline(context.Background(), sdkDefinition.ID, draft.Version); readErr != nil || got.Digest != published.Digest {
		t.Fatal("disabled service lost retained definition read", readErr, got)
	}
	if got, readErr := disabledClient.PipelineRun(context.Background(), run.Operation.ID); readErr != nil || got.Operation.ID != run.Operation.ID {
		t.Fatal("disabled service lost retained run inspection", readErr, got)
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
