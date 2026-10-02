package bifrost

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/maximhq/bifrost/core/schemas"
	"testing"
	"time"
)

func TestEnvironmentProxyFixedDestination(t *testing.T) {
	for _, kind := range []string{"openrouter", "openrouter_rerank"} {
		for _, base := range []string{"", "https://openrouter.ai", "https://openrouter.ai/"} {
			if !environmentProxyDestination(config.Provider{Type: kind, BaseURL: base}, TransportOptions{EnvironmentProxy: true}) {
				t.Fatal(kind, base)
			}
		}
	}
	for _, base := range []string{"http://openrouter.ai", "https://127.0.0.1", "https://localhost", "https://openrouter.ai:443", "https://openrouter.ai.attacker.example", "https://user@openrouter.ai", "https://openrouter.ai?route=private", "https://openrouter.ai/#fragment", "https://openrouter.ai/private", "https://openrouter.ai/%61pi/v1"} {
		if environmentProxyDestination(config.Provider{Type: "openrouter", BaseURL: base}, TransportOptions{EnvironmentProxy: true}) {
			t.Fatal("untrusted destination", base)
		}
	}
	for _, transport := range []TransportOptions{{EnvironmentProxy: true, AllowPrivateNetwork: true}, {EnvironmentProxy: true, CACertPEM: "custom"}} {
		if environmentProxyDestination(config.Provider{Type: "openrouter"}, transport) {
			t.Fatal("trust relaxation")
		}
	}
	if environmentProxyDestination(config.Provider{Type: "openai"}, TransportOptions{EnvironmentProxy: true}) {
		t.Fatal("unapproved provider")
	}
}
func TestEnvironmentProxyRejectedBeforeCredentialLookup(t *testing.T) {
	cfg := environmentProxyTestConfig("https://127.0.0.1")
	calls := 0
	_, err := New(context.Background(), cfg, func(string) (string, bool) { calls++; return "synthetic", true }, TransportOptions{EnvironmentProxy: true})
	if !errors.Is(err, gateway.ErrInput) || calls != 0 {
		t.Fatal("untrusted proxy destination reached credentials", err, calls)
	}
}
func TestEnvironmentProxyAccountRemainsExplicit(t *testing.T) {
	a := account{provider: schemas.OpenRouter, environmentProxy: true}
	cfg, e := a.GetConfigForProvider(schemas.OpenRouter)
	if e != nil || cfg.ProxyConfig == nil || cfg.ProxyConfig.Type != schemas.EnvProxy || cfg.NetworkConfig.AllowPrivateNetwork {
		t.Fatal("proxy lost destination trust boundary", e)
	}
	a.environmentProxy = false
	cfg, e = a.GetConfigForProvider(schemas.OpenRouter)
	if e != nil || cfg.ProxyConfig != nil {
		t.Fatal("default enabled proxy", e)
	}
}

func environmentProxyTestConfig(base string) config.Gateway {
	cfg := config.Defaults().Gateway
	cfg.Bifrost.Providers = []config.Provider{{Name: "openrouter", Type: "openrouter", BaseURL: base, APIKey: "env:SYNTHETIC_KEY"}, {Name: "openrouter-rerank", Type: "openrouter_rerank", BaseURL: base, APIKey: "env:SYNTHETIC_KEY"}}
	for _, role := range config.RoleNames() {
		cfg.Roles[role] = config.Role{Enabled: false, Provider: "openrouter", Model: "synthetic", Timeout: config.Duration(10 * time.Second), MaxTokens: 32}
	}
	e := cfg.Roles["embedding"]
	e.Dimensions = 2
	e.MaxBatchItems = 2
	e.MaxBatchBytes = 1024
	e.ModelRevision = "synthetic"
	cfg.Roles["embedding"] = e
	r := cfg.Roles["rerank"]
	r.Provider = "openrouter-rerank"
	r.Model = "cohere/rerank-4-fast"
	r.MaxCandidates = 2
	r.OnFailure = "fail"
	cfg.Roles["rerank"] = r
	return cfg
}
