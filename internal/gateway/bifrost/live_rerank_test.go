package bifrost_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/identity"
)

// TestLiveOpenRouterRerank is an opt-in paid smoke through the Chartworks
// gateway, never a direct provider probe. The wrapper loads a local secret.
func TestLiveOpenRouterRerank(t *testing.T) {
	if os.Getenv("CHARTWORKS_LIVE_OPENROUTER_RERANK") != "1" {
		t.Skip("opt-in paid provider smoke")
	}
	key := os.Getenv("CHARTWORKS_OPENROUTER_API_KEY")
	if key == "" {
		t.Fatal("OpenRouter credential alias is unset")
	}
	cfg := config.Defaults().Gateway
	cfg.MaxAttemptsPerCall = 1
	cfg.Bifrost.Providers = []config.Provider{
		{Name: "openrouter", Type: "openrouter", APIKey: "env:CHARTWORKS_OPENROUTER_API_KEY"},
		{Name: "openrouter-rerank", Type: "openrouter_rerank", APIKey: "env:CHARTWORKS_OPENROUTER_API_KEY"},
	}
	for _, role := range config.RoleNames() {
		cfg.Roles[role] = config.Role{Enabled: role == "rerank", Provider: "openrouter", Model: "unused", Timeout: config.Duration(10 * time.Second), MaxTokens: 32}
	}
	embedding := cfg.Roles["embedding"]
	embedding.Dimensions = 2
	embedding.MaxBatchItems = 2
	embedding.MaxBatchBytes = 1024
	embedding.ModelRevision = "smoke-only"
	cfg.Roles["embedding"] = embedding
	rerank := cfg.Roles["rerank"]
	rerank.Provider = "openrouter-rerank"
	rerank.Model = "cohere/rerank-4-fast"
	rerank.MaxCandidates = 2
	rerank.OnFailure = "fail"
	cfg.Roles["rerank"] = rerank
	engine, err := bifrost.New(context.Background(), cfg, func(name string) (string, bool) {
		if name == "CHARTWORKS_OPENROUTER_API_KEY" {
			return key, true
		}
		return "", false
	}, bifrost.TransportOptions{})
	if err != nil {
		t.Fatal("gateway construction failed:", err)
	}
	t.Cleanup(engine.Close)
	e, err := identity.FromVerified("smoke", "operator", "smoke-session", []string{"gateway.use", "cw.tenant.use:smoke", "cw.topic.read:first", "cw.topic.read:second"}, time.Now().Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	call, err := gateway.Authorize(e, "gateway.use", "smoke-context", access.Tenant(e, "use"))
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := gateway.AdmitCandidates(call, "gateway.use", []gateway.Candidate{
		{ID: "first", Text: "Revenue by region is an analytics topic.", Resource: access.Resource{Tenant: "smoke", Kind: "topic", Permission: "read", ID: "first"}},
		{ID: "second", Text: "Shipping status is an operational topic.", Resource: access.Resource{Tenant: "smoke", Kind: "topic", Permission: "read", ID: "second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 1, Tokens: 1024, Duration: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	out, err := engine.Rerank(context.Background(), call, budget, "Which topic explains revenue by region?", candidates)
	if err != nil {
		actual := ""
		if len(out.Receipt.Calls) == 1 {
			actual = out.Receipt.Calls[0].ActualModel
		}
		t.Fatalf("Chartworks gateway rerank failed: %v (reported model=%q)", err, actual)
	}
	if len(out.Items) != 2 || out.Items[0].ID != "first" || out.Items[1].ID != "second" || out.Items[0].Score == nil || out.Items[1].Score == nil || len(out.Receipt.Calls) != 1 || out.Receipt.Calls[0].Provider != "openrouter-rerank" || out.Receipt.Calls[0].RequestedModel != "cohere/rerank-4-fast" {
		t.Fatalf("gateway rerank result/receipt invalid: ordered IDs=%v, receipt calls=%d", len(out.Items), len(out.Receipt.Calls))
	}
	usage := out.Receipt.Calls[0]
	t.Logf("Chartworks Bifrost rerank: requested_model=%s reported_model=%s ranked_ids=%s,%s scores_present=true input_tokens_known=%t output_tokens_known=%t cost_known=%t", usage.RequestedModel, usage.ActualModel, out.Items[0].ID, out.Items[1].ID, usage.InputTokens != nil, usage.OutputTokens != nil, usage.CostUSD != nil)
}
