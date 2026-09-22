package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
)

func openRouterRerankFixture(t *testing.T) *gatewayFixture {
	t.Helper()
	return newGatewayFixture(t, func(c *config.Gateway) {
		c.Bifrost.Providers = append(c.Bifrost.Providers, config.Provider{
			Name: "openrouter-rerank", Type: "openrouter_rerank", APIKey: "env:SECONDARY_KEY",
			BaseURL: c.Bifrost.Providers[0].BaseURL,
		})
		r := c.Roles["rerank"]
		r.Provider = "openrouter-rerank"
		r.Model = "cohere/rerank-4-fast"
		c.Roles["rerank"] = r
	})
}

func TestGatewayOpenRouterRerankSDKWire(t *testing.T) {
	f := openRouterRerankFixture(t)
	out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
	if err != nil || len(out.Items) != 2 || out.Items[0].ID != "first" || out.Items[1].ID != "second" || out.Items[0].Score == nil || *out.Items[0].Score <= *out.Items[1].Score {
		t.Fatalf("SDK rerank order or identity: %#v, %v", out, err)
	}
	if len(out.Receipt.Calls) != 1 || out.Receipt.Calls[0].Provider != "openrouter-rerank" || out.Receipt.Calls[0].RequestedModel != "cohere/rerank-4-fast" || out.Receipt.Calls[0].ActualModel != "cohere/rerank-4-fast" || out.Receipt.Calls[0].CostUSD != nil {
		t.Fatalf("incorrect route/model or invented cost: %#v", out.Receipt)
	}
	f.mu.Lock()
	path := f.paths[0]
	key := f.keys[0]
	body := f.requestBodies[0]
	f.mu.Unlock()
	if path != "/api/v1/rerank" || key != "Bearer SYNTHETIC_SECONDARY" {
		t.Fatalf("wrong outbound SDK path/auth: path=%q auth_matches=%t", path, key == "Bearer SYNTHETIC_SECONDARY")
	}
	var request struct {
		Model     string   `json:"model"`
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
		TopN      int      `json:"top_n"`
	}
	if json.Unmarshal([]byte(body), &request) != nil || request.Model != "cohere/rerank-4-fast" || request.Query != "question" || request.TopN != 2 || len(request.Documents) != 2 || request.Documents[0] != "first candidate" || request.Documents[1] != "second candidate" {
		t.Fatalf("wrong outbound SDK request shape: model=%q top_n=%d document_count=%d", request.Model, request.TopN, len(request.Documents))
	}

	f.rerankMode.Store("ties")
	tied, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
	if err != nil || tied.Items[0].ID != "first" || tied.Items[1].ID != "second" {
		t.Fatalf("ties did not preserve candidate order: %#v, %v", tied, err)
	}
}

func TestGatewayOpenRouterPerplexityEmbeddingAlias(t *testing.T) {
	f := newGatewayFixture(t, func(c *config.Gateway) {
		r := c.Roles["embedding"]
		r.Model = "perplexity/pplx-embed-v1-0.6b"
		c.Roles["embedding"] = r
	})
	f.embeddingMode.Store("openrouter-pplx-alias")
	out, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 2), f.engine.Space(), []string{"alias-positive"})
	if err != nil || len(out.Vectors) != 1 || len(out.Vectors[0]) != 2 || len(out.Receipt.Calls) != 1 {
		t.Fatalf("recorded OpenRouter alias response rejected: %v", err)
	}
	u := out.Receipt.Calls[0]
	if u.Role != "embedding" || u.Provider != "primary" || u.RequestedModel != "perplexity/pplx-embed-v1-0.6b" || u.ActualModel != "pplx-embed-v1-0.6b" || f.engine.EmbeddingSpace().Model != u.RequestedModel {
		t.Fatalf("embedding request, wire model, or pinned space drifted: %#v", u)
	}
	f.embeddingMode.Store("model")
	bad, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 2), f.engine.Space(), []string{"alias-negative"})
	if !errors.Is(err, gateway.ErrSpace) || len(bad.Vectors) != 0 || len(bad.Receipt.Calls) != 1 || bad.Receipt.Calls[0].ActualModel != "different-embedding-space" {
		t.Fatalf("unexpected observed model accepted: %#v, %v", bad.Receipt, err)
	}
}

func TestGatewayOpenRouterRerankRejectsMalformedWire(t *testing.T) {
	f := openRouterRerankFixture(t)
	for _, mode := range []string{"missing", "duplicate", "index", "nonfinite"} {
		t.Run(mode, func(t *testing.T) {
			f.rerankMode.Store(mode)
			out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
			if !errors.Is(err, gateway.ErrOutput) && !errors.Is(err, gateway.ErrUnavailable) || len(out.Items) != 0 || len(out.Receipt.Calls) != 1 {
				t.Fatalf("malformed %s response accepted: %#v, %v", mode, out, err)
			}
		})
	}
	for _, row := range []string{`{"index":0}`, `{"index":null,"relevance_score":0.5}`, `{"index":0,"relevance_score":null}`, `{"index":0,"index":0,"relevance_score":0.5}`} {
		f.rerankMode.Store("normal")
		f.mode.Store(`raw:{"results":[` + row + `,{"index":1,"relevance_score":0.1}]}`)
		out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
		if !errors.Is(err, gateway.ErrOutput) || len(out.Items) != 0 {
			t.Fatalf("malformed field accepted: %s, %v", row, err)
		}
	}
	f.mode.Store(`raw:{"model":"unexpected/rerank","results":[{"index":0,"relevance_score":0.8},{"index":1,"relevance_score":0.2}]}`)
	out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
	if err != nil || len(out.Items) != 2 || len(out.Receipt.Calls) != 1 || out.Receipt.Calls[0].RequestedModel != "cohere/rerank-4-fast" || out.Receipt.Calls[0].ActualModel != "unexpected/rerank" {
		t.Fatalf("requested and reported models were conflated: %#v, %v", out, err)
	}
}

func TestGatewayOpenRouterRerankConfiguration(t *testing.T) {
	f := openRouterRerankFixture(t)
	good := f.cfg
	for _, mutate := range []func(*config.Gateway){
		func(c *config.Gateway) { r := c.Roles["rerank"]; r.Model = "other/model"; c.Roles["rerank"] = r },
		func(c *config.Gateway) {
			r := c.Roles["sqlgen"]
			r.Provider = "openrouter-rerank"
			c.Roles["sqlgen"] = r
		},
		func(c *config.Gateway) { c.Bifrost.Providers[len(c.Bifrost.Providers)-1].BaseURL += "/api/v1" },
		func(c *config.Gateway) { c.Bifrost.Providers[len(c.Bifrost.Providers)-1].BaseURL += "/" },
	} {
		copy := good
		copy.Bifrost.Providers = append([]config.Provider(nil), good.Bifrost.Providers...)
		copy.Roles = make(map[string]config.Role, len(good.Roles))
		for k, v := range good.Roles {
			copy.Roles[k] = v
		}
		mutate(&copy)
		if config.ValidateGateway(copy, true) == nil {
			t.Fatal("invalid OpenRouter rerank configuration accepted")
		}
	}
}
