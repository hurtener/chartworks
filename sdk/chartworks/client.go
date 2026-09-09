// Package chartworks is the token-forwarding client for implemented Chartworks operations.
// Tokens and renewal come from the caller's Pengui provider, never a local issuer.
package chartworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

// Policy is operational retention state, not access policy.
type Policy struct {
	Revision       int64 `json:"revision"`
	AuditDays      int   `json:"audit_days"`
	OperationHours int   `json:"operation_hours"`
}

// Audit is an authorized metadata record.
type Audit struct {
	ID        string    `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	CreatedAt time.Time `json:"created_at"`
}

// Operation is the accepted bounded sweep result.
type Operation struct {
	ID                string    `json:"id"`
	Status            string    `json:"status"`
	PolicyRevision    int64     `json:"policy_revision"`
	Cutoff            time.Time `json:"cutoff"`
	Limit             int       `json:"limit"`
	DeletedEvents     int64     `json:"deleted_events"`
	DeletedOperations int64     `json:"deleted_operations"`
}

// GatewayReceipt contains typed attempted inference usage; missing cost is unknown.
type GatewayReceipt = gateway.Receipt

// StatusError exposes the HTTP status and optional bounded usage metadata, never
// the raw rejection body or its message. Error() remains content-free.
type StatusError struct {
	Status  int
	Receipt *GatewayReceipt
}

func (e *StatusError) Error() string { return "chartworks: request rejected" }

// TokenProvider supplies current Pengui credentials. Errors are never echoed.
// The SDK calls it again for each request or explicitly authorized retry. Any
// renewal is the provider's responsibility; a 401 is never silently replayed.
type TokenProvider func(context.Context) (string, error)

// DefaultRequestTimeout bounds one exchange, including credential acquisition.
const DefaultRequestTimeout = 75 * time.Second

// MaximumRequestTimeout is the upper bound for an explicitly configured exchange.
const MaximumRequestTimeout = 15 * time.Minute

// Client is safe for concurrent requests provided its token provider is also safe.
type Client struct {
	base  string
	http  *http.Client
	token TokenProvider
}

// New pins a trusted backend origin and optional configured base path. It refuses
// userinfo, query/fragment credentials, ambiguous path encodings and redirects.
// A supplied zero HTTP timeout selects the bounded default, not infinite work.
func New(base string, client *http.Client, token TokenProvider) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || strings.TrimSpace(base) != base || strings.ContainsAny(base, "?#\\\r\n\t") || !clientBasePath(u.Path) || token == nil {
		return nil, errors.New("chartworks: invalid client configuration")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && (u.Scheme != "http" || ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("chartworks: HTTPS required")
	}
	c := http.Client{Timeout: DefaultRequestTimeout}
	if client != nil {
		c = *client
	}
	if c.Timeout < 0 || c.Timeout > MaximumRequestTimeout {
		return nil, errors.New("chartworks: invalid request timeout")
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultRequestTimeout
	}
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("chartworks: redirect refused") }
	return &Client{strings.TrimSuffix(base, "/"), &c, token}, nil
}

func clientBasePath(path string) bool {
	if path == "" || path == "/" {
		return true
	}
	path = strings.TrimSuffix(path, "/")
	if len(path) > 64 || !strings.HasPrefix(path, "/") {
		return false
	}
	for _, segment := range strings.Split(path[1:], "/") {
		if !wireID(segment) || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func (c *Client) call(ctx context.Context, method, path, key string, body, out any) error {
	return c.callLimit(ctx, method, path, key, body, out, 1<<20)
}
func (c *Client) callLimit(ctx context.Context, method, path, key string, body, out any, limit int64) error {
	var input []byte
	media := ""
	if body != nil {
		var err error
		input, err = json.Marshal(body)
		if err != nil {
			return errors.New("chartworks: invalid request")
		}
		media = "application/json"
	}
	return c.callReader(ctx, method, path, key, media, bytes.NewReader(input), out, limit)
}

// callReader is the only authenticated transport. It reads no file bytes before
// obtaining the current token and never follows credential-bearing redirects.
func (c *Client) callReader(ctx context.Context, method, path, key, media string, input io.Reader, out any, limit int64) error {
	return c.exchange(ctx, method, path, key, media, input, out, limit, wireOptions{})
}

type wireOptions struct {
	accept, protocol string
	accepted         bool
}

// exchange is the sole network credential boundary for ordinary HTTP and MCP.
func (c *Client) exchange(ctx context.Context, method, path, key, media string, input io.Reader, out any, limit int64, options wireOptions) error {
	bounded, cancel, err := c.clientContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	ctx = bounded
	if limit < 1 || limit > 128<<20 {
		return errors.New("chartworks: invalid response limit")
	}
	token, err := c.token(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || token == "" || len(token) > 64<<10 || strings.ContainsAny(token, " \x00\r\n\t,") {
		return errors.New("chartworks: credential unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, input)
	if err != nil {
		return errors.New("chartworks: invalid request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if options.accept != "" {
		req.Header.Set("Accept", options.accept)
	}
	if options.protocol != "" {
		req.Header.Set("Mcp-Protocol-Version", options.protocol)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("chartworks: transport failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusAccepted && options.accepted {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1))
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if readErr != nil || len(data) != 0 {
			return errors.New("chartworks: invalid response")
		}
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		rejected := &StatusError{Status: resp.StatusCode}
		if path == "/v1/charts/select" {
			rejected.Receipt = readFailureReceipt(resp.Body)
		}
		return rejected
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil && int64(len(data)) <= limit {
		if text, ok := out.(*string); ok {
			*text = string(data)
			return nil
		}
	}
	if err != nil || int64(len(data)) > limit || json.Unmarshal(data, out) != nil {
		return errors.New("chartworks: invalid response")
	}
	return nil
}

// RetentionPolicy reads only the caller's signed tenant configuration.
func (c *Client) RetentionPolicy(ctx context.Context) (Policy, error) {
	var p Policy
	err := c.call(ctx, "GET", "/v1/retention-policy", "", nil, &p)
	return p, err
}

// SetRetentionPolicy uses explicit optimistic concurrency, not an unconditional update.
func (c *Client) SetRetentionPolicy(ctx context.Context, expected int64, auditDays, operationHours int) (Policy, error) {
	body := struct {
		Expected       int64 `json:"expected_revision"`
		AuditDays      int   `json:"audit_days"`
		OperationHours int   `json:"operation_hours"`
	}{expected, auditDays, operationHours}
	var p Policy
	err := c.call(ctx, "PUT", "/v1/retention-policy", "", body, &p)
	return p, err
}

// AuditEvents reads a bounded tenant-isolated page.
func (c *Client) AuditEvents(ctx context.Context) ([]Audit, error) {
	var a []Audit
	err := c.call(ctx, "GET", "/v1/audit-events", "", nil, &a)
	return a, err
}

// Sweep preserves the supplied logical operation key; no automatic retries are hidden here.
func (c *Client) Sweep(ctx context.Context, key string) (Operation, error) {
	if key == "" {
		return Operation{}, errors.New("chartworks: idempotency key required")
	}
	var o Operation
	err := c.call(ctx, "POST", "/v1/retention-sweeps", key, struct{}{}, &o)
	return o, err
}

// readFailureReceipt is intentionally narrow: other domain errors retain their
// existing projection and an oversized/malformed body never obscures the status.
func readFailureReceipt(body io.Reader) *GatewayReceipt {
	b, err := io.ReadAll(io.LimitReader(body, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		return nil
	}
	var value struct {
		Receipt *GatewayReceipt `json:"receipt"`
	}
	if json.Unmarshal(b, &value) != nil || value.Receipt == nil || len(value.Receipt.Calls) > 4 || len(value.Receipt.Warning) > 256 {
		return nil
	}
	for _, u := range value.Receipt.Calls {
		if len(u.Role) > 64 || len(u.Provider) > 128 || len(u.RequestedModel) > 256 || len(u.ActualModel) > 256 || u.Attempts < 0 || u.Attempts > 64 || u.DurationMS < 0 || u.InputTokens != nil && *u.InputTokens < 0 || u.OutputTokens != nil && *u.OutputTokens < 0 || u.CostUSD != nil && (*u.CostUSD < 0 || math.IsNaN(*u.CostUSD) || math.IsInf(*u.CostUSD, 0)) {
			return nil
		}
	}
	return value.Receipt
}
