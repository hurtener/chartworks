package reportingapi

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestAuthoringRegistryParityAndClosedAuthorityInputs(t *testing.T) {
	registry, err := AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Definitions()) != 8 {
		t.Fatal("unexpected first-slice operation count")
	}
	documents, err := DocumentsRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := DeliveryRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.Compose(registry, documents, delivery); err != nil {
		t.Fatal("authoring changed or collided with viewer routes", err)
	}
	service, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := AuthoringMCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	tools := mcp.Manifest()
	if len(tools) != len(registry.Definitions()) {
		t.Fatal("HTTP/MCP registration drift")
	}
	for _, d := range registry.Definitions() {
		if d.Public || d.Request == nil || d.Response == nil || d.ResourceLoader == "" || d.Audit == "" || len(d.Errors) == 0 || d.Replay != "never" || d.MaxBodyBytes != MaxBodyBytes {
			t.Fatal("incomplete authoring contract", d.ID)
		}
		if d.Request.Validate([]byte(`{"report":"report","tenant":"foreign","profile":"admin","scopes":["reporting.write"]}`), MaxBodyBytes) == nil {
			t.Fatal("authority injection accepted", d.ID)
		}
		found := false
		for _, tool := range tools {
			if tool.Name != d.ID {
				continue
			}
			found = true
			if tool.Meta["chartworks/action"] != d.Action || tool.Meta["chartworks/effect"] != d.Effect {
				t.Fatal("MCP changed operation authority/effect", d.ID)
			}
			read := strings.HasSuffix(d.ID, "capabilities_v1") || strings.HasSuffix(d.ID, "drafts_v1") || strings.HasSuffix(d.ID, "read_v1")
			if tool.Annotations.ReadOnlyHint != read {
				t.Fatal("incorrect mutation annotation", d.ID)
			}
		}
		if !found {
			t.Fatal("missing MCP operation", d.ID)
		}
	}
	if _, err = AuthoringMCPBindings(nil); err == nil {
		t.Fatal("nil service advertised")
	}
}
