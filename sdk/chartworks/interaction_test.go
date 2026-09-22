package chartworks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/topicapi"
)

func TestEXP03InteractionOrderingAndOutcomeRichness(t *testing.T) {
	state, applied, err := ApplyInteraction(QueryInteractionState{}, QueryInteractionEvent{Generation: 1, Sequence: 1, Kind: "start", Status: "accepted"})
	if err != nil || !applied {
		t.Fatal("start", state, applied, err)
	}
	state, applied, err = ApplyInteraction(state, QueryInteractionEvent{Generation: 1, Sequence: 2, QueryID: "query-1", Kind: "progress", Status: "running"})
	if err != nil || !applied || state.QueryID != "query-1" {
		t.Fatal("progress", state, applied, err)
	}
	// A newer user intent wins. Its result cannot later be replaced by the old
	// generation even when the old response arrives with a larger sequence.
	state, applied, err = ApplyInteraction(state, QueryInteractionEvent{Generation: 2, Sequence: 1, Kind: "start", Status: "accepted"})
	if err != nil || !applied {
		t.Fatal("new generation", state, applied, err)
	}
	unchanged, applied, err := ApplyInteraction(state, QueryInteractionEvent{Generation: 1, Sequence: 99, QueryID: "query-1", Kind: "result", Status: "succeeded"})
	if err != nil || applied || !reflect.DeepEqual(unchanged, state) {
		t.Fatal("stale response replaced current state", unchanged, applied, err)
	}
	disconnected, applied, err := ApplyInteraction(state, QueryInteractionEvent{Generation: 2, Sequence: 2, Kind: "disconnect", Status: "transport_disconnected"})
	if err != nil || !applied || disconnected.Status == "cancelled" {
		t.Fatal("disconnect was treated as cancellation", disconnected, err)
	}
	cancelled, applied, err := ApplyInteraction(disconnected, QueryInteractionEvent{Generation: 2, Sequence: 3, Kind: "cancel", Status: "cancel_requested"})
	if err != nil || !applied || cancelled.Status != "cancel_requested" {
		t.Fatal("explicit cancellation lost", cancelled, err)
	}
	for _, status := range []string{"succeeded", "empty", "truncated", "failed", "uncertain", "cancelled", "timed_out", "interrupted"} {
		got, ok, eventErr := ApplyInteraction(cancelled, QueryInteractionEvent{Generation: 3, Sequence: 1, QueryID: "query-" + status, Kind: "result", Status: status})
		if eventErr != nil || !ok || got.Status != status {
			t.Errorf("outcome %s collapsed: %#v %v", status, got, eventErr)
		}
	}
	for _, event := range []QueryInteractionEvent{
		{Generation: 4, Sequence: 1, Kind: "result", Status: "success"},
		{Generation: 4, Sequence: 1, Kind: "disconnect", Status: "cancelled"},
		{Generation: 4, Sequence: 1, Kind: "view", Status: "cancel_requested", View: "raw_rows"},
	} {
		if _, applied, eventErr := ApplyInteraction(cancelled, event); eventErr == nil || applied {
			t.Error("ambiguous/unknown event accepted", event)
		}
	}
	data, err := json.Marshal(QueryInteractionEvent{Generation: 4, Sequence: 1, QueryID: "query-safe", Kind: "result", Status: "uncertain"})
	if err != nil || strings.Contains(string(data), "sql") || strings.Contains(string(data), "prompt") || strings.Contains(string(data), "rows") {
		t.Fatal("interaction envelope can expose protected content", string(data), err)
	}
}

func TestEXP11CumulativeRegisteredConsumerMatrix(t *testing.T) {
	factories := []func() (*api.Registry, error){
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
	rows, err := ParseOperations(document)
	if err != nil || len(rows) != len(registry.Definitions()) {
		t.Fatalf("generated inventory: rows=%d definitions=%d err=%v", len(rows), len(registry.Definitions()), err)
	}
	byID := make(map[string]OperationInfo, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
		if row.SDKMethod == "" || row.CLICommand == "" || !row.Public && (!hasOperationError(row.Errors, 401) || !hasOperationError(row.Errors, 403)) {
			t.Fatalf("incomplete consumer/error projection: %#v", row)
		}
	}
	for _, id := range []string{"reportingFilterOptions", "reportingExport", "reportingRenditionCreate", "reportingRenditionRead", "reportingRenditionList", "reportingRenditionExpire", "startOnboarding", "getOnboarding", "resumeOnboarding", "answerOnboarding", "cancelOnboarding", "proposeOnboardingDrift"} {
		if _, ok := byID[id]; !ok {
			t.Error("late domain operation missing from generated matrix", id)
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
	journey, err := InteractionJourney(rows)
	if err != nil {
		t.Fatal("incomplete interaction journey", err)
	}
	for role, ids := range journey {
		if len(ids) == 0 {
			t.Error("empty role", role)
		}
	}
}

func TestInteractionMetadataRejectsUnknownRoles(t *testing.T) {
	registry := unitOperationRegistry(t)
	definitions := registry.Definitions()
	definitions[0].Interaction = "invented_success"
	if _, err := api.New(definitions); err == nil {
		t.Fatal("unknown interaction role accepted")
	}
}
