package workapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
)

type registryEngine struct{}

func (registryEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	return gateway.Generated{}, gateway.ErrDisabled
}
func (registryEngine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (registryEngine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (registryEngine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (registryEngine) Space() string                          { return "fixture" }
func (registryEngine) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (registryEngine) Close()                                 {}

func TestAPIRegistryAdvertisesOnlySelectedCapabilities(t *testing.T) {
	if registry, err := APIRegistry(nil, nil); err != nil || registry != nil {
		t.Fatalf("empty capability registry=%v err=%v", registry, err)
	}
	registry, err := APIRegistry(registryEngine{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Operations(); len(got) != 1 || got[0].Path != "/v1/gateway/probes" {
		t.Fatalf("gateway operations=%#v", got)
	}
	wire, err := registry.OpenAPI("Work", "21")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(wire, &document); err != nil {
		t.Fatal(err)
	}
	probe := document["paths"].(map[string]any)["/v1/gateway/probes"].(map[string]any)["post"].(map[string]any)
	if probe["x-chartworks-action"] != "ops.model" || probe["requestBody"] == nil || probe["x-chartworks-max-body-bytes"] != float64(workRequestMaxBytes) {
		t.Fatalf("probe metadata=%#v", probe)
	}
	probeResponses := probe["responses"].(map[string]any)
	unauthorized := probeResponses["401"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	properties := unauthorized["properties"].(map[string]any)
	if _, ok := properties["receipt"]; !ok {
		t.Fatal("gateway probe error metadata omitted bounded receipt")
	}
	if got := properties["error"].(map[string]any)["enum"].([]any); len(got) != 2 {
		t.Fatalf("gateway probe 401 errors=%v", got)
	}
}
