package chartworks_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/workapi"
	chartworks "github.com/hurtener/chartworks/sdk/chartworks"
)

type inventoryEngine struct{}

func (inventoryEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	return gateway.Generated{}, gateway.ErrDisabled
}
func (inventoryEngine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (inventoryEngine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (inventoryEngine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (inventoryEngine) Space() string                          { return "synthetic" }
func (inventoryEngine) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (inventoryEngine) Close()                                 {}

func TestEXP11CumulativeRegisteredConsumerMatrix(t *testing.T) {
	if registry, err := workapi.APIRegistry(nil, nil); err != nil || registry != nil {
		t.Fatalf("disabled work capability advertised: registry=%v err=%v", registry, err)
	}
	withoutMetrics, err := securityapi.APIRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := withoutMetrics.Match("GET", "/metrics"); ok {
		t.Fatal("disabled metrics route advertised")
	}
	withoutBYO, err := nlqapi.BYORegistry(false)
	if err != nil || len(withoutBYO.Definitions()) != 2 {
		t.Fatalf("disabled BYO route advertised: %v", err)
	}
	if _, _, ok := withoutBYO.Match("POST", "/v1/nlq/contexts"); ok {
		t.Fatal("disabled BYO context creation advertised")
	}
	withoutPipeline, err := sourceapi.PipelineAPIRegistry(false)
	if err != nil || len(withoutPipeline.Definitions()) != 3 {
		t.Fatalf("disabled pipeline route advertised: %v", err)
	}
	if _, _, ok := withoutPipeline.Match("POST", "/v1/pipeline-proposals"); ok {
		t.Fatal("disabled pipeline proposal advertised")
	}
	for _, disabled := range []struct {
		name    string
		factory func() (*api.Registry, error)
		id      string
	}{
		{"source warehouse", func() (*api.Registry, error) { return sourceapi.SourceRegistry(false, true) }, "createSource"},
		{"reporting execution", func() (*api.Registry, error) { return reportingapi.DeliveryRegistry(false, false, false) }, "reportingRun"},
		{"reporting runtime execution", func() (*api.Registry, error) { return reportingapi.RuntimeRegistry(false, false) }, "admitReportingRun"},
		{"static rendering", func() (*api.Registry, error) { return reportingapi.DeliveryRegistry(true, false, false) }, "reportingExport"},
	} {
		registry, registryErr := disabled.factory()
		if registryErr != nil {
			t.Fatal(disabled.name, registryErr)
		}
		for _, definition := range registry.Definitions() {
			if definition.ID == disabled.id {
				t.Fatal("disabled feature route advertised", disabled.name, disabled.id)
			}
		}
	}

	factories := []func() (*api.Registry, error){
		foundation.PublicRegistry,
		func() (*api.Registry, error) { return securityapi.APIRegistry(true) },
		func() (*api.Registry, error) { return workapi.APIRegistry(inventoryEngine{}, &jobs.Service{}) },
		func() (*api.Registry, error) { return sourceapi.SourceRegistry(true, true) },
		func() (*api.Registry, error) { return sourceapi.EngineeringAPIRegistry(true, true, 16<<20) },
		sourceapi.ExecutionAPIRegistry,
		func() (*api.Registry, error) { return sourceapi.PipelineAPIRegistry(true) },
		topicapi.Registry,
		nlqapi.Registry,
		nlqapi.ExecutionRegistry,
		func() (*api.Registry, error) { return nlqapi.BYORegistry(true) },
		chartapi.Registry,
		func() (*api.Registry, error) { return reportingapi.Registry(true, true, true) },
		func() (*api.Registry, error) { return reportingapi.RuntimeRegistry(true, true) },
		func() (*api.Registry, error) { return reportingapi.DocumentsRegistry(true) },
		func() (*api.Registry, error) { return reportingapi.DeliveryRegistry(true, true, true) },
		onboardingapi.Registry,
		func() (*api.Registry, error) { return mcpserver.HTTPRegistry(config.DefaultMCP()) },
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
	document, err := registry.OpenAPI("Synthetic complete consumer inventory", "exp-11")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := chartworks.ParseOperations(document)
	if err != nil || len(rows) != len(registry.Definitions()) {
		t.Fatalf("generated inventory: rows=%d definitions=%d err=%v", len(rows), len(registry.Definitions()), err)
	}
	byID := make(map[string]chartworks.OperationInfo, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
		if row.SDKMethod == "" || row.CLICommand == "" || !row.Public && (!hasStatus(row.Errors, 401) || !hasStatus(row.Errors, 403)) {
			t.Fatalf("incomplete consumer/error projection: %#v", row)
		}
	}
	for _, id := range []string{"health", "metrics", "gatewayProbe", "listJobs", "reportingFilterOptions", "reportingExport", "reportingRenditionCreate", "reportingRenditionRead", "reportingRenditionList", "reportingRenditionExpire", "searchBusinessGoal", "chooseBusinessGoal", "startOnboarding", "getOnboarding", "resumeOnboarding", "answerOnboarding", "cancelOnboarding", "proposeOnboardingDrift"} {
		if _, ok := byID[id]; !ok {
			t.Error("enabled operation missing from generated matrix", id)
		}
	}
	for _, id := range []string{"reportingFilterOptions", "reportingRenditionCreate", "resumeOnboarding", "answerOnboarding", "cancelOnboarding"} {
		row := byID[id]
		if !strings.Contains(string(row.RequestSchema), "expected_version") && id != "reportingFilterOptions" && id != "reportingRenditionCreate" {
			t.Error("late mutable operation lost version fence", id)
		}
		if row.Action == "" || row.ResourceLoader == "" || row.Effect == "" || row.Audit == "" {
			t.Error("late operation lost authority/effect contract", id)
		}
	}
	journey, err := chartworks.InteractionJourney(rows)
	if err != nil {
		t.Fatal("incomplete interaction journey", err)
	}
	expected := map[string][]string{
		"query_start_or_clarify":    {"admitReportingRun", "preflightNLQ", "reportingRun", "routeNLQ"},
		"query_progress_or_clarify": {"inspectReportingRun", "planNLQ"},
		"query_cancel":              {"cancelRead", "cancelReportingRun"},
		"query_result":              {"executeReportingRun", "readExecution", "readReportingRun", "runNLQ"},
		"query_view":                {"reportingRunOutput", "reportingRunRows", "reportingView"},
		"query_feedback":            {"feedbackNLQ"},
		"query_refine_or_clarify":   {"refineNLQ"},
	}
	for role, ids := range expected {
		for _, id := range ids {
			if !contains(journey[role], id) {
				t.Errorf("interaction family %s missing %s: %v", role, id, journey[role])
			}
		}
	}
}

func hasStatus(errors []chartworks.OperationError, status int) bool {
	for _, item := range errors {
		if item.Status == status {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
