package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func matrixCatalogFixture(t *testing.T, registry *api.Registry, tools []*mcp.Tool) (*Client, *Client) {
	t.Helper()
	document, err := registry.OpenAPI("Synthetic bounded MCP inventory", "1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/openapi.json":
			if r.Header.Get("Authorization") != "Bearer synthetic-http" {
				t.Error("HTTP authority changed")
			}
			_, _ = w.Write(document)
		case "/v1/mcp":
			if r.Header.Get("Authorization") != "Bearer synthetic-mcp" {
				t.Error("MCP authority changed")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"tools": tools}})
		default:
			t.Error("matrix invoked a domain operation", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-http", nil })
	if err != nil {
		t.Fatal(err)
	}
	mcpClient, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-mcp", nil })
	if err != nil {
		t.Fatal(err)
	}
	return client, mcpClient
}

func TestOperationMatrixManualLifecycleParity(t *testing.T) {
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := reportingapi.AuthoringMCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	client, mcpClient := matrixCatalogFixture(t, registry, mcpRegistry.Manifest())
	rows, err := client.OperationMatrix(t.Context(), mcpClient)
	if err != nil || len(rows) != 25 {
		t.Fatal("actual authoring matrix drift", len(rows), err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.MCPTool != row.ID || row.MCPDisposition != "bound" || row.Replay != "never" {
			t.Fatal("actual lifecycle contract not joined", row.ID)
		}
		seen[row.ID] = true
	}
	for _, id := range []string{"reporting_authoring_lifecycle_v1", "reporting_authoring_block_publish_v1", "reporting_authoring_rebind_published_v1", "reporting_authoring_report_transition_v1"} {
		if !seen[id] {
			t.Fatal("missing lifecycle matrix entry", id)
		}
	}
}

func TestOperationMatrixUsesExistingRegisteredInventoryCeiling(t *testing.T) {
	// This metadata-only fixture isolates SDK capacity. The foundation inventory
	// test separately composes all actual 91 default/96 optional service bindings.
	empty, err := api.SchemaFor("boundedEmpty", reflect.TypeFor[struct{}](), false)
	if err != nil {
		t.Fatal(err)
	}
	definitions := make([]api.Definition, 97)
	for i := range definitions {
		definitions[i] = api.Definition{Operation: api.Operation{Method: "POST", Path: fmt.Sprintf("/v1/bounded/%d", i), Action: "reporting.read", Effect: "retained_metadata_read"}, ID: fmt.Sprintf("bounded_%d", i), Summary: "Synthetic metadata-only inventory capacity fixture", ResourceLoader: "exact current signed target reach", Audit: "read_only_no_domain_audit", Replay: "never", Request: empty, Response: empty, Errors: []api.ErrorResponse{{Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}}, MaxBodyBytes: 1024}
	}
	registry, err := api.New(definitions)
	if err != nil {
		t.Fatal(err)
	}
	tools := make([]*mcp.Tool, len(definitions))
	for i, d := range definitions {
		binding, err := mcpserver.Bind(registry, d.ID, d.ID, "reporting", "Synthetic metadata-only inventory capacity fixture", func(context.Context, identity.Envelope, struct{}) (struct{}, error) {
			t.Error("metadata fixture invoked")
			return struct{}{}, nil
		}, func(error) mcpserver.Fault { return mcpserver.Fault{Code: "forbidden"} })
		if err != nil {
			t.Fatal(err)
		}
		single, err := mcpserver.NewRegistry([]mcpserver.Binding{binding})
		if err != nil {
			t.Fatal(err)
		}
		tools[i] = single.Manifest()[0]
	}
	if mcpserver.MaxRegisteredTools != 96 {
		t.Fatal("registration ceiling changed")
	}
	for _, count := range []int{64, 91, 96, 97} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			client, mcpClient := matrixCatalogFixture(t, registry, tools[:count])
			rows, err := client.OperationMatrix(t.Context(), mcpClient)
			if count == 97 {
				if !errors.Is(err, ErrInvalidCatalog) {
					t.Fatal("oversized MCP inventory admitted", err)
				}
				return
			}
			if err != nil {
				t.Fatal("bounded MCP inventory rejected", count, err)
			}
			bound := 0
			for _, row := range rows {
				if row.MCPDisposition == "bound" {
					bound++
				}
			}
			if bound != count {
				t.Fatal("tool dropped from matrix", bound, count)
			}
		})
	}
	for _, tc := range []string{"duplicate-name", "duplicate-operation", "invalid-name", "long-name"} {
		t.Run(tc, func(t *testing.T) {
			changed := append([]*mcp.Tool(nil), tools[:96]...)
			last := *changed[95]
			changed[95] = &last
			switch tc {
			case "duplicate-name":
				last.Name = changed[0].Name
			case "invalid-name":
				last.Name = "invalid-name"
			case "long-name":
				last.Name = "this_tool_name_exceeds_the_registered_forty_eight_character_limit"
			case "duplicate-operation":
				last.Meta = map[string]any{}
				for k, v := range changed[0].Meta {
					last.Meta[k] = v
				}
			}
			client, mcpClient := matrixCatalogFixture(t, registry, changed)
			if _, err := client.OperationMatrix(t.Context(), mcpClient); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatal("bad MCP inventory accepted", tc, err)
			}
		})
	}
}
