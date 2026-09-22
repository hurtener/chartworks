package recorded

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestExactRecordedGatewayUsesReviewedCallAndBudget(t *testing.T) {
	envelope, err := identity.FromVerified("t", "user", "session", []string{"ops.read", "cw.tenant.read:t"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	call, err := gateway.Authorize(envelope, "ops.read", "synthetic", access.Resource{Tenant: "t", Kind: "tenant", Permission: "read", ID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := gateway.RuntimeConfig{Model: "model-sql", Models: []gateway.RuntimeModel{{Role: "embedding", Model: "model-embed"}}, SystemInstruction: "reviewed instruction", AttemptCostUSD: 0.01}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	ctx, err := gateway.WithRuntimeConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := gateway.NewSchema("sql", []byte(`{"type":"object","additionalProperties":false,"required":["sql"],"properties":{"sql":{"type":"string"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	space := gateway.EmbeddingSpace{Provider: "recorded", Route: "integration", Endpoint: "local", Model: "model-embed", Revision: "fixture-v1", Dimensions: 2, Preprocessing: "utf8-exact;float32-finite", InputType: "text", Normalization: "no-normalization"}
	_, system, digest := gateway.ApplyRuntimeConfig(ctx, "sql_generation", "model-sql", "base")
	generatedKey := InputDigest(call, "sql_generation", "model-sql", digest, "sql_generation", system, "synthetic question", schema.Name(), schema.Document())
	otherPartition, err := gateway.Authorize(envelope, "ops.read", "another-run-id", access.Resource{Tenant: "t", Kind: "tenant", Permission: "read", ID: "t"})
	if err != nil || otherPartition.Key() == call.Key() || InputDigest(otherPartition, "sql_generation", "model-sql", digest, "sql_generation", system, "synthetic question", schema.Name(), schema.Document()) != generatedKey {
		t.Fatal("equivalent authorized inputs across distinct run IDs need one exact cassette key", err)
	}
	if InputDigest(otherPartition, "sql_generation", "model-sql", digest, "sql_generation", system, "changed question", schema.Name(), schema.Document()) == generatedKey {
		t.Fatal("changed model input selected a stale cassette")
	}
	differentReach, err := gateway.Authorize(envelope, "ops.read", "another-run-id",
		access.Resource{Tenant: "t", Kind: "tenant", Permission: "read", ID: "t"},
		access.Resource{Tenant: "t", Kind: "tenant", Permission: "read", ID: "t"})
	if err != nil || InputDigest(differentReach, "sql_generation", "model-sql", digest, "sql_generation", system, "synthetic question", schema.Name(), schema.Document()) == generatedKey {
		t.Fatal("changed resolved resource reach selected the same cassette", err)
	}
	embeddedKey := InputDigest(call, "embedding", "model-embed", digest, space.Key(), []string{"synthetic question"})
	engine, err := New(space, map[string]string{"sql_generation": "model-sql", "embedding": "model-embed"}, []Recording{
		{Role: "sql_generation", InputDigest: generatedKey, JSON: []byte(`{"sql":"SELECT 1"}`)},
		{Role: "embedding", InputDigest: embeddedKey, Vectors: [][]float32{{0.25, 0.75}}},
	})
	if err != nil || engine.PerformanceModelMode() != "recorded" {
		t.Fatalf("recorded constructor: %v", err)
	}
	budget := func() *gateway.Budget {
		b, err := gateway.NewBudget(call, gateway.Limits{Calls: 2, Tokens: 4096, Duration: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	generated, err := engine.Generate(ctx, call, budget(), "sql_generation", "base", "synthetic question", schema)
	if err != nil || string(generated.JSON) != `{"sql":"SELECT 1"}` || len(generated.Receipt.Calls) != 1 || generated.Receipt.Calls[0].Provider != "recorded" || generated.Receipt.Calls[0].ConfigurationDigest != cfg.Digest || generated.Receipt.Calls[0].CostUSD != nil {
		t.Fatalf("recorded generation: %v, %+v", err, generated)
	}
	embedded, err := engine.Embed(ctx, call, budget(), space.Key(), []string{"synthetic question"})
	if err != nil || len(embedded.Vectors) != 1 || embedded.Vectors[0][0] != 0.25 || embedded.Space != space.Key() {
		t.Fatalf("recorded embedding: %v, %+v", err, embedded)
	}
	embedded.Vectors[0][0] = 9
	again, err := engine.Embed(ctx, call, budget(), space.Key(), []string{"synthetic question"})
	if err != nil || again.Vectors[0][0] != 0.25 {
		t.Fatal("recording mutated by consumer", err)
	}
	if _, err := engine.Generate(ctx, call, budget(), "sql_generation", "base", "changed question", schema); !errors.Is(err, gateway.ErrUnavailable) {
		t.Fatalf("unrecorded prompt: %v", err)
	}
	if _, err := engine.Generate(context.Background(), call, budget(), "sql_generation", "base", "synthetic question", schema); !errors.Is(err, gateway.ErrInput) {
		t.Fatalf("missing reviewed runtime config: %v", err)
	}
	engine.Close()
	if _, err := engine.Generate(ctx, call, budget(), "sql_generation", "base", "synthetic question", schema); !errors.Is(err, gateway.ErrClosed) {
		t.Fatalf("closed gateway: %v", err)
	}
}
