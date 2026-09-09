package chartworks

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/gateway"
)

// OperationMatrix joins installed HTTP contracts with currently visible MCP tools.
// Supply a distinct MCP client/provider when Pengui uses separate audiences. A nil
// MCP client means "not queried", not "unsupported". Visibility is not a resource
// grant: all calls still run the owning service's current authority checks.
//
// Every ordinary row names the implemented generic SDK method and CLI command;
// no hand-maintained future endpoint list is generated. MCP metadata must match
// the HTTP owner's operation, action, effect and audit before it can be joined.
func (c *Client) OperationMatrix(ctx context.Context, mcpClient *Client) ([]OperationInfo, error) {
	ctx, cancel, err := c.clientContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rows, err := c.Operations(ctx)
	if err != nil || mcpClient == nil {
		return rows, err
	}
	raw, err := mcpClient.MCP(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		return nil, err
	}
	if _, err := gateway.DecodeJSON(raw, 32<<20); err != nil {
		return nil, ErrInvalidCatalog
	}
	var response struct {
		Version string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   json.RawMessage `json:"error"`
		Result  *struct {
			Tools []struct {
				Name string                     `json:"name"`
				Meta map[string]json.RawMessage `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Version != "2.0" || string(response.ID) != "1" || response.Error != nil || response.Result == nil || len(response.Result.Tools) > 64 {
		return nil, ErrInvalidCatalog
	}
	byID := make(map[string]int, len(rows))
	for i, row := range rows {
		byID[row.ID] = i
	}
	names := map[string]bool{}
	for _, tool := range response.Result.Tools {
		if !wireID(tool.Name) || len(tool.Name) > 48 || names[tool.Name] {
			return nil, ErrInvalidCatalog
		}
		names[tool.Name] = true
		get := func(key string) string {
			var value string
			if json.Unmarshal(tool.Meta[key], &value) != nil {
				return ""
			}
			return value
		}
		i, exists := byID[get("chartworks/operation")]
		if !exists || rows[i].MCPTool != "" || rows[i].Action != get("chartworks/action") || rows[i].Effect != get("chartworks/effect") || rows[i].Audit != get("chartworks/audit") {
			return nil, ErrInvalidCatalog
		}
		rows[i].MCPTool = tool.Name
	}
	return rows, nil
}
