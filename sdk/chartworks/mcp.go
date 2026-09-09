package chartworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// MCP performs one bounded JSON-RPC round trip on the shared-port MCP mount.
// Supply a TokenProvider returning current Pengui MCP-audience authority, with
// mcp.use and the requested domain scopes. No token, transport ID, initialization
// or analytical session is retained by the client. The established MCP SDK can
// also consume this endpoint directly; this method supports raw protocol users.
// Successful notifications return nil bytes. Tool failures remain typed JSON-RPC
// results (isError), not fabricated Go transport errors or automatically retried
// operations. Inspect their code/outcome/receipt before any retry.
func (c *Client) MCP(ctx context.Context, request json.RawMessage) (json.RawMessage, error) {
	if len(request) > 10<<20 || !json.Valid(request) {
		return nil, errors.New("chartworks: invalid request")
	}
	var envelope struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	if json.Unmarshal(request, &envelope) != nil || envelope.Method == "" {
		return nil, errors.New("chartworks: invalid request")
	}
	var response json.RawMessage
	options := wireOptions{accept: "application/json, text/event-stream", protocol: "2025-11-25", accepted: envelope.Method == "notifications/initialized" && envelope.ID == nil}
	err := c.exchange(ctx, "POST", "/v1/mcp", "", "application/json", bytes.NewReader(request), &response, 32<<20, options)
	return response, err
}
