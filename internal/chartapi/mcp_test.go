package chartapi

import (
	"testing"

	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
)

func TestRealMCPRegistration(t *testing.T) {
	service, err := chartservice.New(config.DefaultCharts().ServiceOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := MCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Manifest()) != 5 {
		t.Fatal("missing chart bindings")
	}
	absent, err := MCPBindings(nil)
	if err != nil || len(absent) != 0 {
		t.Fatal("unavailable service advertised", err)
	}
}
