package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	errArguments = errors.New("mcp: invalid arguments")
	errResult    = errors.New("mcp: invalid result")
)

type admissionKey struct{}
type requestKey struct{}

func toolFailure(f Fault) *mcp.CallToolResult {
	raw, _ := json.Marshal(struct {
		Error Fault `json:"error"`
	}{f})
	return &mcp.CallToolResult{IsError: true, StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}
func failure(code, outcome string) *mcp.CallToolResult {
	return toolFailure(Fault{Code: code, Outcome: outcome})
}

// dispatch is the only execution path for tools and resources on either surface.
// The original HTTP/client context survives SDK detachment; no session stores a
// bearer or becomes analytical authority.
func (s *Server) dispatch(ctx context.Context, name string, args json.RawMessage) (out *mcp.CallToolResult) {
	started := false
	defer func() {
		if recover() != nil {
			out = failure("unavailable", outcome(started))
		}
	}()
	e, err := identity.FromContext(ctx)
	if err != nil || !e.Valid() || ctx.Value(admissionKey{}) != s {
		return failure("unauthenticated", "not_started")
	}
	if !e.Has("mcp.use") {
		return failure("forbidden", "not_started")
	}
	b, ok := s.registry.find(name)
	if !ok {
		return failure("not_found", "not_started")
	}
	if !e.Has(b.definition.Action) {
		return failure("forbidden", "not_started")
	}
	if ctx.Err() != nil {
		return failure("cancelled_or_timed_out", "not_started")
	}
	limit := s.settings.MaxRequestBytes
	if b.definition.MaxBodyBytes > 0 {
		limit = min(limit, b.definition.MaxBodyBytes)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if len(args) > limit {
		return failure("limit_exceeded", "not_started")
	}
	if b.input.Validate(args, limit) != nil {
		return failure("invalid_request", "not_started")
	}
	started = true
	raw, err := b.invoke(ctx, e, args)
	if err != nil {
		f := b.classify(err)
		if !b.errorCode(f.Code) {
			f = Fault{Code: "unavailable"}
		}
		if errors.Is(err, errArguments) {
			f = Fault{Code: "invalid_request"}
		}
		f.Outcome = "unknown"
		// Receipts are bounded, structured owner output, never arbitrary error text.
		if f.Receipt != nil {
			receipt, encodeErr := json.Marshal(f.Receipt)
			if encodeErr != nil || len(receipt) > 16384 || invalidJSON(receipt) {
				f.Receipt = nil
			}
		}
		failed := toolFailure(f)
		wire, encodeErr := json.Marshal(failed)
		if encodeErr != nil || len(wire)+512 > s.settings.MaxResponseBytes {
			f.Receipt = nil
			failed = toolFailure(f)
		}
		return failed
	}
	if ctx.Err() != nil {
		return failure("cancelled_or_timed_out", "unknown")
	}
	if !e.Valid() {
		return failure("unauthenticated", "unknown")
	}
	if len(raw) > s.settings.MaxResponseBytes {
		return failure("limit_exceeded", "unknown")
	}
	if b.output.Validate(raw, s.settings.MaxResponseBytes) != nil {
		return failure("unavailable", "unknown")
	}
	out = &mcp.CallToolResult{StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded)+512 > s.settings.MaxResponseBytes {
		return failure("limit_exceeded", "unknown")
	}
	return out
}
func outcome(started bool) string {
	if started {
		return "unknown"
	}
	return "not_started"
}
func (b Binding) errorCode(code string) bool {
	for _, e := range b.definition.Errors {
		if e.Code == code {
			return true
		}
	}
	return false
}
func (s *Server) faultFor(result *mcp.CallToolResult) Fault {
	raw, ok := result.StructuredContent.(json.RawMessage)
	if !ok {
		return Fault{Code: "unavailable", Outcome: "unknown"}
	}
	var f struct {
		Error Fault `json:"error"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Error.Code == "" {
		return Fault{Code: "unavailable", Outcome: "unknown"}
	}
	return f.Error
}

func requestAuthority(ctx context.Context) (identity.Envelope, error) {
	e, err := identity.FromContext(ctx)
	if err != nil || !e.Valid() {
		return identity.Envelope{}, access.ErrUnauthenticated
	}
	if !e.Has("mcp.use") {
		return identity.Envelope{}, access.ErrForbidden
	}
	return e, nil
}

func invalidJSON(raw []byte) bool { _, err := gateway.DecodeJSON(raw, 16384); return err != nil }
