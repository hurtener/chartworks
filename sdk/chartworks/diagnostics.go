package chartworks

import "context"

// RegisteredOperation describes a real operation, not a locally editable grant.
type RegisteredOperation struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Action     string `json:"action"`
	Permission string `json:"permission"`
	Mutation   bool   `json:"mutation"`
}

// Check reports only what the currently supplied signed authority permits.
type Check struct {
	Operation RegisteredOperation `json:"operation"`
	Allowed   bool                `json:"allowed"`
}

// Diagnostics needs ops.inspect and exact tenant read reach.
func (c *Client) Diagnostics(ctx context.Context) ([]Check, error) {
	var checks []Check
	err := c.call(ctx, "GET", "/v1/access/diagnostics", "", nil, &checks)
	return checks, err
}

// Metrics reads deployment-level, content-free counters under the separate ops.metrics scope.
// Pengui should issue that operator permission independently from tenant data permissions.
func (c *Client) Metrics(ctx context.Context) (string, error) {
	var text string
	err := c.call(ctx, "GET", "/metrics", "", nil, &text)
	return text, err
}
