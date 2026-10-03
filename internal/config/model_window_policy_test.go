package config

import (
	"strings"
	"testing"
	"time"
)

func TestSQLRecoveryModelWindowConfiguration(t *testing.T) {
	base := func() Gateway {
		g := Defaults().Gateway
		g.Bifrost.Providers = []Provider{{Name: "chat", Type: "openrouter", APIKey: "env:FIXTURE"}}
		g.Roles = map[string]Role{"sqlgen": {Provider: "chat", Model: "recorded-a", Timeout: Duration(time.Second), MaxTokens: 256}}
		g.ModelWindows = []ModelWindow{{Provider: "chat", Model: "recorded-a", ContextTokens: 8192, ProtocolReserveTokens: 1024}}
		return g
	}
	if err := ValidateGateway(base(), false); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Gateway){
		"duplicate":        func(g *Gateway) { g.ModelWindows = append(g.ModelWindows, g.ModelWindows[0]) },
		"missing_model":    func(g *Gateway) { g.ModelWindows[0].Model = "other" },
		"foreign_route":    func(g *Gateway) { g.ModelWindows[0].Provider = "other" },
		"output_too_large": func(g *Gateway) { g.ModelWindows[0].ContextTokens = 1024 },
		"missing_overhead": func(g *Gateway) { g.ModelWindows[0].ProtocolReserveTokens = 0 },
		"excessive_window": func(g *Gateway) { g.ModelWindows[0].ContextTokens = 1 << 30 },
		"bad_name":         func(g *Gateway) { g.ModelWindows[0].Model = "private-canary\x00" },
	} {
		t.Run(name, func(t *testing.T) {
			g := base()
			change(&g)
			err := ValidateGateway(g, false)
			if err == nil || strings.Contains(err.Error(), "private-canary") {
				t.Fatal("invalid policy accepted or echoed", err)
			}
		})
	}
	legacy := base()
	legacy.ModelWindows = nil
	if err := ValidateGateway(legacy, false); err != nil {
		t.Fatal("legacy config rejected", err)
	}
}
