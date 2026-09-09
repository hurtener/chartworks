package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Source execution-context and dataset identifiers can contain colons. Exercise
// the SDK's actual resource-template dispatcher as well as the in-process path.
// Simple RFC 6570 expansion cannot match these canonical raw identifiers.
func TestReservedResourceIdentifiersThroughSDK(t *testing.T) {
	f := newAuthority(t)
	binding := testRegistry(t, fixtureCall).bindings[0]
	binding.resource = ""
	binding, err := WithResource(binding, "chartworks://fixtures/{+item}")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry([]Binding{binding})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(f.verifier, registry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	token := f.token(t, "one", "reader", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:*")
	client, err := s.Client(func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	templates, err := client.ListResourceTemplates(t.Context())
	if err != nil || len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "chartworks://fixtures/{+item}" {
		t.Fatal("incorrect public template", templates, err)
	}
	for _, id := range []string{"ordinary", "context:partition", "source:dataset:revision", "id_with-dash.and.dot"} {
		t.Run(id, func(t *testing.T) {
			uri := "chartworks://fixtures/" + id
			request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": map[string]any{"uri": uri}})
			response := rpc(t, s.Handler(), token, string(request), nil)
			var wire struct {
				Error  json.RawMessage        `json:"error"`
				Result mcp.ReadResourceResult `json:"result"`
			}
			if json.Unmarshal(response.Body.Bytes(), &wire) != nil || response.Code != 200 || wire.Error != nil || len(wire.Result.Contents) != 1 {
				t.Fatalf("SDK resource %s: %d %s", uri, response.Code, response.Body.String())
			}
			if !strings.Contains(wire.Result.Contents[0].Text, `"item":"`+id+`"`) {
				t.Fatal("identifier changed", wire.Result.Contents[0].Text)
			}
			direct, err := client.ReadResource(t.Context(), uri)
			if err != nil || len(direct.Contents) != 1 || direct.Contents[0].Text != wire.Result.Contents[0].Text {
				t.Fatal("resource path disagreement", direct, err)
			}
		})
	}
	for _, uri := range []string{"chartworks://fixtures/context%3Apartition", "chartworks://fixtures/context:partition/extra", "chartworks://fixtures/..", "chartworks://fixtures/context:partition?token=x"} {
		if _, err := client.ReadResource(t.Context(), uri); err == nil {
			t.Fatal("identifier authority widened", uri)
		}
	}
}
