package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TokenProvider supplies caller-owned Pengui authority for one operation only.
// The callback must be safe for concurrent use; Chartworks never caches its token.
type TokenProvider func(context.Context) (string, error)

// Client is the in-process facade of the same dispatcher, not a second service.
// It verifies the current MCP-audience bearer for every method, including reads.
type Client struct {
	server *Server
	tokens TokenProvider
}

// Client returns an in-process client without creating a transport session.
func (s *Server) Client(tokens TokenProvider) (*Client, error) {
	if s == nil || tokens == nil {
		return nil, ErrRegistration
	}
	return &Client{s, tokens}, nil
}
func (c *Client) with(ctx context.Context, fn func(context.Context, identity.Envelope) error) error {
	if ctx == nil {
		return access.ErrUnauthenticated
	}
	token, err := c.tokens(ctx)
	if err != nil {
		return access.ErrUnauthenticated
	}
	e, err := c.server.verifier.Verify(ctx, token, auth.MCP)
	if err != nil {
		return access.ErrUnauthenticated
	}
	ctx, err = e.Context(ctx)
	if err != nil {
		return access.ErrUnauthenticated
	}
	return c.server.admit(ctx, func(ctx context.Context) error { return fn(ctx, e) })
}

// CallTool validates raw arguments and returns exactly the network tool result.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (out *mcp.CallToolResult, err error) {
	err = c.with(ctx, func(ctx context.Context, _ identity.Envelope) error {
		out = c.server.dispatch(ctx, name, args)
		return nil
	})
	return
}

// ListTools returns only real tools permitted by the current signed actions.
func (c *Client) ListTools(ctx context.Context) (out *mcp.ListToolsResult, err error) {
	err = c.with(ctx, func(_ context.Context, e identity.Envelope) error { out = c.server.listTools(e); return nil })
	return
}

// ListResources lists exact pure metadata resources without loading their content.
func (c *Client) ListResources(ctx context.Context) (out *mcp.ListResourcesResult, err error) {
	err = c.with(ctx, func(_ context.Context, e identity.Envelope) error { out = c.server.listResources(e); return nil })
	return
}

// ListResourceTemplates lists pure read templates, not unbuilt Apps viewers.
func (c *Client) ListResourceTemplates(ctx context.Context) (out *mcp.ListResourceTemplatesResult, err error) {
	err = c.with(ctx, func(_ context.Context, e identity.Envelope) error { out = c.server.listTemplates(e); return nil })
	return
}

// ReadResource resolves through the same binding and resource enforcement as tools.
func (c *Client) ReadResource(ctx context.Context, uri string) (out *mcp.ReadResourceResult, err error) {
	err = c.with(ctx, func(ctx context.Context, _ identity.Envelope) error {
		var err error
		out, err = c.server.readResource(ctx, &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: uri}})
		return err
	})
	return
}
